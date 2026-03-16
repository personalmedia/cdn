package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

func main() {
	rawPath := "/photo.jpg"
	var actionName, relPath string
	ext := strings.ToLower(filepath.Ext(rawPath))

	if ext == ".webp" || ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".csv" || ext == ".json" || ext == ".txt" {
		actionName = strings.TrimPrefix(ext, ".")
		relPath = strings.TrimSuffix(rawPath, ext)
		if actionName == "txt" {
			actionName = "text"
		}
		if actionName == "jpg" || actionName == "jpeg" || actionName == "png" || actionName == "gif" {
			actionName = "resize"
		}
	} else {
		actionName = "resize"
		relPath = rawPath
	}

	fmt.Println("actionName:", actionName)
	fmt.Println("relPath:", relPath)
}
