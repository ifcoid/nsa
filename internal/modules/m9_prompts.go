package modules

// Aturan bersama (Bagian C Modul 9) ditempel ke setiap prompt section.
const m9Rules = `

ATURAN WAJIB:
- Tulis HANYA section yang diminta, Bahasa Inggris akademik (Markdown). Tanpa preamble/kalimat meta.
- DILARANG menyebut AI/Claude/LLM/GPT/"Pass 1-2"/sesi/nama file internal (outputs/, .xlsx, "Modul X", qdrant).
- Framing manusia: "Reviewer 1/2", "Extractor 1/2", "Rater 1/2" (BUKAN AI). Kappa = inter-reviewer/extractor/rater agreement.
- Angka (N studi, κ, %, GRADE) HARUS dari ARTEFAK yang diberikan; JANGAN mengarang/estimasi. Bila tak tersedia, tulis netral tanpa angka palsu.
- Hedging sesuai GRADE: HIGH→tegas; MODERATE→"likely/probably"; LOW→"may/suggests"; VERY LOW→"tentative/uncertain".
- Geographic honesty: jangan klaim "global" bila data dominan regional; sebut region + persentase aktual dari descriptive.
- Jika synthesis_path JALUR A: hindari "pooled effect/d=X across studies/overall effect size". Jika JALUR B: boleh bahasa meta-analitik (I², pooled estimate).
- Terminologi: "systematic review", "extraction", "synthesis"/"meta-analysis", "PICO". Hindari calque ("It is known that", "It can be concluded", "Many studies have" sebagai opener).

GAYA ANTI-CIRI-AI (WAJIB — agar tidak terbaca seperti tulisan AI):
- JANGAN gunakan tanda em-dash ("—") atau en-dash ("–") sama sekali. Ganti dengan koma, tanda kurung, titik dua, atau pecah jadi kalimat baru.
- Hindari transisi klise yang bertumpuk: "Moreover", "Furthermore", "In addition", "Notably", "It is worth noting", "It is important to note", "On the other hand". Pakai seperlunya & bervariasi.
- Hindari pola terlalu rapi "not only X but also Y" dan tiga-serangkai "X, Y, and Z" berulang.
- Hindari kata over-pakai AI: "delve", "leverage", "underscore", "pivotal", "realm", "tapestry", "intricate", "crucial"/"vital" berlebihan, "robust" berlebihan.
- Variasikan panjang & struktur kalimat (jangan seragam). Jangan menyisipkan bullet-list di tengah prosa argumentatif.
- Jangan pakai EMOJI atau IKON/SIMBOL DEKORATIF apa pun di prosa maupun heading (mis. ✅ ⚠️ ❌ ✓ ✗ → ➔ ★ ● ◆ ▪ 🎯 🚀 🔑 dan sejenisnya). Pengecualian SATU-SATUNYA: simbol ✓ / ⚠ / ✗ boleh dipakai HANYA di dalam tabel checklist formal (mis. PRISMA 2020 checklist), tidak di prosa, judul, abstract, atau heading.
- Jangan pakai kutip keriting (“ ” ‘ ’), panah unicode sebagai pengganti kata, atau bullet ber-ikon. Tulisan harus terbaca natural seperti ditulis akademisi manusia.`

const promptMethods = `Anda penulis akademik. Tulis section METHODS sebuah systematic review yang PATUH PRISMA 2020 (item 5-19), dalam author voice.
Cakup: 5 Eligibility (PICO + reason codes), 6 Information sources (database + tanggal pencarian terakhir), 7 Search strategy (string Boolean + filter + update policy), 8 Selection process (dua tahap, Reviewer 1 & 2, κ_TA & κ_FT dari artefak, resolusi disagreement; perkenalkan Figure 1 PRISMA flow di sini), 9 Data collection (Extractor 1 & 2, κ_extract, framework TCCM/ADO/PICO, 100% validasi author), 10 Data items, 11 RoB (tool aktual + Rater 1 & 2 + κ_rob + threshold 3-tier + sensitivity), 12 Effect measures (hanya bila Jalur B), 13 Synthesis methods (Jalur A/B tegas), 14 Reporting bias (bila Jalur B), 15 Certainty (GRADE per outcome).
Setiap keputusan + justifikasi ("We did X because Y"). Panjang 1200-1800 kata.` + m9Rules

const promptResults = `Tulis section RESULTS systematic review, terstruktur per framework (TCCM/ADO/PICO sesuai artefak), OBJEKTIF (tanpa interpretasi — interpretasi ada di Discussion).
Cakup: 2.1 Evolution of field (narasi angka PRISMA flow per tahap + cross-ref Figure 1, tren tahun, total studi, distribusi geografis JUJUR dari descriptive), 2.2 Dominant theories (jika TCCM), 2.3 Contexts, 2.4 Characteristics + constructs, 2.5 Methodological trends, 2.6 Synthesis results (Jalur A: tematik consistent+contradictory, indicative ranges per studi; Jalur B: pooled + subgroup + forest plot Figure; + tabel GRADE evidence profile).
Rujuk Table 1 (Characteristics), Table 2 (Quality), Table 3 (GRADE). Panjang 2000-3000 kata.` + m9Rules

