package cache

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func MakeETag(info os.FileInfo) string {
	return fmt.Sprintf(`W/"%x-%x"`, info.ModTime().UTC().UnixNano(), info.Size())
}

func WriteValidators(c *gin.Context, info os.FileInfo) {
	mod := info.ModTime().UTC()
	c.Header("ETag", MakeETag(info))
	c.Header("Last-Modified", mod.Format(http.TimeFormat))
}

func IsNotModified(c *gin.Context, info os.FileInfo) bool {
	WriteValidators(c, info)

	etag := MakeETag(info)
	if inm := strings.TrimSpace(c.GetHeader("If-None-Match")); inm != "" && inm == etag {
		c.Status(http.StatusNotModified)
		return true
	}

	if ims := strings.TrimSpace(c.GetHeader("If-Modified-Since")); ims != "" {
		if t, err := time.Parse(http.TimeFormat, ims); err == nil {
			mod := info.ModTime().UTC().Truncate(time.Second)
			if !mod.After(t.UTC()) {
				c.Status(http.StatusNotModified)
				return true
			}
		}
	}

	return false
}

func ServeNotModifiedOrMappedOrFile(c *gin.Context, filename, mimeType string) bool {
	info, err := os.Stat(filename)
	if err != nil {
		return false
	}

	if IsNotModified(c, info) {
		return true
	}

	if data, ok, err := GetMappedFile(filename, mimeType); err == nil && ok {
		c.Data(http.StatusOK, mimeType, data)
		return true
	}

	http.ServeFile(c.Writer, c.Request, filename)
	return true
}

func GenerateCached(c *gin.Context, cacheFile, mimeType string, fallback []byte, builder func() ([]byte, error)) {
	if FileExists(cacheFile) {
		c.Header("X-CDN-Status", "HIT")
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		if ServeNotModifiedOrMappedOrFile(c, cacheFile, mimeType) {
			return
		}
		c.Data(http.StatusOK, mimeType, fallback)
		return
	}

	c.Header("X-CDN-Status", "MISS")

	result, err, _ := SF.Do(cacheFile, func() (interface{}, error) {
		if FileExists(cacheFile) {
			return &BuildResult{
				CacheFile: cacheFile,
				MimeType:  mimeType,
				Data:      nil,
			}, nil
		}

		data, err := builder()
		if err != nil {
			return nil, err
		}

		if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
			return nil, err
		}

		if err := os.WriteFile(cacheFile, data, 0o644); err != nil {
			return nil, err
		}

		InvalidateMappedFile(cacheFile)

		return &BuildResult{
			CacheFile: cacheFile,
			MimeType:  mimeType,
			Data:      data,
		}, nil
	})

	if err != nil {
		c.Data(http.StatusOK, mimeType, fallback)
		return
	}

	br := result.(*BuildResult)

	c.Header("Cache-Control", "public, max-age=31536000, immutable")

	if len(br.Data) > 0 {
		if info, err := os.Stat(br.CacheFile); err == nil {
			WriteValidators(c, info)
		}
		c.Data(http.StatusOK, br.MimeType, br.Data)
		return
	}

	if ServeNotModifiedOrMappedOrFile(c, br.CacheFile, br.MimeType) {
		return
	}

	c.Data(http.StatusOK, br.MimeType, fallback)
}
