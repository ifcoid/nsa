# GENERATEARTIKEL.md — Membaca MongoDB → Menulis Artikel Q1 Elsevier (siap submit)

Dokumen ini adalah **panduan untuk sesi lain** (Claude/penulis) yang **HANYA diberi akses
MongoDB** dan ditugaskan **menulis naskah artikel ilmiah** dari hasil sistem SLR ini — naskah
yang **lolos peer-review jurnal Q1 Elsevier** (mis. *Information & Management*, *IJIM*,
*Computers & Education*, *Government Information Quarterly*, dll — sesuaikan dengan topik sesi).

> **Baca `GENERATEREPORT.md` LEBIH DAHULU.** Dokumen itu adalah **peta field kanonik**
> (koleksi + `bson` tag di `internal/model/slr.go`) untuk **mengambil seluruh state SLR dari
> Mongo tanpa ada yang tertinggal**. GENERATEARTIKEL.md **tidak mengulang** peta itu; ia
> memakainya sebagai sumber data, lalu fokus pada hal yang TIDAK dibahas report:
> **cara menulis** naskah yang (a) etis, (b) tanpa jejak-AI, (c) diperkuat teori pondasi,
> (d) lolos audit Q1.

**Report ≠ Artikel.** `GENERATEREPORT.md` menghasilkan *dokumen transparansi* (semua state
di-dump apa adanya, defensible untuk audit). GENERATEARTIKEL.md menghasilkan *naskah publikasi*:
selektif, ber-argumen, ber-narasi manusia, ber-teori, dan tunduk pada standar pelaporan jurnal.
**Jangan menyerahkan report sebagai artikel, dan jangan menyalin `manuscript.*` mentah sebagai
artikel** (lihat §5 — itu tulisan LLM = risiko AI-tell + etika).

---

## 0. Prasyarat & koneksi (read-only)

Sama seperti `GENERATEREPORT.md §0`. Kredensial di `/home/adb/awangga/.env` (JANGAN dibocorkan;
`set +x` sebelum export, redact di log):

```bash
set +x
export $(grep -E '^(MONGO_URI|DB_NAME)=' /home/adb/awangga/.env | xargs)
mongosh "$MONGO_URI" --quiet --eval \
  "db.getSiblingDB('${DB_NAME:-slr_agentic_db}').slr_sessions.find({}, {_id:1, topic:1, status:1}).toArray()"
```

Pilih **satu sesi** (`_id` = `SID`). Satu sesi = satu SLR = satu artikel.

Akses **read-only**. **JANGAN pernah menulis/mengedit Mongo** (koreksi data = lewat UI/HITL,
lihat CLAUDE.md "Self-heal, BUKAN edit DB manual"). Tugas Anda hanya **membaca → menulis .md/.tex**.

---

## 1. Prinsip menyeluruh: ambil DATA utuh, tulis naskah SELEKTIF

1. **Tarik SEMUA state dulu** (jangan menulis dari ingatan sebagian). Pakai resep pull di §2.
   Kelengkapan pengambilan data = syarat mutlak; lihat checklist `GENERATEREPORT.md §12`.
2. **Angka = ground-truth dari koleksi, BUKAN dari `manuscript.*`.** PRISMA, N included,
   kappa, jumlah studi per tema **DIHITUNG ULANG** dari `slr_screening`/`slr_extraction`
   (`GENERATEREPORT.md §9`). Narasi LLM boleh keliru berhitung; DB tidak.
3. **Naskah menyeleksi, report tidak.** Artikel Q1 punya batas kata & alur argumen. Anda
   memilih apa yang naik ke narasi (yang menjawab RQ + memperkuat klaim) dan menaruh sisanya
   ke **Supplementary/Appendix** (protokol penuh, tabel ekstraksi lengkap, daftar studi).
4. **Tiap klaim dapat ditelusuri** ke `evidence` (kutipan+section) di `slr_extraction.fields[]`
   atau ke keputusan di `slr_screening`. Tidak ada kalimat hasil tanpa jangkar data.

---

## 2. Resep pull: ambil struktur data secara UTUH (satu sesi)

Jalankan dan simpan ke file kerja (lampiran/replikasi + bahan tulis). Peta field per bagian
ada di `GENERATEREPORT.md §3–§5`.

