# 🚀 Datamix Universal Transformer (Go-Gin Edition)

🇫🇷 Français | 🇬🇧 [English](README.md)

Un moteur de transformation d'actifs ultra-performant et sécurisé. Conçu pour être greffé en tant que proxy derrière **Caddy** ou **Nginx**, il transforme n'importe quel stockage statique en un CDN moderne capable de traiter des images et de convertir des données **Excel (XLSX) en JSON/CSV** en temps réel.

## ✨ Nouvelles Fonctionnalités & Points Forts

* **Traitement Multimodal** : Gère désormais les **Images** (Resize, WebP, Blur) et les **Données** (Excel vers CSV/JSON).
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

`GET /o/:action/*path?{largeur}x{hauteur}`

* **Conversion WebP** : `GET /o/webp/photo.jpg` (Taille originale)
* **Redimensionnement** : `GET /o/resize/photo.png?800x600`
* **Flou (Blur)** : `GET /o/blur/bg.jpg?1920x1080`
* *Note : Si le fichier source est absent, un placeholder gris neutre est généré automatiquement.*

### 📊 Transformation Excel

`GET /o/:action/*path`

* **Excel vers CSV** : `GET /o/csv/data/rapport.xlsx` (Extrait la première feuille).
* **Excel vers JSON** : `GET /o/json/data/rapport.xlsx` (Extrait toutes les feuilles dans un objet structuré).

### 🔍 Métadonnées

`GET /metadata/*path`
Retourne un JSON : dimensions (pour les images), poids, type MIME et date de modification de l'asset original sans décoder l'image complète.

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

* **Protection Path Traversal** : Tous les chemins sont nettoyés (`filepath.Clean`) et validés.
* **Efficacité MMap** : Les fichiers du cache sont servis via mapping mémoire, réduisant les appels système et les copies de données.
* **Cache-Control Agressif** : Les assets sont servis avec `public, max-age=31536000, immutable`.
* **Nettoyage Auto** : Les limiteurs de débit par IP sont automatiquement supprimés après inactivité pour économiser la RAM.
