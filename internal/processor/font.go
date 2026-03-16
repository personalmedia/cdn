package processor

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
	"github.com/golang/freetype/truetype"
	"github.com/personalmedia/cdn/internal/cache"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// HandleFontAction generates a WebP image preview of a font file.
func HandleFontAction(c *gin.Context, req *ActionRequest) {
	cacheFile := cache.CacheFileForImage(req.Action, req.RelPath, req.W, req.H, req.Page, req.Filter, req.Quality)
	mimeType := cache.DetectOutputMime(cacheFile)

	if req.Action == "webp" || req.Action == "resize" {
		mimeType = "image/webp"
		if !strings.HasSuffix(strings.ToLower(cacheFile), ".webp") {
			cacheFile += ".webp"
		}
	} else if strings.HasSuffix(strings.ToLower(cacheFile), ".png") {
		mimeType = "image/png"
	}

	if cache.FileExists(cacheFile) {
		c.Header("X-CDN-Status", "HIT")
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		if cache.ServeNotModifiedOrMappedOrFile(c, cacheFile, mimeType) {
			return
		}
		c.Data(http.StatusOK, mimeType, nil)
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

		fontBytes, err := os.ReadFile(req.SourceFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read font file: %w", err)
		}

		var f font.Face
		fontSize := 48.0
		dpi := 72.0

		// try opentype first, then fallback to truetype
		if otFont, err := opentype.Parse(fontBytes); err == nil {
			f, err = opentype.NewFace(otFont, &opentype.FaceOptions{
				Size:    fontSize,
				DPI:     dpi,
				Hinting: font.HintingNone,
			})
			if err != nil {
				return nil, err
			}
		} else if ttFont, err := truetype.Parse(fontBytes); err == nil {
			f = truetype.NewFace(ttFont, &truetype.Options{
				Size:    fontSize,
				DPI:     dpi,
				Hinting: font.HintingNone,
			})
		} else {
			return nil, fmt.Errorf("failed to parse font: %w", err)
		}

		defer f.Close()

		text := "The quick brown fox jumps over the lazy dog\n0123456789\nAujourd'hui, l'élève a été très fort!"
		lines := strings.Split(text, "\n")

		// Create a canvas that's large enough for our preview
		// A standard preview size could be 800x400
		canvasW, canvasH := 1024, 400
		
		bgColor := color.RGBA{255, 255, 255, 255} // White background
		fgColor := color.RGBA{0, 0, 0, 255}       // Black text

		img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
		draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

		d := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(fgColor),
			Face: f,
		}

		// Calculate total height of all lines
		metrics := f.Metrics()
		lineHeight := (metrics.Ascent + metrics.Descent).Ceil()
		totalHeight := lineHeight * len(lines)
		
		y := (canvasH - totalHeight) / 2 + metrics.Ascent.Ceil()

		for _, line := range lines {
			d.Dot = fixed.P(40, y) // 40px left padding
			d.DrawString(line)
			y += lineHeight
		}

		// Resize if distinct dimensions are requested (and not handled by standard default)
		dstImg := image.Image(img)
		if req.W > 0 || req.H > 0 {
			// Find bounds of text content (rough approximation based on bottom line y)
			// to avoid huge white space if resized heavily.
			contentH := int(math.Min(float64(canvasH), float64(y+40)))
			croppedImg := imaging.Crop(dstImg, image.Rect(0, 0, canvasW, contentH))
			
			w, h := req.W, req.H
			if w == 0 {
				w = croppedImg.Bounds().Dx() * h / croppedImg.Bounds().Dy()
			}
			if h == 0 {
				h = croppedImg.Bounds().Dy() * w / croppedImg.Bounds().Dx()
			}
			dstImg = imaging.Resize(croppedImg, w, h, imaging.Lanczos)
		}

		if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
			return nil, err
		}

		var saveErr error
		if mimeType == "image/webp" {
			buf := new(bytes.Buffer)
			q := 75
			if req.Quality > 0 {
				q = req.Quality
			}
			if err := webp.Encode(buf, dstImg, &webp.Options{Quality: float32(q)}); err == nil {
				saveErr = os.WriteFile(cacheFile, buf.Bytes(), 0o644)
			} else {
				saveErr = err
			}
		} else {
			saveErr = imaging.Save(dstImg, cacheFile) // default save via imaging if webp isn't requested explicitly.
		}

		if saveErr != nil {
			return nil, saveErr
		}

		return &cache.BuildResult{
			CacheFile: cacheFile,
			MimeType:  mimeType,
		}, nil
	})

	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	br := result.(*cache.BuildResult)

	c.Header("Cache-Control", "public, max-age=31536000, immutable")

	if cache.ServeNotModifiedOrMappedOrFile(c, br.CacheFile, br.MimeType) {
		return
	}

	c.AbortWithStatus(http.StatusInternalServerError)
}
