package modules

// m9Rules contains shared rules appended to every M9 section prompt.
// Enforces LaTeX output, \cite{} citations, anti-AI style, and structured per-claim referencing.
const m9Rules = `

ATURAN WAJIB (FORMAT & REFERENCING):
- Output HARUS dalam format LaTeX. Gunakan \section{}, \subsection{}, \subsubsection{} sesuai hierarki.
- Setiap klaim faktual HARUS disertai \cite{authorYear} yang merujuk ke PAPER CATALOG di bawah.
- Format per-klaim: "Finding [klaim spesifik] \cite{authorYear}." atau "Several studies confirmed X \cite{smith2023, lee2024}."
- Setiap paragraf WAJIB mengutip minimal 2-3 referensi berbeda menggunakan \cite{}.
- JANGAN menulis referensi inline penuh (e.g., "Smith (2023) found..."). Gunakan HANYA \cite{key}.
- JANGAN mengarang citation key. Gunakan HANYA key dari PAPER CATALOG yang diberikan.
- Untuk referensi guideline/metodologi yang TIDAK ADA di PAPER CATALOG (misalnya PRISMA 2020, Cochrane Handbook, GRADE working group), gunakan format inline text citation seperti "following PRISMA 2020 guidelines (Page et al., 2021)" atau "as recommended by the Cochrane Handbook (Higgins et al., 2023)". JANGAN gunakan \cite{} untuk referensi yang tidak ada di PAPER CATALOG.
- Tulis HANYA section yang diminta. Tanpa preamble/kalimat meta/pembuka "Here is the section".
- TRANSPARANSI AI (WAJIB, sesuai COPE/Elsevier): Methods HARUS menyatakan JUJUR bahwa penyaringan, ekstraksi, dan penilaian kualitas dilakukan oleh dua penilai berbasis large language model (LLM/AI) sebagai alat bantu keputusan (decision-support), dengan verifikasi dan keputusan akhir oleh reviewer manusia melalui gerbang human-in-the-loop, dan kesepakatan antar-penilai diukur dengan Cohen's kappa. JANGAN menyamarkan AI sebagai manusia. Sebut "two LLM-based reviewers/extractors/raters" + kendali manusia apa adanya.
- Yang TETAP DILARANG (kebocoran IMPLEMENTASI INTERNAL, bukan peran AI): nama produk/model dagang di prosa ("Claude/GPT/Gemini" — taruh nama+versi model di AI Declaration/supplementary), "Pass 1-2", nama file/path (outputs/, .xlsx), "Modul X", ID sesi, "qdrant/neo4j/mongo", "draft v1".
- Angka (N studi, kappa, %, GRADE) HARUS dari ARTEFAK yang diberikan; JANGAN mengarang/estimasi. Bila tak tersedia, tulis netral tanpa angka palsu.
- ANGKA IDENTIFIKASI PRISMA — "records identified" (total hasil pencarian) dan "duplicate records removed" — HANYA boleh muncul di Figure 1 (diagram alur PRISMA yang di-generate deterministik & disegarkan otomatis saat unduh). JANGAN menuliskan kedua angka itu sebagai literal di prosa/abstract; untuk merujuknya gunakan "Figure~\ref{fig:prisma}". Angka lain (records screened, reports assessed, studies included, kappa) BOLEH disebut di teks. Alasan: kedua angka identifikasi bisa dikoreksi kemudian (recount), dan menaruhnya HANYA di Figure 1 membuat naskah tetap sinkron tanpa menulis ulang manuskrip.
- Hedging sesuai GRADE: HIGH = tegas; MODERATE = "likely/probably"; LOW = "may/suggests"; VERY LOW = "tentative/uncertain".
- Geographic honesty: jangan klaim "global" bila data dominan regional; sebut region + persentase aktual dari descriptive.
- Jika synthesis_path JALUR A: hindari "pooled effect/d=X across studies/overall effect size". Jika JALUR B: boleh bahasa meta-analitik (I-squared, pooled estimate).
- Terminologi: "systematic review", "extraction", "synthesis"/"meta-analysis", "PICO". Hindari calque ("It is known that", "It can be concluded", "Many studies have" sebagai opener).

GAYA ANTI-CIRI-AI (WAJIB KERAS):
- DILARANG MUTLAK menggunakan em-dash (---), en-dash (--), karakter Unicode em-dash atau en-dash. Ganti dengan koma, tanda kurung, titik dua, atau pecah jadi kalimat baru.
- DILARANG filler phrases: "it is worth noting", "importantly", "notably", "furthermore", "moreover", "in addition", "it is important to note", "on the other hand", "it should be noted", "interestingly". Jika ingin transisi, gunakan variasi alami atau langsung masuk substansi.
- DILARANG bullet points (\begin{itemize}, \item) di dalam paragraf prosa argumentatif. Bullet HANYA boleh di tabel atau daftar terstruktur eksplisit.
- Variasikan panjang kalimat: campur pendek (8-12 kata) dengan panjang (25-40 kata). Jangan seragam.
- Gunakan hedging natural: "suggests", "indicates", "appears to", "the evidence points toward". Hindari overclaiming.
- DILARANG kata over-pakai AI: "delve", "leverage", "underscore", "pivotal", "realm", "tapestry", "intricate", "crucial"/"vital" berlebihan, "robust" berlebihan, "multifaceted", "nuanced", "comprehensive" berlebihan.
- DILARANG pola "not only X but also Y" berulang dan tiga-serangkai "X, Y, and Z" berulang.
- Jangan pakai EMOJI atau IKON/SIMBOL DEKORATIF.
- Jangan pakai kutip keriting, panah unicode, atau bullet ber-ikon.
- Tulisan harus terbaca natural seperti ditulis akademisi manusia yang berpengalaman menulis jurnal.`

