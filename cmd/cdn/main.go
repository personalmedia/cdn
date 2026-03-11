package main

import (
	"io"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/l3dlp/logfile"
	"github.com/personalmedia/cdn/internal/cache"
	"github.com/personalmedia/cdn/internal/config"
	"github.com/personalmedia/cdn/internal/processor"
	"github.com/personalmedia/cdn/internal/server"
)

func main() {
	config.Load()
	
	f := logfile.Use(config.App.LogFile)
	if f != nil {
		gin.DefaultWriter = io.MultiWriter(f, os.Stdout)
	}

	cache.Init()
	processor.InitImage()

	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
