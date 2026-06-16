# Neuro-Symbolic AI dengan Active Learning

Sistem AI untuk riset jurnal internasional yang sepenuhnya transparan dan dapat diaudit. Sistem ini memadukan pendekatan:

1. Active Learning / Human-in-the-Loop.
2. XAI (Explainable AI).
3. Neuro-Symbolic AI.

```txt
slr-agentic/
├── cmd/
│   └── app/
│       └── main.go             # Entry point aplikasi
├── internal/
│   ├── model/
│   │   ├── slr.go              # Struct Session, Paper, Graph (PICO/PRISMA)
│   ├── repository/
│   │   ├── mongo.go            # CRUD data mentah & State HitL
│   │   └── neo4j.go            # Ingest Knowledge Graph (Symbolic AI)
│   ├── llm/
│   │   └── gemini.go           # Adapter untuk API LLM (Gemini/Claude)
│   ├── delivery/
│   │   └── http/               # Endpoint API (router, session_handler, llm_handler)
│   ├── agent/
│   │   ├── agent.go            # Base struct / interface Agent
│   │   ├── pico_agent.go       # Spesialis analisis PICO
│   │   ├── criteria_agent.go   # Spesialis perumus Inklusi/Eksklusi
│   │   └── screener_agent.go   # Spesialis penyaring abstrak
│   └── orchestrator/
│       └── pipeline.go         # Otak pengatur alur kerja & jeda HitL
├── go.mod
└── go.sum
```

## Fondasi Teori SLR + Aturan Global

Menggunakan rule Standar PRISMA dan Cochrane: 
1. Dalam standar Cochrane atau rumpun ilmu kesehatan/IT, peneliti tidak boleh mencari jurnal tanpa kompas [1]. PICO adalah kompas tersebut.
2. PRISMA (Preferred Reporting Items for Systematic reviews and Meta-Analyses) adalah panduan pelaporan yang dirancang untuk meningkatkan pelaporan systematic review

## Quick Start

Fitur :

1. **Pembuatan Layer HTTP Delivery (`net/http`)**:
   - `router.go`: Menampung rute-rute *endpoint* (memanfaatkan mux *routing* gaya baru dari Go 1.22+).
   - `session_handler.go`: Menangani siklus *state machine* melalui Endpoint API (Create, Get, Approve, Revise).
   - `llm_handler.go`: Endpoint khusus untuk *update* konfigurasi Provider dan API key.

2. **Dokumentasi Terstandardisasi**:
   Saya telah membuatkan dokumen **[openapi.json](./api/openapi.json)** (OpenAPI 3.0) yang berisi semua spesifikasi *endpoint* dan *payload* yang Anda minta. Fail ini siap diimpor ke Postman, Insomnia, atau Swagger.

3. **Orkestrasi Asinkron**:
   Titik eksekusi pipeline (`pipeline.go`) kini memiliki fitur `ExecuteAsync()`. API akan segera memberikan respon `200 OK`, dan melempar proses penalaran (*reasoning*) AI ke belakang layar tanpa khawatir aplikasi akan macet atau terkena batasan _timeout_.

4. **Validasi Kompilasi**:
   Sistem telah dikompilasi ulang dan lolos pengujian tanpa _error_ sama sekali.

**Langkah Anda Selanjutnya:**
Anda sudah bisa menjalankan servernya melalui terminal dengan perintah:

```bash
go run cmd/app/main.go
```

Dan seketika itu pula Anda bisa me-reset dan memulai eksperimen topik SLR Anda melalui `POST http://localhost:50607/api/sessions`.

## Konfigurasi (Environment Variables)

Aplikasi ini sangat portabel dan mengikuti prinsip *12-Factor App*. Penggunaan file `.env` bersifat **opsional**. Anda dapat dengan mudah menimpa nilai bawaan ini dengan membuat file `.env` di folder utama aplikasi, atau dengan mendefinisikannya langsung di variabel sistem operasi (*OS Environment Variables*) saat memindahkan (*deploy*) aplikasi ke server produksi atau layanan Cloud (seperti Fly.io).

Berikut adalah daftar lengkap variabel *environment* yang dikenali sistem beserta peranannya:

### 1. Database (MongoDB) - *Wajib*
- `MONGO_URI`: Alamat server MongoDB (Contoh: `mongodb+srv://...` atau bawaannya `mongodb://localhost:27017`).
- `DB_NAME`: Nama database yang digunakan (Bawaan: `slr_agentic_db`).

### 2. Knowledge Graph (Neo4j/AuraDB) - *Neuro-Symbolic AI*
- `NEO4JURI`: URI koneksi server Neo4j/AuraDB (Contoh: `neo4j+s://[ID].databases.neo4j.io`).
- `NEO4JUSER`: *Username* untuk otentikasi Neo4j.
- `NEO4JPASSWORD`: *Password* untuk otentikasi Neo4j.

### 3. Vector Database (Qdrant) - *RAG*
- `QDRANT_ENDPOINT`: URL endpoint server Qdrant Anda (Contoh: `https://...cloud.qdrant.io`).
- `QDRANT_API_KEY`: Kunci API otentikasi Qdrant.

### 4. Embedding Server
- `EMBED_ENDPOINT`: URL endpoint server model *embedding* teks (Contoh: `https://...trycloudflare.com/v1`, BAAI/bge-m3 via *cloudflared*). Dipakai screening Modul 6 (embedding query dense).
- `EMBED_API_KEY`: Kunci API server *embedding* (sekaligus dipakai untuk endpoint `/search`).
- `EMBED_MODEL`: Nama spesifik model *embedding* (Contoh: `BAAI/bge-m3`).
- `SEARCH_ENDPOINT` *(opsional)*: URL endpoint **`/search` hybrid** server PEDE (dense+sparse, RRF) untuk verifikasi klaim/sitasi Modul 9. Jika dikosongkan, **diturunkan otomatis** dari `EMBED_ENDPOINT` (`.../v1` → `.../search`), jadi cukup kelola satu URL tunnel. Bila endpoint bukan server PEDE (tak punya `/search`), `SemanticSearch` otomatis fallback ke pencarian dense.

### 5. Notifikasi Telegram
- `TELEGRAM_BOT_TOKEN`: Token otentikasi dari BotFather.
- `CHAT_ID`: ID percakapan pribadi/grup untuk menerima laporan (*alert*) status dari backend.

### 6. Lain-lain
- `PORT`: *Port* server API berjalan (Bawaan: `50607`).
- `SESSION_ID`: (Opsional) Injeksi ID Sesi riset di mode *development/seed*.

## Release

Proses pembuatan rilis (*release*) dan kompilasi *cross-platform* sekarang sudah diotomatisasi. Anda bisa menggunakan *script* PowerShell berikut:

```powershell
.\build.ps1 -Version "1.0.0"
```

Perintah di atas akan:
1. Membuat Git tag `v1.0.0` dan melakukan `git push` secara otomatis.
2. Memicu GitHub Actions untuk mengompilasi *binary* untuk Windows, Linux, dan macOS lalu menambahkannya ke tab **Releases** di GitHub.
3. Membuat *build* lokal untuk Windows (`if-slr.exe`) dan mengompresnya menjadi `aplikasi-slr.zip`.