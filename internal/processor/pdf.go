package processor

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ledongthuc/pdf"
	"github.com/personalmedia/cdn/internal/cache"
)

// HandlePDFText extracts plain text from a PDF for indexing purposes.
func HandlePDFText(c *gin.Context, req *ActionRequest) {
	mimeType := "text/plain; charset=utf-8"
	
	if !cache.SourceExists(req.SourceFile) {
		c.Data(http.StatusOK, mimeType, []byte(""))
		return
	}

	cacheFile := cache.CacheFileForDerived(req.Action, req.RelPath, ".txt")

	if cache.FileExists(cacheFile) {
		c.Header("X-CDN-Status", "HIT")
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		if cache.ServeNotModifiedOrMappedOrFile(c, cacheFile, mimeType) {
			return
		}
		c.Data(http.StatusOK, mimeType, []byte(""))
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

		content, err := extractPDFText(req.SourceFile)
		if err != nil {
			return nil, fmt.Errorf("failed to extract text: %w", err)
		}

		if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
			return nil, err
		}

		if err := os.WriteFile(cacheFile, []byte(content), 0o644); err != nil {
			return nil, err
		}

		return &cache.BuildResult{
			CacheFile: cacheFile,
			MimeType:  mimeType,
			Data:      []byte(content),
		}, nil
	})

	if err != nil {
		c.Data(http.StatusOK, mimeType, []byte(""))
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

	c.Data(http.StatusOK, br.MimeType, []byte(""))
}

func extractPDFText(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if f != nil {
		defer f.Close()
	}
	if err != nil {
		return "", err
	}

	var content strings.Builder
	for p := 1; p <= r.NumPage(); p++ {
		page := r.Page(p)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		content.WriteString(text)
		content.WriteString("\n")
	}

	return content.String(), nil
}
