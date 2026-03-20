🇬🇧 English | 🇫🇷 [Français](README.fr.md)

# Transmuter

Turn anything into anything.

Not an API. A transformation language.

---

## Example

/transmute/photo.jpg?800x600  
/transmute/photo.jpg.webp?0x0:q80  
/transmute/document.pdf.txt  
/transmute/data.xlsx.csv  

---

## What it is

Transmuter transforms any file on the fly:

- Images → resize, crop, blur, WebP  
- PDFs → render, extract text  
- Excel → CSV / JSON  

One syntax. One engine.

---

## How it works

/transmute/*path.ext?{operations}

Chain operations with `:`.  
Same input → same output.

---

## Why

Most tools do one thing.

Transmuter does everything.

- No Cloudinary  
- No converters  
- No pipelines to glue together  

---

## Core

input → fetch → validate → transform → cache → output

Stateless. Deterministic. Fast.

---

## Features

- Zero-copy streaming  
- Worker pool  
- Anti-cache stampede  
- Smart ETag / 304  
- Rate limiting  
- Secure by default  

---

## Philosophy

Every input is hostile.  
Every transformation is explicit.  
Every output is predictable.

---

## Status

WIP. Core is solid.

---

## License

MIT
