package processor

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
	"github.com/muesli/smartcrop"
	"github.com/muesli/smartcrop/nfnt"
	"github.com/personalmedia/cdn/internal/cache"
	"github.com/personalmedia/cdn/internal/config"
)

type ProcessorFunc func(img image.Image, w, h int) image.Image

var ImageProcessors = map[string]ProcessorFunc{
	"resize":   processResize,
	"webp":     processResize,
	"blur":     processBlur,
	"portrait": ProcessPortraitWithFaceDetect,
}

var ResizePool chan struct{}

func InitImage() {
	ResizePool = make(chan struct{}, config.App.Workers)
}

func HandleImageAction(c *gin.Context, req *ActionRequest) {
	proc, ok := ImageProcessors[req.Action]
	if !ok {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	cacheFile := cache.CacheFileForImage(req.Action, req.RelPath, req.W, req.H)
	mimeType := cache.DetectOutputMime(cacheFile)

	if req.Action == "webp" {
		mimeType = "image/webp"
	} else if strings.HasSuffix(strings.ToLower(cacheFile), ".pdf") {
		mimeType = "image/png"
	}

	if !cache.FileExists(cacheFile) {
		pattern := filepath.Join(config.App.CacheBase, req.Action, "*", req.RelPath)
		if req.Action == "webp" {
			pattern += ".webp"
		}
		if matches, _ := filepath.Glob(pattern); len(matches) >= config.MaxCacheVariants {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "too many cache variants generated for this image",
			})
			return
		}
	}

	if cache.FileExists(cacheFile) {
		c.Header("X-CDN-Status", "HIT")
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		if cache.ServeNotModifiedOrMappedOrFile(c, cacheFile, mimeType) {
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Header("X-CDN-Status", "MISS")

	result, err, _ := cache.SF.Do(cacheFile, func() (interface{}, error) {
		if cache.FileExists(cacheFile) {
			return &cache.BuildResult{
				CacheFile: cacheFile,
				MimeType:  mimeType,
				Data:      nil,
			}, nil
		}

		var img image.Image

		if !cache.SourceExists(req.SourceFile) {
			img = generatePlaceholder(req.W, req.H)
		} else {
			var err error
			img, err = cache.LoadSourceImage(req.SourceFile, req.W, req.H)
			if err != nil {
				img = generatePlaceholder(req.W, req.H)
			}
		}

		ResizePool <- struct{}{}
		dst := proc(img, req.W, req.H)
		<-ResizePool

		if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
			return nil, err
		}

		if req.Action == "webp" {
			buf := new(bytes.Buffer)

			if err := webp.Encode(buf, dst, &webp.Options{Quality: 75}); err != nil {
				return nil, err
			}

			if err := os.WriteFile(cacheFile, buf.Bytes(), 0o644); err != nil {
				return nil, err
			}

			cache.InvalidateMappedFile(cacheFile)

			return &cache.BuildResult{
				CacheFile: cacheFile,
				MimeType:  mimeType,
				Data:      buf.Bytes(),
			}, nil
		}

		var saveErr error
		if strings.HasSuffix(strings.ToLower(cacheFile), ".pdf") {
			tmpPng := cacheFile + ".png"
			if err := imaging.Save(dst, tmpPng); err == nil {
				saveErr = os.Rename(tmpPng, cacheFile)
			} else {
				saveErr = err
			}
		} else {
			saveErr = imaging.Save(dst, cacheFile)
		}
		
		if saveErr != nil {
			return nil, saveErr
		}

		cache.InvalidateMappedFile(cacheFile)

		return &cache.BuildResult{
			CacheFile: cacheFile,
			MimeType:  mimeType,
			Data:      nil,
		}, nil
	})

	if err != nil {
		log.Println("HandleImageAction Error:", err)
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

	c.AbortWithStatus(http.StatusInternalServerError)
}

func processResize(img image.Image, w, h int) image.Image {
	if w == 0 && h == 0 {
		return img
	}

	if w == 0 || h == 0 {
		return imaging.Resize(img, w, h, imaging.Lanczos)
	}

	analyzer := smartcrop.NewAnalyzer(nfnt.NewDefaultResizer())
	topCrop, err := analyzer.FindBestCrop(img, w, h)
	if err == nil {
		img = imaging.Crop(img, topCrop)
	}

	return imaging.Resize(img, w, h, imaging.Lanczos)
}

func processBlur(img image.Image, w, h int) image.Image {
	return imaging.Blur(processResize(img, w, h), 5.0)
}

func generatePlaceholder(w, h int) image.Image {
	if w == 0 && h > 0 {
		w = h
	}
	if h == 0 && w > 0 {
		h = w
	}
	if w == 0 && h == 0 {
		w, h = 400, 400
	}

	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(
		canvas,
		canvas.Bounds(),
		&image.Uniform{C: color.RGBA{180, 180, 180, 255}},
		image.Point{},
		draw.Src,
	)

	return canvas
}
