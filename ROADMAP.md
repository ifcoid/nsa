# Roadmap — Modul SLR yang Belum Diimplementasi

> Dokumen ini memuat **spesifikasi desain** untuk modul/langkah yang **belum ada di kode** (masih stub atau belum dibuat). Dipisahkan dari [AGENT.md](AGENT.md) agar AGENT.md fokus pada alur yang sudah berjalan (Modul 2–5 + Modul 6 Langkah 1).
>
> Status saat ini di kode: SELURUH modul (M1–M9) **sudah diimplementasi** — lihat [AGENT.md](AGENT.md). Pipeline SLR end-to-end lengkap. (Publish figur ke GitHub Pages: ✅ config-driven via Settings.)
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

=== BAGIAN B: BRIEF AWAL CONTEXT ===

- Tugas: data extraction (RIGID a priori form) + QA wajib + sensitivity analysis
- Standar: PRISMA 2020 + Cochrane (Modul 1 Section E)
- Framework synthesis: TCCM / ADO / PICO-based / Custom (pilih di L1)
- QA tool: pilih sesuai design dominan (RoB 2 / NOS / CASP / MMAT / AMSTAR 2)
- AI assist: spot-verification 20%, decision FINAL = human
- Output per langkah: file di outputs/ + extraction.xlsx
- Bahasa: Indonesia. Output format: tabel ringkas, no preamble.

Konfirmasi setup + context siap untuk eksekusi L1-L4.

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

## Modul 8 — Analysis + Synthesis (A/B) → synthesis_results + figures  ✅ Implemented

> **Sudah diimplementasi** (L1–L4) — lihat [AGENT.md](AGENT.md). Figur = SVG native Go + publish GitHub Pages (config-driven). Spec rinci di bawah = referensi desain.

BRIEF AWAL CONTEXT:

- Tugas: descriptive analysis + synthesis (Jalur A or B) + GRADE
- Standar: PRISMA 2020 + Cochrane (Modul 1 Section E)
- Synthesis path: DEFAULT Jalur A NARRATIVE; UPGRADE ke Jalur B META hanya
  jika 5-criteria eligibility lolos (di L2)
- Visualisasi dual-format: SVG (submission) + PNG (slide/preview, DPI 300)
- AI assist: descriptive analysis + drafting synthesis text. Statistik
  meta-analysis di software (R metafor / Stata / RevMan), cowork sebagai
  advisor + interpreter.
- Output per langkah: file di outputs/
- Bahasa: Indonesia. Output format: tabel ringkas, no preamble.


### LANGKAH 1: DESCRIPTIVE ANALYSIS + HETEROGENEITY DEEP-DIVE

Berdasarkan extraction + synthesis_prep.

Eksekusi 3 task. Tulis ke descriptive_analysis + generate figures.

=== TASK 1: DESCRIPTIVE OVERVIEW ===

1. STUDY DESIGN distribution (frekuensi + % dominan)
2. TEMPORAL distribution (min/max/median, trend, burst period)
3. GEOGRAPHIC distribution (negara breakdown + dominasi regional jika ada)
   ⚠️ PENTING: ini bahan disclosure bias geografis di Discussion Modul 9
4. SAMPLE SIZE statistics (range, median, mean, total participants)
5. FRAMEWORK COMPONENT distribution (per TCCM/ADO/PICO)
6. QUALITY distribution: HIGH/MODERATE/LOW + %

=== TASK 2: VISUALISASI (SVG + PNG dual-format) ===

Generate ke outputs/figures/ di dalam repo github yang diinput di bagian config di frontend dan disimpan di mongodb(token, url repo, github pages adress):
- fig_temporal.svg + .png (line chart distribusi tahun)
- fig_geographic.svg + .png (bar chart top regions)
- fig_design.svg + .png (pie chart study design)
- fig_quality.svg + .png (stacked bar HIGH/MOD/LOW)

Aspect 16:9. Generate via Python matplotlib/seaborn:
    plt.savefig('outputs/figures/fig_temporal.svg', bbox_inches='tight')
    plt.savefig('outputs/figures/fig_temporal.png', dpi=300, bbox_inches='tight')

Jika cowork tidak bisa render → output script .py yang peserta jalankan.

=== TASK 3: HETEROGENEITY DEEP-DIVE ===

Kritis untuk Jalur A/B decision di L2.

A. CLINICAL/CONTEXTUAL HETEROGENEITY:
- Populasi COMPARABLE? (rentang usia, demographics)
- Intervention COMPARABLE? (durasi, intensitas, format)
- Outcomes COMPARABLE? (konstruk + measurement tools)

B. METHODOLOGICAL HETEROGENEITY:
- Designs seragam? Analysis techniques serupa? Settings serupa?

C. STATISTICAL HETEROGENEITY (jika data tersedia):
- Effect sizes range (lebar atau sempit?)
- Direction of effect (konsisten atau kontradiktif?)
- Estimasi I² preliminary

VERDICT:
- LOW (clinical+method+stat homogen) → meta-analysis kuat dibenarkan
- MODERATE → meta feasible random-effects
- HIGH → meta berisiko, prefer narrative
- VERY HIGH → narrative only, effect sizes indicative

Cluster identification: studi yang COMPARABLE → mungkin meta subset (HYBRID).



### LANGKAH 2: SYNTHESIS PATH DECISION + EXECUTION (JALUR A DEFAULT atau B UPGRADE)


Berdasarkan descriptive_analysis (heterogeneity verdict) +
synthesis_prep (M7 feasibility flag).

=== FASE 1: ELIGIBILITY CHECK + KEPUTUSAN ===

DEFAULT: JALUR A NARRATIVE/THEMATIC SYNTHESIS.
UPGRADE ke JALUR B META-ANALYSIS hanya jika SEMUA 5 syarat lolos:

[ ] Heterogeneity verdict (L1) = LOW atau MODERATE
[ ] ≥3 studi outcome COMPARABLE (same construct, same measurement)
[ ] Effect size data tersedia + dapat di-ekstrak konsisten
[ ] Studi design sebanding (semua RCT, atau cross-sectional similar)
[ ] Operational def outcomes ≥80% similar across studies

Jika SEMUA 5 YES → UPGRADE Jalur B (proceed FASE 3)
Jika ada NO → STAY Jalur A (proceed FASE 2)

Tulis verdict ke synthesis_path_decision:
"Verdict: [JALUR A / JALUR B / HYBRID]
Per-syarat: [check mark per syarat]
Rationale: [3-4 kalimat untuk Methods]"

⚠️ ATURAN KONSISTENSI:
- Methods: eksplisit jalur + justifikasi
- Results: tidak boleh bahasa meta-analitik jika Jalur A
- Discussion: interpretasi konsisten dengan jalur
- Abstract: sebut jalur eksplisit

REKOMENDASI KUAT: jika ambigu → pilih Jalur A. Reviewer tidak akan menolak
narrative well-executed; meta dipaksakan = tolak.

=== FASE 2: JALUR A EXECUTION (DEFAULT NARRATIVE/THEMATIC) ===

(Skip ke FASE 3 jika upgrade Jalur B.)

Tulis ke synthesis_results.

PERINGATAN BAHASA — JALUR A:
✗ JANGAN: "pooled effect", "d = 0.XX across studies", "overall effect size"
✓ BOLEH: indicative ranges INDIVIDUAL studies ("d = 0.3 to 0.9")
✓ BENAR: "synthesis", "thematic pattern", "evidence suggests", "consistent
  finding across studies"

Per komponen framework (TCCM/ADO/PICO) dari synthesis_prep:

A. THEORY SYNTHESIS (TCCM):
- Teori dominan? Under-utilized? Konsistensi penerapan?
- Sintesis: "Studies predominantly drew on [Theory X] (n=...), while [Theory Y]
  was less represented (n=...). Notable gap: limited application of [Theory Z]
  despite relevance."

B. CONTEXT SYNTHESIS:
- Geographic dominance (kritis — bahan Discussion limitasi)
- Sectoral patterns | Temporal trends

C. CHARACTERISTICS SYNTHESIS:
- Sample demographics aggregate | Unit of analysis | Variables measured

D. METHODOLOGY SYNTHESIS:
- Design distribution | Analytical approaches | Methodological sophistication

E. PATTERN IDENTIFICATION (3 jenis):
1. CONSISTENT FINDINGS:
   "Across [N] studies, [finding X] consistently observed, effect sizes
   [Y]–[Z] (indicative range, not pooled)."
2. CONTRADICTORY FINDINGS:
   "Evidence regarding [X] mixed. [N1] reported [direction A] (e.g., Smith 2021);
   [N2] reported [direction B] (e.g., Wang 2023). Divergence may reflect
   [context/method/population differences]."
3. EMERGING/UNDER-RESEARCHED → Future Research agenda Modul 9 L4

F. VOTE COUNTING (qualified):
"Of [N] studies examining [X], [N1] positive, [N2] negative, [N3] no
significant. We note vote counting does not account for sample size or effect
magnitude; findings interpreted alongside qualitative synthesis."

G. QUALITY-STRATIFIED SYNTHESIS:
- HIGH quality findings vs MODERATE vs LOW
- Sensitivity argument: "Findings dari HIGH selaras dengan keseluruhan →
  robustness."