const promptMethods = `Anda penulis akademik. Tulis \section{Methods} sebuah systematic review yang PATUH PRISMA 2020 (item 5-19), dalam author voice, format LaTeX.
Gunakan \subsection{} untuk tiap sub-bagian. Setiap keputusan metodologis HARUS didukung \cite{} ke paper yang menerapkan metode serupa atau ke guideline (jika ada di catalog).

Cakup subseksi:
\subsection{Eligibility Criteria} -- PICO + reason codes, kutip guideline/paper pembanding
\subsection{Information Sources} -- database + tanggal pencarian terakhir
\subsection{Search Strategy} -- string Boolean + filter + update policy
\subsection{Selection Process} -- dua tahap, Reviewer 1 & 2, kappa_TA & kappa_FT dari artefak, resolusi disagreement; perkenalkan "Figure 1" PRISMA flow
\subsection{Data Collection Process} -- Extractor 1 & 2, kappa_extract, framework TCCM/ADO/PICO, 100% validasi author
\subsection{Data Items} -- variabel yang diekstrak
\subsection{Study Risk of Bias Assessment} -- tool aktual + Rater 1 & 2 + kappa_rob + threshold 3-tier + sensitivity
\subsection{Effect Measures} -- (hanya bila Jalur B)
\subsection{Synthesis Methods} -- Jalur A/B tegas, kutip studi yang di-synthesize
\subsection{Reporting Bias Assessment} -- (bila Jalur B)
\subsection{Certainty Assessment} -- GRADE per outcome

Setiap keputusan + justifikasi ("We did X because Y \cite{key}"). Minimal 2-3 \cite{} per paragraf. Panjang 1200-1800 kata.` + m9Rules

const promptResults = `Tulis \section{Results} systematic review dalam format LaTeX, terstruktur per framework (TCCM/ADO/PICO sesuai artefak), OBJEKTIF (tanpa interpretasi).
Setiap temuan HARUS dikaitkan ke paper spesifik: "Study by \cite{authorYear} found that..." atau "This finding was corroborated by \cite{key1, key2}."

Cakup subseksi:
\subsection{Study Selection and Characteristics} -- deskripsikan PROSES seleksi dua-tahap + rujuk "Figure~\ref{fig:prisma}" untuk angka alur PRISMA. JANGAN menuliskan angka "records identified" maupun "duplicate records removed" sebagai literal (kedua angka HANYA di Figure 1 — lihat ATURAN WAJIB); boleh sebut jumlah studi FINAL yang di-include, records screened, tren tahun, total studi, distribusi geografis JUJUR dari descriptive
\subsection{Dominant Theories} -- (jika TCCM) kutip paper yang menggunakan tiap teori
\subsection{Contexts} -- konteks penelitian, kutip paper per konteks
\subsection{Characteristics and Constructs} -- variabel utama, kutip paper yang mengukurnya
\subsection{Methodological Trends} -- desain riset dominan, kutip contoh
\subsection{Synthesis of Findings} -- Jalur A: tematik consistent+contradictory, per studi; Jalur B: pooled + subgroup + forest plot Figure; + tabel GRADE evidence profile

Rujuk Table 1 (Characteristics), Table 2 (Quality), Table 3 (GRADE). Minimal 2-3 \cite{} per paragraf. Panjang 2000-3000 kata.` + m9Rules

