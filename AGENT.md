Modul	Topik(Langkah di dalamnya)	Output
1	Fondasi Teori + Aturan Global	-> (briefing)
	Langkah :
	1.1. PENGENALAN SYSTEMATIC LITERATURE REVIEW (Teori Definisi, Tujuan, dan Jenis-jenis Literature Review)
	1.2. METODOLOGI SLR(Research Question Formulasi PICO Framework)
	1.3. PENGENALAN GENERATIVE AI DALAM PENELITIAN(Etika Penggunaan AI Transparansi, Bias, Limitasi)
	1.4. KAPABILITAS LLM UNTUK PENELITIAN SLR
	1.5. ATURAN GLOBAL SLR + COWORK (BERLAKU UNTUK SEMUA MODUL 2-9)
2	Topik Penelitian (PICO)	-> pico_definitions 
	LANGKAH 1: TENTUKAN TOPIK + KLASIFIKASI TIPE GAP
    ```txt
    Topik penelitian ambil di mongo

    Gunakan web search:
    1. Cari systematic review terbaru (3 tahun terakhir) di bidang ini
    2. Identifikasi research gap belum terjawab
    3. Cek apakah topik saya sudah pernah di-review

    Suggest 3 topik penelitian dengan kriteria:
    - Gap jelas + terverifikasi dari literatur terbaru
    - Cocok untuk SLR
    - Relevan praktik saat ini

    KLASIFIKASI tiap gap ke salah satu tipe (penting untuk argumentasi Intro):
    - TIPE A — FRAGMENTASI LITERATUR: studi tersebar tanpa sintesis
    - TIPE B — KONTRADIKSI ANTAR STUDI: temuan primer bertentangan
    - TIPE C — KETIADAAN INTEGRATIVE FRAMEWORK: konsep belum terikat framework

    Update atau insert array  suggested_topics di collection SLRSession yang ada di mongo db dengan berisi informasi:
    TOPIK 1: [nama]
    - Gap: [...] | Tipe: A/B/C + alasan | Bukti: [DOI/URL] | Mengapa penting: [...]
    TOPIK 2-3: format serupa

    Konfirmasi + tunggu user pilih 1 topik untuk langkah selanjutnya.
    ```
    Cara Mengujinya Nanti:
    1. Jalankan pipeline. Tunggu hingga terminal memunculkan pesan [System] DIJEDA.
    2. Buka MongoDB Compass Anda.
    3. Buka collection slr_sessions, cari sesi Anda. Anda akan melihat kolom baru bernama suggested_topics berisi 3 pilihan topik, lengkap beserta Gap, Tipe A/B/C, Alasan, dan DOI buktinya.
    4. Cara Konfirmasi: Salin salah satu name topik pilihan Anda dari array tersebut, lalu timpakan/ganti (replace) nilai field topic yang lama dengan judul pilihan Anda.
    5. Terakhir, ubah field status menjadi "M2_STEP1_APPROVED". 
    6. Tekan "Update", dan jalankan ulang go run ./cmd/app/main.go! Sistem akan otomatis melaju ke Prior Reviews.
	LANGKAH 2: REVIEW OF PRIOR REVIEWS (MATRIKS)
	LANGKAH 3: PICO FRAMEWORK + OPERATIONAL DEFINITIONS + TERMINOLOGI KANONIKAL
	LANGKAH 4: JUSTIFIKASI BATASAN SCOPE (3-LAPIS)
	LANGKAH 5: FORMULASIKAN RESEARCH QUESTIONS
	LANGKAH 6: CEK FINER + NOVELTY + INTERNAL COHERENCE + HASIL AKHIR
3	Search Strategy	-> search_log 
	LANGKAH 1: DATABASE SELECTION + JUSTIFICATION
	LANGKAH 2: KEYWORDS DEVELOPMENT (PICO + AVOID LIST)
	LANGKAH 3: SEARCH STRING + FILTER SPECIFICATIONS
	LANGKAH 4: PRE-VALIDASI + EKSEKUSI + DATE STAMP + UPDATE POLICY + HASIL AKHIR
