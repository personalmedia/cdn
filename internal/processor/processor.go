package processor

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/personalmedia/cdn/internal/config"
)

type ActionRequest struct {
	Action     string
	Kind       string
	RelPath    string
	SourceFile string
	SourceExt  string
	W          int
	H          int
	Page       int
	Quality    int
	Filter     string
	Query      string
	AutoFormat string
	AutoHash   string
}

func SanitizeRelativePath(raw string) (string, bool) {
	relPath := filepath.Clean(strings.TrimPrefix(raw, "/"))

	if relPath == "." {
		return "", false
	}
	if strings.HasPrefix(relPath, "..") || strings.HasPrefix(relPath, "/") {
		return "", false
	}

	full := filepath.Join(config.App.SourceDir, relPath)
	if !strings.HasPrefix(full, config.App.SourceDir+string(os.PathSeparator)) && full != config.App.SourceDir {
		return "", false
	}

	return relPath, true
}

func ParseDims(rawQuery string) (int, int, int, string, int) {
	if rawQuery == "" {
		return 0, 0, 1, "", 0
	}

	page := 1
	filter := ""
	quality := 0
	pageParts := strings.Split(strings.ToLower(rawQuery), ":")
	dimsPart := pageParts[0]

	for i := 1; i < len(pageParts); i++ {
		val := pageParts[i]
		if p, err := strconv.Atoi(val); err == nil && p > 0 {
			page = p
		} else if strings.HasPrefix(val, "q") {
			if q, err := strconv.Atoi(strings.TrimPrefix(val, "q")); err == nil && q > 0 && q <= 100 {
				quality = q
			}
		} else if val == "blur" || val == "portrait" {
			filter = val
		}
	}

	parts := strings.Split(dimsPart, "x")
	if len(parts) != 2 {
		return 0, 0, page, filter, quality
	}

	w, _ := strconv.Atoi(parts[0])
	h, _ := strconv.Atoi(parts[1])

	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}

	if w > config.MaxResizeDim {
		w = config.MaxResizeDim
	}
	if h > config.MaxResizeDim {
		h = config.MaxResizeDim
	}

	return w, h, page, filter, quality
}
