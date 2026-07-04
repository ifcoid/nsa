# GENERATEREPORT.md — Membaca MongoDB → Laporan SLR Utuh & Transparan

Dokumen ini menjelaskan **cara membaca seluruh state SLR dari MongoDB dan menyusunnya
menjadi satu laporan utuh** — dari dasar teori, protokol, alur (flow), metode, hingga hasil
dan sintesis — **tanpa ada yang tertinggal**. Setiap bagian laporan dipetakan ke **koleksi +
field Mongo yang sebenarnya** (nama field = `bson` tag di `internal/model/slr.go`), plus
artefak transparansi/audit (provenance) yang membuat laporan ini **defensible untuk Q1**.

> Prinsip: **satu sesi = satu SLR**. Hampir seluruh laporan berasal dari **satu dokumen**
> di `slr_sessions`. Keputusan per-paper ada di `slr_screening` (screening) dan
> `slr_extraction` (ekstraksi). Angka PRISMA **dihitung ulang** dari `slr_screening` (bukan
> disimpan) agar tak pernah berbeda dari narasi.

---

## 0. Prasyarat & koneksi (read-only)

Kredensial ada di `/home/adb/awangga/.env` (JANGAN dibocorkan; redact di log):

- `MONGO_URI` — string koneksi.
- `DB_NAME` — default `slr_agentic_db`.

Akses read-only, mis. via `mongosh`:

```bash
set +x
export $(grep -E '^(MONGO_URI|DB_NAME)=' /home/adb/awangga/.env | xargs)
mongosh "$MONGO_URI" --quiet --eval "db.getSiblingDB('${DB_NAME:-slr_agentic_db}').slr_sessions.countDocuments()"
```

Untuk satu sesi tertentu (ID = nama topik/ObjectId yang dipakai sebagai `_id`):

```js
use slr_agentic_db
const SID = "<session_id>";          // = slr_sessions._id
const S  = db.slr_sessions.findOne({_id: SID});
```

> Semua contoh di bawah memakai `SID` dan `S` ini. Field bertag `omitempty` **tidak muncul**
> di dokumen bila bernilai kosong/nil — ketiadaan field = tahap itu belum dijalankan
> (lihat §11 "Gotcha").

---

## 1. Topologi koleksi (apa ada di mana)

| Koleksi | Isi | Granularitas |
|---|---|---|
| **`slr_sessions`** | Seluruh artefak per-modul M1–M9 (teori, protokol, log, ringkasan, manuskrip) | 1 dok / sesi |
| **`slr_screening`** | Keputusan screening per-paper (judul/abstrak M5 + full-text M6) | 1 dok / paper |
| **`slr_extraction`** | Hasil ekstraksi data + QA per-paper (M7) | 1 dok / paper |
| `slr_papers` / `slr_papers_post_dedup` | Korpus hasil pencarian (M3) sebelum/sesudah dedup (M4) | 1 dok / record |
| `llm_providers`, `llm_roles` | Konfigurasi LLM (provider, model default, role routing) | konfig |
| `github_config`, `embed_config`, `scopus_config` | Konfigurasi pendukung | konfig |

Korpus full-text vektor (BGE-M3) ada di **Qdrant** (`scientific_articles`, global lintas-sesi),
bukan Mongo — relevan untuk verifikasi sumber, bukan untuk hitungan laporan.

Helper repo (di `internal/repository/mongo.go`): `GetSessionCollection()` → `slr_sessions`,
`GetScreeningCollection()` → `slr_screening`, `GetExtractionCollection()` → `slr_extraction`.

---

## 2. Tulang punggung: dokumen sesi & state-machine

Field kontrol di `slr_sessions`:

- `_id`, `topic`, `status` (state-machine M1–M9, mis. `M7_STEP2_WAITING_APPROVAL`),
  `feedback`, `system_error`, `updated_at`.
