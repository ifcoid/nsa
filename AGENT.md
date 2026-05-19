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
    Output: suggested_topics dan SelectedTopic
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
    4. Jika Anda SUKA salah satu topik:
       a. "Copy (salin) keseluruhan object/document dari 1 topik pilihan Anda."
       b. "Buat field baru bernama selected_topic (di root document), lalu Paste isinya di sana."
       c. Terakhir, ubah field status menjadi "M2_STEP1_APPROVED". 
    5. Jika Anda TIDAK SUKA (Butuh Revisi):
       a. Ubah field status menjadi "M2_STEP1_NEEDS_REVISION".
       b. Isi field feedback dengan alasan Anda (misal: "carikan topik yang lebih condong ke algoritma X").
    6. Tekan "Update", dan jalankan ulang go run ./cmd/app/main.go! Jika approved akan lanjut ke Langkah 2. Jika revisi, akan men-generate ulang 3 saran topik baru.
	LANGKAH 2: REVIEW OF PRIOR REVIEWS (MATRIKS)
    output: prior_reviews_matrix 
    ```txt
    Dengan menyertakan 1 dokumen yang dipilih berupa SelectedTopic dari langkah 1(Name,Gap,Type,TypeREason,Evidence,Importance) diikutkan sebagai RAG(atau menurutmu enaknya sebagai apa)

    Perintahkan LLM untuk menggunakan web search untuk identifikasi 3-5 systematic review/literature review
    TERDEKAT (5-10 tahun terakhir).

    Bangun Tabel Matriks (sesuaikan kolom):
    | # | Author (Year) | Scope (Pop, Area, Period) | Methodology (SLR/Bibliometric/SLNA, DB, n) | Key Findings | Limitations | Selisih | SINTESIS NOVELTY |

    Kolom "Selisih" eksplisit tunjukkan: BEDA POPULASI / BEDA METODE / BEDA PERIODE /
    BEDA FOKUS / BEDA FRAMEWORK.

    Isi kolom "SINTESIS NOVELTY" (150-200 kata): apa SUDAH dilakukan prior
    reviews kolektif, apa BELUM, mengapa riset saya MENUTUP gap.

    Jika prior reviews sangat sedikit (1-2): sampaikan apa adanya.
    Jika tidak ada: catat "No prior systematic review identified" + tunjukkan
    review naratif terdekat sebagai banding.

    Update ke database sebagai session.prior_reviews_matrix(atau nama yang layak menurutmu)

    Konfirmasi + tunggu user memvalidasi matriks untuk lanjut ke langkah selanjutnya.
    ```
    Cara Mengujinya Nanti:
    1. Sistem akan mencetak pesan [System] DIJEDA setelah tabel matriks dibuat.
    2. Buka MongoDB Compass, temukan field prior_reviews_matrix pada sesi Anda.
    3. Periksa array reviews dan field synthesis_novelty apakah sudah sesuai.
    4. Jika SUDAH sesuai, ubah field status menjadi "M2_STEP2_APPROVED" lalu Update.
    5. Jika TIDAK sesuai (butuh revisi), ubah status menjadi "M2_STEP2_NEEDS_REVISION", lalu isi instruksi perbaikan Anda di dalam field feedback.
    6. Jalankan ulang aplikasi. Jika approved, sistem akan maju ke Langkah 3 (PICO). Jika revisi, sistem akan memperbaiki matriksnya dan kembali menjeda (WAITING_APPROVAL).
	LANGKAH 3: PICO FRAMEWORK + OPERATIONAL DEFINITIONS + TERMINOLOGI KANONIKAL
    output: pico_definitions dan scope_filters
    ```txt
    Sertakan RAG dari SelectedTopic(dari Langkah 1) dan prior_reviews_matrix(dari Langkah 2)

    Bangun PICO 3-lapis. Tulis/update document field pico_definitions di collection SLRSession. Yang berisi data 3 lapisan:

    === LAPISAN 1: PICO ===
    P (Population): siapa yang diteliti?
    I (Intervention/Exposure): apa yang diteliti?
    C (Comparison): pembanding (atau "no comparison" jika SLR deskriptif)
    O (Outcome): hasil yang diukur

    === LAPISAN 2: OPERATIONAL DEFINITIONS (per komponen) ===
    Untuk tiap P/I/C/O:
    - WHAT COUNTS: kriteria eksplisit yang MEMBUAT studi memenuhi
    - WHAT DOESN'T COUNT: kriteria eksplisit yang MENGGUGURKAN
    - EDGE CASES: borderline + keputusan default (include/exclude + alasan)

    === LAPISAN 3: TERMINOLOGI KANONIKAL ===
    Untuk komponen P (atau I jika kompleks):
    - Kanonikal: "[term]"
    - Definisi baku 1 kalimat
    - Alternatif yang DITOLAK + alasan ditolak

    Output WAJIB siap dipakai sebagai inclusion criteria di Modul 5.
    ```
    Cara Mengujinya Nanti:
    1. Sistem akan mencetak pesan [System] DIJEDA setelah PICO 3-Lapis dibuat.
    2. Buka MongoDB Compass, temukan field pico_definitions pada sesi Anda.
    3. Periksa P, I, C, O, operational_def, dan canonical_term.
    4. Jika SUDAH sesuai, ubah field status menjadi "M2_STEP3_APPROVED" lalu Update.
    5. Jika TIDAK sesuai (butuh revisi), ubah status menjadi "M2_STEP3_NEEDS_REVISION", lalu isi instruksi perbaikan Anda di field feedback.
    6. Jalankan ulang aplikasi. Jika approved, sistem akan membuat template scope_filters dan memindah status ke "M2_STEP3_5_WAITING_FILTERS".
    7. Anda WAJIB mengisi template "scope_filters" (Rentang Tahun, Geografis, Bahasa, Sektor) dengan menghapus teks [ISI DI SINI]. Lalu ubah status menjadi "M2_STEP3_5_FILTERS_PROVIDED". Sistem akan memvalidasinya secara otomatis sebelum berlanjut ke Langkah 4.
	LANGKAH 4: JUSTIFIKASI BATASAN SCOPE (3-LAPIS)
    output: scope_justifications
    ```txt
    Sertakan RAG dari pico_definitions dan scope_filters


    Untuk SETIAP scope_filters, bangun justifikasi 3-lapis:

    1. TEORETIS — landasan konseptual
    Contoh: rentang usia 18-35 → konsep "emerging adulthood" (Arnett 2000)

    2. METODOLOGIS — mengapa memperbaiki kualitas review
    Contoh: 2020-2024 → era pasca-COVID yang struktural mengubah lanskap

    3. PRAKTIS — relevansi kebijakan/praktik
    Contoh: SDG 8 target 8.6 → relevansi ILO + Bappenas

    Gunakan web search untuk verifikasi klaim teoretis (tahun publikasi, target SDG,
    definisi resmi).

    Tulis ke dokumen scope_justifications dalam database dengan format:
    BATASAN 1: [nama]
    - Teoretis: [150-200 kata + referensi]
    - Metodologis: [100-150 kata]
    - Praktis: [100-150 kata]
    BATASAN 2-N: format serupa

    Jika batasan tidak bisa lolos 3-lapis → tandai untuk diubah/dihapus.
    ```

    Cara Mengujinya Nanti:
    1. Sistem akan mencetak pesan [System] DIJEDA setelah justifikasi scope selesai.
    2. Buka dokumen scope_justifications dalam database.
    3. Periksa apakah setiap batasan (tahun, geografi, dll.) memiliki justifikasi 3 lapis (Teoretis, Metodologis, Praktis) dan referensi yang relevan.
    4. Jika SUDAH sesuai dan kredibel, ubah field status menjadi "M2_STEP4_APPROVED" lalu Update.
    5. Jika TIDAK sesuai (justifikasinya lemah atau tidak ada referensi), ubah status menjadi "M2_STEP4_NEEDS_REVISION", lalu isi instruksi perbaikan Anda di field feedback (misalnya: "Perkuat justifikasi teoretis dengan teori X").
    6. Jalankan ulang aplikasi. Jika approved, sistem akan melanjutkan ke Langkah 5 (Research Questions).
	LANGKAH 5: FORMULASIKAN RESEARCH QUESTIONS
    output: research_questions
    ```txt
    Sertakan RAG dari:
    - SelectedTopic
    - prior_reviews_matrix
    - pico_definitions
    - scope_justifications

    Formulasikan:
    1. PRIMARY RQ — jelas, fokus, dapat dijawab SLR
    2. 3 SECONDARY RQs — mendukung primary

    3. GAP-RQ TRACEABILITY (untuk primary + setiap secondary):
    (a) Menjawab gap apa (trace ke tipe gap L1)
    (b) Berbeda dari prior reviews di aspek apa (trace ke selisih L2)
    (c) Dapat dijawab dengan PICO (trace ke L3)
    (d) Sesuai dengan batasan (trace ke scope_justifications L4)

    Tulis ke dokumen research_questions di database dengan format:

    PRIMARY RQ: [question]
    → Menjawab gap: [...]
    → Selisih prior reviews: [...]
    → Answerable via PICO: [...]
    → Sesuai dengan Scope: [...]

    SECONDARY RQ 1-3: format serupa.

    PERINGATAN: jika ada RQ tidak bisa di-trace ke 3 elemen → tandai "RQ-orphan",
    peserta harus revisi sebelum Langkah 6.
    ```
    Cara Mengujinya Nanti:
    1. Sistem akan mencetak pesan [System] DIJEDA setelah RQ dibuat (atau ada peringatan jika ada RQ-orphan).
    2. Buka MongoDB Compass, cari array research_questions pada sesi Anda.
    3. Periksa 1 Primary RQ dan 3 Secondary RQs beserta traceability-nya (gap, prior_reviews, pico). Pastikan is_orphan false.
    4. Jika SUDAH sesuai, ubah field status menjadi "M2_STEP5_APPROVED" lalu Update.
    5. Jika TIDAK sesuai (ada orphan atau RQs kurang tajam), ubah status menjadi "M2_STEP5_NEEDS_REVISION", isi instruksi revisi di field feedback.
    6. Jalankan ulang aplikasi. Jika approved, sistem akan berlanjut ke Langkah 6 (Validasi FINER).
	LANGKAH 6: CEK FINER + NOVELTY + INTERNAL COHERENCE + HASIL AKHIR
    output: finer_novelty_check dan modul2_summary
    ```txt
    Gunakan RAG dari :
    - research_questions
    - prior_reviews_matrix
    - pico_definitions
    - scope_justifications

    Eksekusi 2 output dokumen di database:

    === OUTPUT 1: finer_novelty_check ===

    FINER evaluation:
    - F (FEASIBLE): web search estimasi jumlah studi di Scopus dengan PICO+batasan
    Benchmark: >50 ideal SLR; 20-50 viable; <20 thin evidence
    - I (INTERESTING): audience primer (peneliti/praktisi/policymaker)?
    - N (NOVEL): tabel "Novelty per Prior Review" — kolom: Prior Review | Overlap |
    BARU di riset saya | Signifikansi novelty. Cross-check via web search untuk
    SLR yang publish dalam 6 bulan terakhir.
    - E (ETHICAL): isu hak cipta, populasi sensitif?
    - R (RELEVANT): selaras agenda SDGs, kebijakan nasional, prioritas sektoral?

    CROSS-CHECK INTERNAL COHERENCE:
    1. PICO cukup untuk jawab primary RQ?
    2. Scope tidak terlalu sempit untuk feasibility?
    3. Terminologi kanonikal konsisten di RQ?

    Tunjukkan PASS/FAIL per kriteria + rekomendasi revisi langkah mana
    (L1/L2/L3/L4/L5).

    === OUTPUT 2: modul2_summary (HASIL AKHIR) ===

    Format:
    === RESEARCH QUESTION PACKAGE (SLR) ===

    RESEARCH AREA: [...]
    SELECTED TOPIC: [...]

    1. GAP CHARACTERIZATION (→ Modul 9 L4 Intro)
    Tipe: A/B/C | Deskripsi: [...] | Evidence: [...]

    2. PRIOR REVIEWS MATRIX (→ Modul 9 L4 "Review of Prior Reviews")
    [tabel ringkas + novelty synthesis]

    3. PICO + OPERATIONAL DEFINITIONS (→ Modul 5 inclusion criteria)
    P/I/C/O: [ringkas WHAT COUNTS/DOESN'T per komponen]

    4. CANONICAL TERMINOLOGY (→ konsisten di seluruh modul)
    [term + definisi + alternatif ditolak]

    5. SCOPE JUSTIFICATION (→ Modul 9 L4 Intro)
    [batasan + justifikasi 3-lapis]

    6. RESEARCH QUESTIONS (→ Modul 9 L4 objectives)
    Primary + 3 Secondary dengan traceability

    7. FINER + NOVELTY CHECK
    F/I/N/E/R: PASS dengan ringkasan
    Internal coherence: ✓

    Konfirmasi 2 dokumen tersimpan di database
    kemudian user approve atau revisi 
    setelah itu baru NEXT: Search strategy (Modul 3)
    ```
    Cara Mengujinya Nanti:
    1. Sistem akan mencetak pesan [System] DIJEDA setelah evaluasi FINER.
    2. Buka MongoDB Compass, cari 'finer_novelty_check' dan 'modul2_summary'.
    3. Periksa evaluasi FINER, tabel novelty, internal coherence, dan ringkasan akhir.
    4. Jika SUDAH lulus, ubah status menjadi "M2_STEP6_APPROVED". Sistem akan berpindah ke Modul 3.
    5. Jika BUTUH REVISI, ubah status menjadi "M2_STEP6_NEEDS_REVISION" dan isi keluhan di feedback.

