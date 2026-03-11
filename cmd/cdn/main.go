package main

import (
	"log"

	"github.com/personalmedia/cdn/internal/cache"
	"github.com/personalmedia/cdn/internal/config"
	"github.com/personalmedia/cdn/internal/processor"
	"github.com/personalmedia/cdn/internal/server"
)

func main() {
	config.Load()
	cache.Init()
	processor.InitImage()

	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
