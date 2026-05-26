# Modul Topik(Langkah di dalamnya) Output

## 1 Fondasi Teori + Aturan Global -> (briefing)

 Langkah :

### 1. PENGENALAN SYSTEMATIC LITERATURE REVIEW (Teori Definisi, Tujuan, dan Jenis-jenis Literature Review)

### 2. METODOLOGI SLR(Research Question Formulasi PICO Framework)

### 3. PENGENALAN GENERATIVE AI DALAM PENELITIAN(Etika Penggunaan AI Transparansi, Bias, Limitasi)

### 4. KAPABILITAS LLM UNTUK PENELITIAN SLR

### 5. ATURAN GLOBAL SLR + COWORK (BERLAKU UNTUK SEMUA MODUL 2-9)

## 2 Topik Penelitian (PICO) -> pico_definitions

### LANGKAH 1: TENTUKAN TOPIK + KLASIFIKASI TIPE GAP

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
   * "Copy (salin) keseluruhan object/document dari 1 topik pilihan Anda."
   * "Buat field baru bernama selected_topic (di root document), lalu Paste isinya di sana."
   * Terakhir, ubah field status menjadi "M2_STEP1_APPROVED".
5. Jika Anda TIDAK SUKA (Butuh Revisi):
   * Ubah field status menjadi "M2_STEP1_NEEDS_REVISION".
   * Isi field feedback dengan alasan Anda (misal: "carikan topik yang lebih condong ke algoritma X").
6. Tekan "Update", dan jalankan ulang go run ./cmd/app/main.go! Jika approved akan lanjut ke Langkah 2. Jika revisi, akan men-generate ulang 3 saran topik baru.

### LANGKAH 2: REVIEW OF PRIOR REVIEWS (MATRIKS)

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

### LANGKAH 3: PICO FRAMEWORK + OPERATIONAL DEFINITIONS + TERMINOLOGI KANONIKAL

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

### LANGKAH 4: JUSTIFIKASI BATASAN SCOPE (3-LAPIS)

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

### LANGKAH 5: FORMULASIKAN RESEARCH QUESTIONS

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

### LANGKAH 6: CEK FINER + NOVELTY + INTERNAL COHERENCE + HASIL AKHIR

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

4. GAP CHARACTERIZATION (→ Modul 9 L4 Intro)
Tipe: A/B/C | Deskripsi: [...] | Evidence: [...]

5. PRIOR REVIEWS MATRIX (→ Modul 9 L4 "Review of Prior Reviews")
[tabel ringkas + novelty synthesis]

