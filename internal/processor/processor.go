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

func ParseDims(rawQuery string) (int, int) {
	if rawQuery == "" {
		return 0, 0
	}

	parts := strings.Split(strings.ToLower(rawQuery), "x")
	if len(parts) != 2 {
		return 0, 0
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

	return w, h
}