```bash
set +x
export $(grep -E '^(MONGO_URI|DB_NAME)=' /home/adb/awangga/.env | xargs)
SID="<session_id>"; DB="${DB_NAME:-slr_agentic_db}"
mongoexport --uri "$MONGO_URI" --db "$DB" --collection slr_sessions \
  --query "{\"_id\":\"$SID\"}" --jsonArray --out session.json
mongoexport --uri "$MONGO_URI" --db "$DB" --collection slr_screening \
  --query "{\"session_id\":\"$SID\"}" --jsonArray --out screening.json
mongoexport --uri "$MONGO_URI" --db "$DB" --collection slr_extraction \
  --query "{\"session_id\":\"$SID\"}" --jsonArray --out extraction.json
# opsional: korpus pra/pasca-dedup untuk angka Identification
mongoexport --uri "$MONGO_URI" --db "$DB" --collection slr_papers_post_dedup \
  --query "{\"session_id\":\"$SID\"}" --jsonArray --out papers_dedup.json
```

Verifikasi kelengkapan (tiap tahap M1–M9 punya artefaknya; field `omitempty` yang hilang =
tahap belum jalan — lihat gotcha `GENERATEREPORT.md §11`):

```js
const S = db.slr_sessions.findOne({_id: SID});
["foundation","selected_topic","prior_reviews_matrix","pico_definitions","research_questions",
 "database_selection","keywords","search_string","search_log","data_mining_log",
 "kalibrasi_log","fulltext_kappa","framework_selection","extraction_log",
 "qa_threshold_justification","synthesis_path_decision","synthesis_results",
 "grade_evidence_table","interpretation_package","manuscript"
].forEach(k => print((S[k]?"✓":"✗ MISSING")+"  "+k));
```

**Jika ada `✗ MISSING` pada tahap inti (PICO, RQ, search, synthesis, grade)** → SLR belum
matang; jangan paksakan menulis artikel. Laporkan tahap yang kurang, jangan mengarang isinya.

---

## 3. Kerangka artikel Q1 (IMRaD + PRISMA 2020) → sumber Mongo

Struktur naskah dan **dari mana narasinya dibangun** (peta detail: `GENERATEREPORT.md §6`).
Kolom "peran teori" menandai bagian yang WAJIB berpijak pada teori pondasi (§4).

| Bagian artikel | Sumber Mongo (bahan) | Peran teori |
|---|---|---|
| **Title** | `selected_topic.name`, `pico_definitions.canonical_term` | — |
| **Abstract** (terstruktur: Background/Objective/Methods/Results/Conclusion) | recompute + `synthesis_results`, `grade_evidence_table` | — |
| **1. Introduction** | `foundation.theory_markdown`, `selected_topic` (Gap, Importance), `prior_reviews_matrix` | **WAJIB**: kerangka teori + gap teoretis |
| **2. Related work / Background** | `prior_reviews_matrix.reviews[]` (selisih, synthesis_novelty), `foundation` | **WAJIB**: posisikan vs teori & review lain |
| **RQ / Objectives** | `research_questions[]` (+ traceability) | hubungkan RQ ke konstruk teori |
| **3. Methods → Protocol** | `pico_definitions`, `scope_*`, `inclusion/exclusion_criteria` | — (a priori, lihat §5 anti-HARKing) |
| **3. Methods → Search** | `database_selection`, `keywords`, `search_string`, `search_log` | — (reproducible) |
| **3. Methods → Selection & appraisal** | `kalibrasi_log`, `fulltext_kappa`, `framework_selection`, `qa_threshold_justification`, `qa_calibration` | — |
| **3. Methods → AI-assisted disclosure** | `xai_log`, `*.model_used`/`model_extraction` | — (etika, §5) |
| **PRISMA Fig.1 (flow)** | **DIHITUNG ULANG** dari `slr_screening` (`GENERATEREPORT.md §9`) | — |
| **4. Results → Study selection** | PRISMA counts + `screening_corrections` (deviasi) | — |
| **4. Results → Study characteristics** | `slr_extraction.fields[]` (pivot), `descriptive_analysis`, `bibliometric_*` | — |
| **4. Results → Synthesis (per RQ/tema)** | `synthesis_path_decision`, `synthesis_results`, `slr_extraction.key_findings` | kaitkan tema ke konstruk teori |
| **4. Results → Certainty (GRADE)** | `grade_evidence_table` | — |
| **5. Discussion** | `interpretation_package`, `synthesis_results`, `prior_reviews_matrix` | **WAJIB**: interpretasi lewat lensa teori, kontras vs prior |
| **5.x Theoretical & practical implications** | `interpretation_package`, `foundation` | **WAJIB**: kontribusi ke teori |
| **5.x Limitations** | `inaccessible_impact`, `extraction_log` (NRNote/VerifiedSample), `sensitivity_analysis`, GRADE | jujur, lihat §5 |
| **6. Conclusions & future research** | `manuscript.{conclusions,future_research}`, `slna_integration.convergent_gaps` | agenda riset berbasis gap teoretis |
| **References** | `.bib` dari `manuscript` (Crossref) + verifikasi §5.6 | — |
| **Appendix/Supplementary** | protokol penuh, tabel ekstraksi lengkap, daftar studi included, PRISMA checklist | — |