const promptDiscussion = `Tulis section DISCUSSION systematic review dengan 6 subseksi WAJIB:
3.1 Summary of findings (jawab RQ, sintesis high-level + interpretasi, BUKAN repetisi Results),
3.2 Geographic & Contextual Honesty (DI AWAL; akui bias geografis dgn angka aktual + penjelasan struktural + implikasi),
3.3 Dialog with existing theory (mendukung/menantang/memperluas + teori under-utilized + kontribusi),
3.4 Heterogeneity analysis (mengapa temuan bervariasi; studi kontradiktif; moderator),
3.5 Comparison with prior reviews (konsisten/berbeda vs prior_reviews; novelty — "how findings dialogue", post-findings),
3.6 Limitations 3-tier (review-level, study-level [+inaccessible N,%], synthesis-level) tiap limitasi + mitigasi.
Kutip angka dari Results lalu beri interpretasi baru (jangan ulang). Panjang 2000-2800 kata.` + m9Rules

const promptFuture = `Tulis subseksi FUTURE RESEARCH AGENDA (setelah Discussion), turunan gaps dari interpretation + prior_reviews.
Struktur: 4.1 Pendahuluan agenda (1 paragraf), 4.2 Matriks prioritas (tabel: Priority | Timeframe | Rationale[link ke gap] | Research Question spesifik | Suggested Methodology) — min 3 HIGH + 2-3 MEDIUM + 1-2 LONG-TERM, 4.3 Prioritization rationale, 4.4 Methodological advancements needed.
Tiap agenda = RESEARCH QUESTION spesifik (BUKAN "more research needed") + metodologi eksplisit; trace ke gap konkret. Beda dari Discussion 3.6 (yang = limitasi current). Panjang 800-1200 kata.` + m9Rules

const promptIntro = `Tulis section INTRODUCTION systematic review dengan 5 subseksi WAJIB:
5.1 Background (field overview + importance + why now), 5.2 Review of Prior Reviews (subseksi tersendiri: 3-5 prior reviews [scope/method/findings/limitations] + paragraf "Synthesis Novelty" — apa SUDAH/BELUM dilakukan kolektif, mengapa riset ini menutup gap; FORWARD-looking, JANGAN preview findings), 5.3 Problem statement dgn tipe gap (A/B/C sebagai framing konseptual, author voice spesifik topik), 5.4 Scope justification (dari scope_justifications, author voice), 5.5 Research questions + objectives (primary + 3 secondary + preview framework TCCM/ADO/PICO; sebut PRISMA 2020 + Cochrane terintegrasi).
JANGAN preview N/κ spesifik (itu di Results). Panjang 1000-1500 kata.` + m9Rules

const promptConclusions = `Tulis section CONCLUSIONS yang LEAN (3-4 paragraf, 400-600 kata):
P1 Main conclusions (jawab primary RQ + bukti ringkas, bukan repetisi Results), P2 Theoretical contributions (+ insight framework, hedging per GRADE), P3 Practical implications (kebijakan/praktisi/policymaker, grounded), P4 (opsional) brief forward look 1-2 kalimat ke Future Research.
BUKAN expanded Discussion; jangan ulang kalimat Discussion verbatim; jangan data baru.` + m9Rules

const promptAbstract = `Tulis ABSTRACT terstruktur systematic review, 250-300 kata, 4 bagian (tanpa label berlebihan):
Background & Objective (problem + rationale + objective + tipe gap; eksplisit "systematic review"); Methods (PRISMA 2020 framing + database + rentang & tanggal pencarian + framework TCCM/ADO/PICO + jalur sintesis + N final included); Results (N studi + 2-3 temuan utama hedged per GRADE + cakupan geografis jujur + ringkas GRADE); Conclusions (kesimpulan utama + top 1-3 prioritas future research).
Semua angka dari artefak. NO jargon berat. Konsisten dgn Methods/Results.` + m9Rules

const promptTitle = `Usulkan 3-5 alternatif TITLE untuk systematic review ini.
Kriteria: deskriptif, spesifik, WAJIB cantumkan "systematic review"/"systematic literature review", 10-14 kata (atau 12-18 bila SLNA), searchable, geographic honesty (jangan "global" bila regional).
Untuk tiap alternatif: judul | jumlah kata | keywords | geographic honesty PASS/FAIL | rationale 2-3 kalimat. Di akhir beri REKOMENDASI judul terbaik + justifikasi (Markdown).` + m9Rules