H. NARRATIVE INTEGRATION:
Tulis paragraf sintesis per RQ (primary + 3 secondary). Tiap paragraf:
- Jawab RQ langsung
- Grounded di evidence (sitasi)
- Akui nuansa contexts/populations
- Tidak overclaim

=== FASE 3: JALUR B EXECUTION (META-ANALYSIS — opsional upgrade) ===

(Skip jika Jalur A.)

Tulis ke synthesis_results.

⚠️ Statistik di software (R metafor / Stata / RevMan), cowork sebagai
advisor + interpreter. JANGAN minta cowork hitung pooled effect — tidak akurat.

A. MODEL SELECTION:
- Fixed Effects: studi sangat homogen (replicasi)
- Random Effects (RE) — DEFAULT untuk SLR lintas-konteks
  Estimator: DerSimonian-Laird atau REML

B. EFFECT SIZE STANDARDIZATION:
- Continuous: SMD (Cohen's d / Hedges' g) atau raw mean diff jika tool sama
- Dichotomous: OR / RR (lebih interpretable) / RD
- Correlation: Fisher's z transformed → back-transform untuk presentation

C. POOLED EFFECT + 95% CI + 95% PI:
"Pooled [type] = [X] (95% CI: [Y]–[Z]; 95% PI: [P]–[Q]) from [N] studies
using random-effects (REML)."

D. HETEROGENEITY STATISTICS:
- Cochran's Q + p-value
- I² (0-40 might not be important / 30-60 moderate / 50-90 substantial /
  75-100 considerable)
- τ² + Prediction Interval

E. PUBLICATION BIAS (jika ≥10 studi):
- Funnel plot + Egger's regression test
- Trim-and-fill jika ada bias
- <10 studi: funnel boleh, tests tidak powered cukup

F. FOREST PLOT (WAJIB):
Generate dual-format SVG + PNG ke outputs/figures/fig_forest_plot.*
Elements: individual effects + CI, overall pooled + CI, weight, heterogeneity stats.

G. SUBGROUP ANALYSIS / META-REGRESSION (jika I² >50%):
Subgroup by: quality tier, study design, geographic, year, framework moderators.
Meta-regression untuk continuous moderators.

H. REPORTING (PRISMA 2020 compliant):
Methods: model + estimator + effect metric + software + heterogeneity approach +
publication bias + subgroup plan.
Results: pooled effect + CI + PI + forest plot + heterogeneity + publication bias
+ subgroup + sensitivity.

🚨 TOLAK UPGRADE jika I² >75% atau publication bias berat. Kembali Jalur A,
dokumentasi keputusan di Methods sebagai transparency.


### LANGKAH 3: GRADE EVIDENCE GRADING + ROBUSTNESS CHECKS

Berdasarkan synthesis_results + sensitivity_analysis
+ extraction row QA_Scores.

Tulis ke grade_evidence_table.

=== TASK 1: GRADE PER OUTCOME ===

Per outcome / RQ, grade evidence confidence dengan 5 GRADE domains:
1. Study limitations (RoB aggregate)
2. Inconsistency (I² atau heterogeneity narrative)
3. Indirectness (PICO alignment — apakah studi jawab RQ langsung?)
4. Imprecision (CI width Jalur B / small n Jalur A)
5. Publication bias (funnel Jalur B / database coverage Jalur A)

CONFIDENCE LEVELS:
- HIGH: semua domain lolos
- MODERATE: 1 domain meragukan
- LOW: 2+ domain meragukan
- VERY LOW: banyak domain concern

Tabel:
| Outcome | Studies | RoB | Inconsistency | Indirectness | Imprecision | Pub Bias | Overall GRADE |

=== TASK 2: ROBUSTNESS SUMMARY ===

1. Sensitivity analysis (M7 L3.4): findings stable lintas threshold?
2. Subgroup/stratified: konsisten lintas quality/geographic/design?
3. Publication bias (Jalur B): trim-and-fill ubah conclusion?
4. Missing studies (inaccessible M6): impact assessment

VERDICT: ROBUST / CONDITIONALLY ROBUST / NOT ROBUST → Discussion Modul 9.

=== TASK 3: CONFIDENCE STATEMENTS (untuk Discussion) ===

Template Jalur B:
"We have [HIGH/MOD/LOW] confidence in pooled estimate that [X] based on [N]
studies with [quality profile]. Reflects [domains support/threaten]."

Template Jalur A:
"We have [HIGH/MOD/LOW] confidence in thematic finding that [X] based on [N]
studies. Consistent patterns across [domains]; however [caveats heterogeneity/
quality/context]."


### LANGKAH 4: INTERPRETATION PREPARATION + HASIL AKHIR (BRIDGE KE MODUL 9)

Berdasarkan semua output L1-L3.

Eksekusi 2 dokumen output:

=== OUTPUT 1: interpretation_package (untuk Modul 9) ===

1. ANSWERS TO RESEARCH QUESTIONS:
- Primary RQ: [50-100 kata, grounded synthesis + GRADE confidence]
- Secondary RQ 1-3: format serupa

2. KEY FINDINGS (3-5 headline messages):
- Finding 1-5: statement grounded
- Tiap finding: kontribusi baru? dialog teori? implikasi praktik?

3. SURPRISING/UNEXPECTED FINDINGS:
- Statement + kemungkinan penjelasan + implikasi

4. CONTRADICTIONS WORTH DISCUSSING:
- Studi X vs Y: penjelasan (contextual/methodological/populasi) + implikasi

5. DIALOG DENGAN TEORI (TCCM emphasis):
- Teori dominan? Findings KONFIRMASI atau MEMPERLUAS atau MENANTANG?
- Teori UNDER-UTILIZED?

6. HETEROGENEITY NARRATIVE (untuk Discussion):
- Geographic bias acknowledgment
- Methodological/temporal differences

7. GAPS FOR FUTURE RESEARCH AGENDA:
- 3 HIGH priority + 2 MEDIUM + 1 LONG-TERM
- Tiap gap: research question (PCC-aligned) + suggested methodology
- Trace ke evidence specific dari synthesis

8. LIMITATIONS SELF-AUDIT (3-tier):
- Review-level: database coverage, geographic bias, language, timeframe,
  PICO consistency
- Study-level: quality distribution, measurement heterogeneity, missing
  (inaccessible)
- Synthesis-level:
  - Jalur A: subjectivity thematic coding
  - Jalur B: publication bias residual, subgroup power
  - Framework selection subjectivity

=== OUTPUT 2: modul8_summary (HASIL AKHIR) ===

=== ANALYSIS + SYNTHESIS PACKAGE (SLR) ===

DESCRIPTIVE ANALYSIS (→ Results Modul 9 L2):
- Studies: [N] | Designs/Temporal/Geographic breakdown
- Quality: HIGH [N] / MOD [N] / LOW [N]
- Framework component breakdown

HETEROGENEITY VERDICT: LOW/MODERATE/HIGH/VERY HIGH

SYNTHESIS PATH: JALUR A NARRATIVE (default) / JALUR B META-ANALYSIS / HYBRID
- Justifikasi 100-150 kata (Jalur B: 5 syarat checklist; Jalur A: default
  confirmation)

JALUR A (jika dipakai):
- Framework-driven synthesis per komponen
- Consistent + contradictory findings
- Indicative ranges (jika applicable)
- Quality-stratified

JALUR B (jika dipakai):
- Model: [Fixed/Random + estimator]
- Pooled effect: [value] (95% CI, 95% PI)
- I² + τ² + Prediction Interval
- Forest plot di repo github : outputs/figures/fig_forest_plot.svg + .png
- Publication bias: [assessment]
- Subgroup analyses: [findings]

GRADE EVIDENCE TABLE (→ Discussion Modul 9 L3):
| Outcome | GRADE Confidence | di grade_evidence_table

ROBUSTNESS VERDICT: ROBUST / CONDITIONALLY ROBUST / NOT ROBUST

INTERPRETATION PACKAGE (→ Modul 9 L2-L5):
- Answers to RQs (primary + secondary)
- 3-5 key findings
- Surprising findings
- Contradictions
- Dialog teori
- Heterogeneity narrative
- Gaps untuk Future Research
- Limitations self-audit

