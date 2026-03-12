# 🚀 Datamix Universal Transformer (Go-Gin Edition)

🇫🇷 Français | 🇬🇧 [English](README.md)

Un moteur de transformation d'actifs ultra-performant et sécurisé. Conçu pour être greffé en tant que proxy derrière **Caddy** ou **Nginx**, il transforme n'importe quel stockage statique en un CDN moderne capable de traiter des images et de convertir des données **Excel (XLSX) en JSON/CSV** en temps réel.

## ✨ Nouvelles Fonctionnalités & Points Forts

* **Traitement Multimodal** : Gère désormais les **Images** (Resize, WebP, Blur), les **PDFs** (Pixellisation par page, Extraction de texte) et les **Données** (Excel vers CSV/JSON).
* **Robustesse "Zero-Errors"** : Le moteur garantit un rendu client ininterrompu en substituant de manière transparente toute erreur technique (404/500) par des représentations vides et saines (image grise, ou tableau `[]` vide JSON).
* **Double Cache LRU (In-Memory)** :
* **Source Cache** : Conserve les images décodées en RAM pour éviter les lectures disque répétitives.
* **MMap Cache** : Utilise `mmap-go` pour mapper les fichiers du cache directement dans l'espace d'adressage mémoire (latence quasi nulle).


* **Pool de Workers & Rate Limiting** : Limitation stricte de l'usage CPU pour les tâches lourdes et protection par IP intégrée.
* **Anti-Cache Stampede** : Implémentation de `singleflight` pour garantir qu'un seul processus de génération est lancé pour une ressource donnée, même en cas de pic de trafic.
* **Gestion Native du Cache Navigateur** : Support complet des ETags, `If-None-Match` et `304 Not Modified`.

## 🛠️ Installation

1. **Dépendances**
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

# Paramétrage Avancé
WORKERS=8              # Max de processus de traitement simultanés
SOURCE_CACHE_CAP=512   # Nb d'images sources gardées en RAM
MMAP_CACHE_CAP=256     # Nb de fichiers mappés en mémoire
RATE_PER_SEC=100       # Limite de requêtes par IP

```



## 📖 Utilisation

### 🖼️ Traitement d'Images

`GET /cdn/*path.ext?{largeur}x{hauteur}:{filtre}:{qualité}`

* **Conversion WebP** : `GET /cdn/photo.jpg.webp?0x0:q80` (Taille originale, compression 80%)
* **Redimensionnement & Smart Crop** : `GET /cdn/photo.png?800x600` (Détecte automatiquement le sujet de l'image pour un recadrage intelligent en cas de changement de proportions)
* **Portrait (Faces)** : `GET /cdn/photo.jpg?400x400:portrait:q75` (Utilise un point d'ancrage "Top" optimisé spécifiquement pour garantir que les visages ne soient jamais coupés)
* **Flou (Blur)** : `GET /cdn/bg.jpg?1920x1080:blur:q60`
* *Note : Si le fichier source est absent, un placeholder gris neutre est généré automatiquement. L'ancienne syntaxe `/o/:action/*path` reste supportée.*

### 📄 Document Intelligence (PDF)

`GET /cdn/*path.ext`

* **Rendu de Page PDF** : `GET /cdn/document.pdf.jpg?800x600:2` (Rasterise la page 2 précise du document de manière transparente).
* **Extraction de Texte** : `GET /cdn/document.pdf.txt` (Extrait le texte brut du PDF pour l'indexation).

### 📊 Transformation Excel

`GET /cdn/*path.ext`

* **Excel vers CSV** : `GET /cdn/data/rapport.xlsx.csv` (Extrait la première feuille).
* **Excel vers JSON** : `GET /cdn/data/rapport.xlsx.json` (Extrait toutes les feuilles dans un objet structuré).

### 🔍 Métadonnées

`GET /metadata/*path`
Retourne un JSON : dimensions (pour les images), poids, type MIME, décompte de pages (pour les PDFs) et date de modification sans décoder l'asset original volumineux.

## 🔗 Configuration Caddy (Le "Greffon")

```caddy
cdn.monsite.com {
    # Redirige les requêtes de transformation vers le moteur Go
    handle_path /o/* {
        reverse_proxy localhost:9999
    }

    # Endpoint des métadonnées
    handle_path /metadata/* {
        reverse_proxy localhost:9999
    }

    # Serveur de fichiers pour les assets originaux (fallback)
    root * /var/www/media
    file_server
    
    encode zstd gzip
}

```

## 📂 Structure du Cache

Le cache est organisé par action et dimensions pour permettre des purges chirurgicales :

```text
/cache
├── webp/
│   ├── original/    # Conversion WebP uniquement
│   └── 800x600/     # Versions WebP redimensionnées
├── csv/
│   └── data/        # CSV générés à partir des XLSX
└── json/
    └── data/        # JSON générés à partir des XLSX

```

## 🛡️ Sécurité & Performance

* **Protection Path Traversal Stricte** : Validation du chemin absolu après `Join` pour garantir l'isolation stricte dans `SOURCE_DIR`.
* **Mitigation des Image Bombs** : Lecture de l'en-tête (header) pour limiter les dimensions (ex: 8000x8000) avant le décodage complet en RAM.
* **Protection CPU (Resize)** : Plafonnement des dimensions demandées par l'utilisateur à un maximum sûr (ex: 4000x4000).
* **Prévention du Cache Flooding** : Limitation du nombre de variantes générées par image (ex: 20 max) pour éviter la saturation du disque.
* **Défense Excel Zip Bomb** : Limite stricte de la taille de décompression XML (250MB) pour les fichiers XLSX.
* **Rate Limiting & IP Spoofing** : Limiteur de requêtes par IP avec support de `TRUSTED_PROXIES` pour les configurations derrière inverse-proxy.
* **Headers de Sécurité** : Injection automatique de `X-Content-Type-Options: nosniff` et `X-Frame-Options: DENY`.
* **Efficacité MMap** : Les fichiers du cache sont servis via mapping mémoire, réduisant les appels système et les copies de données.
* **Cache-Control Agressif** : Les assets sont servis avec `public, max-age=31536000, immutable`.
* **Journalisation Centralisée (Logging)** : Formatage clair sur stdout et persistance concurrente automatique dans `CDN_LOG_FILE` (basé sur le package `l3dlp/logfile`).
