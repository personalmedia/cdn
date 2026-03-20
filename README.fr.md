🇫🇷 Français | 🇬🇧 [English](README.md)

# Transmuter

Transforme n’importe quoi en n’importe quoi.

Pas une API. Un langage de transformation.

---

## Exemple

/transmute/photo.jpg?800x600  
/transmute/photo.jpg.webp?0x0:q80  
/transmute/document.pdf.txt  
/transmute/data.xlsx.csv  

---

## Ce que c’est

Transmuter transforme n’importe quel fichier à la volée :

- Images → resize, crop, blur, WebP  
- PDF → rendu, extraction de texte  
- Excel → CSV / JSON  

Une seule syntaxe. Un seul moteur.

---

## Comment ça marche

/transmute/*path.ext?{opérations}

Chaîne les opérations avec `:`.  
Même entrée → même sortie.

---

## Pourquoi

La plupart des outils font une seule chose.

Transmuter fait tout.

- Pas de Cloudinary  
- Pas de convertisseurs  
- Pas de pipelines à assembler  

---

## Core

input → fetch → validate → transform → cache → output

Stateless. Déterministe. Rapide.

---

## Features

- Streaming zero-copy  
- Worker pool  
- Anti cache stampede  
- ETag / 304 natif  
- Rate limiting  
- Sécurité par défaut  

---

## Philosophie

Chaque entrée est hostile.  
Chaque transformation est explicite.  
Chaque sortie est prévisible.

---

## Status

WIP. Le core est solide.

---

## Licence

MIT

