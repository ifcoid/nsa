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