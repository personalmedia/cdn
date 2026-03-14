package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const (
	MaxImageDim      = 8000
	MaxResizeDim     = 4000
	MaxCacheVariants = 20
)

type AppConfig struct {
	SourceDir      string
	CacheBase      string
	CDNPath        string
	Port           string
	RatePerSec     int
	RateBurst      int
	IPTTLMinutes   int
	IPGCMinutes    int
	Workers        int
	SourceCacheCap int
	MMapCacheCap   int
	TrustedProxies []string
	LogFile        string
}

// App holds the global configuration state after Load() is called.
var App *AppConfig

// Load reads from the environment and populates the global App config.
func Load() {
	_ = godotenv.Load()

	App = &AppConfig{
		SourceDir:      filepath.Clean(getEnv("SOURCE_DIR", "/datamix/cdn")),
		CacheBase:      filepath.Clean(getEnv("CACHE_BASE", "/cache")),
		CDNPath:        getEnv("CDN_PATH", "/cdn"),
		Port:           getEnv("PORT", "9999"),
		RatePerSec:     getEnvInt("RATE_PER_SEC", 100),
		RateBurst:      getEnvInt("RATE_BURST", 100),
		IPTTLMinutes:   getEnvInt("IP_TTL_MINUTES", 10),
		IPGCMinutes:    getEnvInt("IP_GC_MINUTES", 5),
		Workers:        getEnvInt("WORKERS", runtime.NumCPU()),
		SourceCacheCap: getEnvInt("SOURCE_CACHE_CAP", 512),
		MMapCacheCap:   getEnvInt("MMAP_CACHE_CAP", 256),
		LogFile:        filepath.Clean(getEnv("CDN_LOG_FILE", "/var/log/cdn.log")),
	}

	if App.Workers < 1 {
		App.Workers = 1
	}
	if App.SourceCacheCap < 1 {
		App.SourceCacheCap = 1
	}
	if App.MMapCacheCap < 1 {
		App.MMapCacheCap = 1
	}

	proxies := getEnv("TRUSTED_PROXIES", "")
	if proxies != "" {
		App.TrustedProxies = strings.Split(proxies, ",")
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}