**Bahasa naskah:** submission Q1 = **Inggris**. `manuscript_lang` menandai draft (`id`) vs
submission (`en`). Anda menulis ulang ke Inggris akademik (lihat §6), bukan menerjemahkan
kata-per-kata teks LLM.

---

## 4. Teori pondasi — WAJIB, agar tulisan kuat & tidak "melayang"

Artikel Q1 ditolak bila hanya "merangkum apa kata paper" tanpa **kerangka teori** yang
memandu pertanyaan, interpretasi, dan kontribusi. Sistem sudah menyiapkan bahannya — tugas
Anda **mengangkatnya menjadi tulang punggung argumen**.

### 4.1 Dari mana teori pondasi diambil
- **`foundation.theory_markdown`** (M1) — dasar teori SLR + teori domain yang di-generate
  menyesuaikan topik. Ini titik awal, **bukan** untuk disalin mentah.
- **`selected_topic`** (Gap, Type, TypeReason, Importance) — akar *mengapa* studi ini perlu.
- **`prior_reviews_matrix.reviews[]`** — lensa teori review terdahulu + `selisih` (beda posisi
  Anda) + `synthesis_novelty` (klaim kebaruan).
- **`framework_selection`** (TCCM / ADO / PICO / CUSTOM) — kerangka ekstraksi = sering
  merupakan **kerangka teori** yang bisa menstruktur Discussion (mis. TCCM → Theory, Context,
  Characteristics, Methodology).
- **`pico_definitions.canonical_term`** + `research_questions[].traceability` — konstruk inti
  dan bagaimana ia menjembatani gap → RQ.
- **`slna_integration` / `cluster_interpretation`** (bila ada) — struktur intelektual bidang
  (klaster, structural holes) = peta teori untuk memosisikan kontribusi.