- `manuscript_lang` (`id`/`en`), `rescreen_pending` (artefak hilir stale setelah mundur),
  `xai_log` (jejak panggilan LLM lintas-modul, lihat §10).

`status` memberi tahu **sejauh mana** SLR berjalan. Modul-N selesai bila artefak `ModulN…`
dan/atau status sudah melewati tahap itu.

---

## 3. Pemetaan MODUL → bagian laporan → field Mongo

Urut sesuai alur pipeline. Untuk tiap modul: **apa yang dilaporkan** + **field sumber** di
`slr_sessions` (kecuali disebut lain).

### M1 — Fondasi / dasar teori & topik
- **Background / latar teori, justifikasi topik:** `foundation` (`FoundationBriefing`),
  `suggested_topics[]`, `selected_topic` (Name, Gap, Type, TypeReason, Evidence, Importance).
- Laporan: bagian **Introduction/Background** + pernyataan **gap & urgensi**.

### M2 — Prior reviews, PICO, RQ, scope (PROTOKOL inti)
- **Posisi terhadap review terdahulu (novelty):** `prior_reviews_matrix.reviews[]`
  (author_year, scope, methodology, key_findings, limitations, **selisih**, synthesis_novelty,
  **verification** = UNVERIFIED/VERIFIED) + `prior_reviews_matrix.search_guidance`.
- **PICO / kriteria:** `pico_definitions` (canonical term, P/I/C/O), `scope_filters`,
  `scope_justifications[]`.
- **Research Questions:** `research_questions[]` (+ traceability ke prior reviews).
- **Kelayakan (FINER):** `finer_novelty_check`. Ringkasan: `modul2_summary`.
- Laporan: **Review Protocol** (PICO, RQ, kriteria a priori), **Related Work**.

### M3 — Strategi pencarian (METODE: search)
- **Database & justifikasi:** `database_selection`.
- **Keyword development:** `keywords`. **Search string final (per database):** `search_string`.
- **Log eksekusi pencarian (jumlah per sumber):** `search_log`. Ringkasan: `modul3_summary`.
- Korpus mentah: koleksi `slr_papers`. Laporan: **Methods → Search Strategy** (reproducible).

### M4 — Identifikasi & dedup (METODE: identification)
- **Audit kualitas + dedup (records identified, duplicates removed):** `data_mining_log`
  (`QualityAudit.TotalRecords`, `Dedup.TotalDuplicates`). Korpus bersih: `slr_papers_post_dedup`.
- **Setup screening (reason codes, kriteria operasional):** `screening_setup`. Ringkasan: `modul4_summary`.
- Laporan: kotak **Identification** pada PRISMA + Methods.

### M5 — Screening judul/abstrak (METODE + FLOW)
- **Briefing screener (instruksi + validation gap):** `screener_briefing`.
- **Kalibrasi dual-reviewer (kappa, sampel):** `kalibrasi_log[]`.
- **Perspektif R1/R2:** `reviewer1_perspectives[]`, `reviewer2_perspectives[]`.
  Per-paper keputusan ada di `slr_screening` (lihat §4).
- **Log batch screening:** `screening_results_log[]`. **Tabel eksklusi:** `exclusion_table`.
- **Audit konsistensi PICO (false-include):** `pico_audit_log` (Coverage, Action, Slipped[]),
  aturan scope yang diedit peneliti: `audit_scope_rules`. Ringkasan: `modul5_summary`.
- Laporan: **Screening** PRISMA + reliabilitas antar-penilai (kappa).

### M6 — Akuisisi & screening full-text (METODE + FLOW)
- **Akuisisi PDF (retrieved / inaccessible):** `acquisition_log`.
- **Screening full-text dual-reviewer:** `fulltext_screening_log[]`, `fulltext_kappa`.
  Per-paper di `slr_screening` (`*_Decision_Full`, lihat §4).
