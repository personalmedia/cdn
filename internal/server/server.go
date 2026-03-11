package server

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/personalmedia/cdn/internal/config"
)

func Start() error {
	gin.SetMode(gin.ReleaseMode)

	go CleanupVisitors()

	r := gin.New()

	if len(config.App.TrustedProxies) > 0 {
		_ = r.SetTrustedProxies(config.App.TrustedProxies)
	} else {
		_ = r.SetTrustedProxies(nil)
	}

	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(SecurityHeaders())
	r.Use(IPRateLimiter())

	r.GET("/metadata/*path", HandleMetadata)

	r.GET("/o/:action/*path", RouteProcessor)

	// Compat vieux format
	r.GET("/:action/*path", func(c *gin.Context) {
		if c.Param("action") == "metadata" {
			c.AbortWithStatus(404)
			return
		}
		RouteProcessor(c)
	})

	log.Printf(
		"🚀 Universal Transformer :%s | Source: %s | Cache: %s | Workers: %d | SourceLRU: %d | MMapLRU: %d\n",
		config.App.Port,
		config.App.SourceDir,
		config.App.CacheBase,
		config.App.Workers,
		config.App.SourceCacheCap,
		config.App.MMapCacheCap,
	)

	return r.Run(":" + config.App.Port)
}