### 4.2 Cara memakainya (bukan menempel)
1. **Named theories.** Sebut teori/kerangka dengan **nama + pencetus + sitasi** (mis.
   "Technology Acceptance Model (Davis, 1989)", "Resource-Based View", "DeLone & McLean IS
   Success"). Jika `foundation` menyebut teori, **verifikasi keberadaannya** dan cari sitasi
   primernya (lihat §7 Scopus AI untuk memperkuat). **Jangan mengarang teori/tahun.**
2. **Introduction** dibangun dari umum→spesifik: fenomena → **kerangka teori** → gap teoretis
   (`selected_topic.Gap`) → RQ. Teori muncul SEBELUM RQ, bukan sesudah.
3. **Discussion** menafsirkan temuan **melalui lensa teori**: apakah bukti mendukung/menantang/
   memperluas teori? Ini yang membedakan SLR Q1 dari daftar rangkuman.
4. **Kontribusi teoretis eksplisit** di implikasi: apa yang studi ini tambahkan ke teori
   (konsolidasi, batas berlaku, konstruk baru dari `convergent_gaps`).
5. **Konsistensi konstruk:** pakai `canonical_term` secara konsisten di seluruh naskah
   (hindari sinonim yang membingungkan reviewer).

> Bila `foundation.theory_markdown` tipis/terlalu umum untuk topik, **perkuat lewat Scopus AI
> (§7)** — susun pertanyaan untuk menemukan teori pijakan + sitasi primer, lalu peneliti
> (HITL) memverifikasi. AI **mengusulkan**, manusia **memutuskan** (invariant proyek).

---

## 5. Etika akademik — tidak boleh dilanggar (gate WAJIB)

Pelanggaran di sini = *desk reject* / retraksi. Semua poin ini **memblokir** "siap submit".

### 5.1 Anti-fabrikasi & anti-falsifikasi
- **Semua angka dari DB ground-truth.** N, PRISMA, kappa, %, jumlah per tema **dihitung ulang**
  (`GENERATEREPORT.md §9`), tidak disalin dari `manuscript.*`. Bila narasi LLM ≠ hitungan DB,
  **DB menang**; perbaiki narasi.
- **Jangan menambah/mengurangi studi** demi angka yang "lebih enak" (selection bias / HARKing —
  lihat CLAUDE.md "Validitas metodologi"). Set studi = keputusan FINAL di `slr_screening`.

### 5.2 Protokol a priori (anti-HARKing)
- PICO, kriteria, framework ekstraksi **ditetapkan sebelum** hasil. `framework_selection` &
  `pico_definitions` **stabil**. Setiap perubahan pasca-hasil WAJIB dilaporkan sebagai
  **amendemen protokol terdokumentasi** (`screening_corrections[]` → tulis di Methods/Results
  sebagai deviasi, dengan alasan). Jangan menyembunyikan perubahan.

### 5.3 Provenance & sitasi berintegritas
- **Hanya kutip studi yang ADA di korpus.** Verifikasi tiap sitasi klaim hasil ke
  `slr_extraction`/`slr_screening` (DOI/judul cocok). Klaim yang berjangkar ke paper di luar
  korpus (mis. halusinasi LLM di `manuscript.references`) **HARUS dibuang atau diverifikasi**
  ke sumber nyata. Sitasi teori pondasi (§4) di luar korpus = boleh, tapi **wajib sitasi
  primer yang benar** (verifikasi via Scopus AI/Crossref, jangan tebak tahun/penulis).
- Tiap pernyataan hasil membawa jejak ke `evidence` (`slr_extraction.fields[].evidence`).
  Status `INFERRED`/`AMBIGUOUS` **tidak** boleh dinaikkan jadi klaim pasti tanpa catatan.

### 5.4 Pengungkapan penggunaan AI (WAJIB, kebijakan Elsevier/COPE)
- **AI bukan penulis.** Jangan cantumkan AI sebagai author/co-author.
- Cantumkan **pernyataan penggunaan AI** (biasanya sebelum References atau di Methods),
  mis.: *"AI-assisted tooling (large language models, role-based) was used under human
  supervision for literature screening, data extraction, and drafting support. All
  AI-generated outputs were verified by the authors, who take full responsibility for the
  work."* Sebut **peran** yang dijalankan AI (screening dual-reviewer, ekstraksi RAG,
  drafting) — datanya di `xai_log`, `*.model_used`, `extraction_log.model_extraction`.
- **Transparansi metode** (bukan menyembunyikan bahwa pipeline AI-assisted). Justru kekuatan:
  reproducible + auditable (kappa, provenance). Laporkan model + peran (xAI atribusi).

### 5.5 Keterbukaan keterbatasan
- Laporkan **jujur**: paper tak terakses (`inaccessible_impact`), verifikasi ekstraksi
  (`extraction_log.VerifiedSample`/`DisagreementRate` — bila `VerifiedSample=0` padahal
  `TotalExtracted>0`, verifikasi **GAGAL**, jangan klaim "diverifikasi 20%"), sensitivitas
  (`sensitivity_analysis`), certainty rendah (`grade_evidence_table`). Menyembunyikan
  keterbatasan = pelanggaran etik pelaporan.

### 5.6 Data availability & reproducibility
- Sertakan **PRISMA 2020 checklist** + **flow** (dari DB), protokol, search string, daftar
  studi — sebagai Supplementary. Cantumkan **Data Availability Statement**. Q1 menuntut studi
  bisa direplikasi.

### 5.7 Plagiarisme
- Jangan salin kalimat sumber. Parafrase + sitasi. Kutipan langsung (jika perlu) diberi tanda
  kutip + halaman. Naskah harus lolos Turnitin/iThenticate (target similarity rendah).

---

## 6. No AI-tell — menulis dengan suara manusia

`manuscript.*` ditulis LLM (role Brain) → **kaya jejak-AI**. Jika ditempel apa adanya, reviewer
Q1 mengenalinya dan curiga (kredibilitas turun) + melanggar semangat §5.4. **Tulis ULANG.**

