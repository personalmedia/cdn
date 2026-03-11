package server

import (
	"image"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
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
		} else {
			c.AbortWithStatus(http.StatusUnsupportedMediaType)
			return
		}
	}

	w, h := 0, 0
	if kind == "image" {
		w, h = processor.ParseDims(c.Request.URL.RawQuery)
	}

	req := &processor.ActionRequest{
		Action:     actionName,
		Kind:       kind,
		RelPath:    relPath,
		SourceFile: filepath.Join(config.App.SourceDir, relPath),
		SourceExt:  ext,
		W:          w,
		H:          h,
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
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	width, height := 0, 0
	kind := extensionKind[strings.ToLower(filepath.Ext(relPath))]

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
		"modified": info.ModTime().Format("2006-01-02 15:04:05"),
	})
}
