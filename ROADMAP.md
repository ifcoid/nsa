# Roadmap — Modul SLR yang Belum Diimplementasi

> Dokumen ini memuat **spesifikasi desain** untuk modul/langkah yang **belum ada di kode** (masih stub atau belum dibuat). Dipisahkan dari [AGENT.md](AGENT.md) agar AGENT.md fokus pada alur yang sudah berjalan (Modul 2–5 + Modul 6 Langkah 1).
>
> Status saat ini di kode: Modul 6 & 7 **sudah diimplementasi** — lihat [AGENT.md](AGENT.md). Modul 8, 8b, 9 masih *stub* (log + transisi status).
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

## Modul 7 — Data Extraction + QA → extraction  ✅ Implemented

> **Sudah diimplementasi penuh** (L1–L4) — dokumentasi ringkas + alur status di [AGENT.md](AGENT.md). Spesifikasi rinci di bawah dipertahankan sebagai referensi desain.

### LANGKAH 1: FRAMEWORK SELECTION + EXTRACTION TEMPLATE

Berdasarkan pico_definitions + screening breakdown study designs.

Eksekusi 2 task + create extraction:

=== TASK 1: FRAMEWORK RECOMMENDATION ===

Decision tree:

OPSI A — TCCM (Theory–Context–Characteristics–Methodology)
- Cocok: management, entrepreneurship, social sciences
- Target gap Tipe C (ketiadaan integrative framework)
- RQ menanyakan "bagaimana konsep X beroperasi dalam konteks Y"

OPSI B — ADO (Antecedents–Decisions–Outcomes)
- Cocok: decision science, consumer behavior, organizational behavior
- RQ "apa pemicu X, bagaimana keputusan, apa hasil"
- Studi dominan causal/process-oriented

OPSI C — PICO-BASED (classical)
- Cocok: health/medical/intervention science
- RQ efektivitas intervensi
- Studi dominan eksperimental/kuasi-eksperimental

OPSI D — CUSTOM (hybrid atau domain-specific)
- Tidak ada framework standar yang fit
- Wajib justifikasi ekstensif

REKOMENDASI untuk topik saya: [pilih + alasan 3-4 kalimat].

=== TASK 2: EXTRACTION TEMPLATE (turunkan dari framework) ===

Contoh untuk TCCM (adapt untuk ADO/PICO/CUSTOM):

| Kolom | Kategori | Isi |
| ID | — | SLR ID |
| Author (Year) | Meta | Citation |
| Journal / DOI | Meta | |
| **THEORY** | T | Teori/framework studi (nama + ref) |
| Theoretical_Lens | T | Paradigma (positivist/interpretivist/critical) |
| **CONTEXT_Geographic** | C | Negara/region |
| CONTEXT_Sector | C | Industri/bidang |
| CONTEXT_Timing | C | Periode data collection |
| **CHARACTERISTICS_Sample** | Ch | Ukuran + profil |
| CHARACTERISTICS_Unit | Ch | Unit analisis (individu/firm/dyad) |
| CHARACTERISTICS_Variables | Ch | Konstruk kunci |
| **METHODOLOGY_Design** | M | Study design |
| METHODOLOGY_DataCollection | M | Survey/interview/observation/secondary |
| METHODOLOGY_Analysis | M | Teknik analisis |
| METHODOLOGY_Validity | M | Strategi validitas |
| **Key_Findings** | Output | Temuan utama (1-2 kalimat) |
| **Quality_Score** | QA | Score dari L3 |
| **Notes** | Manual | Catatan extractor + QA_RED flags dari M6 |

=== CREATE extraction ===

row "Extraction":
- Header meta Row 1-5: Framework + canonical PICO + tanggal
- Kolom data Row 6+: sesuai template
- Pre-populate baris untuk setiap source dari screening
  Final_Decision=INCLUDE. Isi kolom meta dasar (ID, Author, Year, Journal, DOI).

row "QA_Scores":
Kolom: ID | Tool_Item_1 | ... | Tool_Item_N | Total_Score | Category (HIGH/MOD/LOW)

row "Sensitivity":
Tabel skenario threshold: Baseline | Ketat (+10%) | Longgar (-10%)
Per skenario: n included | findings stable/changed.

row "Summary":
Auto-calc: total extracted, NR rate per kolom, design breakdown, QA distribution.

Tulis framework selection + template ke framework_selection



### LANGKAH 2: SYSTEMATIC EXTRACTION (AI-ASSISTED + 20% SPOT-VERIFICATION)

Berdasarkan framework_selection + extraction + artikel di qdrant.

=== EKSTRACTOR 1 (LEAD): FULL EXTRACTION ===

Per batch 5-10 artikel PDF dari qdrant:

Prompt cowork:
"Extract dari artikel [list IDs] sesuai template extraction row
'Extraction' (kolom dari framework_selection). Operational defs:
pico_definitions

Per record:
1. Per field: kutip kalimat pendukung + section reference
   ('Methods p.5: We surveyed 234...')