- **Dampak paper tak-terakses:** `inaccessible_impact`. **Kesiapan ekstraksi:** `extraction_readiness`.
- **Audit PICO final:** `final_pico_audit_md`, `final_pico_audit_ok`. Ringkasan: `modul6_summary`.
- Laporan: kotak **Retrieval/Eligibility** PRISMA + alasan eksklusi full-text.

### M7 — Ekstraksi data + QA (HASIL mentah + kualitas)
- **Protokol/framework ekstraksi:** `framework_selection` (Framework, Justification,
  Columns[]{Key,Category,Desc}, ModelUsed). **Stabil** — lihat aturan validitas di CLAUDE.md.
- **Koreksi include/exclude HITL (audit deviasi):** `screening_corrections[]`
  (paper, From→To, Reason, At). Wajib dilaporkan sebagai deviasi (lihat §9 PRISMA).
- **Log ekstraksi + verifikasi (dual-rater):** `extraction_log` (TotalExtracted,
  **VerifiedSample**, **DisagreementRate**, AmbiguousCount, NRNote, FailedCount, ModelExtraction,
  ModelRefineProtocol). Data per-paper di `slr_extraction` (lihat §5).
- **QA tool/threshold + kalibrasi + sensitivitas:** `qa_threshold_justification`,
  `qa_calibration`, `sensitivity_analysis`. **Persiapan sintesis:** `synthesis_prep`.
  **Graph (Neo4j) summary:** `graph_extraction_summary`. Ringkasan: `modul7_summary`.
- Laporan: **Data extraction & quality appraisal** (Methods) + tabel ekstraksi (Results).

### M8 — Analisis & sintesis (HASIL + GRADE)
- **Analisis deskriptif + heterogenitas:** `descriptive_analysis`.
- **Keputusan jalur sintesis (naratif vs meta-analisis):** `synthesis_path_decision`.
- **Hasil sintesis (markdown + forest plot script):** `synthesis_results`.
- **GRADE certainty + robustness:** `grade_evidence_table`.
- **Paket interpretasi:** `interpretation_package`. Ringkasan: `modul8_summary`.

### M8B — Bibliometrik / SLNA (opsional)
- `bibliometric_data`, `vosviewer_parameters`, `bibliometric_input`,
  `cluster_interpretation`, `slna_integration`, `modul_bibliometric_summary`.

### M9 — Manuskrip + PRISMA (LAPORAN FINAL)
- **Manuskrip per-section:** `manuscript` (methods, results, discussion, future_research,
  introduction, conclusions, abstract, title, **prisma_flow** [teks], references, .tex/.bib).
- **PRISMA flow:** **DIHITUNG ULANG** dari `slr_screening` (deterministik) — bukan field
  tersimpan; lihat §9. Jejak koreksi (§ `screening_corrections`) otomatis disisipkan ke
  narasi Methods/Results sebagai deviasi protokol.

Kriteria final: `inclusion_criteria[]`, `exclusion_criteria[]`.

---

## 4. `slr_screening` — keputusan per-paper (sumber kebenaran flow)

Satu dokumen per paper. Field kunci (apa adanya, dipakai modul):

- Metadata: `Title`, `Abstract`, `Keywords`, `DOI`, `Journal`, `Year`, `Authors`, `Article_Type`.
- **Abstrak (M5):** `Screener_1_Decision`, `Screener_2_Decision` (INCLUDE/EXCLUDE/UNCERTAIN),
  `Final_Decision`, `Screener_1_Reason_Code`, `Screener_1_Notes`.
- **Full-text (M6):** `Screener_1_Decision_Full`, `Screener_2_Decision_Full`,
  `Final_Decision_Full`, `Screener_1_Reason_Code_Full`, `Conflict_Resolution_Full`.
- **Akuisisi:** `full_text_retrieved` (vektor ada di Qdrant), `inaccessible`,
  `full_text_location`, `download_url`, `documentation_inaccessible`.