FORWARD ARTIFACTS (→ Modul 9):
- interpretation_package
- grade_evidence_table
- synthesis_results
- outputs/figures/* (semua dual-format SVG + PNG) pada repo github
- Geographic bias disclosure (akan jadi Discussion paragraph + Title framing)

NEXT: Manuscript Writing (Modul 9)

Konfirmasi 2 dokumen tersimpan di database.

---

## Modul 8b — Bibliometric (SLNA, opsional) — VOSviewer + integration  ✅ Implemented

> **Sudah diimplementasi** (L1–L4) — lihat [AGENT.md](AGENT.md). VOSviewer dijalankan manual oleh user (paste hasil). Spec rinci di bawah = referensi desain.


BRIEF AWAL CONTEXT:

- Tugas: bibliometric analysis (SLNA — Systematic Literature Network Analysis)
- Tool: VOSviewer (https://www.vosviewer.com)
- Standar: Aria & Cuccurullo (2017) bibliometrix; Donthu et al. (2021)
  bibliometric review guidelines
- Output: network visualization + cluster interpretation + integration SLR
- Visualisasi dual-format: SVG (submission) + PNG (slide/preview)
- AI assist: thesaurus construction + parameter justifikasi + cluster
  interpretation. VOSviewer execution = peserta manual (cowork tidak run app).
- Output per langkah: file di outputs/
- Bahasa: Indonesia. Output format: tabel ringkas, no preamble.

### LANGKAH 1: DATA PREPARATION + THESAURUS CONSTRUCTION

Berdasarkan raw exports scopus csv (RAW dari M4 — sebelum dedup atau setelah,
tergantung scope SLNA Anda).

⚠️ Untuk SLNA: gunakan SAMA dataset dengan SLR untuk integrasi koheren.

Eksekusi 3 task:

=== TASK 1: VERIFIKASI DATA ===
- Total records di dari hasil raw import CSV
- Fields essential: Authors, Author Keywords, Index Keywords, Cited References,
  Title, Year, Source title
- Identify missing fields (akan affect bibliometric metrics)

=== TASK 2: THESAURUS CONSTRUCTION ===

Bibliometric butuh terminologi konsisten — variasi kata sama (mis. "AI",
"artificial intelligence") harus di-merge.

Generate thesaurus_keywords + thesaurus_authors format VOSviewer:
[label] [synonym1, synonym2, ...]

Contoh thesaurus_keywords:
artificial intelligence    AI, A.I., artificial-intelligence
machine learning           ML, machine-learning
big data                   big-data, large-scale data
[dst — auto-generate dari Author Keywords + Index Keywords dari raw import CSV]

Aturan thesaurus:
- Lowercase semua
- Replace "-" dengan space (atau sebaliknya, konsisten)
- Merge plural/singular
- Hapus stop words ("study", "analysis", "research")
- Domain-specific synonyms

Letakkan di collection atau document exports

=== TASK 3: DOKUMENTASI ===
Tulis ke bibliometric_log:
- Total records analyzed
- Thesaurus entries generated (jumlah merged terms)
- Approach: bibliometrix R atau VOSviewer direct?
- Date executed



### LANGKAH 2: VOSVIEWER ANALYSIS + 9-PARAMETER JUSTIFICATION

Eksekusi VOSviewer dengan 9 parameter eksplisit-justifikasi.

Tulis ke vosviewer_parameters (siap-Methods Modul 9):

=== TABEL 9 PARAMETER + JUSTIFIKASI ===

| # | Parameter | Setting | Justifikasi (1-2 kalimat) |
| 1 | Type of analysis | Co-occurrence / Co-authorship / Citation / Bibliographic coupling | [pilih sesuai RQ] |
| 2 | Unit of analysis | Author keywords / Index keywords / All keywords / Authors / Documents / Sources | [pilih + alasan] |
| 3 | Counting method | Full counting / Fractional counting | Full untuk volume, fractional untuk equal contribution |
| 4 | Min occurrences threshold | [N] | Justifikasi: balance noise vs signal (umumnya 5-10 untuk N=200 records) |
| 5 | Min cluster size | [N] | Justifikasi: cluster <[N] item dianggap noise |
| 6 | Resolution | [0.5-1.5] | Default 1.0; lower = fewer larger clusters; higher = more smaller |
| 7 | Normalization | Association strength / Fractionalization / LinLog/modularity | Association strength = default VOSviewer |
| 8 | Clustering algorithm | LinLog/modularity (default) | Sesuai literatur bibliometric standard |
| 9 | Visualization | Network / Overlay (temporal) / Density | Generate ALL 3 untuk insight komprehensif |

ATURAN: setiap parameter MUST ada justifikasi. Reviewer SLNA akan tanya
"why this threshold" — siapkan jawaban di Methods.

=== EKSEKUSI MANUAL OLEH USER ===

1. Buka VOSviewer (https://www.vosviewer.com — gratis)
2. Create > Map based on bibliographic data
3. Read data from bibliographic database files (Scopus → exports/scopus_*.csv)
4. Apply thesaurus_keywords.txt (atau thesaurus_authors.txt)
5. Set 9 parameter sesuai justifikasi
6. Generate network → 3 visualisasi:
   - Network visualization (cluster colors)
   - Overlay visualization (temporal — color = year)
   - Density visualization (heatmap)
7. Export ke vosviewer/ + figures/ (dual format SVG + PNG):
   - Right click → Save image as → SVG (vector untuk submission)
   - Right click → Save image as → PNG (DPI 300 untuk slide)

=== PESERTA PASTE HASIL ===

Setelah ekspor, peserta paste:
- Total nodes (keywords/authors/documents)
- Total edges (links)
- Total clusters (jumlah)
- Top-3 clusters by size + label
- Notable bridge nodes (keywords yang menghubungkan ≥2 clusters)
- Temporal trend dari overlay

Append ke bibliometric_log.




### LANGKAH 3: CLUSTER INTERPRETATION + KRITERIA KUANTITATIF (TIER 1-4)

Berdasarkan hasil VOSviewer (peserta paste data cluster).

Eksekusi cluster classification dengan kriteria KUANTITATIF (bukan subjective).
Tulis ke cluster_interpretation.

=== TIER 1 — CORE CLUSTERS (DOMINANT) ===
Kriteria kuantitatif:
- Size: ≥X% dari total nodes (mis. ≥15%)
- Total Link Strength (TLS): tinggi (top-quartile)
- Stability: dominan di multiple resolutions

Per Tier 1 cluster:
- Label (cowork suggest dari top-5 keywords + literature context)
- Size (n nodes, % total)
- TLS aggregate
- Top-5 keywords by occurrences
- Theme interpretation (1 paragraf)
- Foundational works yang typical di cluster ini

=== TIER 2 — EMERGING CLUSTERS ===
Kriteria:
- Size moderate
- Mayoritas keywords publish dalam 5 tahun terakhir
- Trend meningkat di overlay temporal

Per Tier 2 cluster:
- Label + ukuran + emerging trend rationale
- Top-5 keywords + tahun rata-rata
- Implikasi: research direction yang sedang berkembang

=== TIER 3 — ESTABLISHED CLUSTERS ===
Kriteria:
- Size moderate
- Mayoritas keywords publish 5-10+ tahun lalu
- TLS tinggi (saturated topic)

Per Tier 3 cluster:
- Label + insight: research area yang matur

=== TIER 4 — PERIPHERAL CLUSTERS ===
Kriteria:
- Size kecil (<5% total nodes)
- TLS rendah
- Bisa overlooked atau underexplored area

Per Tier 4 cluster:
- Label + label "potential gap" atau "niche topic"

=== BRIDGE KEYWORDS ===
Identifikasi keywords yang muncul di ≥2 clusters dengan link strength tinggi:
- Bridge antara Tier X dan Y
- Implikasi: konsep yang menjembatani 2 areas → opportunity untuk integratif research

=== STRUCTURAL HOLES (untuk Future Research M9 L4) ===
Identifikasi area dimana 2 clusters HARUSNYA terhubung tapi tidak ada bridge:
- Gap konseptual antara Cluster A dan Cluster B
- Future research opportunity: develop bridge konsep

Output table summary:
| Tier | Cluster Label | Size (%) | TLS | Top-5 Keywords | Interpretation |





### LANGKAH 4: SLNA INTEGRATION (BIBLIOMETRIC + SLR) + HASIL AKHIR

Berdasarkan cluster_interpretation + synthesis_results
(dari Modul 8) + interpretation_package (M8).

Eksekusi 2 dokumen output:

=== OUTPUT 1: slna_integration ===

INTEGRATION FRAMEWORK: theme validation lintas-method.

Per tema/finding utama dari SLR (Modul 8):

| SLR Theme/Finding | Bibliometric Cluster | Validation Status |
| [theme 1] | [cluster X Tier Y] | CONVERGENT |
| [theme 2] | [tidak ada cluster match] | SLR-ONLY |
| [theme 3 — implicit di SLR] | [cluster Z Tier 2] | BIB-ONLY (potential gap) |

VALIDATION CATEGORIES:
- CONVERGENT: SLR finding + bibliometric cluster sejalan → kuat
- SLR-ONLY: tema di SLR tapi tidak prominent di network → mungkin emerging atau
  niche
- BIB-ONLY: cluster bibliometric tapi tidak terangkat di SLR synthesis →
  possibly missed insight (re-examine SLR atau temuan baru)

RESEARCH LANDSCAPE POSITIONING:
- Posisi riset Anda dalam network: cluster mana?
- Bridge node potential: konsep yang Anda riset menjembatani area mana?

CONVERGENT GAPS (untuk Future Research M9 L4 — paling kuat):
Gap yang teridentifikasi DI KEDUA: SLR synthesis + bibliometric structural holes:
- Convergent gap 1: [...]
- Convergent gap 2: [...]
- Convergent gap 3: [...]

Trace ke evidence dari kedua method.

=== OUTPUT 2: modul_bibliometric_summary (HASIL AKHIR) ===

=== BIBLIOMETRIC + SLNA SUMMARY ===

DATA PREPARATION:
- Records analyzed: [N]
- Thesaurus entries: keywords [N], authors [N]
- Source: exports/scopus_*.csv

VOSVIEWER ANALYSIS:
- Type of analysis: [dari L2]
- 9-parameter table: vosviewer_parameters
- Generated: 3 visualizations (network + overlay + density)
- Export ke github repository: figures/fig_network_keyword.svg + .png, dst.

CLUSTER INTERPRETATION:
- Total clusters: [N]
- Tier 1 (Core): [N] clusters | [labels]
- Tier 2 (Emerging): [N] | [labels]
- Tier 3 (Established): [N] | [labels]
- Tier 4 (Peripheral): [N] | [labels]
- Bridge keywords: [list]
- Structural holes: [list]

SLNA INTEGRATION:
- Theme validation table (CONVERGENT / SLR-ONLY / BIB-ONLY)
- Research landscape positioning
- Convergent gaps (HIGH priority untuk Future Research)

FORWARD ARTIFACTS (→ Modul 9):
- vosviewer_parameters (→ Methods 9-parameter table)
- cluster_interpretation (→ Results subsection bibliometric)
- slna_integration (→ Results subsection SLNA + Discussion)
- outputs/figures/* (network + overlay + density, dual SVG/PNG)
- Convergent gaps (→ Future Research Agenda M9 L4 — HIGH priority)

NEXT: Manuscript Writing (Modul 9) — SLNA-specific sections:
- Methods: include 9-parameter table
- Results: subseksi "Bibliometric Cluster Analysis" + "Integrated SLNA Findings"
- Title/Abstract: "Systematic Literature Network Analysis"

Konfirmasi 2 file tersimpan + path absolut.

---

## Modul 9 — Manuscript Writing → manuscript_final  ✅ Implemented

> **Sudah diimplementasi** (L1–L10, grouped 3 gate) — lihat [AGENT.md](AGENT.md). References via Crossref, .tex Go-native. Spec rinci di bawah = referensi desain.

BRIEF AWAL CONTEXT:

INPUT UTAMA: interpretation_package (dari M8) +
             semua file dan atau figures di repo github 

OUTPUT TARGET:
- L1-L9: dokumen per section di collection manuscript
- L10: manuscript_final + file latex dan bibtex di repo github + prisma_2020_checklist + coherence_audit +
       modul9_summary

=== BAGIAN C: ATURAN MODUL 9 (BERLAKU UNTUK L1-L10) ===

Tambahan dari Aturan Global Modul 1 Section E:

A. STANDAR: PRISMA 2020 27-item + Cochrane Handbook
   (BUKAN PRISMA-ScR yang 22-item — beda standar)

B. BAHASA SLR (Jalur A vs B berbeda, ikut keputusan M8):
   JALUR A NARRATIVE:
   ✓ "synthesis indicates", "consistent finding across studies", "evidence suggests"
   ✗ "pooled effect", "d = X across studies", "overall effect size"
   JALUR B META-ANALYSIS:
   ✓ "pooled estimate", "[N] studies meta-analyzed", "I² = X%"
   ✗ Mencampur dengan vote counting tanpa kualifikasi

C. TERMINOLOGI WAJIB:
   - "Systematic review" / "Systematic literature review" (eksplisit Title/Abstract/Methods)
   - "Extraction" (BUKAN charting)
   - "Synthesis" / "Meta-analysis" sesuai jalur
   - Canonical terminology dari outputs/pico_definitions.md

D. GEOGRAPHIC HONESTY: jangan klaim "global" jika dominasi regional
E. ANTI-OVERCLAIMING: hedging tepat sesuai GRADE (HIGH/MOD/LOW/VERY LOW)
F. OUTPUT FORMAT: tulis langsung ke .md di outputs/manuscript/, no preamble,
   paragraf akademik standar
G. FOUNDATIONAL REFERENCES (CONTEXTUAL pool — cite hanya yang
   benar-benar mendukung argumen di draft, BUKAN sebagai checklist):

   SLR methodology grounding:
   - Page et al. (2021) PRISMA 2020 — reporting standard
   - Higgins et al. (2023) Cochrane Handbook — synthesis methodology
   - Booth et al. / Petticrew & Roberts — SLR generic methodology

   RoB / quality assessment tools (cite tool actually used, bukan
   semuanya):
   - RoB 2 (RCT) — Sterne et al. (2019)
   - ROBINS-I (non-randomized) — Sterne et al. (2016)
   - NOS (observational) — Wells et al.
   - AMSTAR 2 (reviews of reviews) — Shea et al. (2017)
   - MMAT (mixed methods) — Hong et al. (2018)

   GRADE (cite jika certainty assessment per outcome):
   - GRADE Working Group / Guyatt et al. seri

   Scopus coverage justification (cite di Methods Item 6 ONLY IF
   you make a coverage-comparison argument):
   - Mongeon & Paul-Hus (2016), Martín-Martín et al. (2018)

   GUIDANCE — JANGAN treat ini sebagai checklist:
   - Different reviews lean on different refs. Cite hanya yang
     supports argumen spesifik di prose.
   - JANGAN block-cite "(Page, 2021; Higgins, 2023; Cochrane, 2023)"
     tanpa argumen unik per ref — reviewer Q1 langsung lihat ini
     sebagai name-dropping.
   - Verify setiap citation lengkap before submission (Claude may
     have older metadata).

H. AI ASSISTANCE DECLARATION — LIMITED to language/readability:
   - AI use disclosed ONLY at Section 12 (compile phase), limited
     to language refinement, grammar, readability improvements
   - TIDAK ada disclosure AI di Methods Items 8/9 (Selection,
     Data collection), Items 11 (RoB), atau di Discussion/Results
   - Banyak Q1 jurnal flag analytical AI use; manuscript framing
     pakai Reviewer 1/2, Extractor 1/2 generik (workflow standar
     SLR dengan dual reviewer)

I. PROVENANCE DISCIPLINE — INTERNAL audit trail (NOT manuscript):
   - Setiap section L1-L9 di-review per-paragraf oleh peserta SEBELUM
     lanjut ke section berikutnya
   - Catat di _provenance_log per section:
     · "drafted [date]", "edited by author [date]", "approved by
       author [date]", "verified claims against artifact files [Y/N]"
   - File _provenance_log = audit trail INTERNAL peserta.
     TIDAK di-include sebagai supplementary file manuscript.
   - Author final responsibility: setiap claim, angka, sitasi,
     interpretasi adalah tanggung jawab author

J. METHODS FRAMING — REVIEWER/EXTRACTOR/RATER GENERIK:
   - Methods Item 8 (Selection process): framing "Reviewer 1" dan
     "Reviewer 2"; κ antar reviewer dilaporkan sebagai inter-reviewer
     agreement. JANGAN menyebut AI/Claude di Item 8.
   - Methods Item 9 (Data collection): framing "Extractor 1" dan
     "Extractor 2" untuk dual-extraction; κ_extract sebagai
     inter-extractor agreement. JANGAN menyebut AI/Claude di Item 9.
   - Methods Item 11 (RoB): framing "Rater 1" dan "Rater 2"; κ_rob
     sebagai inter-rater agreement. JANGAN menyebut AI/Claude di
     Item 11.
   - κ angka tetap diambil dari Sheet "Kappa" di artifact files,
     JANGAN generate atau estimate.

K. SECTION ROLE BOUNDARIES — anti-leak between manuscript sections:

   Setiap section punya peran spesifik. Konten yang sama TIDAK BOLEH
   muncul di dua section dengan framing identik.

   1. PRIOR REVIEWS (Intro 5.2 vs Discussion 3.5):
      - Intro 5.2 = "what prior reviews mapped + what gap remains
        BEFORE this review"
      - Discussion 3.5 = "how THIS review's findings dialogue with
        prior reviews"
      - JANGAN: ulang summary prior reviews dengan framing identik

   2. GAPS (Discussion 3.6 limitations vs Future Research 4.x):
      - Discussion 3.6 = current limitations + impact
      - Future Research = actionable agenda (RQ + methodology)
      - JANGAN: gap statements identik di kedua tempat

   3. NUMERICAL DATA (Methods vs Results vs Abstract):
      - Methods: rubrik + workflow (no specific N yet)
      - Results: N studies, κ values, distribusi (semua angka)
      - Abstract: highlights angka terpilih
      - JANGAN: N atau κ berbeda antar section

   4. IMPLICATIONS (Discussion 3 vs Conclusions):
      - Discussion = detail triple-track (research/practice/policy)
      - Conclusions = ringkasan 1-2 kalimat per track
      - JANGAN: paragraf identik di kedua tempat

   5. AI MENTION (across ALL sections except Section 12):
      - Manuscript text WAJIB tidak menyebut AI/Claude/LLM kecuali
        Section 12
      - Termasuk: Title, Abstract, Introduction, Methods, Results,
        Discussion, Future Research, Conclusions
      - Internal vocab yang BOCOR ke manuscript: "Pass 1/Pass 2",
        "blind prompt", "Claude session", "AI-assisted screening"

   6. INTERNAL WORKFLOW LEAK (provenance log content):
      - Konten _provenance_log (drafted dates, session IDs, model
        versions) JANGAN muncul di manuscript text atau supplementary

   7. TRAINING WORKFLOW VOICE:
      - JANGAN ada hint manuscript dari training: "modul ini",
        "outputs/manuscript folder", "as per the training"
      - Manuscript standalone — reference ke modul internal files
        HANYA di provenance log

   8. GEOGRAPHIC CLAIMS (Title/Abstract/Intro/Discussion):
      - Title geographic scope WAJIB konsisten dengan Abstract,
        Methods eligibility, Discussion 3.2 Geographic Honesty,
        Limitations
      - JANGAN: Title "global" tapi Discussion akui regional

   9. INDONESIAN CALQUE / TRANSLATED PHRASES:
      - "It is known that...", "It can be concluded...", "nampaknya"
        → "it seems that", "Many studies have..." sebagai opener
      - Replace dengan native academic English construction

   10. SLR vs ScR TERMINOLOGY DRIFT:
      - JANGAN slip "scoping review", "charting", "PCC" — manuscript
        ini SLR (PRISMA 2020 + PICO + extraction)

Konfirmasi setup + context + aturan dipegang. Selanjutnya request langkah
demi langkah L1-L10.

### LANGKAH 1: METHODS WRITING (PRISMA 2020 COMPLIANT)

Write the Methods section in academic English to manuscript/methods
Methods must be PRISMA 2020 compliant and consistent with all artifact files.

Read these files from the working folder:
- screening (PRISMA flow numbers, inter-reviewer κ for screening)
- extraction (framework + inter-extractor κ + RoB inter-rater κ)
- synthesis_results (Jalur A/B + GRADE)
- pico_definitions (PICO + scope)
- search_log (search strategy + date stamp + update policy)
- _provenance_log (internal audit trail — DO NOT surface in
  manuscript)

Structure (PRISMA 2020 items 5-19):

5. Eligibility criteria — PICO + reason codes
6. Information sources — DBs + grey lit + date of last search.
   If a Scopus coverage statistic is needed, use web search to fetch
   current figure from Elsevier's content coverage page, cite with
   access date — do NOT recall from training data.
7. Search strategy — Boolean strings full + filter + update policy
8. Selection process — GENERATE FRESH (no template paste)

   Write Item 8 in author voice. Cover these points in fresh prose:
   - Screening conducted at two stages (title/abstract, then
     full-text) by two independent reviewers (Reviewer 1 and
     Reviewer 2) applying the same PICO eligibility rubric
   - Cohen's κ between reviewers reported at each stage — READ
     actual values from screening.xlsx Sheet "Kappa" (κ_TA, κ_FT);
     DO NOT INVENT or estimate
   - Disagreements resolved by author arbitration or third-reviewer
     adjudication (state which approach used)
   - Final inclusion decisions are the responsibility of the author(s)
   - Introduce Figure 1 (PRISMA 2020 flow diagram) here, with brief
     description of stages (Identification → Screening → Eligibility
     → Included). Numerical breakdown in Results Item 16.

   DO NOT mention AI, Claude, or any LLM. AI use limited to Section
   12 (language/readability only).

9. Data collection process — GENERATE FRESH

   - Extraction conducted by two independent extractors (Extractor 1
     and Extractor 2) using a rigid a priori extraction form
   - Inter-extractor κ_extract — read from extraction.xlsx Sheet
     "Kappa"; DO NOT invent
   - Author reconciled disagreements and validated 100% of extracted
     rows against source PDFs post-reconciliation
   - State the extraction framework (TCCM/ADO/PICO from M7 L1)
   - DO NOT mention AI, Claude, or LLM

10. Data items — variabel framework
11. Study risk of bias assessment — GENERATE FRESH

   - RoB tool used (state actual tool: RoB 2 / ROBINS-I / NOS / MMAT /
     AACODS depending on study designs)
   - Two independent raters (Rater 1 and Rater 2) assessed each study
   - Inter-rater κ_rob — read from Sheet "Kappa"
   - Threshold 3-tier (low/some/high) + sensitivity analysis approach
   - DO NOT mention AI, Claude, or LLM

12. Effect measures — only if Jalur B (effect size type + rationale)
13. Synthesis methods — Jalur A DEFAULT or B UPGRADE (declare clearly).
    Cite Higgins/Cochrane Handbook only if specific synthesis approach
    is invoked (do not name-drop).
14. Reporting bias assessment — publication bias if Jalur B
15. Certainty assessment — GRADE per outcome (if applied). Cite GRADE
    Working Group only if certainty assessment is reported.

GENERAL RULES:
- Every decision + justification ("We did X because Y" — not just
  "We did X")
- All parameters replicable (tool versions, thresholds, formulas)
- DO NOT mention AI/Claude/LLM in any Methods item; that disclosure
  is limited to Section 12 (language/readability)
- DO NOT cite foundational refs (Page 2021, Higgins 2023, RoB 2,
  GRADE) as a block; cite each only where it supports a specific
  claim in the prose
- Numbers (κ values, N records) read from artifact files; do not
  invent or estimate

Length target: 1200-1800 words.
Follow global rules L0 Bagian C (especially K Section role boundaries).
After writing, append to _provenance_log:
"methods drafted [date], awaiting author per-paragraph review".



### LANGKAH 2: RESULTS WRITING (STRUKTUR FRAMEWORK TCCM/ADO)

Tulis Results section ke outputs/manuscript/results

Input: descriptive_analysis + synthesis_results +
       grade_evidence_table + figures/

Struktur per framework (TCCM/ADO/PICO sesuai M7 L1):

2.1. Evolution of the Research Field (Temporal + Descriptive)
- Narasi numerik PRISMA 2020 flow per stage (Identification, Screening,
  Eligibility, Included) + cross-reference ke Figure 1 (yang sudah
  diperkenalkan di Methods Item 8). Reviewers see numbers here;
  figure introduced earlier per convention.
- Publication year + trend
- Total studies + descriptive stats
- Geographic distribution JUJUR (sebut dominasi regional jika ada,
  mis. dari data aktual descriptive_analysis — JANGAN invent
  percentage)

2.2. Dominant Theories (T — jika TCCM)
- Teori dominan studi primer (n per teori)
- Under-utilized theories

2.3. Contexts (C)
- Geographic / Sectoral / Temporal contexts

2.4. Key Characteristics + Constructs (Ch)
- Demographic patterns | Variables | Measurement tools heterogeneity

2.5. Methodological Trends (M)
- Study design distribution | Analytical approaches | Methodological gaps

2.6. Synthesis Results
- Jalur A: thematic findings dengan consistent + contradictory patterns,
  indicative effect ranges (per studi)
- Jalur B: forest plot Figure X, pooled results, subgroup analysis
- GRADE evidence profile table

ATURAN TAMBAHAN:
1. TEMPORAL AUDIT: tidak boleh sitasi studi primer publish setelah search
   date (dari search_log). Flag + hapus jika ada.
2. CONSISTENT TERMINOLOGY: canonical dari pico_definitions.
3. FIGURES + TABLES (referensi filename SVG):
   - Figure 1: flow diagram PRISMA 2020 — **placed in Methods Item 8**
     (introduces selection process); cross-referenced from Results 2.1
   - Table 1: Characteristics of Included Studies (Results 2.x)
   - Table 2: Quality Assessment Summary (Results 2.x atau 2.5)
   - Figure 2-N: dari outputs/figures/ (descriptive viz)
   - Forest plot (jika Jalur B)
   - Table 3: GRADE evidence profile
4. NON-INTERPRETATION: present OBJECTIVELY, interpretasi → Discussion

GUARDRAILS (lihat Aturan K):
- DO NOT mention AI, Claude, LLM, Pass 1/2, blind prompt, internal
  workflow vocabulary di Results (any subsection)
- DO NOT interpret findings — pure description per RQ; interpretation
  pindah ke Discussion
- Numbers (N, %, κ, GRADE confidence) WAJIB konsisten dengan Methods
  + Abstract (akan diaudit di L10 Phase 1.E)
- DO NOT reference internal filenames (extraction, qdrantdb, dll)
- Geographic % dari descriptive_analysis, jangan invent

Length: 2000-3000 kata.
Ikuti aturan brief Modul 9 Bagian C.



### LANGKAH 3: DISCUSSION WRITING (6 SUBSEKSI WAJIB)

Tulis Discussion ke outputs/manuscript/discussion.

Input: results (untuk avoid repetisi) + interpretation_package +
       grade_evidence_table + prior_reviews_matrix (M2)

Struktur 6 subseksi WAJIB:

3.1. Summary of Findings (2-3 paragraf)
- Jawab RQ primary + secondary ringkas
- BUKAN repetisi Results — sintesis high-level dengan interpretasi

3.2. Geographic and Contextual Honesty (1-2 paragraf — DI AWAL Discussion)
- Acknowledge geographic bias JIKA ADA, dengan angka aktual dari
  descriptive_analysis (jangan invent percentage)
- Cover (in author voice, not boilerplate paste):
  · Actual dominant region + percentage
  · Structural explanation (research infrastructure, funding patterns,
    cultural focus) — write specific to topic, not generic
  · Implication for interpretation of subsequent findings
- Reframe overclaim: jika data regional, framing harus "X context"
  bukan "global". Specific framing depend on topic and region (do NOT
  paste a canned reframe).

3.3. Dialog with Existing Theory (2-3 paragraf)
- Findings yang MENDUKUNG / MENANTANG / MEMPERLUAS teori existing
  — sebut teori spesifik dengan citation ke primary studies yang
  mendukung argumen
- Teori UNDER-UTILIZED meski relevan untuk topik
- Kontribusi teoretis spesifik: apa yang baru? (write topic-specific,
  not generic)

3.4. Heterogeneity Analysis (2 paragraf)
- Mengapa effect size/findings bervariasi? (mis. "d = 0.3-0.9" — varian dari?)
- Studi kontradiktif: mengapa?
- Moderator teridentifikasi (context/design/population)

3.5. Comparison with Prior Reviews (1-2 paragraf)
- Bandingkan dengan matriks prior reviews (outputs/prior_reviews_matrix.md M2)
- Apa KONSISTEN? Apa BERBEDA? Kontribusi novelty riset ini

3.6. Limitations (3-tier dengan mitigasi per limitasi)

TIER 1 — REVIEW-LEVEL:
- Database coverage (Scopus-only?) | Geographic bias | Bahasa filter |
  Timeframe | PICO consistency caveats

TIER 2 — STUDY-LEVEL:
- Quality distribution | Measurement heterogeneity | Inaccessible ([N], [%])

TIER 3 — SYNTHESIS-LEVEL:
- Jalur A: subjectivity thematic coding
- Jalur B: publication bias residual, subgroup power
- Framework selection subjectivity

ATURAN: tiap limitasi + 1 kalimat mitigasi atau implikasi generalisasi.

ATURAN TAMBAHAN:
1. TIDAK BOLEH REPETISI RESULTS: kutip angka → sertakan interpretasi
   baru (sintesis high-level, dialog dengan teori, atau dialog dengan
   prior reviews)
2. KUTIPAN PRIOR REVIEWS wajib eksplisit di subseksi 3.5.
3. CANONICAL TERMINOLOGY konsisten.
4. HEDGING: GRADE LOW confidence → "evidence suggests" bukan "results prove".

GUARDRAILS (lihat Aturan K):
- DO NOT mention AI/Claude/LLM/Pass 1-2/internal workflow vocabulary
- DO NOT repeat Intro 5.2 prior reviews framing — Discussion 3.5 =
  "how findings dialogue" (post-findings), bukan ulang summary
- DO NOT introduce new data — semua angka harus muncul di Results
- DO NOT use Indonesian calque ("It is known that", "Many studies have")
- Triple-track implications detail di Discussion; Conclusions tetap
  ringkas (akan diaudit Phase 1.A)
- DO NOT use template paragraph paste — write topic-specific paragraphs

Length: 2000-2800 kata.
Ikuti aturan brief Modul 9 Bagian C.


### LANGKAH 4: FUTURE RESEARCH AGENDA (SUBSEKSI KHUSUS)

Tulis ke manuscript/future_research sebagai subseksi tersendiri
setelah Discussion.

Input: interpretation_package (gaps + future research items dari M8 L4) +
       prior_reviews_matrix (gaps yang prior reviews sebut)

Struktur:

4.1. Pendahuluan Agenda (1 paragraf) — justify agenda kritis, turunan findings

4.2. Matriks Prioritas (tabel):
| Priority | Timeframe | Rationale (linked to gap) | Research Question | Suggested Methodology |
| HIGH | 1-2 yr | [gap konkret] | [RQ spesifik] | [survey/qual/SLR] |
| MEDIUM | 2-3 yr + infrastruktur | [...] | [...] | [...] |
| LONG-TERM | 3+ yr | [...] | [...] | [...] |

Aturan:
- Setiap agenda HARUS trace ke gap konkret di Discussion
- Formulasikan sebagai RESEARCH QUESTION spesifik
  ❌ "Future research on emerging markets"
  ✓ "How does [construct X] operate in non-Southeast-Asian Muslim economies
     (Middle East, Africa) compared to observed Indonesia-Malaysia patterns?"
- Minimum: 3 HIGH + 2-3 MEDIUM + 1-2 LONG-TERM
- Suggested methodology eksplisit

4.3. Prioritization Rationale (2-3 paragraf)
- HIGH: mengapa addressable sekarang, data/infra tersedia, impact
- LONG-TERM: mengapa tidak bisa sekarang, apa yang perlu berkembang dulu

4.4. Methodological Advancements Needed (1-2 paragraf)
- Gap metodologis dari findings
- Rekomendasi: dataset baru, framework baru, measurement tools baru

Length: 800-1200 kata.
Ikuti aturan Modul 9 L0 Bagian C. JANGAN "more research needed" — selalu spesifik.

GUARDRAILS (lihat Aturan K):
- Gap statements DI SINI = actionable (RQ + methodology); BEDA dari
  Discussion 3.6 limitations (current scope) — Aturan K.2
- DO NOT mention AI/Claude/internal workflow
- DO NOT use generic "more research needed" framing


### LANGKAH 5: INTRODUCTION WRITING (5 SUBSEKSI WAJIB)

Tulis ke manuscript/introduction.

Input: pico_definitions + scope_justification +
       prior_reviews_matrix + research_questions (semua dari M2) +
       manuscript/results (untuk preview tune)

Struktur 5 subseksi WAJIB:

5.1. Background (2-3 paragraf)
- Field overview + importance
- Current state of knowledge (high-level + foundational works)
- Why this field matters now (SDGs, post-COVID, policy)

5.2. Review of Prior Reviews (SUBSEKSI TERSENDIRI — KRITIS)
- 3-5 prior reviews terdekat (dari outputs/prior_reviews_matrix.md M2)
- Per review: scope, methodology, key findings, limitations
- Paragraf "Sintesis Novelty": apa SUDAH dilakukan kolektif, apa BELUM,
  mengapa riset ini MENUTUP gap

Length: 400-600 kata.

5.3. Problem Statement dengan Tipe Gap (1-2 paragraf) — GENERATE FRESH

Articulate WHY a systematic review addresses the gap for THIS specific
topic. Use the gap type (A/B/C dari gap_characterization M2)
as a CONCEPTUAL FRAMING, NOT a script:

- TYPE A (fragmentation): explain HOW literature is fragmented in
  this specific field, why no prior review has integrated it, what
  systematic synthesis will reveal
- TYPE B (conflicting findings): explain WHAT specific contradictions
  exist among primary studies on this construct, why these matter,
  what systematic synthesis can reconcile
- TYPE C (no integrative framework): explain WHAT theoretical
  fragmentation exists for the author's specific concepts, why
  integration matters, what framework synthesis contributes

Write in author voice specific to the topic; do NOT paste a template
sentence. Cite Page et al. (2021) or Cochrane Handbook only if the
methodology framing explicitly relies on PRISMA 2020 here.

5.4. Scope Justification (1-2 paragraf) — pull from 
scope_justification M2, integrate dalam author voice, do NOT
paste verbatim.

5.5. Research Questions + Objectives (1 paragraf)
- Primary RQ + 3 secondary (from research_questions M2)
- Brief preview objektif (tanpa spoil findings)
- Include sentence locating review within PRISMA 2020 reporting and
  Cochrane methodology — integrated dengan author's specific
  objectives, NOT canned declaration. A reviewer should not detect
  training-template origin from section 5.5 alone.

ATURAN:
- JANGAN repetisi Discussion/Conclusion (akan diaudit L10)
- Canonical terminology konsisten
- Jangan klaim "global" jika regional (disclose di 5.4)
- Preview framework (TCCM/ADO/PICO) yang diadopsi

GUARDRAILS (lihat Aturan K):
- 5.2 Prior reviews = "what mapped + gap remains" (forward-looking);
  Discussion 3.5 nanti = "how findings dialogue" (backward-looking).
  DO NOT preview findings di Intro 5.2.
- DO NOT mention AI/Claude/LLM/internal workflow
- DO NOT use Indonesian calque ("It is known that", "Many studies have")
- DO NOT preview specific N or κ — those belong in Results

Length: 1000-1500 kata.
Ikuti aturan brief Modul 9 Bagian C.


### LANGKAH 6: CONCLUSIONS WRITING (LEAN)

Tulis ke manuscript/conclusions (3-4 paragraf, 400-600 kata).

Input: manuscript/discussion + future_research + interpretation_package

Paragraf 1 — Main Conclusions:
- Direct answer ke primary RQ
- Key evidence (brief, tidak repetisi Results)

Paragraf 2 — Theoretical Contributions:
- Apa yang riset ini tambahkan ke teori existing?
- Framework TCCM/ADO insight
- Hedging tepat per GRADE

Paragraf 3 — Practical Implications:
- Implikasi kebijakan/praktisi/policymaker
- Tetap grounded di evidence

Paragraf 4 (OPTIONAL) — Brief Forward Look:
- 1-2 kalimat → arahkan ke Future Research Agenda subseksi
- JANGAN detail future research (sudah di L4)

ATURAN:
- JANGAN repetisi Discussion substansial
- Canonical terminology konsisten
- Geographic honesty: jangan overclaim jika bias disclosed di Discussion

GUARDRAILS (lihat Aturan K):
- LEAN — Conclusions BUKAN expanded Discussion (Aturan K.4)
- DO NOT introduce new arguments or data
- DO NOT repeat Discussion sentences verbatim (audit Phase 1.A)
- DO NOT mention AI/Claude/internal workflow

Ikuti aturan brief Modul 9 Bagian C.

### LANGKAH 7: REFERENCES (FORMAT + VERIFY + TEMPORAL AUDIT + JOURNAL TIER)

⚠️ PERINGATAN: Tahap paling rawan hallucination. JANGAN minta cowork generate
referensi dari nol. Hanya format, verify, audit yang SUDAH ADA. Sebenernya solusi tercepat ambil bibtex ga sih dari crossref?

Tulis ke manuscript/references

Input: scan semua file di outputs/manuscript/ untuk in-text citations.

Target style: [APA 7th / Vancouver / Harvard / sesuai jurnal target]

Eksekusi 4 fase:

=== FASE 1: FORMAT REFERENCE LIST ===
- Format sesuai style
- Ensure completeness (author, year, title, journal, vol, pages, DOI)
- Sort alfabetis atau urutan kemunculan

=== FASE 2: WEB VERIFICATION (Scopus api,crossref api,WAJIB) ===
Per referensi, web search verifikasi:
- Author, Year, Title, Journal, Volume, Pages, DOI
- Status: ✅ VERIFIED / ⚠️ PARTIAL / ❌ NOT FOUND
Tabel hasil. NOT FOUND → peserta cek manual.

=== FASE 3: TEMPORAL AUDIT vs SEARCH DATE (dari search_log) ===
- BEFORE search date: OK (studi primer atau context)
- AFTER search date:
  · Context citation (Intro/Discussion): OK + flag
  · Studi primer: INKONSISTEN — hapus atau justify update-search

Flag referensi "publish setelah search date + di-include sebagai studi primer":
hapus atau justify (Modul 3 L4 update policy).

=== FASE 4: AUDIT FOUNDATIONAL WORKS + JOURNAL TIER ===

Web search:
1. Foundational works di bidang [topic] — sudah di-cite?
   (mis. entrepreneurship: Shane & Venkataraman 2000, Gartner 1988)
2. Distribusi journal tier — dominasi niche atau ada top-tier?
   (mis. JBV, ETP, Research Policy, Small Business Economics)

Rekomendasi: foundational absent → wajib tambah; dominasi niche → tambah
2-3 sitasi top-journal (relevan, bukan namedropping). Semua addition WAJIB
lolos verifikasi Fase 2 + temporal audit Fase 3.

In-text citation orphan check: in-text tanpa entry → flag untuk peserta resolve.



### LANGKAH 8: ABSTRACT WRITING (250-300 KATA)

Tulis ke manuscript/abstract.

Input: methods + results + discussion + interpretation_package
       (top-3 future research priorities)

Struktur 4 bagian:

Background & Objective (2-3 kalimat):
- Problem + rationale + objective + tipe gap
- Eksplisit "systematic review"

Methods (3-4 sentences) — cover these elements in author voice (NOT
canned sentences):
- Systematic review framing (PRISMA 2020 + Cochrane methodology
  jika genuinely framed that way; do NOT paste canned phrase)
- Search: databases + date range + date of search
- Framework: TCCM/ADO/PICO eksplisit
- Synthesis path: narrative thematic / meta-analysis (Jalur A/B)
- Final N included

Results (4-5 sentences):
- Actual N studies + participants (if applicable)
- 2-3 key findings utama (descriptive, hedged per GRADE)
- Geographic coverage JUJUR (state actual % from data, not invented)
- GRADE confidence ringkas

Conclusions (2-3 sentences):
- Main conclusions
- Top 1-3 future research priorities (pull from interpretation_package.md)

ELEMENTS TO ENSURE PRESENT (CONTENT requirements, not text-to-paste):
- Framework (TCCM/ADO/PICO) referenced
- Methodology framing (PRISMA 2020) integrated
- N final studies
- Geographic coverage honestly described
- Key findings + priorities present

ANTI-OVERCLAIMING:
- Hedging per GRADE
- Geographic framing must match data (regional jika regional, no
  "global" overclaim)
- NO JARGON (abstract dibaca non-specialist dulu)

ANTI-HALLUCINATION: All numbers (N, %, κ) from artifact files. Abstract
should not read as generated from template.

GUARDRAILS (lihat Aturan K):
- Numbers (N, %, GRADE) WAJIB konsisten dengan Methods + Results
  (Phase 1.E audit)
- DO NOT mention AI/Claude/LLM di Abstract
- Geographic scope = konsisten dengan Title + Methods + Discussion
  (Phase 1.J audit)
- DO NOT use Indonesian calque opener

Ikuti aturan brief Modul 9 Bagian C.


### LANGKAH 9: TITLE CREATION (3-5 ALTERNATIF)

Tulis ke manuscript/title.

Input: abstract + framework + synthesis path + geographic coverage + key findings.

Title criteria:
1. DESCRIPTIVE — jelas topik + approach
2. SPECIFIC — include key components
3. METHODOLOGY EKSPLISIT — WAJIB cantumkan "systematic review" / "systematic
   literature review" (PRISMA 2020 reporting standard)
4. CONCISE — 12-18 kata (SLNA), 10-14 kata (SLR biasa)
5. SEARCHABLE — keywords untuk discoverability
6. GEOGRAPHIC HONESTY:
   - JANGAN klaim "global" jika dominasi regional
   - Alternatif framing:
     * "Southeast Asian context with global implications"
     * "Lessons from Indonesia-Malaysia for emerging Muslim economies"
     * "Evidence from [region]: [topic]"

Anti-overclaiming checklist per candidate:
[ ] "Systematic review" eksplisit?
[ ] Geographic scope jujur?
[ ] Tidak overclaim novelty?
[ ] Approach (SLR/SLNA) eksplisit?
[ ] Framework tidak misrepresent?

Format alternatif:
- "[Topic]: A Systematic Literature Review"
- "[Topic]: A Systematic Literature Network Analysis" (jika SLNA)
- "Toward an Integrated Framework of [topic]: [approach]"
- "[Region-qualified topic]: [approach]"

OUTPUT: 3-5 alternatif. Per alternatif:
- Title candidate | Word count | Keywords | Geographic honesty PASS/FAIL |
  Rationale 2-3 kalimat | Appeal audience

Di akhir: REKOMENDASI title terbaik + justifikasi.
Ikuti aturan Modul 9 L0 Bagian C.

GUARDRAILS (lihat Aturan K):
- Title geographic claim WAJIB konsisten dengan Abstract Results,
  Methods eligibility, Discussion 3.2 Geographic Honesty (Phase 1.J)
- DO NOT klaim "global" jika data dominant regional
- DO NOT include AI platform names ("AI-assisted", "Claude-powered")
  di Title
- Title harus standalone — reader tidak boleh detect training-template
  origin dari Title saja


### LANGKAH 10: AUDIT + COMPILE FINAL + HASIL AKHIR


Eksekusi 5 fase otomatis dalam single sesi cowork.

=== FASE 1: COHERENCE AUDIT ===
Tulis ke outputs/coherence_audit (daftar issue + saran replacement).
Audit ini WAJIB lengkap — setiap subcheck harus dilaporkan PASS atau
list issue spesifik.

A. AUDIT REPETISI (Intro ↔ Discussion ↔ Conclusion):
- Identifikasi kalimat/frasa nyaris identik di ≥2 bagian
- Khusus: prior reviews matrix (Intro 5.2 vs Discussion 3.5) dibedakan
  framing (Aturan K.1)
- Khusus: gap content (Discussion 3.6 vs Future Research 4.x) tidak
  repetitif (Aturan K.2)
- Khusus: implications (Discussion 3 vs Conclusions paragraf 3) tidak
  paragraf identik (Aturan K.4)
- Saran rewrite per bagian

B. AUDIT TERMINOLOGI (canonical dari outputs/pico_definitions.md):
- Konsistensi term kanonikal end-to-end
- Cek SLR-style: "systematic review", "extraction", "synthesis"/"meta-analysis"
  sesuai jalur, "PICO"
- Cek tidak ada SLR-vs-ScR mix-up: "scoping review", "charting", "PCC"

C. AUDIT BAHASA SESUAI JALUR:
JALUR A: tidak ada "pooled effect", "d = X across studies", "overall effect"
JALUR B: tidak ada vote counting tanpa kualifikasi

D. AUDIT AI-MENTION LEAK (Jalur A — di luar Section 12):
Scan SEMUA section (Title, Abstract, Intro, Methods, Results, Discussion,
Future Research, Conclusions, References) untuk keyword berikut. Setiap
match = issue KECUALI di Section 12:
- "AI", "artificial intelligence", "LLM", "large language model"
- "Claude", "Anthropic", "ChatGPT", "GPT"
- "Pass 1", "Pass 2", "blind prompt", "AI pass", "Claude session"
- "AI-assisted screening", "AI-assisted extraction"
- "inter-pass agreement" → seharusnya "inter-reviewer/extractor/rater
  agreement"
Saran: rewrite untuk reframe sebagai Reviewer/Extractor/Rater generik.

E. AUDIT NUMERICAL CONSISTENCY (across sections):
Baca artifact files (screening.xlsx, extraction.xlsx Sheet "Kappa" +
"Summary"). Untuk setiap angka kritis berikut, cek MUNCUL KONSISTEN di
Methods + Results + Abstract:
- Total N records identified
- N records screened (T/A) + N excluded
- N reports assessed (full-text) + N excluded with reasons
- N final included
- κ_TA, κ_FT, κ_extract, κ_rob
- Geographic distribution %
- GRADE per outcome (jika applicable)
- Forest plot pooled estimate (Jalur B)
Setiap inkonsistensi numerik = critical issue. Flag + tunjukkan source
angka yang benar.

F. AUDIT INTERNAL VOCABULARY / PROVENANCE LEAK:
Scan untuk content yang seharusnya internal:
- References ke "outputs/", "Sheet \"X\"", "screening.xlsx",
  "extraction.xlsx", filenames internal
- "_provenance_log.md" content (drafted dates, session IDs)
- "modul 2 L5", "Modul 4", references ke training modul
- "draft v1", "iteration 3", "pilot batch"
Manuscript HARUS standalone.

G. AUDIT TRAINING-WORKFLOW VOICE:
Scan untuk hint manuscript dari training:
- "as per the training", "modul ini", "the workflow document"
- "outputs/manuscript folder", "the cowork session"
- "Workflow Modul 2-8", "follow Modul 9 structure"
Replace dengan author-voice standalone academic prose.

H. AUDIT INDONESIAN CALQUE (English manuscript only):
Scan untuk pattern terjemahan Indonesia → English yang awkward:
- "It is known that...", "It can be concluded...", "It seems that..."
- "Many studies have..." sebagai opener generic
- "On the other hand" overused
- "Furthermore" / "Moreover" stacked sebagai filler
- Sentence dengan struktur S-O-V dalam English
Suggest revisi ke native academic English.

I. AUDIT GEOGRAPHIC CLAIM CONSISTENCY:
Pull geographic scope claims dari:
- Title (geographic qualifier jika ada)
- Abstract Results (geographic coverage statement)
- Introduction 5.4 (scope justification geographic)
- Methods Item 5 (eligibility geographic)
- Discussion 3.2 (geographic honesty)
- Limitations 3.6 (geographic constraint jika ada)
Verify CONSISTENCY. Setiap divergensi = issue.

J. AUDIT GRADE/RoB CONSISTENCY:
Setiap claim hedge wording WAJIB sesuai dengan GRADE certainty per
outcome dari grade_evidence_table.md:
- HIGH certainty → strong language acceptable
- MODERATE → "likely", "probably"
- LOW → "may", "suggests"
- VERY LOW → "tentative", "uncertain"
Setiap mismatch hedge ↔ GRADE = issue.

=== FASE 2: PRISMA 2020 27-ITEM COMPLIANCE CHECK ===
Tulis ke outputs/prisma_2020_checklist (akan jadi supplementary material).

Format tabel: | # | Item | Section/Paragraf | Status (✓/⚠/✗) |

- Item 1 (Title) — title.md
- Item 2 (Abstract) — abstract.md
- Item 3-4 (Intro: rationale, objectives) — introduction.md
- Item 5-19 (Methods) — methods.md
- Item 20-22 (Results: study selection, characteristics, RoB, individual
  results, syntheses) — results.md
- Item 23-25 (Discussion: discussion, limitations, conclusions) — discussion.md
  + conclusions.md
- Item 26-27 (Other: registration, support, COI, data availability) —
  tambahkan section di compile

Status: ✓ COVERED / ⚠ PARTIAL / ✗ MISSING. Untuk MISSING/PARTIAL: rekomendasi fix.

Item 24a-c (Limitations, Discussion implications): wajib mencakup study-level,
review-level, missing data.

=== FASE 3: COMPILE dokumen MASTER ===
Compile semua section dari outputs/manuscript/ ke outputs/manuscript_final.

Urutan 16 section:
1. Title
2. Abstract
3. Keywords (3-5, generate dari abstract)
4. Introduction
5. Methods
6. Results
7. Discussion
8. Future Research Agenda
9. Conclusions
10. Funding [placeholder peserta]
11. Conflict of Interest [placeholder]
12. AI Assistance Declaration — LIMITED to language/readability

    Write a brief section (2-4 sentences, author voice, fresh prose)
    titled "AI Assistance Declaration" stating ONLY:
    - AI tools were used to assist with language refinement, grammar
      checking, and readability improvements of the manuscript
    - AI was NOT used for analytic decisions, screening, extraction,
      RoB rating, synthesis, or any aspect of evidence synthesis
      methodology
    - All scholarly content, methodological decisions, and final
      wording are the responsibility of the author(s)

    DO NOT:
    - Mention AI involvement in screening, extraction, RoB rating
      (those framed as Reviewer/Extractor/Rater in Methods)
    - List specific Claude version or session IDs
    - Provide per-stage AI workflow table
    - Cite ICMJE or COPE guidelines (not applicable when limited to
      language)
    - Surface outputs/_provenance_log.md as supplementary material

    Generate fresh wording in author voice. Length: ~50-100 words.
13. References
14. Figure Captions (filename SVG dari outputs/figures/)
15. Tables (dari results)
16. Supplementary list:
    - prisma_2020_checklist
    - extraction (full data, anonymized)
    - screening
    - protocol PROSPERO [URL]

Pastikan: smooth transitions, heading hierarchy konsisten (# section, ##
subseksi), figure/table callouts ke SVG filename, canonical terminology.

=== FASE 4: COMPILE .tex (SUBMISSION-READY) ===
Convert outputs/manuscript_final → outputs/manuscript_final.tex.

OPSI A — Pandoc (recommended atau ada tool golang lebih gampang):
  pandoc outputs/manuscript_final -o outputs/manuscript_final.tex \
    --reference-doc=[template jurnal jika ada] \
    --citeproc --bibliography=outputs/manuscript/references.bib

OPSI B — Python python-docx (jika Pandoc tidak ada atau ada saran):
  Generate convert_to_docx.py: parse heading hierarchy → Word styles,
  tables, figure placeholders.

OPSI C — Manual fallback: instruksi install Pandoc + run command.

Formatting: Times New Roman 12pt, double-spaced, margin 1", heading styles
built-in, references hanging indent 0.5", page numbers, line numbers continuous.

Catatan SVG: Word tidak embed SVG native — pakai PNG version dari
outputs/figures/ untuk insert (atau convert SVG→PNG/EMF dulu).

=== FASE 5: PRE-SUBMISSION CHECKLIST + HASIL AKHIR ===
Tulis ke outputs/modul9_summary.md (HASIL AKHIR + checklist):

=== MANUSCRIPT WRITING COMPLETE (SLR) ===

PER-SECTION Documents (outputs/manuscript/):
- methods, results, discussion, future_research, introduction, conclusions,
  references, abstract, title 

FINAL DELIVERABLES:
- manuscript_final (master compile)
- manuscript_final.tex (submission-ready)
- reference.bib
- prisma_2020_checklist (supplementary 27-item)
- coherence_audit (audit log)
- modul9_summary (file ini)

PRE-SUBMISSION CHECKLIST:
[ ] manuscript_final.tex generated
[ ] Word count total + abstract sesuai jurnal
[ ] Reference style sesuai jurnal
[ ] Figures (SVG submission, PNG preview): N files
[ ] Tables embedded di tex
[ ] Funding diisi (BUKAN placeholder)
[ ] Conflict of Interest diisi
[ ] AI Assistance Declaration ada
[ ] Author info + affiliations + ORCID
[ ] Cover letter (terpisah, dibuat sendiri)
[ ] PRISMA 2020 checklist (supplementary)
[ ] PROSPERO URL disclosed
[ ] extraction + screening siap supplementary (anonymized)
[ ] Ethical statement (jika applicable)

REVIEWER-PROOF ELEMENTS (SLR-specific):
- "Systematic review" eksplisit di Title, Abstract, Methods
- PICO framework end-to-end
- PRISMA 2020 flow diagram (Figure 1) — **placed in Methods Item 8**;
  numerical breakdown in Results
- Reviewer 1/2 + κ_TA, κ_FT inter-reviewer agreement (Methods Item 8)
- Extractor 1/2 + κ_extract inter-extractor agreement (Methods Item 9)
- Rater 1/2 + κ_rob inter-rater agreement (Methods Item 11)
- QA threshold 3-lapis + sensitivity analysis
- Synthesis path tegas (Jalur A atau B), tidak dicampur
- GRADE evidence per outcome
- Triple-track implications (research/practice/policy)
- Future Research Agenda matriks prioritas
- Geographic honesty (Title, Abstract, Discussion) — konsisten
  cross-section
- Foundational refs digunakan KONTEKSTUAL (cite hanya yang mendukung
  argumen spesifik); JANGAN block-cite Page+Higgins+Cochrane sekaligus
- PROSPERO registration disclosed
- AI Assistance Declaration LIMITED to language/grammar/readability
  (Q1-compliant Jalur A framing)
- TIDAK ADA mention AI/Claude/LLM di Methods Items 5-19
- _provenance_log.md = INTERNAL audit trail (BUKAN supplementary
  file manuscript)

NEXT: Submission ke jurnal target. Cover letter terpisah.

Konfirmasi semua deliverables tersimpan + path absolut + langkah manual
(insert figures Word, isi funding/COI, write cover letter).
No preamble.
