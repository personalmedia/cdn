# 🚀 Datamix Universal Transformer (Go-Gin Edition)

🇬🇧 English | 🇫🇷 [Français](README.fr.md)

A high-performance, secure asset transformation engine. It acts as a smart proxy to turn any static storage into a modern CDN capable of real-time image processing and **dynamic data conversion (Excel to JSON/CSV)**.

## ✨ New & Core Features

* **Multimodal Processing**: Handles both **Images** (Resize, WebP, Blur) and **Data** (Excel XLSX to CSV/JSON).
* **Dual-Layer LRU Cache**:
* **Source Cache**: Keeps decoded images in memory to avoid redundant disk I/O.
* **MMap Cache**: Uses `mmap-go` to map hot cache files directly into memory address space for near-zero latency delivery.


* **Worker Pool & Rate Limiting**: Strict CPU limiting for heavy tasks and built-in IP-based protection.
* **Anti-Cache Stampede**: Uses `singleflight` to ensure a resource is generated only once, even under massive concurrent hits.
* **Smart ETag & 304**: Native support for `If-None-Match` and `If-Modified-Since` to save bandwidth.

## 🛠️ Installation

1. **Dependencies**
```bash
go get github.com/gin-gonic/gin
go get github.com/disintegration/imaging
go get github.com/xuri/excelize/v2
go get github.com/hashicorp/golang-lru/v2
go get github.com/edsrzf/mmap-go

```


2. **Configuration (`.env`)**
```env
SOURCE_DIR=/var/www/media
CACHE_BASE=/var/www/cache
PORT=9999

# Advanced Tuning
WORKERS=8              # Max concurrent processing
SOURCE_CACHE_CAP=512   # Number of decoded source images in RAM
MMAP_CACHE_CAP=256     # Number of memory-mapped files
RATE_PER_SEC=100

```



## 📖 Usage

### 🖼️ Image Processing

`GET /o/:action/*path?{width}x{height}`

* **WebP Conversion**: `GET /o/webp/photo.jpg`
* **Resize & Smart Crop**: `GET /o/resize/photo.png?800x600` (Automatically detects subjects and uses content-aware smart cropping when aspect ratios change)
* **Portrait Crop**: `GET /o/portrait/photo.jpg?400x400` (Uses an optimized Top-anchor crop specifically designed to prioritize and frame faces/heads perfectly)
* **Blur**: `GET /o/blur/bg.jpg?1920x1080`
* *Note: If the source is missing, a neutral placeholder is automatically generated.*

### 📊 Excel Transformation

`GET /o/:action/*path`

* **Excel to CSV**: `GET /o/csv/data/report.xlsx` (Extracts the first sheet).
* **Excel to JSON**: `GET /o/json/data/report.xlsx` (Extracts all sheets into a keyed object).

### 🔍 Metadata

`GET /metadata/*path`
Returns a JSON summary: dimensions (for images), file size, MIME type, and last modified date.

## 🔗 Infrastructure Integration (Caddy)

```caddy
cdn.example.com {
    # Route all transformation requests to the Go engine
    handle_path /o/* {
        reverse_proxy localhost:9999
    }

    # Metadata endpoint
    handle_path /metadata/* {
        reverse_proxy localhost:9999
    }

    # Serve original assets directly for everything else
    root * /var/www/media
    file_server
}

```

## 📂 Cache Architecture

The cache is structured for easy maintenance:

```text
/cache
├── webp/
│   └── 800x600/      # Resized WebP images
├── csv/
│   └── data/         # Generated CSV files from Excel
└── json/
    └── data/         # Generated JSON files from Excel

```

## 🛡️ Security & Performance

* **Strict Path Traversal Protection**: Validates final absolute paths post-`Join` to ensure isolation within `SOURCE_DIR`.
* **Image Bomb Mitigation**: Reads image headers to enforce a strict dimension limit (e.g., 8000x8000) before full RAM decoding.
* **CPU Exhaustion Protection**: Clamps user-requested dimensions to a safe maximum (e.g., 4000x4000) during resizing.
* **Cache Flooding Prevention**: Limits the number of generated cache variants per image (e.g., max 20) to prevent disk fill attacks.
* **Excel Zip Bomb Defense**: Enforces a strict XML decompression size limit (250MB) on XLSX files to prevent memory exhaustion.
* **Rate Limiting & IP Spoofing**: IP-based rate limiter with support for `TRUSTED_PROXIES` behind load balancers.
* **Security Headers**: Injects `X-Content-Type-Options: nosniff` and `X-Frame-Options: DENY`.
* **MMap Efficiency**: Cached files are served via memory mapping, reducing system calls and memory copying.
* **Aggressive Caching**: Headers include `public, max-age=31536000, immutable`.