2. [NOT REPORTED] jika tidak ada — JANGAN mengira
3. Konsisten canonical terminology
4. [AMBIGUOUS: alasan] untuk borderline
5. RED FLAGS QA:
   - Sample size kecil tanpa power analysis
   - Missing data tidak dijelaskan
   - Confounders tidak ditangani
   - Outcome tidak validated
   Tag dengan prefix 'QA_RED:' di Notes

Output tabel:
| ID | Author (Year) | [kolom framework] | Key Findings | QA Red Flags | Notes |

Di akhir batch:
1. Coverage summary: COMPLETE vs NR per studi
2. Daftar studi >3 NR (kandidat POOR-QUALITY)
3. Daftar AMBIGUOUS fields (verifikasi manual)

Update extraction row 'Extraction' langsung."

=== EKSTRACTOR 2 (VERIFIER): SPOT-VERIFICATION 20% ===

Random 20% sample + semua AMBIGUOUS fields → Ekstractor 2 verifikasi artikel
asli vs entry extraction

Disagreement:
- <5%: acceptable, dokumentasi Limitations
- 5-15%: refine extraction protocol, re-do flagged subset
- >15%: full dual-extraction untuk seluruh studi

Append progress + verifikasi disagreement rate ke extraction_log

=== EXTRACTION KAPPA (jika full dual-extract) ===
Hitung untuk kolom kategorik (study design, country, etc.).


### LANGKAH 3: QUALITY APPRAISAL + THRESHOLD JUSTIFICATION + DUAL-RATER + SENSITIVITY ANALYSIS

Eksekusi 4 fase + 2 output sekaligus.

=== FASE 1: TOOL SELECTION ===

Berdasarkan study designs breakdown (dari extraction):
- RCT: [N] | Kuasi-eks: [N] | Cross-sectional: [N] | Cohort: [N]
- Case-control: [N] | Qualitative: [N] | Mixed: [N] | Review: [N]

Decision tree:

OPSI 1 — TOOL HOMOGEN (jika 1 design dominan >70%)
- RCT → Cochrane RoB 2 atau Jadad
- Observational → NOS (Newcastle-Ottawa Scale)
- Qualitative → CASP Qualitative atau JBI
- SLR → AMSTAR 2

OPSI 2 — TOOL FLEKSIBEL LINTAS-DESAIN
- MMAT (Mixed Methods Appraisal Tool) — quant/qual/mixed dalam 1 rubrik
- JBI Critical Appraisal Tools (set per design, score dinormalisasi)
- Kmet et al. — quant + qual checklist

OPSI 3 — KOMBINASI (designs sangat heterogen)
- NOS observational + CASP qualitative
- Score normalisasi 0-100% untuk komparabilitas

REKOMENDASI: [tool + justifikasi 100-150 kata untuk Methods]

=== FASE 2: THRESHOLD JUSTIFICATION (3-LAPIS) ===

Tetapkan threshold + justifikasi 3-lapis. Tulis ke qa_threshold_justification:

LAYER 1: BERBASIS LITERATUR
- Web search: threshold UMUM dipakai SLR bidang serupa? (60%? 70%? continuous?)
- Cite 2-3 SLR di bidang sama sebagai precedent

LAYER 2: BERBASIS TOOL DEVELOPER
- MMAT (Hong et al.): tidak rekomendasikan single cut-off, suggest reporting per-item + aggregate
- AMSTAR 2 (Shea et al.): "high/moderate/low/critically low" dengan kriteria spesifik
- NOS: ≥7/9 bintang umum sebagai "high quality"

LAYER 3: BERBASIS FEASIBILITY
- Threshold tinggi → terlalu sedikit lolos, thin evidence
- Threshold rendah → bias dari low-quality
- Sweet spot tergantung pool studi + skor distribution

Tetapkan:
- Threshold: [X%]
- Kategorisasi: HIGH ≥[X+10]% | MODERATE [X]–[X+10-1]% | LOW <[X]%

=== FASE 3: DUAL-REVIEWER QA ===

Prosedur:
1. R1 + R2 nilai QA independen per studi via tool terpilih
2. Isi Sheet "QA_Scores" extraction
3. Hitung kappa untuk QA decisions (HIGH/MODERATE/LOW)
4. Disagreement → diskusi → consensus

Target kappa QA: ≥0.60 (Substantial). Jika rendah: pattern di item borderline?
Refine rubric, re-rate subset.

=== FASE 4: SENSITIVITY ANALYSIS ===

Tulis ke sensitivity_analysis

Jalankan 3 skenario:
| Skenario | Threshold | n included | Key Finding 1 | Key Finding 2 | Key Finding 3 |
| Baseline | [X]% | [N] | [...] | [...] | [...] |
| Ketat | [X+10]% | [N] | [...] | [...] | [...] |
| Longgar | [X-10]% | [N] | [...] | [...] | [...] |

INTERPRETASI:
- Findings STABIL lintas 3 skenario → ROBUST
- Findings BERUBAH → tandai "sensitive to quality threshold" + bahas Discussion
- Dokumentasikan di Appendix manuskrip (Modul 9)