3.	Search Strategy	-> search_log 
	LANGKAH 1: DATABASE SELECTION + JUSTIFICATION
    output : database_selection
    ```txt
    Sertakan RAG dari dokumen:
    - pico_definitions
    - scope_justifications
    
    Kemudian lakukan analisis lanskap database via web search.

    Output ke dokumen database_selection di database yang berisi :

    1. CEK COVERAGE BIDANG:
    - Apakah Scopus mencakup mayoritas jurnal inti?
    - Ada literatur penting (regional, non-English, niche) yang TIDAK ter-index?
    - Bidang dominan publikasi konferensi (IEEE/ACM) atau laporan kebijakan?

    2. MATRIKS DATABASE:
    | Database | Coverage strength | Limitation | Fit dengan topik |
    | Scopus | Global peer-reviewed | Bias Anglo-Saxon | [...] |
    | Web of Science | Citation-rich | Lebih selektif | [...] |
    | PubMed | Biomedical | Terbatas domain | [...] |
    | IEEE Xplore | Engineering/CS | Terbatas | [...] |
    | ScienceDirect | Elsevier | Terbatas | [...] |
    | Springer | Springer | Terbatas | [...] |

    3. DECISION:
    - >80% paper kunci di Scopus → SCOPUS-ONLY (justifikasi feasibility)
    - Ada literatur regional kritis → MULTI-DATABASE
    - Topik baru / grey lit penting → tambah Google Scholar / repository

    4. JUSTIFIKASI FINAL (200 kata, siap-Methods):
    "We conducted primary search using [DB] because [coverage]. We acknowledge
    [what's not covered] which may introduce [bias]. Mitigation: [snowballing,
    citation tracking]. Date of search: [YYYY-MM-DD]."
    ```
    Cara Mengujinya Nanti:
    1. Sistem akan mencetak pesan [System] DIJEDA setelah melakukan Database Selection.
    2. Buka MongoDB Compass, cari dokumen `database_selection`.
    3. Periksa isi `cek_coverage_bidang`, tabel `matriks_database`, `decision`, dan paragraf tulisan `justifikasi_final`.
    4. Jika SUDAH tepat dan metodologinya kuat, ubah status menjadi "M3_STEP1_APPROVED".
    5. Jika BUTUH REVISI, ubah status ke "M3_STEP1_NEEDS_REVISION" dan ketikkan instruksi di feedback (contoh: "Tolong jadikan multi-database dengan PubMed karena ini topik medis").
	LANGKAH 2: KEYWORDS DEVELOPMENT (PICO + AVOID LIST)
    output : keywords
    ```txt
    Gunakan RAG dari dokumen:
    - picopico_definitions (canonical term + WHAT COUNTS/DOESN'T):

    Develop keywords PER komponen PICO. Tulis ke dokumen keywords di database. dengan ketentuan:

    PRINSIP KUNCI:
    - Sinonim WAJIB konsisten dengan WHAT COUNTS — tidak memperluas scope diam-diam
    - JANGAN masukkan sinonim yang menangkap WHAT DOESN'T COUNT
    - Istilah kanonikal WAJIB di daftar utama
    - Alternatif yang DITOLAK di Modul 2 L3 → masuk AVOID LIST (trip wire)

    Format per komponen P/I/C/O:
    === P / I / C / O — POPULATION/INTERVENTION/COMPARISON/OUTCOME ===
    - Canonical term: [...]
    - Main synonyms (lolos "what counts" test): [3-5 terms]
    - Alternative terms (kadang dipakai peneliti lain, lolos test): [2-4 terms]
    - AVOID list (trip wire vs "what doesn't count"): [eksplisit catat]

    (Jika C = N/A: tulis "N/A" + alasan singkat)

    Verifikasi via web search untuk istilah ambigu — apakah perlu sinonim
    internasional setara? (lolos operational def "what counts" tetap)
    ```
    Cara Mengujinya Nanti:
    1. Sistem akan mencetak pesan [System] DIJEDA setelah mengembangkan Keywords.
    2. Buka MongoDB Compass, cari dokumen `keywords`.
    3. Periksa isi `canonical_term`, `main_synonyms`, `alternative_terms`, dan `avoid_list` untuk setiap P, I, C, dan O.
    4. Jika SUDAH sesuai aturan (sinonim tidak memperluas *scope*, avoid list benar), ubah status menjadi "M3_STEP2_APPROVED".
    5. Jika BUTUH REVISI, ubah status ke "M3_STEP2_NEEDS_REVISION" dan ketikkan instruksi di feedback (misal: "Hapus kata X dari P karena itu menyalahi what counts").
	LANGKAH 3: SEARCH STRING + FILTER SPECIFICATIONS
    output : search_string
    ```txt
    Gunakan RAG dari dokumen:
    - keywords 
    - scope_justification

    Build search string + filter table. Tulis ke dokumen search_string di database. yang didalamnya terdiri dari 2 bagian yaitu:

    === BAGIAN 1: SEARCH STRING ===

    Format Scopus:
    TITLE-ABS-KEY((P1 OR P2 OR P3) AND (I1 OR I2 OR I3) AND (O1 OR O2))

    Aturan:
    - OR untuk synonyms dalam komponen sama
    - AND untuk antar komponen PICO
    - Wildcard (*) untuk variations (educat* → education, educational, educating)
    - Quotation marks untuk phrase ("machine learning")
    - Hindari sinonim dari AVOID list
    - Comprehensive tapi tidak terlalu broad

    (Jika multi-DB: adapt untuk WoS, Pubmed, dll. di sub-section "Adapted Strings")

    === BAGIAN 2: FILTER TABLE ===

    | Filter | Nilai | Justifikasi (1-2 kalimat dari scope_justification.md) |
    | Publication year | [periode] | [...] |
    | Language | [bahasa] | [...] |
    | Document type | [Article/Review/etc] | [...] |
    | Subject area (jika dibatasi) | [area] | [...] |
    | Open access only (jika iya) | [yes/no] | [...] |

    ATURAN: filter tanpa justifikasi → HAPUS dari daftar (akan ditanya reviewer).
    ```
    Cara Mengujinya Nanti:
    1. Sistem akan mencetak pesan [System] DIJEDA setelah membuat Search String.
    2. Buka MongoDB Compass, cari dokumen `search_string`.
    3. Periksa query Scopus di `scopus_query` (sintaks kurung, OR, AND, asterisk, kutipan).
    4. Periksa array `filters`, pastikan semua memiliki justifikasi yang jelas.
    5. Jika SUDAH tepat dan tidak ngawur sintaksnya, ubah status menjadi "M3_STEP3_APPROVED".
    6. Jika BUTUH REVISI, ubah status ke "M3_STEP3_NEEDS_REVISION" dan ketikkan instruksi di feedback (misal: "Hapus filter open access karena justifikasinya lemah").

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