const promptDiscussion = `Tulis \section{Discussion} systematic review dalam format LaTeX dengan 6 subseksi WAJIB.
Setiap interpretasi HARUS merujuk kembali ke paper spesifik: "Consistent with \cite{authorYear}, our findings suggest..."

\subsection{Summary of Findings} -- jawab RQ, sintesis high-level + interpretasi, BUKAN repetisi Results. Kutip 3-5 paper kunci.
\subsection{Geographic and Contextual Considerations} -- DI AWAL; akui bias geografis dgn angka aktual + penjelasan struktural + implikasi. Kutip paper dari under-represented regions.
\subsection{Dialogue with Existing Theory} -- mendukung/menantang/memperluas + teori under-utilized + kontribusi. Kutip paper per teori.
\subsection{Heterogeneity Analysis} -- mengapa temuan bervariasi; studi kontradiktif; moderator. Kutip paper yang berlawanan.
\subsection{Comparison with Prior Reviews} -- konsisten/berbeda vs prior_reviews; novelty. Kutip prior reviews + paper baru.
\subsection{Limitations} -- 3-tier (review-level, study-level [+inaccessible N,%], synthesis-level) tiap limitasi + mitigasi.

Kutip angka dari Results lalu beri interpretasi baru (jangan ulang). Minimal 2-3 \cite{} per paragraf. Panjang 2000-2800 kata.` + m9Rules

const promptFuture = `Tulis \subsection{Future Research Agenda} (setelah Discussion) dalam format LaTeX, turunan gaps dari interpretation + prior_reviews.
Setiap agenda riset HARUS merujuk gap yang ditemukan di paper tertentu: "Given the limitation identified in \cite{authorYear}, future work should..."

Struktur:
\subsubsection{Introduction} -- 1 paragraf pengantar agenda, kutip 2-3 paper yang menunjukkan gap
\subsubsection{Priority Matrix} -- tabel LaTeX (\begin{table}): Priority | Timeframe | Rationale[link ke gap + \cite{}] | Research Question spesifik | Suggested Methodology. Min 3 HIGH + 2-3 MEDIUM + 1-2 LONG-TERM.
\subsubsection{Prioritization Rationale} -- penjelasan prioritas, kutip paper
\subsubsection{Methodological Advancements Needed} -- kutip studi dengan keterbatasan metodologi

Tiap agenda = RESEARCH QUESTION spesifik (BUKAN "more research needed") + metodologi eksplisit; trace ke gap konkret + \cite{}. Minimal 2-3 \cite{} per paragraf. Panjang 800-1200 kata.` + m9Rules

const promptIntro = `Tulis \section{Introduction} systematic review dalam format LaTeX dengan 5 subseksi WAJIB.
Setiap klaim background HARUS didukung \cite{}: "The field of X has grown rapidly \cite{key1, key2}."

\subsection{Background} -- field overview + importance + why now. Kutip 3-5 paper yang menunjukkan relevansi.
\subsection{Review of Prior Reviews} -- 3-5 prior reviews [scope/method/findings/limitations] + paragraf "Synthesis Novelty": apa SUDAH/BELUM dilakukan kolektif, mengapa riset ini menutup gap; FORWARD-looking, JANGAN preview findings. Kutip setiap prior review.
\subsection{Problem Statement} -- tipe gap (A/B/C sebagai framing konseptual), author voice spesifik topik. Kutip paper yang menunjukkan gap.
\subsection{Scope Justification} -- dari scope_justifications, author voice. Kutip 2-3 paper.
\subsection{Research Questions and Objectives} -- primary + 3 secondary + preview framework TCCM/ADO/PICO; sebut PRISMA 2020 + Cochrane terintegrasi. Kutip guideline papers.

JANGAN preview N/kappa spesifik (itu di Results). Minimal 2-3 \cite{} per paragraf. Panjang 1000-1500 kata.` + m9Rules

const promptConclusions = `Tulis \section{Conclusions} dalam format LaTeX yang LEAN (3-4 paragraf, 400-600 kata):
Setiap kesimpulan HARUS didukung \cite{} ke paper yang menjadi bukti utama.

P1 Main conclusions -- jawab primary RQ + bukti ringkas via \cite{key1, key2, key3}, bukan repetisi Results.
P2 Theoretical contributions -- insight framework, hedging per GRADE, kutip paper yang menyumbang teori.
P3 Practical implications -- kebijakan/praktisi/policymaker, grounded, kutip paper relevan.
P4 (opsional) Brief forward look 1-2 kalimat ke Future Research, kutip 1-2 paper kunci.

BUKAN expanded Discussion; jangan ulang kalimat Discussion verbatim; jangan data baru. Minimal 2-3 \cite{} per paragraf.` + m9Rules

const promptAbstract = `Tulis ABSTRACT terstruktur systematic review dalam format LaTeX (gunakan \begin{abstract}...\end{abstract}), 250-300 kata, 4 bagian implisit (tanpa label eksplisit di abstract):
Background & Objective (problem + rationale + objective + tipe gap; eksplisit "systematic review"); Methods (PRISMA 2020 framing + database + rentang & tanggal pencarian + framework TCCM/ADO/PICO + jalur sintesis + N final included); Results (N studi + 2-3 temuan utama hedged per GRADE + cakupan geografis jujur + ringkas GRADE); Conclusions (kesimpulan utama + top 1-3 prioritas future research).
Semua angka dari artefak. NO jargon berat. Konsisten dgn Methods/Results. JANGAN gunakan \cite{} di abstract (sesuai konvensi jurnal).` + m9Rules

