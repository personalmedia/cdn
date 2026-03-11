package server

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/personalmedia/cdn/internal/config"
	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	visitors   = make(map[string]*visitor)
	visitorsMu sync.Mutex
)

func IPRateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()

		visitorsMu.Lock()
		v, exists := visitors[ip]
		if !exists {
			v = &visitor{
				limiter:  rate.NewLimiter(rate.Limit(config.App.RatePerSec), config.App.RateBurst),
				lastSeen: now,
			}
			visitors[ip] = v
		} else {
			v.lastSeen = now
		}
		visitorsMu.Unlock()

		if !v.limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": fmt.Sprintf("Too many requests (%d/s max)", config.App.RatePerSec),
			})
			return
		}

		c.Next()
	}
}

func CleanupVisitors() {
	ticker := time.NewTicker(time.Duration(config.App.IPGCMinutes) * time.Minute)
	defer ticker.Stop()

	ttl := time.Duration(config.App.IPTTLMinutes) * time.Minute

	for range ticker.C {
		now := time.Now()

		visitorsMu.Lock()
		for ip, v := range visitors {
			if now.Sub(v.lastSeen) > ttl {
				delete(visitors, ip)
			}
		}
		visitorsMu.Unlock()
	}
}

func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Next()
	}
}
