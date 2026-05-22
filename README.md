# Neuro-Symbolic AI dengan Active Learning

Menciptakan sistem AI yang sepenuhnya dapat diaudit (auditable) dan ilmiah untuk jurnal internasional dengan Human in The Loop.

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

Dan seketika itu pula Anda bisa me-reset dan memulai eksperimen topik SLR Anda melalui `POST http://localhost:8080/api/sessions`.