const promptTitle = `Usulkan 3-5 alternatif TITLE untuk systematic review ini.
Kriteria: deskriptif, spesifik, WAJIB cantumkan "systematic review"/"systematic literature review", 10-14 kata (atau 12-18 bila SLNA), searchable, geographic honesty (jangan "global" bila regional).
Format output LaTeX: gunakan \title{...} untuk setiap alternatif.
Untuk tiap alternatif: \title{judul} | jumlah kata | keywords | geographic honesty PASS/FAIL | rationale 2-3 kalimat. Di akhir beri REKOMENDASI judul terbaik + justifikasi.` + m9Rules

// promptVerification is used for the verification pass that outputs CORRECTED LaTeX text.
const promptVerification = `Anda adalah verifikator akademik. Periksa teks LaTeX berikut dan PERBAIKI langsung:

SUMBER KEBENARAN: daftar "ALLOWED CITATION KEYS" di bawah adalah SATU-SATUNYA key sah. Bagian "VERIFICATION RESULTS" menandai tiap \cite: INVALID_KEY (key tak ada di daftar), UNVERIFIED, WEAK, atau VERIFIED.

TUGAS:
1. Untuk setiap \cite{key}: jika key PERSIS ada di ALLOWED CITATION KEYS dan klaim konsisten, PERTAHANKAN apa adanya.
2. Untuk \cite yang ditandai INVALID_KEY (termasuk key berhias seperti "wang2024femba" atau key deskriptif seperti "femba_gap9_deployment"):
   - Ganti dengan key yang BENAR dari ALLOWED CITATION KEYS (cocokkan via penulis/tahun/topik).
   - Jika merujuk guideline (PRISMA, GRADE, Cochrane) yang tidak ada di daftar, ubah jadi inline citation (contoh: "following PRISMA 2020 guidelines (Page et al., 2021)").
   - Jika tidak ada key yang cocok, HAPUS \cite{} dan tulis ulang klaim tanpa referensi.
3. DILARANG KERAS mengarang atau menghias key. Output HANYA boleh memuat key yang ADA di ALLOWED CITATION KEYS.
4. Jika klaim BERTENTANGAN dengan data, perbaiki klaim. Jika klaim tanpa bukti, tambahkan hedging ("may", "appears to") atau hapus.

OUTPUT: Keluarkan HANYA teks LaTeX yang sudah diperbaiki (section yang sama, sudah terkoreksi). JANGAN keluarkan laporan/daftar/checklist. JANGAN tambah komentar meta. Teks harus siap digunakan langsung sebagai section manuscript.`

// promptStyleCleanup is used for the style cleanup pass that removes AI-style artifacts.
const promptStyleCleanup = `Anda adalah editor gaya akademik. Bersihkan teks LaTeX berikut dari SEMUA ciri tulisan AI.

CHECKLIST PEMBERSIHAN:
1. Hapus SEMUA em-dash (---) dan en-dash (--). Ganti dengan koma, titik dua, tanda kurung, atau kalimat baru.
2. Hapus filler: "it is worth noting", "importantly", "notably", "furthermore", "moreover", "in addition", "it is important to note", "interestingly", "it should be noted".
3. Hapus kata AI: "delve", "leverage", "underscore", "pivotal", "realm", "tapestry", "intricate", "multifaceted", "nuanced". Ganti dengan kata natural.
4. Pecah kalimat yang terlalu panjang (>50 kata). Gabung kalimat yang terlalu pendek berturut-turut.
5. Hapus pola "not only X but also Y" jika muncul >1x. Variasikan.
6. Pastikan TIDAK ada bullet (\item, \begin{itemize}) di tengah paragraf prosa.
7. Pastikan setiap paragraf tetap memiliki minimal 2-3 \cite{}. JANGAN ubah/hias/karang key \cite{}; pakai HANYA key dari ALLOWED CITATION KEYS di bawah, PERSIS seperti tertulis.
8. Pastikan TIDAK ada emoji, ikon, simbol dekoratif.
9. Perbaiki tanda kutip LaTeX: setiap kutipan harus memakai pasangan seimbang (dua backtick pembuka dan dua apostrof penutup), contoh: ` + "``feasibility''" + `. Hapus ` + "''" + ` penutup yang menggantung tanpa pembuka.

Output: teks LaTeX yang telah dibersihkan, siap kompilasi. Jangan tambah komentar meta.`