4	Data Mining dan export Scopus dan source lainnya(multi sources database)	-> screening 
    LANGKAH 1: EKSEKUSI FINAL SEARCH + SANITY CHECK
    LANGKAH 2: EXPORT + MULTI-DB DEDUP + PICO-CONSISTENCY PREVIEW
    LANGKAH 3: SETUP SCREENING DATABASE + EMBEDDED CRITERIA + HASIL AKHIR
5	Title & Abstract Screening	-> screening ( filled)
    LANGKAH 1: SCREENER BRIEFING (FINALISASI INTERPRETASI KRITERIA)
    LANGKAH 2: KALIBRASI DUAL-REVIEW + COHEN'S KAPPA
    LANGKAH 3: BATCH SCREENING MASSAL (AI-ASSISTED, HUMAN-DECIDED)
    LANGKAH 4: REVIEW HASIL + EXCLUSION TABLE + FULL-TEXT PREP + HASIL AKHIR
6	Full-text Acquisition	-> pdfs/ + tracking
    LANGKAH 1: ACQUISITION STRATEGY + AUTO-DOWNLOAD + PRIORITY TRACKING
    LANGKAH 2: FULL-TEXT SCREENING (DUAL-REVIEWER + AI-ASSIST)
    LANGKAH 3: RESOLVE CONFLICTS + AUDIT + EXTRACTION PREP + HASIL AKHIR
7	Data Extraction + QA	-> extraction 
    LANGKAH 1: FRAMEWORK SELECTION + EXTRACTION TEMPLATE
    LANGKAH 2: SYSTEMATIC EXTRACTION (AI-ASSISTED + 20% SPOT-VERIFICATION)
    LANGKAH 3: QUALITY APPRAISAL + THRESHOLD JUSTIFICATION + DUAL-RATER + SENSITIVITY ANALYSIS
    LANGKAH 4: SYNTHESIS PREPARATION + META-ANALYSIS FEASIBILITY + HASIL AKHIR
8	Analysis + Synthesis (A/B)	-> synthesis_results + figures
    LANGKAH 1: DESCRIPTIVE ANALYSIS + HETEROGENEITY DEEP-DIVE
    LANGKAH 2: SYNTHESIS PATH DECISION + EXECUTION (JALUR A DEFAULT atau B UPGRADE)
    LANGKAH 3: GRADE EVIDENCE GRADING + ROBUSTNESS CHECKS
    LANGKAH 4: INTERPRETATION PREPARATION + HASIL AKHIR (BRIDGE KE MODUL 9)
8b	Bibliometric (SLNA, opsional)	VOSviewer + integration
    LANGKAH 1: DATA PREPARATION + THESAURUS CONSTRUCTION
    LANGKAH 2: VOSVIEWER ANALYSIS + 9-PARAMETER JUSTIFICATION
    LANGKAH 3: CLUSTER INTERPRETATION + KRITERIA KUANTITATIF (TIER 1-4)
    LANGKAH 4: SLNA INTEGRATION (BIBLIOMETRIC + SLR) + HASIL AKHIR
9	Manuscript Writing	manuscript_final
    LANGKAH 1: METHODS WRITING (PRISMA 2020 COMPLIANT)
    LANGKAH 2: RESULTS WRITING (STRUKTUR FRAMEWORK TCCM/ADO)
    LANGKAH 3: DISCUSSION WRITING (6 SUBSEKSI WAJIB)
    LANGKAH 4: FUTURE RESEARCH AGENDA (SUBSEKSI KHUSUS)
    LANGKAH 5: INTRODUCTION WRITING (5 SUBSEKSI WAJIB)
    LANGKAH 6: CONCLUSIONS WRITING (LEAN)
    LANGKAH 7: REFERENCES (FORMAT + VERIFY + TEMPORAL AUDIT + JOURNAL TIER)
    LANGKAH 8: ABSTRACT WRITING (250-300 KATA)
    LANGKAH 9: TITLE CREATION (3-5 ALTERNATIF)
    LANGKAH 10: AUDIT + COMPILE FINAL + HASIL AKHIR










