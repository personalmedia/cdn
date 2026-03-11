package cache

import (
	"fmt"
	"image"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/personalmedia/cdn/internal/config"
	"golang.org/x/sync/singleflight"
)

type SourceImage struct {
	img     image.Image
	modUnix int64
	size    int64
}

type BuildResult struct {
	CacheFile string
	MimeType  string
	Data      []byte
}

var (
	SF singleflight.Group

	sourceLRU   *lru.Cache[string, *SourceImage]
	sourceLRUMu sync.Mutex
)

func Init() {
	var err error
	sourceLRU, err = lru.New[string, *SourceImage](config.App.SourceCacheCap)
	if err != nil {
		panic(err)
	}
	initMmap()
}

func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func SourceExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func NormalizedDimsFolder(w, h int) string {
	if w == 0 && h == 0 {
		return "original"
	}
	return fmt.Sprintf("%dx%d", w, h)
}

func CacheFileForImage(action, relPath string, w, h int) string {
	folder := NormalizedDimsFolder(w, h)
	cacheFile := filepath.Join(config.App.CacheBase, action, folder, relPath)

	if action == "webp" && !strings.HasSuffix(strings.ToLower(cacheFile), ".webp") {
		cacheFile += ".webp"
	}
	return cacheFile
}

func CacheFileForDerived(action, relPath, outputExt string) string {
	return filepath.Join(config.App.CacheBase, action, relPath+outputExt)
}

func DetectOutputMime(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if mimeType := mime.TypeByExtension(ext); mimeType != "" {
		return mimeType
	}
	return "application/octet-stream"
}

func LoadSourceImage(path string) (image.Image, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	modUnix := info.ModTime().UTC().UnixNano()
	size := info.Size()

	sourceLRUMu.Lock()
	if entry, ok := sourceLRU.Get(path); ok {
		if entry != nil && entry.modUnix == modUnix && entry.size == size {
			sourceLRUMu.Unlock()
			return entry.img, nil
		}
		sourceLRU.Remove(path)
	}
	sourceLRUMu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if cfg, _, err := image.DecodeConfig(f); err == nil {
		if cfg.Width > config.MaxImageDim || cfg.Height > config.MaxImageDim {
			return nil, fmt.Errorf("image too large: %dx%d (max %d)", cfg.Width, cfg.Height, config.MaxImageDim)
		}
	} else {
		return nil, err
	}

	img, err := imaging.Open(path)
	if err != nil {
		return nil, err
	}

	sourceLRUMu.Lock()
	sourceLRU.Add(path, &SourceImage{
		img:     img,
		modUnix: modUnix,
		size:    size,
	})
	sourceLRUMu.Unlock()

	return img, nil
}