**Keputusan FINAL full-text** (logika `finalFullDecision`): `Final_Decision_Full` bila ada;
jika kosong → INCLUDE bila kedua `*_Decision_Full` = INCLUDE, EXCLUDE bila keduanya EXCLUDE,
selain itu UNCERTAIN. **Paper masuk ekstraksi** bila: lolos abstrak **DAN** `full_text_retrieved`
**DAN** keputusan full-text final = INCLUDE.

Query contoh — hitung included final:
```js
db.slr_screening.find({session_id: SID}).toArray().filter(p => {
  const ab = p.Final_Decision==="INCLUDE" || (!p.Final_Decision && p.Screener_1_Decision==="INCLUDE");
  const ftd = p.Final_Decision_Full || ((p.Screener_1_Decision_Full==="INCLUDE"&&p.Screener_2_Decision_Full==="INCLUDE")?"INCLUDE":"");
  return ab && p.full_text_retrieved && ftd==="INCLUDE";
}).length;
```

---

## 5. `slr_extraction` — data terekstrak + QA per-paper (HASIL)

Satu dokumen per paper INCLUDE. Field kunci:

- Identitas: `session_id`, `paper_id`, `Title`, `Author`, `Year`, `Journal`, `DOI`.
- **Ekstraksi:** `extracted` (bool), `fields[]`{`key`,`value`,`evidence`,`status`
  (REPORTED/NOT_REPORTED/AMBIGUOUS/INFERRED)}, `key_findings`, `qa_red_flags`, `ambiguous[]`,
  `coverage` (COMPLETE/PARTIAL/INCOMPLETE/EMPTY_RESULT/NO_FULLTEXT_RAG/ERROR), `nr_count`,
  `model_extraction` (provider+model, xAI), `enriched_from` (mis. crossref).
- **Verifikasi (Reviewer 2):** `verified`, `verify_disagree`, `verify_notes`.
- **QA appraisal:** `qa_rated`, `qa_total_score`, `qa_final_category`,
  `qa_r1_*` / `qa_r2_*` (score, category, reasoning, evidence, model), `graph_extracted`.

**Setiap nilai membawa `evidence`** (kutipan + section) = transparansi/xAI: bisa ditelusuri
ke teks asli. Status `INFERRED` = simpulan AI yang **wajib diverifikasi manusia**.

Tabel ekstraksi (Results) = pivot dari `fields[]` semua paper: baris = paper, kolom =
`framework_selection.columns[].key`.

---

## 6. Cara MENYUSUN laporan utuh (urutan & sumber)

| Bagian laporan (struktur SLR/PRISMA) | Sumber utama |
|---|---|
| **Title / Abstract** | `manuscript.title`, `manuscript.abstract` |
| **Background / Introduction** (teori, gap) | `selected_topic`, `foundation`, `manuscript.introduction` |
| **Related work / Prior reviews** | `prior_reviews_matrix.reviews[]` (+ verification) |
| **Objectives / RQ** | `research_questions[]` |
| **Methods → Protocol** (PICO, kriteria) | `pico_definitions`, `scope_*`, `inclusion/exclusion_criteria` |
| **Methods → Search** | `database_selection`, `keywords`, `search_string`, `search_log` |
| **Methods → Screening** (kappa) | `kalibrasi_log`, `fulltext_kappa`, `screening_setup` |
| **Methods → Extraction & QA** | `framework_selection`, `extraction_log`, `qa_threshold_justification`, `qa_calibration` |
| **PRISMA Figure 1 (flow)** | **dihitung ulang** dari `slr_screening` (§9) |
| **Results → Study selection** | PRISMA counts + `screening_corrections` (deviasi) |
| **Results → Study characteristics / data** | `slr_extraction.fields[]` (pivot), `descriptive_analysis` |
| **Results → Synthesis** | `synthesis_path_decision`, `synthesis_results` |
| **Results → Certainty (GRADE)** | `grade_evidence_table` |
| **Discussion / Conclusions / Future** | `manuscript.{discussion,conclusions,future_research}`, `interpretation_package` |
| **Bibliometrik (opsional)** | `bibliometric_data`, `cluster_interpretation`, `slna_integration` |
| **References** | `manuscript` (.bib dari Crossref) |

