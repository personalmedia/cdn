package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"image/color"
	"image/draw"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"github.com/muesli/smartcrop"
	"github.com/muesli/smartcrop/nfnt"
	mmap "github.com/edsrzf/mmap-go"
	"github.com/gin-gonic/gin"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/joho/godotenv"
	"github.com/xuri/excelize/v2"
	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
)

type Processor func(img image.Image, w, h int) image.Image
type ActionHandler func(c *gin.Context, req *ActionRequest)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type buildResult struct {
	cacheFile string
	mimeType  string
	data      []byte
}

type mappedFile struct {
	path    string
	data    mmap.MMap
	modUnix int64
	size    int64
	mime    string
}

type sourceImage struct {
	img     image.Image
	modUnix int64
	size    int64
}

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

var (
	SourceDir string
	CacheBase string
	Port      string

	RatePerSec     int
	RateBurst      int
	IPTTLMinutes   int
	IPGCMinutes    int
	Workers        int
	SourceCacheCap int
	MMapCacheCap   int

	visitors   = make(map[string]*visitor)
	visitorsMu sync.Mutex

	sf singleflight.Group

	resizePool chan struct{}

	sourceCache   *lru.Cache[string, *sourceImage]
	sourceCacheMu sync.Mutex

	mmapCache   *lru.Cache[string, *mappedFile]
	mmapCacheMu sync.Mutex
)

const (
	MaxImageDim      = 8000
	MaxResizeDim     = 4000
	MaxCacheVariants = 20
)

var extensionKind = map[string]string{
	".jpg":  "image",
	".jpeg": "image",
	".png":  "image",
	".gif":  "image",
	".webp": "image",
	".xlsx": "excel",
}

var imageProcessors = map[string]Processor{
	"resize": processResize,
	"webp":   processResize,
	"blur":   processBlur,
}

var processors = map[string]map[string]ActionHandler{
	"image": {
		"resize": handleImageAction,
		"webp":   handleImageAction,
		"blur":   handleImageAction,
	},
	"excel": {
		"csv":  handleExcelCSV,
		"json": handleExcelJSON,
	},
}

func init() {
	_ = godotenv.Load()

	SourceDir = filepath.Clean(getEnv("SOURCE_DIR", "/datamix/cdn"))
	CacheBase = filepath.Clean(getEnv("CACHE_BASE", "/cache"))
	Port = getEnv("PORT", "9999")

	RatePerSec = getEnvInt("RATE_PER_SEC", 100)
	RateBurst = getEnvInt("RATE_BURST", 100)
	IPTTLMinutes = getEnvInt("IP_TTL_MINUTES", 10)
	IPGCMinutes = getEnvInt("IP_GC_MINUTES", 5)

	Workers = getEnvInt("WORKERS", runtime.NumCPU())
	if Workers < 1 {
		Workers = 1
	}

	SourceCacheCap = getEnvInt("SOURCE_CACHE_CAP", 512)
	if SourceCacheCap < 1 {
		SourceCacheCap = 1
	}

	MMapCacheCap = getEnvInt("MMAP_CACHE_CAP", 256)
	if MMapCacheCap < 1 {
		MMapCacheCap = 1
	}

	resizePool = make(chan struct{}, Workers)

	var err error

	sourceCache, err = lru.New[string, *sourceImage](SourceCacheCap)
	if err != nil {
		panic(err)
	}

	mmapCache, err = lru.NewWithEvict[string, *mappedFile](MMapCacheCap, func(_ string, mf *mappedFile) {
		if mf != nil && mf.data != nil {
			_ = mf.data.Unmap()
		}
	})
	if err != nil {
		panic(err)
	}
}

