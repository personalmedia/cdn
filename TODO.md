# 🚀 THE NEXT FRONTIER : TODO Roadmap & Extrapolations

Ce projet de Universal Transformer est devenu extrêmement performant et sécurisé pour gérer des **Images** et des **Feuilles de Calcul (Excel)**.

Où va-t-on à partir d'ici ? Voici les prochaines étapes logiques, allant d'optimisations hardcore à de nouvelles passerelles médias (Vidéo et PDF).

---

## ⚡ 1. Optimisations *Hardcore* (Niveau Machine)

Ces améliorations sont prévues pour passer d'un simple "outil Go rapide" à de la "très haute performance".

* [ ] **Zero-Copy Streaming** : Actuellement, le `mmap` cartographie le fichier en mémoire, mais le framework retransfère cette mémoire vers les sockets TCP. Avec Go 1.22+, il est possible d'utiliser `splice()` (Linux) ou `sendfile()` (MacOS/Linux) pour autoriser le kernel système à **copier directement la data du cache disque vers le socket réseau**, sans jamais transiter par la mémoire user-space du programme. C'est l'essence même d'un Nginx.
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

* [ ] **Génération Automatique de Cover** :
    * Transformer la première page d'un PDF dense en une image JPG ou WebP parfaitement optimisée (`GET /o/cover/document.pdf`). Utilisable pour les cartes de partage social (OpenGraph).
* [ ] **Extracteur Textuel & Synthèse** :
    * À la manière d'Excel, extraire le contenu texte brut du PDF (`GET /o/text/document.pdf`), pour indexer le fichier dans un moteur de recherche tel qu'Elasticsearch ou Meilisearch sans client externe.
* [ ] **Outils Requis** : On devra s'orienter vers des wrappers C pour `MuPDF` ou `poppler`, ou bien utiliser des libs natives Go partielles (ex: `pdfcpu` ou `unidoc` si les licences sont viables).

---

## ☁️ 4. Abstraction de l'Origine (Backend S3)

* [ ] Actuellement, le CDN se base sur un **SourceDir** en *file system* local `/var/www/media`. 
* [ ] **L'évolution finale** : Permettre à `SourceDir` d'accepter le schéma `s3://mon-bucket/` et charger dynamiquement, **streamer et mettre en cache** des objets depuis n'importe quel stockage compatible S3/R2/GCS, rendant notre CDN totalement `Stateless` côté Source. L'agent deviendra un proxy global *edge*.
