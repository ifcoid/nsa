# Roadmap — Modul SLR yang Belum Diimplementasi

> Dokumen ini memuat **spesifikasi desain** untuk modul/langkah yang **belum ada di kode** (masih stub atau belum dibuat). Dipisahkan dari [AGENT.md](AGENT.md) agar AGENT.md fokus pada alur yang sudah berjalan (Modul 2–5 + Modul 6 Langkah 1).
>
> Status saat ini di kode: Modul 6 (termasuk Langkah 2 & 3) **sudah diimplementasi** — lihat [AGENT.md](AGENT.md). Modul 7, 8, 8b, 9 masih *stub* (log + transisi status).
>
> Saat sebuah langkah selesai diimplementasi, pindahkan spesifikasinya kembali ke AGENT.md dan ganti penanda menjadi ✅.

---

## Modul 6 — Full-text Acquisition (lanjutan)  ✅ Implemented

> **Sudah diimplementasi penuh** (Langkah 1–3) — dokumentasi ringkas + alur status ada di [AGENT.md](AGENT.md). Spesifikasi rinci di bawah dipertahankan sebagai referensi desain asli.

### LANGKAH 2: FULL-TEXT SCREENING (DUAL-REVIEWER + AI-ASSIST)

Reviewer 1 + Reviewer 2 jalankan paralel di sesi masing-masing.

REASON CODES TAMBAHAN (full-text stage, di samping 8 codes Modul 4):
- METHODS-UNCLEAR — full-text tidak cukup deskripsi metodologi untuk
  kualifikasi
- NO-EMPIRICAL-DATA — konseptual paper tanpa data empiris (tertangkap
  abstract screen)
- DUPLICATE-POSTHOC — overlap dataset/konten (pilih versi paling komprehensif)
- POOR-QUALITY — kualitas metodologis di bawah threshold (formal di Modul 7;
  gunakan code ini hanya kasus ekstrem mis. predatory journal tanpa metodologi)
- OTHER

=== PROMPT PER REVIEWER ===

"Saya Reviewer [1 atau 2]. Full-text screening untuk SLR [topik].
Operational definitions: pico_definitions
Reason codes: 12 codes (8 abstract + 4 full-text).

Per artikel di database vector qdrant yang dibuat oleh aplikasi ../pede (yang Screener_[X]_Decision_Full kosong) sebagai RAG agar menghindari halusinasi dari LLM(jangan sampai ada fakta atau kesimpulan yang didapatkan dari luar kutipan RAG), batch bisa 5-10 artikel:

Output tabel:
| ID | Priority | Design Confirmed | P Match | I Match | O Match | Red Flags QA | Strict | Liberal | Recommend | Reason | Confidence |

Per artikel di analyze:
1. STUDY DESIGN: actual design dari methods section, konsisten dengan abstract?
2. POPULATION: detail demografi → match WHAT COUNTS? (kutip kalimat methods)
3. INTERVENTION/EXPOSURE: detail → match WHAT COUNTS?
4. OUTCOME: primer + sekunder, measurement tools validated?
5. METHODOLOGICAL RED FLAGS (untuk QA Modul 7):
   - Sample kecil tanpa power analysis?
   - Confounders tidak ditangani?
   - Follow-up inadequate?
   - Missing data tidak dilaporkan?
   Tag dengan prefix 'QA_RED:' di Notes
6. DUAL PERSPECTIVE (Strict default EXCLUDE / Liberal default INCLUDE)
7. RECOMMEND: INCLUDE/EXCLUDE/UNCERTAIN + reason code + confidence

Saya decide final → update screening Screener_[X]_Decision_Full +
Reason_Code_Full + Notes_Full."

=== ALUR HUMAN DECISION ===
1. Baca tabel rekomendasi
2. Spot-check kutipan ke artikel
3. Update screening
4. UNCERTAIN dari Modul 5 yang sekarang bisa di-decide → final decision

=== KEDUA REVIEWER INDEPENDEN ===
Tidak saling lihat sampai keduanya selesai. Kappa full-text auto-calculate.

Append progress + drift events ke fulltext_screening_log


### LANGKAH 3: RESOLVE CONFLICTS + AUDIT + EXTRACTION PREP + HASIL AKHIR

Setelah seluruh full-text screening selesai + kappa final dihitung.

Eksekusi 4 file output:

=== OUTPUT 1: fulltext_screening_log (FINAL) ===

A. RESOLVE DISAGREEMENTS:
Hasil dual-review: total screened, agreement, disagreements, kappa full-text.

Per DISAGREE case:
- Pattern: tier tertentu (LOW lebih sering)? Komponen tertentu (selalu di I)?
  Confusion antar reason codes mirip (METHODS-UNCLEAR vs POOR-QUALITY)?
- Diskusi 2 reviewer + rujuk operational def + full-text evidence
- Consensus → update Conflict_Resolution_Full di screening
- Jika pattern systematic: update operational def Modul 2 L3 + briefing

