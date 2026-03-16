package processor

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/personalmedia/cdn/internal/cache"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

var mdParser goldmark.Markdown

func init() {
	mdParser = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithUnsafe(),
		),
	)
}

const htmlWrapper = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
	:root {
		--text-color: #111827;
		--bg-color: #ffffff;
		--border-color: #e5e7eb;
		--code-bg: #f3f4f6;
		--accent: #3b82f6;
	}
	body {
		font-family: 'Inter', -apple-system, sans-serif;
		color: var(--text-color);
		background: var(--bg-color);
		line-height: 1.6;
		font-size: 15px;
		margin: 0;
		padding: 48px;
		max-width: 800px;
		margin-left: auto;
		margin-right: auto;
	}
	h1, h2, h3, h4, h5 {
		font-weight: 700;
		margin-top: 1.75em;
		margin-bottom: 0.75em;
		color: #000;
		line-height: 1.2;
	}
	h1 { font-size: 2.25em; letter-spacing: -0.05em; border-bottom: 2px solid #000; padding-bottom: 0.25em; }
	h2 { font-size: 1.75em; letter-spacing: -0.02em; border-bottom: 1px solid var(--border-color); padding-bottom: 0.2em; }
	h3 { font-size: 1.4em; }
	p, ul, ol, blockquote, pre { margin-bottom: 1.2em; }
	a { color: var(--accent); text-decoration: none; }
	a:hover { text-decoration: underline; }
	code {
		font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace;
		background: var(--code-bg);
		padding: 0.2em 0.4em;
		border-radius: 4px;
		font-size: 0.85em;
		color: #1f2937;
	}
	pre {
		background: var(--code-bg);
		padding: 16px;
		border-radius: 8px;
		overflow-x: auto;
		border: 1px solid var(--border-color);
	}
	pre code { background: none; padding: 0; font-size: 0.85em; color: inherit; }
	table {
		width: 100%%;
		border-collapse: collapse;
		margin: 24px 0;
		font-size: 0.95em;
	}
	th, td {
		padding: 12px 16px;
		text-align: left;
		border-bottom: 1px solid var(--border-color);
	}
	th { font-weight: 600; background-color: #f9fafb; color: #374151; }
	blockquote {
		border-left: 4px solid #ced4da;
		margin: 0 0 1.2em 0;
		padding-left: 1em;
		color: #4b5563;
		font-style: italic;
	}
	img { max-width: 100%%; height: auto; border-radius: 6px; display: block; margin: 20px auto; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
	hr { border: 0; border-top: 1px solid var(--border-color); margin: 2em 0; }
</style>
</head>
<body>
%s
<script>
	// Ensure fonts are loaded before we declare ready
	document.fonts.ready.then(function() {
		document.body.classList.add('fonts-loaded');
	});
</script>
</body>
</html>`

// HandleMarkdownPDF reads a markdown file, parses it to HTML, and generates a PDF via a headless browser.
func HandleMarkdownPDF(c *gin.Context, req *ActionRequest) {
	mimeType := "application/pdf"

	if !cache.SourceExists(req.SourceFile) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	cacheFile := cache.CacheFileForDerived("pdf", req.RelPath, ".md.pdf")

	if cache.FileExists(cacheFile) {
		c.Header("X-CDN-Status", "HIT")
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		if cache.ServeNotModifiedOrMappedOrFile(c, cacheFile, mimeType) {
			return
		}
		c.File(cacheFile)
		return
	}

	c.Header("X-CDN-Status", "MISS")

	result, err, _ := cache.SF.Do(cacheFile, func() (interface{}, error) {
		if cache.FileExists(cacheFile) {
			return &cache.BuildResult{
				CacheFile: cacheFile,
				MimeType:  mimeType,
			}, nil
		}

		mdBytes, err := os.ReadFile(req.SourceFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read md file: %w", err)
		}

		var buf bytes.Buffer
		if err := mdParser.Convert(mdBytes, &buf); err != nil {
			return nil, fmt.Errorf("markdown parse err: %w", err)
		}

		htmlContent := fmt.Sprintf(htmlWrapper, buf.String())

		// Start browser
		browser := rod.New().MustConnect()
		defer browser.MustClose()

		page := browser.MustPage()
		defer page.MustClose()

		page = page.Timeout(15 * time.Second)

		err = page.SetDocumentContent(htmlContent)
		if err != nil {
			return nil, err
		}

		// Wait for font load via our injected script class
		_, err = page.Element(".fonts-loaded")
		if err != nil {
			// It probably timed out waiting for fonts, proceed anyway
		}

		paperWidth := 8.27
		paperHeight := 11.69
		marginSize := 0.4

		stream, err := page.PDF(&proto.PagePrintToPDF{
			PrintBackground: true,
			PaperWidth:      &paperWidth,
			PaperHeight:     &paperHeight,
			MarginTop:       &marginSize,
			MarginBottom:    &marginSize,
			MarginLeft:      &marginSize,
			MarginRight:     &marginSize,
		})

		if err != nil {
			return nil, fmt.Errorf("pdf render err: %w", err)
		}

		pdfBytes, err := io.ReadAll(stream)
		if err != nil {
			return nil, fmt.Errorf("read stream err: %w", err)
		}

		if err := os.MkdirAll(filepath.Dir(cacheFile), 0755); err != nil {
			return nil, err
		}

		if err := os.WriteFile(cacheFile, pdfBytes, 0644); err != nil {
			return nil, err
		}

		return &cache.BuildResult{
			CacheFile: cacheFile,
			MimeType:  mimeType,
			Data:      pdfBytes,
		}, nil
	})

	if err != nil {
		fmt.Printf("HandleMarkdownPDF Error: %v\n", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	br := result.(*cache.BuildResult)

	c.Header("Cache-Control", "public, max-age=31536000, immutable")

	if len(br.Data) > 0 {
		if info, err := os.Stat(br.CacheFile); err == nil {
			cache.WriteValidators(c, info)
		}
		c.Data(http.StatusOK, br.MimeType, br.Data)
		return
	}

	if cache.ServeNotModifiedOrMappedOrFile(c, br.CacheFile, br.MimeType) {
		return
	}

	c.File(br.CacheFile)
}
