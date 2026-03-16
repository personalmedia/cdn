package processor

import (
	"crypto/sha1"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// clamp limits val between min and max
func clamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

// NegotiateParams analyzes the request headers and query overrides to determine the optimal image format, dimensions, and quality.
func NegotiateParams(r *http.Request, wOverride, hOverride, qOverride int, filter string) (w int, h int, format string, quality int, hash string) {
	// 1. Format Negotiation (Accept Header)
	accept := strings.ToLower(r.Header.Get("Accept"))
	format = "jpeg" // default
	if strings.Contains(accept, "image/avif") || strings.Contains(accept, "image/webp") {
		// As per plan, we fallback AVIF to WebP for now until a pure-Go AVIF encoder is ready.
		format = "webp"
	}

	// 2. Dynamic Resolution (Viewport-Width, Width, DPR)
	dpr := 1.0
	if dprStr := r.Header.Get("DPR"); dprStr != "" {
		if parsed, err := strconv.ParseFloat(dprStr, 64); err == nil && parsed > 0 {
			dpr = parsed
		}
	}

	viewport := 0
	if vpStr := r.Header.Get("Viewport-Width"); vpStr != "" {
		if parsed, err := strconv.Atoi(vpStr); err == nil && parsed > 0 {
			viewport = parsed
		}
	}
	
	widthHint := 0
	if wStr := r.Header.Get("Width"); wStr != "" {
		if parsed, err := strconv.Atoi(wStr); err == nil && parsed > 0 {
			widthHint = parsed
		}
	}

	// Resolution algorithm: width = viewport * DPR (clamp 320-2400)
	baseWidth := viewport
	if widthHint > 0 {
		baseWidth = widthHint
	}

	if baseWidth > 0 {
		w = int(float64(baseWidth) * dpr)
		w = clamp(w, 320, 2400)
	}
	h = 0 // maintain aspect ratio

	// Apply overrides
	if wOverride > 0 {
		w = wOverride
	}
	if hOverride > 0 {
		h = hOverride
	}

	// 3. Quality Profiles (Save-Data, User-Agent)
	quality = 85 // default high quality
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	saveData := strings.ToLower(r.Header.Get("Save-Data")) == "on"

	if saveData {
		quality = 40
	} else if strings.Contains(ua, "mobile") {
		quality = 65
	} else if strings.Contains(ua, "windows") || strings.Contains(ua, "macintosh") || strings.Contains(ua, "linux") {
		quality = 75
	}

	if qOverride > 0 {
		quality = qOverride
	}

	// 4. Cache Key (Hash of factors)
	hashInput := fmt.Sprintf("%s:%d:%d:%d:%s", format, w, h, quality, filter)
	sum := sha1.Sum([]byte(hashInput))
	hash = fmt.Sprintf("%x", sum)

	return w, h, format, quality, hash
}
