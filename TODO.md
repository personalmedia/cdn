# 🚀 THE NEXT FRONTIER : TODO Roadmap & Extrapolations

Ce projet de Universal Transformer est devenu extrêmement performant et sécurisé pour gérer des **Images** et des **Feuilles de Calcul (Excel)**.

Où va-t-on à partir d'ici ? Voici les prochaines étapes logiques, allant d'optimisations hardcore à de nouvelles passerelles médias (Vidéo et PDF).

---

## ✅ Jalons Complétés (Réalisés)

* [x] **Refonte Architecturelle** : Transition complète d'un script monolithique vers le standard *Go Canonical Project Layout* (`cmd/cdn`, `internal/config`, `internal/cache`, `internal/processor`, `internal/server`).
* [x] **Sécurité & Résilience** : Correction des failles path traversal, protection contre les attaques DoS (limite de décodage d'images et Zip Bomb Excel), ajout d'entêtes sécuritaires et fallback transparent sur des `Placeholders` en cas de corruption de fichier source.
* [x] **Cadrage Intelligent (Smart Crop) & Computer Vision** : Intégration d'un algorithme de recadrage intelligent, et surtout, création du endpoint `/o/portrait/` utilisant la reconnaissance faciale (via réseaux de neurones vectorisés nativement en Go sans CGO avec Pigo) pour toujours centrer avec précision sur le visage.
* [x] **Support Vectoriel Natif (SVG)** : Rasterisation mathématique dynamique au pixel près des fichiers `.svg` avec du pur Go, ce qui élimine pixellisations et flous tout en permettant un chaînage naturel avec les webp/blur/resize.
* [x] **Routage PDF Avancé** : Pixellisation à la demande des PDF via l'utilitaire `pdftoppm` avec sélection précise de la page (`?400x400:2`), extraction de texte structuré et intégration du décompte de pages dans l'endpoint Metadata.
* [x] **URL Schema & Chaînage Dynamique** : Simplification du système de rendu avec `GET /cdn/*path.ext?WxH:filter:qX`, permettant de masquer l'action au profit d'extensions concrètes, et de chaîner les paramètres comme la qualité (`q80`) ou les filtres (`blur`).
* [x] **Robustesse "Zero-Errors" & Centralized Logging** : L'API ne renvoie plus d'erreurs sèches (404/500) pour préserver le client frontend, priorisant le renvoi de "formats vides" (images grises, `[]` JSON, etc.) avec un statut HTTP 200 OK. Le moteur intègre également un système de journalisation complet basé sur `logfile` pour une tracabilité maximale.

---

## ⚡ 1. Optimisations *Hardcore* (Niveau Machine)

Ces améliorations sont prévues pour passer d'un simple "outil Go rapide" à de la "très haute performance".

* [x] **Zero-Copy Streaming** : Actuellement, le `mmap` cartographie le fichier en mémoire, mais le framework retransfère cette mémoire vers les sockets TCP. Avec Go 1.22+, il est possible d'utiliser `splice()` (Linux) ou `sendfile()` (MacOS/Linux) pour autoriser le kernel système à **copier directement la data du cache disque vers le socket réseau**, sans jamais transiter par la mémoire user-space du programme. C'est l'essence même d'un Nginx.
* [ ] **SIMD Hardware Resizing** : Remplacer `disintegration/imaging` par des bindings sur des librairies C/C++ utilisant **AVX/SIMD** (ex: `bimg` avec libvips). Ceci réduirait l'usage CPU par 10 lors du redimensionnement d'images lourdes.
* [ ] **Cache Collapsing & Clustering (Global Singleflight)** : L'implémentation de `singleflight` actuelle évite la répétition du calcul (cache stampede) localement sur un noeud serveur. Quid si on est derrière un Load Balancer sur 5 serveurs ? Le prochain défi est de distribuer cette ressource "en cours de création" via un système comme Redis Pub/Sub Lock.

---

## 🎥 2. Passerelle Vidéo (Video Transformer Gateway)

La demande en médias animés est colossale. L'idée est de rajouter une route `GET /o/video/...` ou d'augmenter les capacités existantes.

* [ ] **Génération de Thumbnails & Previews** :
    * Extraire une frame à la seconde `X` d'une vidéo MP4 via des bindings FFMPEG ou en l'exécutant (`os/exec`). `GET /o/thumb/video.mp4?time=00:01:05`.
* [ ] **Génération de GIF ou WebP animés** :
    * Prendre un extrait vidéo (les 3 premières secondes) et le convertir à la volée en boucle WebP.
* [ ] **Live Transcoding (HLS/DASH)** :
    * La "grosse feature". Avoir un CDN capable de packager un `.mp4` lourd stocké sur disque en flux HLS (fragments `.ts` et fichiers `.m3u8`) à la volée, avec plusieurs qualités (Adaptive Bitrate) et les mettre en cache `mmap`.

> **Sécurité vidéo (Attention !)** : Exécuter ou invoquer FFmpeg ouvre la porte aux attaques par DoS (CPU meltdown) ou vulnérabilités de parsing. Il faudra encapsuler FFmpeg dans des requêtes isolées, un timeout strict (`context.WithTimeout`) et limiter la RAM allouée au conteneur ou processus.

---

## 📄 3. Passerelle PDF (Document Intelligence Gateway)

Les PDF représentent un monde à part. Le but, ici, est l'extraction sémantique et visuelle pour des tableaux de bords / blogs.

* [x] **Génération Automatique de Cover & Pages** :
    * Transformer n'importe quelle page d'un PDF dense en une image JPG ou WebP parfaitement optimisée (`GET /o/resize/document.pdf?800x800:2`). Utilisable pour les cartes de partage social (OpenGraph).
* [x] **Extracteur Textuel & Synthèse** :
    * À la manière d'Excel, extraire le contenu texte brut du PDF (`GET /o/text/document.pdf`), pour indexer le fichier dans un moteur de recherche tel qu'Elasticsearch ou Meilisearch sans client externe.
* [x] **Outils Requis** : On devra s'orienter vers des wrappers C pour `MuPDF` ou `poppler` (via lib/exec `pdftoppm`), ou bien utiliser des libs natives Go partielles (ex: `ledongthuc/pdf` pour metadata).

---

## ☁️ 4. S3 Universel (Abstraction de l'Origine)

**Objectif** : Permettre à Datamix de lire les assets depuis le système de fichiers local ou n'importe quel stockage compatible S3 (AWS, Cloudflare R2, Backblaze B2, Wasabi, DigitalOcean Spaces, etc.) afin d'éviter le vendor lock-in et faciliter les architectures multi-cloud.

* [ ] **Interface `SourceProvider`** : Créer l'interface standardisée (`Get`, `Exists`, `Stat`).
* [ ] **Driver S3 Standard** : Intégrer `minio-go/v7` pour sa forte compatibilité et ses dépendances réduites.
* [ ] **S3 Compatibility Layer** : Détection automatique et ajustement des spécificités du provider (ex: désactiver la région pour Cloudflare R2, forcer le path-style pour Scaleway).
* [ ] **Zero-Buffer Streaming** : S'assurer que les objets S3 soient directement streamés de l'origine au pipeline (`S3 → io.Reader → transformer → cache`).

---

## 🧠 5. Mode CDN Intelligent (Auto-Optimization)

**Objectif** : Permettre des requêtes sans paramètres explicites (`GET /cdn/photo.jpg`), dans lesquelles Datamix détermine automatiquement les meilleures transformations.

* [ ] **Client Signals** : Détection de l'appareil et des capacités via les headers `Accept` (ex: `image/avif`, `image/webp`), `User-Agent`, `Viewport-Width`, `DPR`, `Save-Data`.
* [ ] **Résolution Dynamique** : Algorithme calculant la taille idéale (ex: `width = viewport * DPR`, contraint entre 320 et 2400px).
* [ ] **Négociation de Format** : Privilégier l'ordre de nouvelle génération (AVIF > WebP > JPEG > PNG).
* [ ] **Quality Profiles Automatiques** : Paramétrage intelligent (ex: `Desktop -> q75`, `Mobile -> q65`, `Save-Data -> q40`).
* [ ] **Variations de Cache** : La clé de cache doit intégrer le hash de l'ensemble de ces facteurs (`sha1(format+width+quality+filters)`).

---

## 🧬 6. CDN Programmable (WASM)

**Objectif** : Permettre aux développeurs d'exécuter des transformations personnalisées compilées en WebAssembly (WASM), à la volée (`GET /cdn/photo.jpg?wasm=watermark`).

* [ ] **WASM Runtime** : Intégrer un moteur d'exécution ultra-rapide comme `wasmtime` ou `wazero`.
* [ ] **Standard WASM API** : Implémenter le passage du buffer d'image, des métadonnées et query parameters, et récupérer le buffer modifié (`Transform(input []byte) []byte`).
* [ ] **Sécurité / Sandboxing** : Limite stricte de la mémoire allouée, timeout d'exécution, interdiction formelle d'accès au module réseau ou au système de fichiers local.
* [ ] **Cas d'usage cibles** : Watermarks dynamiques, floutage automatique (visages, EXIFs), upscaling IA, anonymisation (PDF redaction), génération de thumbnails personnalisés.
* [ ] **Invalidation de cache** : Intégrer le hash du module WASM dans la clé de variante du cache.

---

## 🌐 7. Extensions Futures (IPFS & Edge)

* [ ] **Source IPFS** : Utiliser le protocole distribué IPFS comme `SourceProvider` additionnel.
* [ ] **Video Transcoding** : Packages dynamiques HLS/DASH (comme vu à l'étape 2).
* [ ] **Animation AVIF** : Prise en charge optimisée.
* [ ] **Edge Cache Replication** : Synchronisation du cache entre noeuds distribués.

> 💡 **La cible architecturale finale** : Avec **1️⃣ Le S3 Universel**, **2️⃣ Le CDN Intelligent**, **3️⃣ Le WASM Programmable** et **4️⃣ L'option IPFS**, le moteur passe du statut de simple "proxy image" à un véritable **moteur CDN programmable open-source** global.