### 6.1 Buang pola khas AI
- **Kata/frasa telltale:** *delve, moreover, furthermore, in conclusion, it is worth noting,
  a testament to, plays a crucial/pivotal role, navigating the landscape, in the realm of,
  rich tapestry, underscores, leverage (verba berlebihan), robust (berulang), notably,
  importantly.* Ganti dengan kata yang lebih spesifik/lugas.
- **Struktur mekanis:** setiap paragraf dibuka "Firstly/Secondly/Finally"; tiap section
  ditutup ringkasan seragam; daftar tiga-item di mana-mana ("X, Y, and Z" berulang). Variasikan.
- **Hedging kosong & klaim bombastis** tanpa data ("significantly", "dramatically") — ganti
  dengan **angka aktual** dari DB.
- **Em-dash & titik-koma berlebihan**, kalimat panjang seragam. Variasikan panjang kalimat.
- **Simetri paragraf** (semua paragraf ~ sama panjang, pola topik-kalimat identik) — pecah.

### 6.2 Tulis seperti peneliti
- **Kalimat topik yang berargumen**, bukan penanda ("This section discusses…"). Langsung ke isi.
- **Voice aktif** untuk klaim penulis ("We screened 1,204 records…"); pasif seperlunya.
- **Spesifik > umum.** "Empat dari 23 studi mengukur X (17%)" > "beberapa studi mengukur X".
- **Transisi logis** (karena/sehingga/namun demikian dengan sebab jelas), bukan konektor
  dekoratif.
- **Kohesi argumen antar-section**: Introduction menjanjikan → Results memenuhi → Discussion
  menafsirkan. Reviewer menilai *alur*, bukan kelengkapan checklist saja.
- **Konsisten tense**: Methods (past: "we searched"), Results (past), Discussion (present untuk
  klaim umum). Terminologi konsisten dengan `canonical_term`.

### 6.3 Prosedur menulis-ulang (bukan menyalin)
1. Ambil **bahan** dari Mongo (angka, temuan, evidence, teori) — bukan kalimat jadi.
2. Susun **outline argumen** per section (klaim → bukti → sitasi).
3. Tulis prosa baru dari outline. Perlakukan `manuscript.*`, `synthesis_results.markdown`,
   `interpretation_package.markdown` sebagai **catatan mentah**, bukan draft final.
4. Baca-keras (mental): jika terdengar seperti brosur/esai LLM, revisi.
5. Cek pola §6.1 lolos.

> Tujuan bukan "menipu detektor", tapi **tulisan ilmiah yang benar-benar baik**: padat,
> spesifik, ber-argumen, dan jujur pada data. Itu yang lolos Q1 sekaligus bebas jejak-AI.

---

## 7. Scopus AI — pertanyaan untuk memperkuat literatur (maks 500 karakter)

Bila sintesis butuh penguatan dari referensi di luar korpus (menemukan **teori pondasi**,
**sitasi primer**, **prior review**, **kontras temuan**, **definisi konstruk**), susun
**pertanyaan untuk ditanyakan ke Scopus AI** (Elsevier). Peneliti (HITL) yang menjalankannya;
Anda **menyiapkan pertanyaannya**, lalu hasilnya diverifikasi manusia sebelum masuk naskah.

### 7.1 Aturan (WAJIB)
- **Maks 500 karakter per pertanyaan** (hitung! sertakan panjang tiap item). Bila lebih,
  pangkas.
- **Satu pertanyaan = satu fokus** (satu konstruk/gap/teori). Jangan menumpuk banyak hal.
- **Spesifik & dapat dijawab dari literatur**: sebut konstruk (`canonical_term`), populasi
  (PICO P), intervensi (I), outcome (O), rentang tahun bila relevan.
- **Netral**, tidak menggiring ("apa bukti X berhasil" → bias). Tanyakan keadaan bukti.
- Bahasa Inggris (Scopus AI berbahasa Inggris).

