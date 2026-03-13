package cache

import (
	"bytes"
	"fmt"
	"image"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/personalmedia/cdn/internal/config"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
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

func NormalizedDimsFolder(w, h, page int, filter string, quality int) string {
	var base string
	if w == 0 && h == 0 {
		base = "original"
	} else {
		base = fmt.Sprintf("%dx%d", w, h)
	}
	
	if page > 1 {
		base = fmt.Sprintf("%s_p%d", base, page)
	}
	if filter != "" {
		base = fmt.Sprintf("%s_%s", base, filter)
	}
	if quality > 0 {
		base = fmt.Sprintf("%s_q%d", base, quality)
	}
	return base
}

func CacheFileForImage(action, relPath string, w, h, page int, filter string, quality int) string {
	folder := NormalizedDimsFolder(w, h, page, filter, quality)
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

func LoadSourceImage(path string, reqW, reqH, reqPage int) (image.Image, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	modUnix := info.ModTime().UTC().UnixNano()
	size := info.Size()

	var cacheKey string
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".svg":
		cacheKey = fmt.Sprintf("%s@%dx%d", path, reqW, reqH)
	case ".pdf":
		cacheKey = fmt.Sprintf("%s@p%d", path, reqPage)
	default:
		cacheKey = path
	}

	sourceLRUMu.Lock()
	if entry, ok := sourceLRU.Get(cacheKey); ok {
		if entry != nil && entry.modUnix == modUnix && entry.size == size {
			sourceLRUMu.Unlock()
			return entry.img, nil
		}
		sourceLRU.Remove(cacheKey)
	}
	sourceLRUMu.Unlock()

	var img image.Image

	switch ext {
	case ".svg":
		img, err = loadSVGAsImage(path, reqW, reqH)
		if err != nil {
			return nil, err
		}
	case ".pdf":
		img, err = GeneratePDFCover(path, reqPage)
		if err != nil {
			return nil, err
		}
	default:
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

		img, err = imaging.Open(path)
		if err != nil {
			return nil, err
		}
	}

	sourceLRUMu.Lock()
	sourceLRU.Add(cacheKey, &SourceImage{
		img:     img,
		modUnix: modUnix,
		size:    size,
	})
	sourceLRUMu.Unlock()

	return img, nil
}

func loadSVGAsImage(path string, reqW, reqH int) (image.Image, error) {
	icon, err := oksvg.ReadIcon(path, oksvg.IgnoreErrorMode)
	if err != nil {
		return nil, err
	}

	viewW, viewH := float64(icon.ViewBox.W), float64(icon.ViewBox.H)
	if viewW == 0 || viewH == 0 {
		viewW, viewH = 512, 512
	}

	// Calculate target dimensions proportionality
	targetW, targetH := float64(reqW), float64(reqH)
	
	if targetW == 0 && targetH == 0 {
		targetW, targetH = viewW, viewH
	} else if targetW == 0 {
		targetW = targetH * (viewW / viewH)
	} else if targetH == 0 {
		targetH = targetW * (viewH / viewW)
	}

	w, h := int(targetW), int(targetH)

	icon.SetTarget(0, 0, targetW, targetH)

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, img, img.Bounds())
	raster := rasterx.NewDasher(w, h, scanner)
	icon.Draw(raster, 1.0)

	return img, nil
}

// GeneratePDFCover uses os/exec to call pdftoppm (poppler-utils) to rasterize
// the specific page of the PDF into an image.Image.
func GeneratePDFCover(path string, page int) (image.Image, error) {
	if page < 1 {
		page = 1
	}
	pageStr := strconv.Itoa(page)

	tmpDir := os.TempDir()
	prefix := fmt.Sprintf("cdn_pdf_cov_%d", time.Now().UnixNano())
	outPrefix := filepath.Join(tmpDir, prefix)
	
	defer func() {
		os.Remove(outPrefix + "-" + pageStr + ".png")
	}()

	cmd := exec.Command("pdftoppm", "-f", pageStr, "-l", pageStr, "-png", path, outPrefix)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftoppm failed: %v, stderr: %s. Is poppler-utils installed?", err, stderr.String())
	}

	imgFile := outPrefix + "-" + pageStr + ".png"
	if _, err := os.Stat(imgFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("pdftoppm did not produce the expected file: %s", imgFile)
	}

	img, err := imaging.Open(imgFile)
	if err != nil {
		return nil, fmt.Errorf("failed to decode generated pdf cover: %w", err)
	}

	return img, nil
}