B. PICO-CONSISTENCY FINAL AUDIT:
Random 15% FINAL INCLUDED (post-full-text) → cek konsisten WHAT COUNTS.
Issues:
- ≤5%: acceptable, dokumentasi Limitations
- >5%: RE-SCREEN oleh R3 atau re-do pass 2

=== OUTPUT 2: inaccessible_impact ===

Karakteristik [N] inaccessible studies, rate [%]:
1. Skewed ke region/tahun/topik tertentu? Atau random?
2. Potensi bias systematic vs random
3. Mitigation disclosure (template ready-Limitations):
   "[N] studies ([%]) inaccessible despite [strategi]. These were
   [karakterisasi]. May introduce [bias type]. Estimated impact:
   [low/moderate/high] because [alasan]."

=== OUTPUT 3: extraction_readiness ===

CHECKLIST sebelum Modul 7:
[ ] Final INCLUDED list finalized (jumlah + daftar ID)
[ ] Semua DISAGREE resolved (Conflict_Resolution_Full filled)
[ ] Semua UNCERTAIN dari Modul 5 sudah di-decide
[ ] Full-text kappa calculated + dokumentasi
[ ] Exclusion reasons table (full-text stage) compiled
[ ] PICO-consistency audit completed
[ ] Inaccessible dokumentasi ready
[ ] PDFs tersimpan dengan naming convention
[ ] Spreadsheet kolom Full_Text_Location filled

PROCEED Modul 7 jika semua ✓.

=== OUTPUT 4: modul6_summary (HASIL AKHIR) ===

=== FULL-TEXT SCREENING SUMMARY (SLR) ===

ACQUISITION:
- Target: [N] (HIGH: X, MEDIUM: Y, LOW: Z)
- Acquired: [N] | Inaccessible: [N] ([%])
- Methods: [institutional/OA/author/ILL]

FULL-TEXT SCREENING:
- Total screened: [N]
- R1 + R2 complete: ✓
- Full-text kappa: [X]
- Disagreements: [N] | Resolved: [N]

DECISIONS:
- FINAL INCLUDED: [N] studies
- EXCLUDED at full-text: [N]

EXCLUSION REASONS (full-text stage → Methods appendix Modul 9):
| Reason Code | Count | % |
| P-NOMATCH / I-NOMATCH / O-NOMATCH / STUDY-DESIGN / METHODS-UNCLEAR /
  NO-EMPIRICAL-DATA / DUPLICATE-POSTHOC / POOR-QUALITY / OTHER

RESOLVED UNCERTAIN dari Modul 5: [breakdown INCLUDE/EXCLUDE/unresolved]
PICO-CONSISTENCY AUDIT: [issues, action]
INACCESSIBLE IMPACT: di outputs/inaccessible_impact.md
EXTRACTION READINESS: [all ✓ / pending]

FORWARD ARTIFACTS (→ Modul 7):
- Final INCLUDED list dengan PDF paths
- Study design breakdown
- Preliminary QA concerns flagged (POOR-QUALITY, METHODS-UNCLEAR notes)
- Canonical terminology dari pico_definitions.md

NEXT: Data extraction + QA (Modul 7)

Konfirmasi 4 dokumen di database tersimpan.

---

## Modul 7 — Data Extraction + QA → extraction  📝 Planned (stub)

### LANGKAH 1: FRAMEWORK SELECTION + EXTRACTION TEMPLATE

### LANGKAH 2: SYSTEMATIC EXTRACTION (AI-ASSISTED + 20% SPOT-VERIFICATION)

### LANGKAH 3: QUALITY APPRAISAL + THRESHOLD JUSTIFICATION + DUAL-RATER + SENSITIVITY ANALYSIS

### LANGKAH 4: SYNTHESIS PREPARATION + META-ANALYSIS FEASIBILITY + HASIL AKHIR

---

## Modul 8 — Analysis + Synthesis (A/B) → synthesis_results + figures  📝 Planned (stub)

### LANGKAH 1: DESCRIPTIVE ANALYSIS + HETEROGENEITY DEEP-DIVE

### LANGKAH 2: SYNTHESIS PATH DECISION + EXECUTION (JALUR A DEFAULT atau B UPGRADE)

### LANGKAH 3: GRADE EVIDENCE GRADING + ROBUSTNESS CHECKS

### LANGKAH 4: INTERPRETATION PREPARATION + HASIL AKHIR (BRIDGE KE MODUL 9)

---

## Modul 8b — Bibliometric (SLNA, opsional) — VOSviewer + integration  📝 Planned (stub)

### LANGKAH 1: DATA PREPARATION + THESAURUS CONSTRUCTION

### LANGKAH 2: VOSVIEWER ANALYSIS + 9-PARAMETER JUSTIFICATION

### LANGKAH 3: CLUSTER INTERPRETATION + KRITERIA KUANTITATIF (TIER 1-4)

### LANGKAH 4: SLNA INTEGRATION (BIBLIOMETRIC + SLR) + HASIL AKHIR

---

## Modul 9 — Manuscript Writing → manuscript_final  📝 Planned (stub)

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