### 7.2 Dari mana menurunkan pertanyaan (sumber Mongo → maksud pertanyaan)
| Sumber | Pertanyaan untuk… |
|---|---|
| `selected_topic.Gap`, `finer_novelty_check` | konfirmasi gap masih terbuka / kebaruan |
| `foundation.theory_markdown`, `framework_selection` | menemukan **teori pondasi** + sitasi primer |
| `prior_reviews_matrix.reviews[]` + `search_guidance` | prior review nyata untuk memosisikan novelty |
| `research_questions[]` | bukti eksternal per RQ |
| `synthesis_results`, `slna_integration.convergent_gaps` | kontras/perkuat temuan sintesis |
| `grade_evidence_table` (certainty rendah) | mencari bukti tambahan di area lemah |
| `pico_definitions.canonical_term` | definisi/operasionalisasi konstruk yang diterima |

### 7.3 Template & contoh (semua ≤500 char — verifikasi panjang sebelum pakai)
Ganti `{P}`,`{I}`,`{O}`,`{term}`,`{years}` dari `pico_definitions`/`scope_filters`.

- **Teori pondasi:** *"Which established theoretical frameworks are most used to explain
  {I} adoption among {P}, and what are the primary source citations for each?"*
- **Gap/kebaruan:** *"Are there recent systematic reviews ({years}) on {I} for {P} focusing
  on {O}? Summarize their scope and what they did not cover."*
- **Kontras temuan:** *"What does the evidence say about the effect of {I} on {O} in {P} —
  are findings consistent or conflicting, and under what conditions?"*
- **Definisi konstruk:** *"How is '{term}' defined and operationalized in the {P} literature,
  and which definitions are most widely cited?"*
- **Area lemah (GRADE):** *"What is the strength and quality of evidence linking {I} to {O}
  in {P}? Note any noted limitations or risk of bias."*

Simpan pertanyaan ke file `scopus_ai_questions.md` (nomor + teks + panjang char + maksud +
field sumber). Tandai tiap jawaban Scopus AI sebagai **UNVERIFIED** sampai peneliti memverifikasi
sitasi primernya (anti-halusinasi; selaras `prior_reviews_matrix.reviews[].verification`).

> Scopus AI membantu **menemukan & memosisikan**, bukan menggantikan korpus SLR. Temuan inti
> artikel tetap dari studi included (`slr_extraction`). Referensi Scopus AI dipakai untuk
> **Introduction/Related work/Discussion** (teori, posisi, kontras), **bukan** untuk menambah
> "hasil" di luar protokol screening.

---

## 8. Audit "lolos Q1 Elsevier" (gate sebelum menyatakan siap submit)

Jangan nyatakan naskah "siap" sebelum SEMUA kotak ini hijau. Kelompokkan: pelaporan, metode,
substansi, etika, bahasa.

### 8.1 Pelaporan & reproducibility
- [ ] **PRISMA 2020**: checklist 27-item lengkap + flow diagram (dihitung ulang dari DB, §9
  report). Setiap item menunjuk halaman/section.
- [ ] Search string final + tanggal + database + jumlah hits **reproducible** (`search_log`).
- [ ] Kriteria inklusi/eksklusi eksplisit & a priori.
- [ ] Reliabilitas antar-penilai dilaporkan (kappa: `kalibrasi_log`, `fulltext_kappa`).
- [ ] Proses ekstraksi + appraisal kualitas + threshold dilaporkan (`framework_selection`,
  `qa_threshold_justification`).
- [ ] Protokol/registrasi disebut (bila ada) + Data Availability Statement + Supplementary.

### 8.2 Metodologi
- [ ] Dual-reviewer + resolusi konflik dijelaskan.
- [ ] Sintesis sesuai jalur (`synthesis_path_decision`: naratif/meta-analisis/hybrid) dan
  **konsisten** dengan heterogenitas (`descriptive_analysis.heterogeneity_verdict`).
- [ ] **GRADE / certainty of evidence** dilaporkan (`grade_evidence_table`).
- [ ] Sensitivitas/robustness (`sensitivity_analysis`) dibahas.
- [ ] Keterbatasan jujur (§5.5), termasuk bias & paper tak terakses.

### 8.3 Substansi (yang paling sering menentukan Q1)
- [ ] **Kontribusi teoretis eksplisit** (§4) — bukan sekadar rangkuman.
- [ ] **Novelty** jelas vs `prior_reviews_matrix` (apa yang baru).
- [ ] RQ terjawab **tuntas** oleh Results→Discussion (tiap RQ ada jawabannya).
- [ ] Implikasi teoretis **dan** praktis.
- [ ] Agenda riset masa depan konkret (dari `convergent_gaps`), bukan generik.
- [ ] **Journal fit**: scope naskah cocok dengan jurnal target (cek Aims & Scope).

