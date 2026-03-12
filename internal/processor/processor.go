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
	Query      string
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

func ParseDims(rawQuery string) (int, int, int) {
	if rawQuery == "" {
		return 0, 0, 1
	}

	page := 1
	pageParts := strings.Split(strings.ToLower(rawQuery), ":")
	dimsPart := pageParts[0]

	if len(pageParts) > 1 {
		if p, err := strconv.Atoi(pageParts[1]); err == nil && p > 0 {
			page = p
		}
	}

	parts := strings.Split(dimsPart, "x")
	if len(parts) != 2 {
		return 0, 0, page
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

	return w, h, page
}