func main() {
	gin.SetMode(gin.ReleaseMode)

	go cleanupVisitors()

	r := gin.New()

	if proxies := getEnv("TRUSTED_PROXIES", ""); proxies != "" {
		_ = r.SetTrustedProxies(strings.Split(proxies, ","))
	} else {
		_ = r.SetTrustedProxies(nil)
	}

	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Next()
	})
	r.Use(IPRateLimiter())

	r.GET("/metadata/*path", func(c *gin.Context) {
		relPath, ok := sanitizeRelativePath(c.Param("path"))
		if !ok {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		handleMetadata(c, relPath)
	})

	r.GET("/o/:action/*path", routeProcessor)

	// Compat vieux format si tu veux le garder.
	r.GET("/:action/*path", func(c *gin.Context) {
		if c.Param("action") == "metadata" {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		routeProcessor(c)
	})

	fmt.Printf(
		"🚀 Universal Transformer :%s | Source: %s | Cache: %s | Workers: %d | SourceLRU: %d | MMapLRU: %d\n",
		Port,
		SourceDir,
		CacheBase,
		cap(resizePool),
		SourceCacheCap,
		MMapCacheCap,
	)

	if err := r.Run(":" + Port); err != nil {
		panic(err)
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

func IPRateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()

		visitorsMu.Lock()
		v, exists := visitors[ip]
		if !exists {
			v = &visitor{
				limiter:  rate.NewLimiter(rate.Limit(RatePerSec), RateBurst),
				lastSeen: now,
			}
			visitors[ip] = v
		} else {
			v.lastSeen = now
		}
		visitorsMu.Unlock()

		if !v.limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": fmt.Sprintf("Too many requests (%d/s max)", RatePerSec),
			})
			return
		}

		c.Next()
	}
}