Update row "Sensitivity" extraction + sensitivity_analysis.

### LANGKAH 4: SYNTHESIS PREPARATION + META-ANALYSIS FEASIBILITY + HASIL AKHIR

Berdasarkan extraction + sensitivity_analysis

Eksekusi 2 output:

=== OUTPUT 1: synthesis_prep (input Modul 8) ===

1. DESCRIPTIVE OVERVIEW:
- Distribusi study designs
- Distribusi geografis (TANDA bias geografis untuk Discussion)
- Distribusi tahun publikasi
- Distribusi per komponen framework (TCCM theories, contexts, dll.)
- Quality distribution (HIGH/MODERATE/LOW counts)

2. HETEROGENEITY ASSESSMENT (kritis untuk Jalur A vs B di Modul 8):

A. CLINICAL/CONTEXTUAL:
- Populasi COMPARABLE? (rentang usia, demographics)
- Intervention COMPARABLE? (durasi, intensitas, format)
- Outcomes COMPARABLE? (konstruk sama, measurement tools sama)

B. METHODOLOGICAL:
- Designs seragam atau campur?
- Analysis techniques serupa?
- Settings serupa (lab vs field)?

C. STATISTICAL (jika data tersedia):
- Effect sizes range: lebar atau sempit?
- Direction of effect: konsisten atau kontradiktif?
- Estimasi I² preliminary

VERDICT HETEROGENEITY:
- LOW (clinical+method+stat homogen): meta-analysis kuat dibenarkan
- MODERATE: meta-analysis feasible random-effects
- HIGH: meta-analysis berisiko, prefer narrative
- VERY HIGH: narrative only, effect sizes indicative

3. META-ANALYSIS FEASIBILITY FLAG (5-criteria check):

[ ] Heterogeneity verdict = LOW atau MODERATE
[ ] ≥3 studi outcome COMPARABLE (same construct, same measurement)
[ ] Effect size data tersedia (means+SDs / OR / RR / correlations)
[ ] Studi design sebanding (semua RCT, atau cross-sectional similar)
[ ] Operational def outcomes ≥80% similar across studies

Jika SEMUA YES → JALUR B FEASIBLE (meta-analysis di Modul 8)
Jika ada NO → JALUR A NARRATIVE (default)
Jika subset homogen → HYBRID (rare, perlu justifikasi)

⚠️ PERINGATAN KRITIS:
- JANGAN klaim pooled effect tanpa meta-analysis formal
- Keputusan Jalur A vs B harus TEGAS, dipertahankan konsisten Modul 8-9

4. FRAMEWORK-DRIVEN GROUPINGS (untuk narrative synthesis Jalur A):

Contoh TCCM:
- Theory groups: studi pakai Theory X (n=...), Theory Y (n=...)
- Context groups: geografis, sektor, temporal
- Characteristics groups: demografi
- Methodology groups: design, analysis

Setiap group → sub-section di Results (Modul 9 L2).

=== OUTPUT 2: modul7_summary (HASIL AKHIR) ===

=== EXTRACTION + QA SUMMARY (SLR) ===

FRAMEWORK SELECTION (→ Methods):
- Framework: [TCCM/ADO/PICO/CUSTOM]
- Justifikasi: [100-150 kata]

EXTRACTION:
- Total extracted: [N]
- Method: lead extractor + 20% spot-verification
- Disagreement rate: [%]
- NR fields prevalence: [ringkas]
- AMBIGUOUS resolved: [N]

QUALITY ASSESSMENT (→ Methods):
- Tool: [nama]
- Justifikasi tool: [100-150 kata]
- Threshold: [X%] dengan justifikasi 3-lapis di qa_threshold_justification
- Kategorisasi: HIGH [N] / MODERATE [N] / LOW [N]
- Dual-rater kappa QA: [X]
- Disagreements resolved: [N]

SENSITIVITY ANALYSIS (→ Appendix Modul 9):
- Baseline → Ketat → Longgar findings
- Robustness verdict: ROBUST / CONDITIONALLY ROBUST / SENSITIVE

DESCRIPTIVE OVERVIEW (→ Results Modul 9):
- Designs / Geographic / Temporal / Framework breakdown

HETEROGENEITY VERDICT: LOW / MODERATE / HIGH / VERY HIGH

META-ANALYSIS FEASIBILITY:
- Verdict: JALUR A NARRATIVE (default) / JALUR B META-ANALYSIS / HYBRID
- 5-criteria check: [breakdown]

FRAMEWORK-DRIVEN GROUPINGS (→ Modul 9 L2 Results structure):
- Group 1-N: [list]

FORWARD ARTIFACTS (→ Modul 8):
- extraction (semua data + QA scores + sensitivity)
- synthesis_prep (meta-analysis flag + groupings)
- sensitivity_analysis
- qa_threshold_justification

NEXT: Data Analysis + Synthesis (Modul 8)

Konfirmasi 2 dokumen tersimpan di database.

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