### 8.4 Etika (§5)
- [ ] Angka = DB ground-truth (bukan narasi LLM). PRISMA konsisten.
- [ ] Semua sitasi klaim-hasil ada di korpus; sitasi teori terverifikasi (tahun/penulis benar).
- [ ] Pernyataan penggunaan AI ada + benar; AI bukan author.
- [ ] Tidak ada HARKing; deviasi protokol terlaporkan (`screening_corrections`).
- [ ] Plagiarisme rendah (parafrase + sitasi).

### 8.5 Bahasa & format
- [ ] **No AI-tell** (§6) — sudah ditulis ulang, tidak menempel `manuscript.*`.
- [ ] Inggris akademik konsisten (tense, terminologi = `canonical_term`).
- [ ] Abstract terstruktur sesuai jurnal + keywords.
- [ ] Referensi lengkap & format sesuai jurnal (mis. Elsevier `elsarticle`, gaya numerik/nama);
  `.bib`/`.tex` di `manuscript.bibtex`/`manuscript.latex` sebagai titik awal — **verifikasi**.
- [ ] Gambar/tabel bernomor, ber-caption, dirujuk di teks (PRISMA Fig.1, tabel karakteristik).
- [ ] Cover letter + highlights (3–5 poin) + author contributions disiapkan.

> **Jika satu kotak §8.4 (etika) gagal → JANGAN submit.** Kotak substansi/bahasa yang gagal →
> revisi dulu. Audit ini adalah cerminan invariant proyek: HITL + xAI + jujur pada data.

---

## 9. Alur kerja penulis (ringkas)

1. **Pilih sesi**, pastikan matang (§2 verifikasi kelengkapan; tidak ada `✗ MISSING` inti).
2. **Pull semua data** (§2) + **recompute angka** PRISMA/kappa/N (`GENERATEREPORT.md §9`).
3. **Angkat teori pondasi** (§4); bila kurang, **susun pertanyaan Scopus AI** (§7) → peneliti
   jalankan → verifikasi (HITL).
4. **Outline argumen** per section (§3, §6.3), petakan tiap klaim ke evidence.
5. **Tulis naskah baru** (bukan menyalin `manuscript.*`) dengan suara manusia (§6), Inggris.
6. **Bangun tabel/gambar** (PRISMA flow, karakteristik studi dari `slr_extraction`, GRADE).
7. **Jalankan audit Q1** (§8). Perbaiki sampai hijau; etika (§5/§8.4) mutlak.
8. **Rakit artefak submission**: naskah (.md/.tex dari `manuscript.latex` yang diverifikasi),
   Supplementary (protokol, PRISMA checklist, daftar studi), cover letter, highlights,
   AI-disclosure, Data Availability.
9. Serahkan sebagai **"siap diuji peneliti"**, bukan "final" — peneliti (HITL) meninjau &
   memutuskan submit (model pengujian proyek: manusia yang memvalidasi).

---

## 10. Gotcha (jangan keliru)

- **`manuscript.*` = draft LLM, bukan naskah final.** Sumber bahan, bukan teks siap tempel
  (etika §5.4 + AI-tell §6).
- **Angka dari DB, bukan dari narasi.** Selalu recompute (`GENERATEREPORT.md §9`).
- **`omitempty`**: field hilang = tahap belum jalan / nilai zero, **bukan** error. Jangan
  mengarang isinya (`GENERATEREPORT.md §11`).
- **Verifikasi ekstraksi 0** (`extraction_log.VerifiedSample=0` saat `TotalExtracted>0`) =
  verifikasi **GAGAL**; jangan tulis "diverifikasi". Laporkan apa adanya (limitasi).
- **Multi-tenant**: kriteria/ambang dari DATA sesi (`pico_definitions`, `audit_scope_rules`),
  bukan default global. Tulis sesuai sesi ini.
- **Read-only**: jangan pernah menulis Mongo. Koreksi data lewat UI/HITL, bukan dari sesi penulis.
- **Scopus AI ≤500 char/pertanyaan**, hasil **UNVERIFIED** sampai peneliti verifikasi; dipakai
  untuk teori/posisi/kontras, **bukan** menambah "hasil" di luar protokol screening.