func cleanupVisitors() {
	ticker := time.NewTicker(time.Duration(IPGCMinutes) * time.Minute)
	defer ticker.Stop()

	ttl := time.Duration(IPTTLMinutes) * time.Minute

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

func routeProcessor(c *gin.Context) {
	actionName := c.Param("action")

	relPath, ok := sanitizeRelativePath(c.Param("path"))
	if !ok {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	ext := strings.ToLower(filepath.Ext(relPath))
	kind, ok := extensionKind[ext]
	if !ok {
		// Infer kind from the action being requested to support files without extensions
		if actionName == "resize" || actionName == "webp" || actionName == "blur" {
			kind = "image"
		} else if actionName == "csv" || actionName == "json" {
			kind = "excel"
		} else {
			c.AbortWithStatus(http.StatusUnsupportedMediaType)
			return
		}
	}

	kindHandlers, ok := processors[kind]
	if !ok {
		c.AbortWithStatus(http.StatusUnsupportedMediaType)
		return
	}

	handler, ok := kindHandlers[actionName]
	if !ok {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	w, h := 0, 0
	if kind == "image" {
		w, h = parseDims(c.Request.URL.RawQuery)
	}

	req := &ActionRequest{
		Action:     actionName,
		Kind:       kind,
		RelPath:    relPath,
		SourceFile: filepath.Join(SourceDir, relPath),
		SourceExt:  ext,
		W:          w,
		H:          h,
		Query:      c.Request.URL.RawQuery,
	}

	handler(c, req)
}

func sanitizeRelativePath(raw string) (string, bool) {
	relPath := filepath.Clean(strings.TrimPrefix(raw, "/"))

	if relPath == "." {
		return "", false
	}
	if strings.HasPrefix(relPath, "..") || strings.HasPrefix(relPath, "/") {
		return "", false
	}

	full := filepath.Join(SourceDir, relPath)
	if !strings.HasPrefix(full, SourceDir+string(os.PathSeparator)) && full != SourceDir {
		return "", false
	}

	return relPath, true
}

func parseDims(rawQuery string) (int, int) {
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

	if w > MaxResizeDim {
		w = MaxResizeDim
	}
	if h > MaxResizeDim {
		h = MaxResizeDim
	}

	return w, h
}

func normalizedDimsFolder(w, h int) string {
	if w == 0 && h == 0 {
		return "original"
	}
	return fmt.Sprintf("%dx%d", w, h)
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func sourceExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func cacheFileForImage(req *ActionRequest) string {
	folder := normalizedDimsFolder(req.W, req.H)
	cacheFile := filepath.Join(CacheBase, req.Action, folder, req.RelPath)

	if req.Action == "webp" && !strings.HasSuffix(strings.ToLower(cacheFile), ".webp") {
		cacheFile += ".webp"
	}

	return cacheFile
}

func cacheFileForDerived(action, relPath, outputExt string) string {
	return filepath.Join(CacheBase, action, relPath+outputExt)
}

func detectOutputMime(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if mimeType := mime.TypeByExtension(ext); mimeType != "" {
		return mimeType
	}
	return "application/octet-stream"
}

func makeETag(info os.FileInfo) string {
	return fmt.Sprintf(`W/"%x-%x"`, info.ModTime().UTC().UnixNano(), info.Size())
}

func writeValidators(c *gin.Context, info os.FileInfo) {
	mod := info.ModTime().UTC()
	c.Header("ETag", makeETag(info))
	c.Header("Last-Modified", mod.Format(http.TimeFormat))
}

func isNotModified(c *gin.Context, info os.FileInfo) bool {
	writeValidators(c, info)

	etag := makeETag(info)
	if inm := strings.TrimSpace(c.GetHeader("If-None-Match")); inm != "" && inm == etag {
		c.Status(http.StatusNotModified)
		return true
	}

	if ims := strings.TrimSpace(c.GetHeader("If-Modified-Since")); ims != "" {
		if t, err := time.Parse(http.TimeFormat, ims); err == nil {
			mod := info.ModTime().UTC().Truncate(time.Second)
			if !mod.After(t.UTC()) {
				c.Status(http.StatusNotModified)
				return true
			}
		}
	}

	return false
}

func serveNotModifiedOrMappedOrFile(c *gin.Context, filename, mimeType string) bool {
	info, err := os.Stat(filename)
	if err != nil {
		return false
	}

	if isNotModified(c, info) {
		return true
	}

	if data, ok, err := getMappedFile(filename, mimeType); err == nil && ok {
		c.Data(http.StatusOK, mimeType, data)
		return true
	}

	http.ServeFile(c.Writer, c.Request, filename)
	return true
}

func getMappedFile(path, mimeType string) ([]byte, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false, err
	}

	modUnix := info.ModTime().UTC().UnixNano()
	size := info.Size()

	mmapCacheMu.Lock()
	defer mmapCacheMu.Unlock()

	if mf, ok := mmapCache.Get(path); ok {
		if mf != nil && mf.modUnix == modUnix && mf.size == size {
			return mf.data, true, nil
		}
		mmapCache.Remove(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	mm, err := mmap.Map(f, mmap.RDONLY, 0)
	if err != nil {
		return nil, false, err
	}

	entry := &mappedFile{
		path:    path,
		data:    mm,
		modUnix: modUnix,
		size:    size,
		mime:    mimeType,
	}

	mmapCache.Add(path, entry)

	return entry.data, true, nil
}

func invalidateMappedFile(path string) {
	mmapCacheMu.Lock()
	defer mmapCacheMu.Unlock()
	mmapCache.Remove(path)
}

func loadSourceImage(path string) (image.Image, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	modUnix := info.ModTime().UTC().UnixNano()
	size := info.Size()

	sourceCacheMu.Lock()
	if entry, ok := sourceCache.Get(path); ok {
		if entry != nil && entry.modUnix == modUnix && entry.size == size {
			sourceCacheMu.Unlock()
			return entry.img, nil
		}
		sourceCache.Remove(path)
	}
	sourceCacheMu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if cfg, _, err := image.DecodeConfig(f); err == nil {
		if cfg.Width > MaxImageDim || cfg.Height > MaxImageDim {
			return nil, fmt.Errorf("image too large: %dx%d (max %d)", cfg.Width, cfg.Height, MaxImageDim)
		}
	} else {
		// If decoding the config fails (e.g. file is not a valid image format), return an error
		// so the caller can fallback to a placeholder
		return nil, err
	}

	img, err := imaging.Open(path)
	if err != nil {
		return nil, err
	}

	sourceCacheMu.Lock()
	sourceCache.Add(path, &sourceImage{
		img:     img,
		modUnix: modUnix,
		size:    size,
	})
	sourceCacheMu.Unlock()

	return img, nil
}

func generateCached(c *gin.Context, cacheFile, mimeType string, builder func() ([]byte, error)) {
	if fileExists(cacheFile) {
		c.Header("X-CDN-Status", "HIT")
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		if serveNotModifiedOrMappedOrFile(c, cacheFile, mimeType) {
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Header("X-CDN-Status", "MISS")

	result, err, _ := sf.Do(cacheFile, func() (interface{}, error) {
		if fileExists(cacheFile) {
			return &buildResult{
				cacheFile: cacheFile,
				mimeType:  mimeType,
				data:      nil,
			}, nil
		}

		data, err := builder()
		if err != nil {
			return nil, err
		}

		if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
			return nil, err
		}

		if err := os.WriteFile(cacheFile, data, 0o644); err != nil {
			return nil, err
		}

		invalidateMappedFile(cacheFile)

		return &buildResult{
			cacheFile: cacheFile,
			mimeType:  mimeType,
			data:      data,
		}, nil
	})

	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	br := result.(*buildResult)

	c.Header("Cache-Control", "public, max-age=31536000, immutable")

	if len(br.data) > 0 {
		if info, err := os.Stat(br.cacheFile); err == nil {
			writeValidators(c, info)
		}
		c.Data(http.StatusOK, br.mimeType, br.data)
		return
	}

	if serveNotModifiedOrMappedOrFile(c, br.cacheFile, br.mimeType) {
		return
	}

	c.AbortWithStatus(http.StatusInternalServerError)
}

func handleImageAction(c *gin.Context, req *ActionRequest) {
	proc, ok := imageProcessors[req.Action]
	if !ok {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	cacheFile := cacheFileForImage(req)
	mimeType := detectOutputMime(cacheFile)

	if req.Action == "webp" {
		mimeType = "image/webp"
	}

	if !fileExists(cacheFile) {
		pattern := filepath.Join(CacheBase, req.Action, "*", req.RelPath)
		if req.Action == "webp" {
			pattern += ".webp"
		}
		if matches, _ := filepath.Glob(pattern); len(matches) >= MaxCacheVariants {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "too many cache variants generated for this image",
			})
			return
		}
	}

	if fileExists(cacheFile) {
		c.Header("X-CDN-Status", "HIT")
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		if serveNotModifiedOrMappedOrFile(c, cacheFile, mimeType) {
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Header("X-CDN-Status", "MISS")

	result, err, _ := sf.Do(cacheFile, func() (interface{}, error) {
		if fileExists(cacheFile) {
			return &buildResult{
				cacheFile: cacheFile,
				mimeType:  mimeType,
				data:      nil,
			}, nil
		}

		var img image.Image

		if !sourceExists(req.SourceFile) {
			img = generatePlaceholder(req.W, req.H)
		} else {
			var err error
			img, err = loadSourceImage(req.SourceFile)
			if err != nil {
				// Fallback to placeholder on decode failure (e.g. not an image)
				img = generatePlaceholder(req.W, req.H)
			}
		}

		resizePool <- struct{}{}
		dst := proc(img, req.W, req.H)
		<-resizePool

		if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
			return nil, err
		}

		if req.Action == "webp" {
			buf := new(bytes.Buffer)

			if err := webp.Encode(buf, dst, &webp.Options{Quality: 75}); err != nil {
				return nil, err
			}

			if err := os.WriteFile(cacheFile, buf.Bytes(), 0o644); err != nil {
				return nil, err
			}

			invalidateMappedFile(cacheFile)

			return &buildResult{
				cacheFile: cacheFile,
				mimeType:  mimeType,
				data:      buf.Bytes(),
			}, nil
		}

		if err := imaging.Save(dst, cacheFile); err != nil {
			return nil, err
		}

		invalidateMappedFile(cacheFile)

		return &buildResult{
			cacheFile: cacheFile,
			mimeType:  mimeType,
			data:      nil,
		}, nil
	})

	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	br := result.(*buildResult)

	c.Header("Cache-Control", "public, max-age=31536000, immutable")

	if len(br.data) > 0 {
		if info, err := os.Stat(br.cacheFile); err == nil {
			writeValidators(c, info)
		}
		c.Data(http.StatusOK, br.mimeType, br.data)
		return
	}

	if serveNotModifiedOrMappedOrFile(c, br.cacheFile, br.mimeType) {
		return
	}

	c.AbortWithStatus(http.StatusInternalServerError)
}

func handleExcelCSV(c *gin.Context, req *ActionRequest) {
	cacheFile := cacheFileForDerived("csv", req.RelPath, ".csv")
	filename := strings.TrimSuffix(filepath.Base(req.RelPath), filepath.Ext(req.RelPath)) + ".csv"

	c.Header(
		"Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, url.PathEscape(filename)),
	)

	generateCached(c, cacheFile, "text/csv; charset=utf-8", func() ([]byte, error) {
		f, err := excelize.OpenFile(req.SourceFile, excelize.Options{UnzipXMLSizeLimit: 250 * 1024 * 1024})
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()

		sheet := primarySheetName(f)
		rows, err := f.GetRows(sheet)
		if err != nil {
			return nil, err
		}

		buf := new(bytes.Buffer)
		writer := csv.NewWriter(buf)

		for _, row := range rows {
			if err := writer.Write(row); err != nil {
				return nil, err
			}
		}

		writer.Flush()
		if err := writer.Error(); err != nil {
			return nil, err
		}

		return buf.Bytes(), nil
	})
}

func handleExcelJSON(c *gin.Context, req *ActionRequest) {
	cacheFile := cacheFileForDerived("json", req.RelPath, ".json")
	filename := strings.TrimSuffix(filepath.Base(req.RelPath), filepath.Ext(req.RelPath)) + ".json"

	c.Header(
		"Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, url.PathEscape(filename)),
	)

	generateCached(c, cacheFile, "application/json; charset=utf-8", func() ([]byte, error) {
		f, err := excelize.OpenFile(req.SourceFile, excelize.Options{UnzipXMLSizeLimit: 250 * 1024 * 1024})
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()

		sheets := f.GetSheetList()
		payload := make(map[string][][]string, len(sheets))

		for _, sheet := range sheets {
			rows, err := f.GetRows(sheet)
			if err != nil {
				return nil, err
			}
			payload[sheet] = rows
		}

		return json.Marshal(payload)
	})
}

func primarySheetName(f *excelize.File) string {
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return "Sheet1"
	}
	return sheets[0]
}

func handleMetadata(c *gin.Context, relPath string) {
	sourceFile := filepath.Join(SourceDir, relPath)

	info, err := os.Stat(sourceFile)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	width, height := 0, 0
	kind := extensionKind[strings.ToLower(filepath.Ext(relPath))]

	if kind == "image" || kind == "" {
		reader, err := os.Open(sourceFile)
		if err == nil {
			defer reader.Close()
			if config, _, err := image.DecodeConfig(reader); err == nil {
				width = config.Width
				height = config.Height
				if kind == "" {
					kind = "image"
				}
			}
		}
	}

	if kind == "" {
		kind = "unknown"
	}

	writeValidators(c, info)

	c.JSON(http.StatusOK, gin.H{
		"filename": filepath.Base(sourceFile),
		"path":     relPath,
		"kind":     kind,
		"size":     info.Size(),
		"mime":     mime.TypeByExtension(filepath.Ext(sourceFile)),
		"width":    width,
		"height":   height,
		"modified": info.ModTime().Format("2006-01-02 15:04:05"),
	})
}

func processResize(img image.Image, w, h int) image.Image {
	if w == 0 && h == 0 {
		return img
	}

	// Simple scale if only one dimension is provided (keeps proportional ratio)
	if w == 0 || h == 0 {
		return imaging.Resize(img, w, h, imaging.Lanczos)
	}

	// For specific wxh bounded dimensions, dynamically crop to the best content first
	analyzer := smartcrop.NewAnalyzer(nfnt.NewDefaultResizer())
	topCrop, err := analyzer.FindBestCrop(img, w, h)
	if err == nil {
		img = imaging.Crop(img, topCrop)
	}

	// Final scale down to exact bounds
	return imaging.Resize(img, w, h, imaging.Lanczos)
}

func processBlur(img image.Image, w, h int) image.Image {
	return imaging.Blur(processResize(img, w, h), 5.0)
}

func generatePlaceholder(w, h int) image.Image {
	if w == 0 && h > 0 {
		w = h
	}
	if h == 0 && w > 0 {
		h = w
	}
	if w == 0 && h == 0 {
		w, h = 400, 400
	}

	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(
		canvas,
		canvas.Bounds(),
		&image.Uniform{C: color.RGBA{180, 180, 180, 255}},
		image.Point{},
		draw.Src,
	)

	return canvas
}