6. PICO + OPERATIONAL DEFINITIONS (→ Modul 5 inclusion criteria)
P/I/C/O: [ringkas WHAT COUNTS/DOESN'T per komponen]

7. CANONICAL TERMINOLOGY (→ konsisten di seluruh modul)
[term + definisi + alternatif ditolak]

8. SCOPE JUSTIFICATION (→ Modul 9 L4 Intro)
[batasan + justifikasi 3-lapis]

9. RESEARCH QUESTIONS (→ Modul 9 L4 objectives)
Primary + 3 Secondary dengan traceability

10. FINER + NOVELTY CHECK
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

## 3. Search Strategy -> search_log

### LANGKAH 1: DATABASE SELECTION + JUSTIFICATION

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

### LANGKAH 2: KEYWORDS DEVELOPMENT (PICO + AVOID LIST)

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

### LANGKAH 3: SEARCH STRING + FILTER SPECIFICATIONS

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

### LANGKAH 4: PRE-VALIDASI + EKSEKUSI + DATE STAMP + UPDATE POLICY + HASIL AKHIR

output : search_log dan modul3_summary

```txt
Gunakan RAG dari dokumen:
- search_string
- keywords
- database_selection

Eksekusi 3 fase + 2 dokumen output.

=== FASE 1: PRE-VALIDASI SEARCH STRING ===

Sebelum user mencari di Scopus, validasi:
1. Syntax check: TITLE-ABS-KEY format, Boolean operators, wildcard position,
quotation marks
2. Web search verifikasi keywords benar-benar dipakai di literatur bidang
3. Identifikasi "trap keyword" (homonim / istilah ambigu)
4. Estimasi awal: terlalu banyak/sedikit/cukup hasil?
- Jika sempit: saran sinonim tambahan (lolos operational def)
- Jika luas: saran ketatkan komponen mana

Output saran revisi (jika ada).

=== FASE 2: PRAKTIK EKSEKUSI DI SCOPUS ===

Instruksi untuk user jalankan manual:
1. Login Scopus → Advanced Search
2. Input search string + apply filters
3. Catat: total hits pre-filter, total post-filter, tanggal pencarian
4. (Repeat untuk multi-DB jika applicable)

Setelah user input hasil → cowork lakukan FASE 3:

=== FASE 3: EVALUASI HASIL + SANITY CHECK ===

Berdasarkan hasil yang user input:
- Jumlah results reasonable untuk SLR? (tergantung scope)
- Cek sample 5-10 judul: relevan dengan PICO?
- Identifikasi noise dari "trap keyword"?
- Saran adjustment (jika perlu) dengan trade-off precision vs recall

=== OUTPUT 1: dokumen search_log di database ===

Format:
- Search string final: [...]
- Filters applied: [tabel]
- Database(s): [list]
- Date executed: [YYYY-MM-DD per DB]
- Total hits per DB: [breakdown]

DATE STAMP & UPDATE POLICY:
- Tanggal pencarian = cut-off temporal. Sumber publish setelah tanggal ini
TIDAK BOLEH jadi studi primer (akan ditandai inkonsisten di audit Modul 9).
- TRIGGER WAJIB RE-RUN:
· Manuscript belum di-submit dalam 6 bulan sejak search date
· Reviewer minta update di revision round
· Major breakthrough publication di topik inti
- PROSEDUR RE-RUN: ulang search string sama dengan cut-off baru, screening
records baru, dokumentasi di Methods "Search updated on [date], additional
[N] studies identified, [X] included."

=== OUTPUT 2: dokumen modul3_summary di database (HASIL AKHIR) ===

=== COMPLETE SEARCH STRATEGY (SLR) ===

DATABASE: [primary + supplementary + justifikasi]
KEYWORDS: [main + synonyms + AVOID list per PICO]
SEARCH STRING: [final string per DB]
FILTERS: [tabel + justifikasi]
SEARCH EXECUTION: [date, hits per DB, sanity check verdict]
UPDATE POLICY: [trigger + prosedur]

Konfirmasi 2 dokumen tersimpan di database.
baru bisa lanjut ke NEXT: Data mining + export (Modul 4)
```

Cara Mengujinya Nanti:

1. Sistem akan menampilkan Fase 1 (Pre-Validasi) di layar terminal dan meminta Anda menjalankan Fase 2.
2. Jalankan pencarian di Scopus, lalu buka MongoDB Compass. Isi total hits Anda di field `feedback` (misal: "Scopus 300 paper") dan ubah status ke "M3_STEP4_EVALUATION".
3. Sistem akan membuat `search_log` dan `modul3_summary` lalu berhenti.
4. Buka MongoDB Compass, periksa 2 dokumen tersebut (khususnya date stamp dan update policy).
5. Jika SUDAH lengkap, ubah status ke "M3_STEP4_APPROVED".
6. Jika BUTUH REVISI, ubah ke "M3_STEP4_NEEDS_REVISION" dan isi feedback.

## 4. Data Mining dan export Scopus dan source lainnya(multi sources database) -> screening

### LANGKAH 1: EKSEKUSI FINAL SEARCH + SANITY CHECK

output: data_mining_log

```txt
Minta user melakukan search dahulu di Scopus (+ DB lain jika multi-DB) sesuai
dengan dokumen search_string yang sudah dibuat di modul3, sistem lalu membukan kerangka dokumen untuk input dari feedback user(misal: sampling_pencarian_awal) di database berupa:
- Total hits pre-filter / post-filter per DB
- Tanggal pencarian (YYYY-MM-DD)
- Sample 20-50 judul pertama

LAKUKAN TAHAPAN SANITY CHECK (Human In The Loop):

1. PAPER-KUNCI CHECK:
user input 5-10 paper kunci (judul) ke dokumen yang sudah disiapkan sistem di database (misal: paper_kunci_cek) yang harus ada (dari supervisor /
prior reading). Cek apakah muncul di hasil search.
Jika BANYAK paper kunci absen → search string masih bermasalah,
balik ke Modul 3.

2. KONFIRMASI VOLUME:
Reasonable untuk SLR berdasar scope?
- >> expected: trap keyword? identifikasi
- << expected: filter terlalu ketat? saran longgarkan

1. GO/NO-GO DECISION:
- PROCEED: lanjut ke export Langkah 2
- REVISE: balik ke Modul 3 Langkah berapa?, alasan

Append hasil ke dokumen data_mining_log di database
```

Cara Mengujinya Nanti:

1. Aplikasi akan membuat format kosong `data_mining_log.initial_sample` di MongoDB dan masuk ke `M4_STEP1_WAITING_INPUT`.
2. Jalankan pencarian di Scopus (atau sesuai db_selection), lalu catat hasilnya.
3. Buka Compass, isi field `total_hits_pre_filter`, `total_hits_post_filter`, `sample_titles`, `key_papers_found`, dan `key_papers_missing` (Timpa semua tulisan [ISI DISINI]).
4. Ubah status ke `M4_STEP1_EVALUATE` dan Update. Sistem akan mengeksekusi agen Sanity Check.
5. Periksa dokumen `data_mining_log.sanity_check`. Jika agen merekomendasikan `PROCEED` dan Anda setuju, ubah status ke `M4_STEP1_APPROVED`.
6. Jika keputusan `REVISE`, cukup ubah status ke `M4_STEP1_NEEDS_REVISION`. Sistem akan melempar Anda KEMBALI ke Modul 3 Langkah 3 secara otomatis dan meminta agen merumuskan ulang kueri.

### LANGKAH 2: EXPORT + MULTI-DB DEDUP + PICO-CONSISTENCY PREVIEW

output: data_mining_log dan slr_papers_post_dedup

```txt
Minta user sudah melakukan export hasil search per DB (Misalnya: Scopus, IEEE, Web of Science) sesuai dengan dokumen search_string yang sudah dibuat di modul3. kemudian hasilnya masukkan ke dalam collection(saya minta saran kamu baiknya collecton atau dokumen? ) sources(misalnya: source_scopus, source_ieee, source_webofscience, dst) dengan isian data structure(bagaimana struktur data terbaiknya?):
- Authors
- Title
- Year
- Abstract
- Author Keywords
- Index Keywords
- DOI
- Source title
- Document Type
- lainnya yang perlu kamu tambahkan agar lebih detail dan komprehensif

Setelah source ada dan validitasnya isi database terkonfirmasi, eksekusi 3 task:
=== TASK 1: BASIC QUALITY AUDIT ===
Per source/collection:
- Verify total records
- Identify essential fields
- Data quality issues: missing abstract / keywords / DOI %
- Ringkasan: distribusi tahun, top 5 journal, breakdown document type

=== TASK 2: MULTI-DB DEDUPLICATION (jika multi-DB sources/collections) ===
Strategi bertingkat:
1. PRIMARY: DOI match
2. SECONDARY: normalized title (lowercase + strip punctuation) + year
3. TERTIARY: fuzzy title >90% similarity + first author surname

Output:
- Total records across all DB sources/collections
- Unique post-dedup disimpan dalam database (menurutmu di collection baru ok? hasil deduplikasinya)
- Duplicates breakdown per strategi
- Records eksklusif per DB sources/collections
- FLAG records dengan matching ambigu

Jika overlap lintas-DB sources/collections jauh lebih sedikit dari ekspektasi → cek normalisasi
DOI/title (formatting beda antar DB sources/collections).

=== TASK 3: PICO-CONSISTENCY PREVIEW CHECK ===
Ambil RANDOM 20 records post-dedup. Untuk setiap, klasifikasi berdasar
title+abstract+keywords vs operational definitions di dokumen pico_definitions:

- MATCH "WHAT COUNTS": memenuhi
- MATCH "WHAT DOESN'T": termasuk yang harus dieksklusi
- AMBIGU: tidak cukup info di abstract
- OFF-TOPIC: noise

Sajikan tabel + persentase.

INTERPRETASI:
- MATCH "WHAT COUNTS" >60% → search bagus, PROCEED L3
- 30-60% → acceptable, screening workload tinggi
- <30% → BACK TO MODUL 3 (perbaikan AVOID list / operational def)
- MATCH "WHAT DOESN'T" tinggi → AVOID list belum lengkap

Verdict + saran.
Append ke dokumen data_mining_log
```

Cara Mengujinya Nanti:

1. Lakukan export dataset dari masing-masing database (Scopus, WoS, IEEE) ke format CSV.
2. [PENTING] Buka CSV di Excel/Sheets, tambahkan kolom baru bernama `Database`, lalu isi dengan nama sumbernya (misal: "Scopus"). Simpan ulang CSV-nya.
3. Buka MongoDB Compass, klik Add Data -> Import File. Tumpuk/gabungkan semua CSV Anda ke dalam SATU collection bernama `slr_papers`. (Sistem Go kita sudah dilengkapi parser cerdas yang bisa mengenali format judul/abstrak yang beda antar database).
4. Ubah status sesion menjadi `M4_STEP2_PROCESS` dan biarkan program menghitung kualitas, mendeduplikasi (mencari irisan data dari tumpukan multi-DB tersebut), dan mencuplik 20 sampel paper untuk dites PICO Consistency oleh AI.
5. Cek dokumen `data_mining_log` di MongoDB (perhatikan isian `quality_audit`, `dedup`, dan `pico_preview`).
6. Anda juga WAJIB mengecek collection baru bernama `slr_papers_post_dedup` di MongoDB. Ini adalah hasil kumpulan data unik yang tervalidasi dan siap digunakan untuk proses *Screening*.
7. Jika `match_counts_pct` > 60% dan verdict "PROCEED L3", ubah status ke `M4_STEP2_APPROVED`.
8. Jika PICO Preview buruk (<30%), ubah status ke `M4_STEP2_NEEDS_REVISION`. Sistem akan membatalkan seluruh dataset ini dan menyuruh Anda mengulang kembali Modul 3 (membangun ulang *search string*).

### LANGKAH 3: SETUP SCREENING DATABASE + EMBEDDED CRITERIA + HASIL AKHIR

output: collection screening

```txt
Gunakan RAG dari:
- collection slr_papers_post_dedup
- pico_definitions

Eksekusi 2 output:

=== OUTPUT 1: collection screening ===

Bagian 1 "Screening":

HEADER META (Row 1-6) — embedded criteria:
Row 1: "Tanggal pencarian: [dari search_log]"
Row 2: "P Canonical: [term]"
Row 3: "P WHAT COUNTS: [...]"
Row 4: "P WHAT DOESN'T: [...]"
Row 5: "I/C/O WHAT COUNTS: [ringkas]"
Row 6: (separator kosong)

KOLOM DATA (mulai Row 7):
| ID | Source_DB | Authors | Year | Title | Abstract | Keywords | DOI | Journal |
| Screener_1_Decision | Screener_1_Reason_Code | Screener_1_Notes |
| Screener_2_Decision | Screener_2_Reason_Code | Screener_2_Notes |
| Agreement (formula =IF(S1=S2,"AGREE","DISAGREE")) |
| Conflict_Resolution | Final_Decision |
| Full_Text_Retrieved | Full_Text_Location |

REASON CODES STANDAR:
- P-NOMATCH | I-NOMATCH | O-NOMATCH | STUDY-DESIGN | LANGUAGE | DUPLICATE |
NO-ABSTRACT | OTHER (wajib isi notes)

Bagian 2 "Kappa_Calculation":
Tabel 2x2 + formula Cohen's kappa (po, pe, kappa).
Interpretasi: <0.20 Poor / 0.21-0.40 Fair / 0.41-0.60 Moderate /
0.61-0.80 Substantial (TARGET) / 0.81-1.00 Almost Perfect.

Bagian 3 "Summary":
Auto-calculated: total records, source DB breakdown, year distribution,
screening progress, agreement rate, current kappa, final INCLUDE count.

AUTO-FILL kolom Authors/Year/Title/Abstract/Keywords/DOI/Journal/Source_DB
dari collection slr_papers_post_dedup

=== OUTPUT 2: dokumen modul4_summary di database (HASIL AKHIR) ===

=== DATA MINING SUMMARY (SLR) ===

SEARCH EXECUTION:
- Date: [YYYY-MM-DD per DB]
- Total hits per DB: [breakdown]

SANITY CHECK:
- Paper-kunci: [X/Y found]
- Volume verdict: reasonable
- Go/no-go: PROCEED

EXPORT:
- Files: [list document atau collection]
- Records per DB sources
- Fields preserved

DEDUPLICATION:
- Total → Unique slr_papers_post_dedup
- Duplicates breakdown
- Manual review flags: [N]

PICO-CONSISTENCY PREVIEW:
- MATCH "WHAT COUNTS": X%
- Verdict: PROCEED

SCREENING DATABASE:
- Collection: screening
- Embedded criteria Row 1-5: ✓
- Reason codes: ✓ (8 standard)
- Kappa sheet: ✓
- Dual-reviewer columns: ✓

Konfirmasi collection screening + summary tersimpan.
NEXT: Title/abstract screening + kappa calibration (Modul 5)
```

Cara Mengujinya Nanti:

1. Pastikan Anda sudah melewati Langkah 2 dengan `M4_STEP2_APPROVED`.
2. Program akan secara otomatis mengubah *papers* dari `slr_papers_post_dedup` dan membentuk sebuah collection baru bernama `slr_screening`.
3. Buka MongoDB Compass, periksa `slr_screening` tersebut. Anda akan melihat bahwa data asli (Title, Abstract) telah dipadukan dengan kolom kosong untuk `Screener_1_Decision`, `Agreement`, `Final_Decision`, dll.
4. Periksa juga *field* `screening_setup` (berisi *embedded criteria*) dan `modul4_summary` di dalam dokumen sesi Anda.
5. Jika semua terlihat sempurna sebagai meja kerja *Screening* Anda, ubah status ke `M4_STEP3_APPROVED`. Modul 4 pun rampung!

## 5. Title & Abstract Screening -> screening (filled)

### LANGKAH 1: SCREENER BRIEFING (FINALISASI INTERPRETASI KRITERIA)

output : screener_briefing

```txt
Berdasarkan pico_definitions dan reason codes di screening_setup

Eksekusi 2 task:

=== TASK 1: VALIDASI KELENGKAPAN KRITERIA ===
- WHAT COUNTS per PICO sudah testable?
- EDGE CASES cukup menangkap borderline?
- Reason codes komprehensif untuk semua eksklusi plausible?

Jika ada gap → rekomendasi update ke Modul 2 L3 (bukan ad-hoc patch).

=== TASK 2: GENERATE SCREENER BRIEFING ===

Tulis ke dokumen screener_briefing di database (source of truth untuk
2 reviewer):

---
SCREENER BRIEFING — [topik]
Date: [YYYY-MM-DD]
Reviewers: [R1] & [R2]

1. CANONICAL TERMINOLOGY: [dari M2 L3]

2. OPERATIONAL DEFINITIONS (quick reference):
P/I/C/O: [WHAT COUNTS | WHAT DOESN'T | EDGE CASES per komponen]

3. DECISION TREE (kasus ambigu):
If [kondisi X] AND [Y] → INCLUDE
If [X] BUT NOT [Y] → UNCERTAIN, flag diskusi
If NOT [X] → EXCLUDE
[turunkan dari EDGE CASES]

4. REASON CODES (8 standard, wajib dipakai, no freeform):
[list dari screening_setup pada SLRSession]

5. UNCERTAIN PROTOCOL:
- Cukup info di abstract tapi sulit decide → UNCERTAIN + notes
- Abstract tidak cukup info → "pending full-text" di Notes
- JANGAN decide INCLUDE/EXCLUDE tanpa grounded operational def

1. AI-ASSISTANT WORKFLOW:
- Cowork berikan DUAL PERSPECTIVE (Strict + Liberal) untuk record sulit
- Reviewer baca, decide independen
- Decision/Reason/Notes = ditulis HUMAN
- Cowork tidak menggantikan judgment, hanya enrich pertimbangan

1. REPORTING (untuk Methods M9):
- Cohen's kappa = R1 vs R2 (HUMAN, bukan AI)
- AI-assistance dideklarasikan di Methods
---
```

Cara Mengujinya Nanti:

1. Pastikan Anda berada di status `M5_INIT` atau `M5_STEP1_BRIEFING`.
2. Sistem akan memvalidasi apakah PICO Definitions di Modul 2 cukup solid, dan men-*generate* `screener_briefing` di MongoDB.
3. Buka Compass, cek dokumen sesi, temukan kolom `screener_briefing`.
4. Cermati isi instruksi (*Decision Tree*, dsb). Jika AI menemukan ambiguitas parah, ia akan meminta Anda merevisi (`decision: REVISE_M2`).
5. Jika keputusannya revisi dan Anda sepakat, ubah status ke `M5_STEP1_NEEDS_REVISION` (akan melempar Anda ke Modul 2).
6. Jika keputusannya `PROCEED` dan Anda setuju, ubah ke `M5_STEP1_APPROVED`.

### LANGKAH 2: KALIBRASI DUAL-REVIEW + COHEN'S KAPPA

output :

```txt
Gunakan RAG:
- screener_briefing
- collection slr_screening

Prosedur kalibrasi (Hitl):
1. Siapkan dua agent untuk dijadikan reviewer 1 dan 2, Reviewer 1 menggunakan API Z-AI GLM dan Reviewer 2 menggunakan API groq. Keduanya menggunakan temperature 0.2.
2. Ambil Random 20 sample dari collection slr_screening (15 clear + 5 ambigu)
3. Kedua reviewer INDEPENDEN (tidak saling lihat)
4. Isi Screener_1_* dan Screener_2_* (Decision + Reason_Code + Notes), dimana Notes disi PERSPEKTIF + VERDICT-AID
5. Kemudian lakukan "Kappa_Calculation" dari hasil review

EVALUASI KAPPA (Hitl setelah user lihat hasil dari 20 sample):

Berdasarkan kappa hasil:

=== JIKA KAPPA ≥0.60 ===
- Konfirmasi: PROCEED ke L3 batch screening
- Append ke dokumen kalibrasi_log: "Iterasi 1, kappa=[X], PASSED"

=== JIKA KAPPA <0.60 ===
Root-cause analysis:
1. PATTERN DISAGREEMENT:
- Records mana yang berbeda?
- Disagreement di komponen tertentu (P/I/C/O)?
- Reason code berbeda meski decision sama (precision issue)?
1. AKAR MASALAH:
- Operational def ambigu? Komponen mana perlu dipertajam?
- Edge case belum tercover di briefing?
- Personality bias (R1 strict, R2 liberal)?
1. REKOMENDASI:
- Revisi screener_briefing (tambah edge cases)
- Diskusi 2 reviewer untuk disagreement → consensus → update interpretasi
- Rerun kalibrasi 20 sample BARU (bukan 20 yang sama)

Iterasi sampai kappa ≥0.60. Append setiap iterasi ke kalibrasi_log.
format: Iterasi | Tanggal | Kappa | Revisi yang dilakukan.
PROCEED ke Langkah3(L3) hanya jika kappa ≥0.60.
```

Cara Mengujinya Nanti:

1. Sistem kita telah dilengkapi *fallback*. Jika Anda belum mensetting kredensial API `z-ai` atau `groq` di MongoDB (`llm_configs`), program akan cerdas melakukan *fallback* ke `gemini`.
2. Ubah status sesi Anda ke `M5_STEP2_CALIBRATION` lalu jalankan `go run cmd/app/main.go`.
3. Program akan secara acak memungut 20 *paper* dari `slr_screening`, dan mendelegasikannya ke dua agen independen. Anda akan melihat log proses eksekusinya di terminal (1 hingga 20).
4. Setelah selesai, Go akan langsung membedah matriks probabilitas dan menghitung rasio **Cohen's Kappa**.
5. Jika nilainya `< 0.60`, program otomatis mengunci status di `M5_STEP2_WAITING_APPROVAL`. Buka Compass Anda, cari baris-baris berlabel `DISAGREE` di collection `slr_screening` untuk mendiagnosis penyebab pertengkaran AI.
6. **PENTING SEBELUM RERUN**: Anda wajib merevisi teks pada `screener_briefing.briefing_doc` (tambahkan *edge cases* atau pertajam definisi. Contoh: Tambahkan kalimat: "If method = Simulation -> EXCLUDE") langsung dari dalam Compass agar AI tidak mengulangi kesalahan yang sama.
7. Setelah direvisi, barulah *rerun* kalibrasi dengan mengembalikan status ke `M5_STEP2_CALIBRATION`. Program akan menarik 20 sampel **baru** secara otomatis.
8. Jika nilainya `>= 0.60`, program akan secara otomatis mengesahkannya dengan status `M5_STEP2_APPROVED`.

### LANGKAH 3: BATCH SCREENING MASSAL (AI-ASSISTED, HUMAN-DECIDED)

output :

```txt
Setelah kappa kalibrasi ≥0.60.

Workflow per reviewer (jalankan agen review 1 dan 2 secara paralel di sesi masing-masing untuk seluruh collection slr_screening):

=== PROMPT PER BATCH (sesuaikan dengan input output maksimal token a-ai dan groq, atau gunakan teknik chunking / batching sederhana dengan menjaga hemat token dan bebas halusinasi) ===

"Anda Reviewer [1 atau 2] untuk SLR [topik]. Briefing ada di dokumen
screener_briefing, kalibrasi κ=[X] (passed).

Proses records ID [start]-[end] dari collection slr_screening (yang
Screener_[X]_Decision masih kosong).

Per record output tabel:
| ID | Title (singkat) | Strict | Liberal | Recommend | Reason Code | Evidence | Confidence |

Recommend: INCLUDE / EXCLUDE / UNCERTAIN
Confidence: HIGH / MEDIUM / LOW

append dalam dokumen reviewer1_perspective dan reviewer2_perspective dan update slr_screening kolom Screener_[X]_*."

=== Human-in-the-loop ===
1. Baca reviewer1_perspective dan reviewer2_perspective
2.  Spot-check abstract original (llm bisa hallucinate paraphrase)
3.  Cek hasil Screener_[X]_Decision + Reason_Code + Notes di slr_screening
4.  Jika ada UNCERTAIN → flag untuk diskusi atau defer ke full-text

=== RESOLVE DISAGREEMENTS ===

"Tampilkan DISAGREE cases dari slr_screening (Screener_1 != Screener_2) +
UNCERTAIN cases. Per kasus berikan analisis 1-2 kalimat + saran resolusi
(DISCUSS / DEFER-TO-FULLTEXT / UPDATE-BRIEFING jika pattern systematic)."

R1 + R2 + supervisor diskusi → consensus → update Conflict_Resolution.

=== MONITOR KAPPA REAL-TIME ===
hitung Kappa_Calculation. Target kappa stabil ≥0.60. Jika drop
signifikan (drift interpretasi) → pause, refresh briefing, lanjut.

Append progress + drift events ke screening_results_log dalam database   
```

Cara Mengujinya Nanti:

1. Pastikan status sesi Anda `M5_STEP3_BATCH_SCREENING`. Jalankan `go run cmd/app/main.go`.
2. Program akan menyedot 20 paper yang belum di-screen, mengirimnya ke AI, dan berhenti sejenak di status `M5_STEP3_WAITING_RESOLUTION`.
3. Tugas Anda di Compass: Buka `slr_screening`, filter "Agreement" yang bernilai "DISAGREE" atau "UNCERTAIN".
4. Baca kolom Notes (Anda bisa membandingkan argumen Strict vs Liberal dari kedua AI untuk paper yang sama).
5. Tentukan nasib paper tersebut: isi `Conflict_Resolution` dengan keputusan akhir Anda, dan samakan `Final_Decision` ke INCLUDE atau EXCLUDE.
6. Kembalikan status ke `M5_STEP3_BATCH_SCREENING` dan jalankan Go lagi untuk memproses 20 paper berikutnya. Ulangi sampai semua paper habis (Go akan otomatis pindah ke Langkah 4).

### LANGKAH 4: REVIEW HASIL + EXCLUSION TABLE + FULL-TEXT PREP + HASIL AKHIR

output :

```txt
Eksekusi 2 dokumen output sekaligus:

=== OUTPUT 1: dokumen exclusion_table (Methods-ready appendix) ===

1.  FLOW NUMBERS untuk PRISMA 2020 diagram (Modul 9):
- Total records identified
- Duplicates removed
- Records screened
- Records excluded
- Records included for full-text

1. EXCLUSION REASONS TABLE (agregasi Reason_Code semua EXCLUDE):
| Reason Code | Count | % | Deskripsi |
| P-NOMATCH | X | Y% | Population tidak sesuai |
| I-NOMATCH | X | Y% | ... |
| O-NOMATCH | X | Y% | ... |
| STUDY-DESIGN | X | Y% | ... |
| LANGUAGE | X | Y% | ... |
| DUPLICATE | X | Y% | ... |
| NO-ABSTRACT | X | Y% | ... |
| OTHER | X | Y% | (ringkas notes) |

2. KAPPA REPORT untuk Methods:
- Kalibrasi iterasi 1: [X]
- Jumlah iterasi
- Kalibrasi final: [≥0.60]
- Batch massal final: [Y]
- Klasifikasi: Substantial/Almost Perfect
- Disagreements: [N], Resolved via discussion: [N], Deferred ke full-text: [N]

1. PICO-CONSISTENCY POST-SCREENING AUDIT:
Random 10% INCLUDED → cek konsisten dengan WHAT COUNTS.
Slipped-through: [N], Action: [re-screening / none]

2. FULL-TEXT PRIORITIZATION (untuk Modul 6):
- HIGH: clearly match PICO berdasar abstract
- MEDIUM: match sebagian, butuh full-text verify
- LOW: UNCERTAIN, defer ke full-text

=== OUTPUT 2: dokumen modul5_summary (HASIL AKHIR) ===

=== TITLE/ABSTRACT SCREENING SUMMARY (SLR) ===

KALIBRASI:
- Sample 20 records, [N] iterasi
- Kappa iter 1 → final: [X] → [Y]
- Revisi briefing: [ringkas]

BATCH SCREENING:
- Total screened: [N]
- R1 + R2 complete: ✓
- Agreement rate: [%]
- Final kappa: [X]
- AI-assistance: [dideklarasikan di Methods]

DECISIONS:
- INCLUDE for full-text: [N] ([%])
- EXCLUDE: [N] ([%])
- UNCERTAIN deferred: [N] ([%])

DISAGREEMENT RESOLUTION:
- Total: [N]
- Resolved discussion: [N]
- Resolved via full-text deferral: [N]

EXCLUSION REASONS (→ Methods appendix): di exclusion_table
PICO-CONSISTENCY AUDIT: [slipped-through count + action]
KAPPA REPORT (→ Methods): di exclusion_table
FULL-TEXT PREP (→ Modul 6):
- HIGH: [N] | MEDIUM: [N] | LOW: [N]

Konfirmasi 2 dokumen valid dan tersimpan.
NEXT: Full-text acquisition + screening (Modul 6)
```

Panduan Cara Mengujinya:

1. Pastikan semua *paper* telah lolos kalibrasi dan *batch screening*, atau ubah status secara manual (untuk *bypass*) menjadi `M5_STEP4_REVIEW_HASIL`.
2. Jalankan perintah terminal: `go run cmd/app/main.go`.
3. Go akan secara otomatis mengagregasi seluruh perhitungan matematis: **PRISMA Flow Numbers** dan **Tabel Persentase Exclusion Reasons**.
4. Go juga akan mengambil 10% *paper* yang berstatus `INCLUDE` secara acak dan memanggil AI Auditor (`z-ai`) untuk melakukan *PICO-Consistency Audit* untuk memastikan tidak ada paper melenceng yang lolos (*slipped-through*).
5. Terakhir, AI akan memprioritaskan semua paper `INCLUDE` menjadi tier HIGH/MEDIUM/LOW sebagai bekal Anda di Modul 6 nanti.
6. Semua hasil agregasi ini (*ExclusionTable* & *Modul5Summary*) akan ditanam secara permanen ke dalam koleksi dokumen sesi SLR Anda di MongoDB, lalu status dikunci menjadi `M5_DONE`. Silakan verifikasi wujud dokumen JSON tersebut melalui MongoDB Compass.

## 6 Full-text Acquisition -> pdfs/ + tracking

### LANGKAH 1: ACQUISITION STRATEGY + AUTO-DOWNLOAD + PRIORITY TRACKING

### LANGKAH 2: FULL-TEXT SCREENING (DUAL-REVIEWER + AI-ASSIST)

### LANGKAH 3: RESOLVE CONFLICTS + AUDIT + EXTRACTION PREP + HASIL AKHIR

## 7 Data Extraction + QA -> extraction

### LANGKAH 1: FRAMEWORK SELECTION + EXTRACTION TEMPLATE

### LANGKAH 2: SYSTEMATIC EXTRACTION (AI-ASSISTED + 20% SPOT-VERIFICATION)

### LANGKAH 3: QUALITY APPRAISAL + THRESHOLD JUSTIFICATION + DUAL-RATER + SENSITIVITY ANALYSIS

### LANGKAH 4: SYNTHESIS PREPARATION + META-ANALYSIS FEASIBILITY + HASIL AKHIR

## 8 Analysis + Synthesis (A/B) -> synthesis_results + figures

### LANGKAH 1: DESCRIPTIVE ANALYSIS + HETEROGENEITY DEEP-DIVE

### LANGKAH 2: SYNTHESIS PATH DECISION + EXECUTION (JALUR A DEFAULT atau B UPGRADE)

### LANGKAH 3: GRADE EVIDENCE GRADING + ROBUSTNESS CHECKS

### LANGKAH 4: INTERPRETATION PREPARATION + HASIL AKHIR (BRIDGE KE MODUL 9)

## 8b Bibliometric (SLNA, opsional) VOSviewer + integration

### LANGKAH 1: DATA PREPARATION + THESAURUS CONSTRUCTION

### LANGKAH 2: VOSVIEWER ANALYSIS + 9-PARAMETER JUSTIFICATION

### LANGKAH 3: CLUSTER INTERPRETATION + KRITERIA KUANTITATIF (TIER 1-4)

### LANGKAH 4: SLNA INTEGRATION (BIBLIOMETRIC + SLR) + HASIL AKHIR

## 9 Manuscript Writing manuscript_final

### LANGKAH 1: METHODS WRITING (PRISMA 2020 COMPLIANT)

### LANGKAH 2: RESULTS WRITING (STRUKTUR FRAMEWORK TCCM/ADO)

### LANGKAH 3: DISCUSSION WRITING (6 SUBSEKSI WAJIB)

### LANGKAH 4: FUTURE RESEARCH AGENDA (SUBSEKSI KHUSUS)

### LANGKAH 5: INTRODUCTION WRITING (5 SUBSEKSI WAJIB)

### LANGKAH 6: CONCLUSIONS WRITING (LEAN)

### LANGKAH 7: REFERENCES (FORMAT + VERIFY + TEMPORAL AUDIT + JOURNAL TIER)

### LANGKAH 8: ABSTRACT WRITING (250-300 KATA)

### LANGKAH 9: TITLE CREATION (3-5 ALTERNATIF)

### LANGKAH 10: AUDIT + COMPILE FINAL + HASIL AKHIR