- **Jangan menggelembungkan N** atau mengubah protokol pasca-hasil (HARKing) — validitas Q1
  runtuh (CLAUDE.md "Validitas metodologi").

Bila §2–§8 terpenuhi dari DB, naskahnya **utuh, etis, ber-teori, bebas jejak-AI, dan siap
diaudit untuk Q1**.

---

## Update: Modul 10, verifikasi klaim, disclosure AI, & Zenodo (rantai reproducible)

> Bagian ini menyesuaikan panduan dengan revisi terbaru sistem (M9 xAI + M10 Audit).
> Field teknis lengkap ada di `GENERATEREPORT.md` (§ Field BARU).

### 1. Dua jalur regenerasi
- **Jalur cepat (tanpa DB):** manuskrip LaTeX final (`manuscript.latex`/`bibtex`) SUDAH lengkap — sempurnakan prosa langsung. Rantai tertutup tanpa akses database.
- **Jalur penuh (DB read-only):** tulis ulang/perkaya dari sumber (Mongo/Qdrant/Neo4j) mengikuti panduan ini.

### 2. Gunakan bukti verifikasi klaim (neuro-symbolic) — KUNCI defensibilitas Q1
`manuscript.claim_verifications[]` memuat triangulasi tiap klaim ke Qdrant+Neo4j+MongoDB.
- Di *Results/Discussion*: **kutip HANYA klaim dengan `sources>=2`**. Klaim `<2` → jangan diklaim tegas; laporkan sebagai keterbatasan (kejujuran evidential).
- Di supplementary: sertakan ringkasan `n terverifikasi / total` + breakdown sumber. Ini bukti bahwa klaim tertaut bukti — reviewer Q1 menghargainya.

### 3. Disclosure AI yang AKURAT (JANGAN menyamarkan AI)
Sistem memakai AI sebagai **decision-support** untuk skrining, ekstraksi, penilaian kualitas/risiko bias, dan sintesis — SELALU dengan **verifikasi manusia (HITL)** di tiap gerbang, **dua penilai AI independen + Cohen's κ**, dan **neuro-symbolic** (aturan simbolik + neural).
- **Methods HARUS mendeskripsikan ini secara transparan** (mis. "two LLM-based reviewers with human-in-the-loop verification; inter-rater agreement quantified via Cohen's kappa"). JANGAN menulis "AI tidak dipakai untuk analisis" atau menyamarkan rater AI sebagai manusia — itu misrepresentasi & risiko integritas (COPE/Elsevier).
- Sertakan **AI Assistance Declaration** yang jujur + rujuk provenance (log xAI) sebagai supplementary.

### 4. Artefak Modul 10 (siap pakai — jangan digenerate ulang)
Dari `session.audit_report`:
- `protocol_markdown` = **Protokol a-priori gaya PROSPERO** → daftarkan di PROSPERO/OSF (idealnya sebelum/saat submit).
- `repro_package_markdown` = **Paket Reproducibility** (PRISMA-S: search string per-DB, κ screening/ekstraksi/QA, form+rubrik, GRADE, PRISMA flow+checklist, jejak keputusan, pernyataan AI).
- `verdict` (`READY`/`READY_WITH_WARNINGS`/`NOT_READY`) = status kesiapan submit; selesaikan bloker sebelum submit.

### 5. Deposit Zenodo + Data Availability Statement (reproducible + DOI)
1. Deposit satu paket ke **Zenodo** (login via ORCID → New upload): protokol + paket reproducibility + laporan + manuskrip `.tex`/`.bib` + Panduan Handoff. (Opsional: ekspor read-only data ekstraksi/screening — HAPUS kredensial.)
2. Metadata + lisensi (mis. CC-BY-4.0) → Publish → dapat **DOI** (`10.5281/zenodo.XXXXXXX`).
3. Tulis **Data Availability Statement** di manuskrip: "Protocol, extraction data, and reproducibility package are openly available at Zenodo, DOI: 10.5281/zenodo.XXXXXXX." + tautkan registrasi PROSPERO/OSF.
4. Revisi berikutnya → *New version* Zenodo (concept-DOI tetap, version-DOI baru).

**Rantai reproducible tertutup:** protokol a-priori (PROSPERO) → data & keputusan HITL (DB) → paket reproducibility → arsip ber-DOI (Zenodo) → disitasi di manuskrip. Pihak ketiga memverifikasi dari DOI tanpa akses sistem asli.
