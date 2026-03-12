package server

import (
	"image"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ledongthuc/pdf"
	"github.com/personalmedia/cdn/internal/cache"
	"github.com/personalmedia/cdn/internal/config"
	"github.com/personalmedia/cdn/internal/processor"
)

var extensionKind = map[string]string{
	".jpg":  "image",
	".jpeg": "image",
	".png":  "image",
	".gif":  "image",
	".webp": "image",
	".svg":  "image",
	".pdf":  "image",
	".xlsx": "excel",
}

func RouteProcessor(c *gin.Context) {
	actionName := c.Param("action")

	relPath, ok := processor.SanitizeRelativePath(c.Param("path"))
	if !ok {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	ext := strings.ToLower(filepath.Ext(relPath))
	kind, ok := extensionKind[ext]
	if !ok {
		if actionName == "resize" || actionName == "webp" || actionName == "blur" || actionName == "portrait" {
			kind = "image"
		} else if actionName == "csv" || actionName == "json" {
			kind = "excel"
		} else if actionName == "text" {
			kind = "pdf"
		} else {
			c.AbortWithStatus(http.StatusUnsupportedMediaType)
			return
		}
	}

	// Override kind if requesting text extraction specifically
	if actionName == "text" && ext == ".pdf" {
		kind = "pdf"
	}

	w, h, page := 0, 0, 1
	if kind == "image" {
		w, h, page = processor.ParseDims(c.Request.URL.RawQuery)
	}

	req := &processor.ActionRequest{
		Action:     actionName,
		Kind:       kind,
		RelPath:    relPath,
		SourceFile: filepath.Join(config.App.SourceDir, relPath),
		SourceExt:  ext,
		W:          w,
		H:          h,
		Page:       page,
		Query:      c.Request.URL.RawQuery,
	}

	if kind == "image" {
		processor.HandleImageAction(c, req)
	} else if kind == "excel" {
		if actionName == "csv" {
			processor.HandleExcelCSV(c, req)
		} else if actionName == "json" {
			processor.HandleExcelJSON(c, req)
		} else {
			c.AbortWithStatus(http.StatusBadRequest)
		}
	} else if kind == "pdf" && actionName == "text" {
		processor.HandlePDFText(c, req)
	} else {
		c.AbortWithStatus(http.StatusUnsupportedMediaType)
	}
}

func HandleMetadata(c *gin.Context) {
	relPath, ok := processor.SanitizeRelativePath(c.Param("path"))
	if !ok {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	sourceFile := filepath.Join(config.App.SourceDir, relPath)

	info, err := os.Stat(sourceFile)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"filename": filepath.Base(sourceFile),
			"path":     relPath,
			"kind":     "unknown",
			"size":     0,
			"mime":     "application/octet-stream",
			"width":    0,
			"height":   0,
			"pages":    0,
			"modified": "1970-01-01 00:00:00",
		})
		return
	}

	width, height, pages := 0, 0, 0
	ext := strings.ToLower(filepath.Ext(relPath))
	kind := extensionKind[ext]

	if kind == "image" || kind == "" {
		reader, err := os.Open(sourceFile)
		if err == nil {
			defer reader.Close()
			if conf, _, err := image.DecodeConfig(reader); err == nil {
				width = conf.Width
				height = conf.Height
				if kind == "" {
					kind = "image"
				}
			}
		}
	}

	if kind == "pdf" || ext == ".pdf" {
		if f, reader, err := pdf.Open(sourceFile); err == nil {
			pages = reader.NumPage()
			f.Close()
			if kind == "" {
				kind = "pdf"
			}
		}
	}

	if kind == "" {
		kind = "unknown"
	}

	cache.WriteValidators(c, info)

	c.JSON(http.StatusOK, gin.H{
		"filename": filepath.Base(sourceFile),
		"path":     relPath,
		"kind":     kind,
		"size":     info.Size(),
		"mime":     mime.TypeByExtension(filepath.Ext(sourceFile)),
		"width":    width,
		"height":   height,
		"pages":    pages,
		"modified": info.ModTime().Format("2006-01-02 15:04:05"),
	})
}