Dua lapis kebenaran: **(a) narasi** = `manuscript.*` (ditulis LLM, role Brain); **(b)
ground-truth** = recompute dari koleksi (`slr_screening`/`slr_extraction`). Laporan transparan
**menyertakan keduanya** dan memastikan keduanya konsisten (angka PRISMA dari ground-truth).

---

## 7. Metode pengumpulan & pemrosesan (yang WAJIB dilaporkan)

- **Dual-reviewer + arbiter** (M5/M6): dua penilai independen + Supervisor untuk konflik;
  reliabilitas = `fulltext_kappa` / `kalibrasi_log`.
- **Atribusi model (xAI):** setiap output AI membawa **provider + nama model asli**
  (mis. `extraction_log.model_extraction`, `framework_selection.model_used`,
  `slr_extraction.qa_r1_model`). Laporkan model mana mengerjakan apa.
- **Verifikasi ekstraksi 20%** (Reviewer 2): `extraction_log.{VerifiedSample,DisagreementRate}`.
  ⚠ Bila `VerifiedSample = 0` padahal `TotalExtracted > 0` → verifikasi **GAGAL** (mis. model
  terkunci/404); `DisagreementRate 0%` **bukan** valid (lihat `NRNote`). Laporkan apa adanya.
- **GRADE** untuk certainty of evidence: `grade_evidence_table`.

---

## 8. Transparansi penuh — apa yang membuat laporan ini auditable

1. **Provenance per nilai:** `slr_extraction.fields[].evidence` (kutipan + section).
2. **Atribusi model** di setiap artefak AI (provider + model).
3. **Jejak LLM lintas-modul:** `slr_sessions.xai_log[]` (prompt/role/model per langkah).
4. **Audit koreksi keputusan:** `screening_corrections[]` (apa diubah, dari→ke, **alasan**, kapan).
5. **Status verifikasi prior-review:** `prior_reviews_matrix.reviews[].verification`.
6. **Konsistensi aritmetika PRISMA:** warning bila flow tak menutup (§9).

Tidak ada langkah AI yang "tak terlihat": semua tersimpan & bisa diekspor.

---

## 9. PRISMA flow — DIHITUNG ULANG, bukan disimpan

Angka PRISMA **tidak** disimpan sebagai field; M9 menghitungnya ulang dari `slr_screening`
(+ `data_mining_log` untuk identified/duplicates) lewat `computePrismaFlow`/
`countPrismaFromPapers` (`internal/modules/m9_prisma.go`). Komponen:
Identified, DuplicatesRemoved, Screened, ExcludedTA, UncertainTA, Sought, NotRetrieved,
Assessed, ExcludedFT (+ reasons), UncertainFT, Included, + `Warnings` (aritmetika tak menutup).

Karena dihitung ulang dari keputusan FINAL, **koreksi include/exclude otomatis tercermin** di
angka. Jejak `screening_corrections` disisipkan sebagai **catatan deviasi protokol** ke narasi
Methods/Results (`prismaCorrectionsNote`). Untuk laporan: **regenerasi PRISMA dari DB**, jangan
salin angka lama.

---

## 10. Resep ekstraksi cepat satu sesi → bahan laporan

```js
use slr_agentic_db
const SID = "<session_id>";
const S = db.slr_sessions.findOne({_id: SID});

// Inti protokol
printjson({status:S.status, pico:S.pico_definitions, rq:S.research_questions,
           inc:S.inclusion_criteria, exc:S.exclusion_criteria});
// Pencarian
printjson({db:S.database_selection, keywords:S.keywords, search:S.search_string, log:S.search_log});
// Protokol ekstraksi + ringkasan QA
printjson({framework:S.framework_selection, extlog:S.extraction_log,
           qa:S.qa_threshold_justification, kappa:S.fulltext_kappa});
// Sintesis + GRADE + manuskrip
printjson({path:S.synthesis_path_decision, synth:S.synthesis_results,
           grade:S.grade_evidence_table, manuscript:S.manuscript});
// Audit/transparansi
printjson({corrections:S.screening_corrections, prior:S.prior_reviews_matrix, xai:(S.xai_log||[]).length});

// Per-paper
db.slr_screening.countDocuments({session_id: SID});
db.slr_extraction.find({session_id: SID, extracted:true}).toArray().length;
```

Ekspor mentah satu sesi (untuk lampiran/replikasi):
```bash
mongoexport --uri "$MONGO_URI" --db "${DB_NAME:-slr_agentic_db}" \
  --collection slr_sessions --query "{\"_id\":\"$SID\"}" --jsonArray --out session.json
mongoexport --uri "$MONGO_URI" --db "${DB_NAME:-slr_agentic_db}" \
  --collection slr_screening --query "{\"session_id\":\"$SID\"}" --jsonArray --out screening.json
mongoexport --uri "$MONGO_URI" --db "${DB_NAME:-slr_agentic_db}" \
  --collection slr_extraction --query "{\"session_id\":\"$SID\"}" --jsonArray --out extraction.json
```

---

## 11. Gotcha & catatan validitas (jangan keliru baca)

- **`omitempty`:** field kosong/false/nil **TIDAK muncul** di dokumen. Ketiadaan field ≠ error
  — bisa berarti tahapnya belum jalan, atau nilai memang zero. Lihat juga gotcha `$set`/`omitempty`
  di CLAUDE.md sebelum menyimpulkan "kok kosong".
- **Sumber kebenaran:** untuk angka, **recompute dari `slr_screening`/`slr_extraction`**, jangan
  hanya percaya teks `manuscript.*` (narasi LLM bisa keliru menghitung; angka harus dari DB).
- **Protokol stabil:** `framework_selection` tidak boleh "berubah sendiri" antar-run (validitas
  SLR; lihat CLAUDE.md "Validitas metodologi"). Bila berbeda, periksa apakah ada amendemen
  terdokumentasi (ResetModul7) atau revisi framework eksplisit.
- **Verifikasi gagal vs lolos:** cek `extraction_log.VerifiedSample`/`NRNote` sebelum mengklaim
  QA dual-rater berjalan (0 verified = tidak berjalan, sering karena model verifier terkunci/404).
- **Multi-tenant:** kriteria/aturan/ambang berasal dari DATA sesi (mis. `pico_definitions`,
  `audit_scope_rules`) — laporkan sebagaimana tersimpan di sesi, jangan asumsikan default global.
- **Qdrant terpisah & global:** `full_text_retrieved=true` berarti vektor ada di Qdrant
  (`scientific_articles`, lintas-sesi); pencocokan via DOI/title-similarity.

---

## 12. Checklist "tidak ada yang tertinggal"

- [ ] Background/gap (`selected_topic`, `foundation`) — teori.
- [ ] Prior reviews + novelty (`prior_reviews_matrix`).
- [ ] PICO + RQ + kriteria (`pico_definitions`, `research_questions`, inc/exc).
- [ ] Search strategy reproducible (`database_selection`, `keywords`, `search_string`, `search_log`).
- [ ] Identification + dedup (`data_mining_log`).
- [ ] Screening + kappa (`kalibrasi_log`, `fulltext_kappa`) + keputusan per-paper (`slr_screening`).
- [ ] PRISMA flow **dihitung ulang dari DB** (§9) + deviasi (`screening_corrections`).
- [ ] Protokol ekstraksi (`framework_selection`) + data per-paper (`slr_extraction.fields`).
- [ ] QA/verifikasi (`extraction_log`, `qa_*`) + GRADE (`grade_evidence_table`).
- [ ] Sintesis (`synthesis_path_decision`, `synthesis_results`) + (opsional) bibliometrik.
- [ ] Manuskrip final (`manuscript`) konsisten dengan angka ground-truth.
- [ ] Transparansi: provenance (`evidence`), atribusi model, `xai_log`, audit koreksi.

Bila semua kotak terisi dari DB, laporannya **utuh, reproducible, dan transparan**.

---

## Field BARU — Modul 9 (xAI provenance) & Modul 10 (Audit & Defensibility Gate)

> Ditambahkan setelah revisi M9/M10. Cowork-LLM yang regen dari DB WAJIB memakai
> field ini agar laporan/manuskrip memuat bukti xAI + audit pra-submisi terbaru.

### `slr_sessions.manuscript` — provenance penulisan (xAI)
| Field (bson) | Tipe | Isi |
|---|---|---|
| `model_used` | string | Nama model Brain penulis section (atribusi xAI, mis. `groq (llama-3.3-70b)`) |
| `claim_verifications` | array | Bukti triangulasi neuro-symbolic per klaim manuskrip |

`claim_verifications[]` (satu per klaim):
| Field | Tipe | Isi |
|---|---|---|
| `section` | string | Section manuskrip (Methods/Results/…) |
| `claim` | string | Teks klaim |
| `citation_key` | string | \cite key yang dirujuk |
| `qdrant_verified` | bool | Klaim cocok semantik ke DOI tersitasi (Qdrant) |
| `neo4j_verified` | bool | Didukung relasi di knowledge graph (Neo4j) |
| `mongo_verified` | bool | Cocok dengan `key_findings` ekstraksi (MongoDB) |
| `sources` | int | Jumlah sumber cocok; **≥2 = terverifikasi** |

→ Untuk *Results/Discussion*: kutip HANYA klaim `sources>=2`; laporkan yang `<2` sebagai keterbatasan (jujur). Untuk supplementary: sertakan ringkasan `n terverifikasi / total`.

### `slr_sessions.audit_report` — hasil Modul 10 (audit pra-submisi + artefak)
| Field (bson) | Tipe | Isi |
|---|---|---|
| `checks` | array | Cek simbolik deterministik (lihat di bawah) |
| `verdict` | string | `READY` / `READY_WITH_WARNINGS` / `NOT_READY` |
| `summary` | string | Ringkasan verdict |
| `pass_count`/`warn_count`/`fail_count` | int | Rekap status cek |
| `generated_at` | string (RFC3339) | Waktu audit |
| `protocol_markdown` | string | **Protokol a-priori gaya PROSPERO** (dapat didaftarkan) |
| `repro_package_markdown` | string | **Paket Reproducibility** (PRISMA-S + κ + provenance + Data Availability/Zenodo) |
| `attested_by` / `attested_at` | string | Jejak atestasi peneliti (HITL) |

`checks[]`: `{ id, category (PRISMA/Reliabilitas/Kelengkapan/Pelaporan/Integritas), name, status (PASS/WARN/FAIL), detail, fix }`.

→ `protocol_markdown` & `repro_package_markdown` adalah artefak SIAP-PAKAI (jangan digenerate ulang; ekspor apa adanya untuk Zenodo/PROSPERO). `audit_report.verdict` menandakan kesiapan submit.

### Pernyataan AI (WAJIB akurat — COPE/Elsevier)
Sistem memakai AI sebagai **decision-support** (skrining/ekstraksi/appraisal/sintesis) dengan **verifikasi manusia (HITL)** + **Cohen's κ** + neuro-symbolic. JANGAN menulis "AI tidak dipakai untuk analisis" (keliru). Deskripsikan peran AI + kendali manusia secara transparan di Methods + AI Assistance Declaration.

### Data Availability / arsip
Deposit `protocol_markdown` + `repro_package_markdown` + laporan + manuskrip ke **Zenodo** (dapat DOI) & daftarkan protokol di **PROSPERO/OSF**; sitasi DOI di *Data Availability Statement*.
