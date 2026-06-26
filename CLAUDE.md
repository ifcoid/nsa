## Topologi sistem: 3 repo terpisah (sejajar di folder `awangga`)

Sistem ini tersebar di **tiga repo** yang di-clone bersebelahan di `/home/adb/awangga/`:

- **`nsa`** — **BACKEND** (Go). Orkestrator pipeline SLR M1–M9, state-machine via
  `session.Status`, REST API, akses MongoDB/Qdrant/Neo4j. Repo: `ifcoid/nsa`. Deploy: fly.io.
- **`slr`** — **FRONTEND** (vanilla JS). UI HITL: panel keputusan, gate, tabel, tombol Sync,
  editor scope. Repo: `ifcoid/slr`. Memanggil REST API `nsa`.
- **`pede`** — **INGESTION + EMBEDDING** (Python). "PDF → Embedding": konversi PDF→markdown
  (`pymupdf4llm`, OCR PaddleOCR fallback), ekstraksi metadata (3-layer + CrossRef), chunking,
  embedding **BGE-M3 hybrid (dense+sparse)**, tulis ke **Qdrant** (collection
  `scientific_articles`, payload key lowercase `title`/`doi`/`article_id`). Repo: `ifcoid/pede`.
  - `ingest.py` = CLI batch (jalur yang sama dipakai notebook Colab `pede_colab.ipynb`).
  - **Dijalankan via Google Colab** (tidak semua user punya GPU; bge-m3 hybrid butuh GPU).
    Notebook `notebooks/pede_colab.ipynb` melakukan `git clone`/`git pull` dari `ifcoid/pede`
    lalu `subprocess` `python ingest.py` — jadi **semua perbaikan logika HARUS di-push ke
    `main` `pede`** (notebook auto-pull saat re-run cell setup). JANGAN menaruh logika
    inti inline di sel notebook (akan basi); notebook hanya orkestrasi (mount Drive, pull,
    secrets, pip, loop-retry, notif Telegram). Embedding ingestion **lokal** di Colab.
  - `embed_server_colab.ipynb` = server embedding runtime terpisah yang di-query RAG `nsa`
    (BUKAN bagian ingestion).
  - Kontrak: `nsa` SyncQdrant mencocokkan rekod screening ↔ Qdrant via **DOI exact ∪
    title-similarity >0.8**. Field payload `pede` HARUS cocok dengan yang dibaca `nsa`.
  - **Qdrant `scientific_articles` itu GLOBAL/shared lintas-sesi & lintas-tenant.** Maka
    self-heal data sampah/duplikat HARUS dilakukan **by-DOI di dalam ingest PEDE** (DOI sama
    = paper sama untuk semua user → aman menyatukan), **BUKAN** session-scoped delete di
    `nsa` Sync (akan menghapus paper milik peneliti/sesi lain → kehilangan data lintas-tenant).
    Penyebab umum duplikat: paper sama dapat `article_id` beda antar-run (hash-file saat
    DOI belum ke-resolve → DOI-based setelah CrossRef). Sudah ditangani self-heal by-DOI.

Kredensial untuk verifikasi langsung (read-only, JANGAN bocorkan) ada di `/home/adb/awangga/.env`:
`MONGO_URI`, `DB_NAME` (default `slr_agentic_db`), `QDRANT_ENDPOINT`+`QDRANT_API_KEY`
(catatan: kode `pede` baca `QDRANT_URL`, jadi map `QDRANT_ENDPOINT`→`QDRANT_URL` saat run),
`NEO4JURI`/`NEO4JUSER`/`NEO4JPASSWORD`, plus `GHPAT`/`TELEGRAM_BOT_TOKEN`/`CHAT_ID`.

## Deploy & distribusi (GitHub Pages, domain if.co.id)

- **BACKEND `nsa`** → di-**build via GitHub workflow**; file hasil build (binary) di-**upload ke
  `https://if.co.id/download/`** dari repo **`github.com/ifcoid/download`** (GitHub Pages, branch
  `main`). User yang menjalankan backend **LOKAL** (mis. `http://localhost:50607`) **mengunduh
  binary dari sini**. Tiap commit `ifcoid/download` menyebut SHA `nsa` sumbernya (mis. pesan
  "Update SLR backend binaries (commit: <nsa-sha>)"). (Selain itu nsa juga bisa jalan di fly.io.)
- **FRONTEND `slr`** → di-**deploy langsung ke `https://if.co.id/slr/`** dari repo
  **`github.com/ifcoid/slr`** (GitHub Pages). `notify.yml` mengirim Telegram saat Pages deploy
  sukses. CATATAN: `apiBaseURL` default frontend = `http://localhost:50607/api` → user yang
  pakai backend lokal, frontend if.co.id/slr menunjuk ke localhost-nya sendiri.
- **WAJIB diverifikasi SEBELUM MEMBALAS pelapor bug** (atau mengklaim fix "sudah live") — JANGAN
  cukup "build hijau" di repo sumber. Untuk **backend (`if.co.id/download`)**, pastikan ketiganya:
  1. binary = **commit TERBARU `ifcoid/download`** —
     `GET /repos/ifcoid/download/commits?per_page=1`;
  2. **GitHub Pages build SUKSES** untuk commit itu —
     `GET /repos/ifcoid/download/pages/builds/latest` → `status:"built"`, `error:null`, dan
     `.commit` == SHA commit terbaru download;
  3. **pesan commit `ifcoid/download` memuat SHA `nsa` TERBARU** (format
     `Update SLR backend binaries (commit: <nsa-sha>)`) → artinya binary yang disajikan benar
     hasil build commit nsa terbaru. Bandingkan dgn `git -C nsa rev-parse HEAD` /
     `GET /repos/ifcoid/nsa/commits?per_page=1`.
  Untuk **frontend (`if.co.id/slr`)**: `…/ifcoid/slr/pages/builds/latest` `status:built` + `curl`
  file live (mis. `if.co.id/slr/js/...`) dan grep string dari commit terbaru. Auth pakai `GHPAT`.
  Kalau belum hijau/match → **JANGAN balas**; tunggu workflow build + Pages selesai dulu.
- **Implikasi untuk user backend-lokal:** push `nsa` saja TIDAK cukup — mereka harus **unduh
  ulang binary** dari if.co.id/download (tunggu workflow build + Pages selesai). Push `slr` →
  frontend live di if.co.id/slr (cukup user Ctrl+F5).

## Notifikasi Telegram (langsung via Bot API)

TIDAK memakai MCP server (cocote) lagi. Notifikasi progres dikirim **langsung ke
Telegram Bot API** memakai kredensial dari file `.env` di folder `awangga`:
`TELEGRAM_BOT_TOKEN` dan `CHAT_ID`.

### Cara kirim notif
```bash
set +x
export $(grep -E '^(TELEGRAM_BOT_TOKEN|CHAT_ID)=' /home/adb/awangga/.env | xargs)
curl -s "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
  --data-urlencode "chat_id=${CHAT_ID}" \
  --data-urlencode "text=<pesan progres ringkas dalam Bahasa Indonesia>"
```

### Aturan
- Kirim notif saat: **milestone selesai**, **error/blocker**, atau **butuh keputusan**.
- Pesan ringkas, jelas, Bahasa Indonesia. **Jangan bocorkan token** (redact `***` di log;
  jalankan `set +x` sebelum meng-export kredensial).
- Notif Telegram hanya **satu arah** (pemberitahuan). Untuk **pertanyaan/keputusan**,
  tetap tanyakan **jelas dan lengkap di Claude Code** sebagai teks; jangan mengandalkan
  balasan dari Telegram.
- Kalau `.env` / kredensial tidak ada, lewati notif dan lanjutkan; jangan gagal karenanya.

## Debugging: CEK STATE DATABASE DULU sebelum menyalahkan deploy/server

Pelajaran (2026-06-16): saat sesuatu tampak "tidak ter-update" / stale, **JANGAN
langsung menyalahkan deployment / cache / server**. Periksa dulu state yang tersimpan
di MongoDB dan logika persistence-nya. (Pada kasus "Audit Ulang PICO seperti ter-skip",
saya 2-3 kali keliru menyalahkan deploy/fly.io padahal akarnya bug `omitempty` di DB.)

### Gotcha `omitempty` + `$set` (PENTING)
- `MongoRepository.UpdateSession` memakai `bson.M{"$set": session}`. Field dengan tag
  `bson:"...,omitempty"` yang di-set ke **nil / false / 0 / ""** akan **DIBUANG** dari
  dokumen `$set`, sehingga nilai lama di DB **TIDAK ter-clear**.
- Untuk benar-benar meng-CLEAR field session: pakai **`$unset` eksplisit** (contoh:
  `ClearPICOAudit`, `ClearManuscript`), ATAU **jangan beri `omitempty`** pada field yang
  di-toggle (mis. bool seperti `RescreenPending`).
- `UpdateSession` punya 150+ pemanggil yang load→modify→save, jadi JANGAN ubah ke
  `ReplaceOne` tanpa audit menyeluruh (risiko clobber struct parsial).

### Checklist saat "kok tidak berubah / state nyangkut"
1. Lihat dokumen aktual di Mongo (`slr_sessions`, dll) — apa field-nya benar berubah?
2. Cek apakah field itu `omitempty` dan sedang di-set ke nilai zero (nil/false/"").
3. Cek log runtime: langkahnya benar dijalankan atau di-SKIP (mis. `firstAuditPass`)?
4. BARU setelah 1-3 bersih, curigai deploy/cache/CI (cek commit SHA yang live + log).

## Arsitektur WAJIB: HITL, xAI, Neuro-Symbolic, Multi-tenant

Setiap fitur yang menyentuh **keputusan ilmiah** (screening, audit, inklusi/eksklusi,
ekstraksi, sintesis, manuskrip) WAJIB memenuhi empat invariant ini:

- **HITL (Human-in-the-Loop):** AI **mengusulkan/menandai**, MANUSIA **memutuskan**.
  Jangan auto-apply keputusan inklusi/eksklusi tanpa konfirmasi manusia. Sediakan **gate**
  yang memblok kemajuan sampai keputusan manusia lengkap.
- **xAI (Explainable):** setiap flag/keputusan AI membawa **provenance** yang bisa diaudit
  (sumber sinyal + bukti + **klausa kriteria yang dikutip**), tersimpan & bisa diekspor.
- **Neuro-Symbolic:** gabungkan **aturan simbolik deterministik** (diturunkan dari
  definisi/kriteria yang TERSIMPAN di sesi) DENGAN **penilaian neural (LLM)**. Aturan
  menjamin recall pada kasus pasti; LLM menangani nuansa. LLM **bukan** satu-satunya hakim.
- **Multi-tenant — TIDAK boleh hardcode:** sistem dipakai banyak peneliti dengan PICO
  berbeda. SEMUA kriteria/aturan/ambang HARUS berasal dari **DATA sesi** (`PICODefinitions`,
  `AuditScopeRules`, dll) yang **bisa diedit user** — JANGAN menanam aturan review-spesifik
  di kode/prompt. Butuh aturan baru → beri **mekanisme edit (HITL)** + simpan di sesi.
- **Self-heal, BUKAN edit DB manual:** koreksi data dilakukan lewat **aksi UI normal**
  (tombol/HITL); JANGAN pernah menyuruh user mengedit MongoDB langsung. State tak-konsisten/
  legacy harus **pulih sendiri** saat operasi rutin (mis. reconcile both-true flag saat
  Sync). Dan jaga **invariant di SETIAP titik tulis**: bila dua field saling-eksklusif,
  set satu → **clear lawannya** (mis. `full_text_retrieved` vs `inaccessible`).

## Reproducible Error (xAI): setiap kegagalan WAJIB bisa di-reproduksi user→developer

Mirip aturan **"steps to reproduce"** pada pull request / issue GitHub: **setiap error yang
melibatkan LLM/provider WAJIB bisa diproduksi-ulang oleh USER langsung dari UI** (tanpa akses
ke server/DB/log developer), agar laporan ke developer langsung *actionable*. Ini **perluasan
invariant xAI** (provenance + **reproducibility**), bukan opsional.

- **Tangkap jejak LENGKAP saat call LLM gagal:** `{step, role, provider, NAMA model, system
  prompt LENGKAP, user prompt LENGKAP, error mentah, durasi, jumlah char/estimasi token,
  timestamp}`. Simpan di store khusus (mis. capped / last-failed per sesi) — JANGAN gemukkan
  `xai_log` (full-text akan meledak; `UserPromptPreview` di situ sengaja dipotong 500 char).
- **Sediakan REPLAY dari UI:** endpoint + tombol **"Uji Coba"** yang mengirim ULANG prompt
  asli (boleh diedit, mis. dipotong full-text-nya) ke provider lalu menampilkan **respons
  mentah / error apa adanya** + timing. Inilah cara user & developer pinpoint *error sebelah
  mana*: context overflow (0 token balik), JSON rusak, 429, 401, base URL salah, dll.
- **Redaksi rahasia:** API key/secret TIDAK pernah ikut di payload debug/replay yang dikirim
  ke klien (redact `***`). Prompt boleh memuat data paper (milik sesi user sendiri).
- **Pesan error self-explanatory + actionable:** sebut **NAMA model** + dugaan akar + langkah
  perbaikan, sehingga user paham tanpa membaca kode (selaras atribusi model xAI).
- **JANGAN telan error diam-diam:** `console.warn`/`catch{}` tanpa surfacing = melanggar
  invariant ini (user tak bisa melaporkan apa yang tak terlihat). Bedakan dari notif Telegram
  (satu-arah, untuk progres) — reproduksi error adalah jejak yang TERSIMPAN + bisa di-replay.

## Pelaporan bug & cara baca inbox @BugLaporBot (operasional)

Backend bisa dijalankan **LOKAL per-user** (BUKAN satu deploy terpusat), jadi laporan bug TIDAK
boleh mengandalkan DB backend (developer tak bisa baca Mongo lokal user). Kanal terpusat satu-
satunya = bot Telegram **@BugLaporBot**. Konten laporan DIBAWA lewat pesan/FILE dari user ke bot.

### Alur lapor (user, frontend `slr`)
- Tombol **🐞 Lapor / Debug Bug** (di ☰ Menu header, gerbang error M7, panel ERROR, Live Log,
  Pengaturan, Health) → modal `js/components/llmdebug.js`.
- Bug tampilan/UX: user isi **Keterangan**. Error LLM: detail prompt+error terisi otomatis
  (dari koleksi `llm_call_debug`, di-tangkap `xaiLoggingClient` saat call gagal) + bisa
  **Uji Coba** (replay ASYNC: `POST /api/llm/replay` → poll `GET /api/llm/replay/{id}`).
- **State ditangkap OTOMATIS** (session, modul/step dari `display-status`, **`api_base`**=
  backend yang dipakai, url, viewport, userAgent). Klik **Report Bug** → frontend fetch
  `GET /sessions/{id}/diagnostic` dan **menyisipkan SNAPSHOT STATE DB** (status/error/flags/
  counts/roles/log terakhir, **`backend_version`**=commit nsa binary, TANPA rahasia) ke file →
  unduh **file .txt LENGKAP** → buka `t.me/BugLaporBot` → user **lampirkan file** & kirim. (FILE,
  bukan deep-link `?text=`, agar utuh.) **Laporan jadi swasembada: developer TAK perlu akses
  Mongo user** (krusial karena backend bisa lokal per-user → connection-string tak boleh & tak
  perlu dibocorkan). `api_base` memberi tahu backend mana bila perlu hit endpoint langsung.
- **Bekal reproduksi (jangan dikurangi):** laporan WAJIB memuat cukup untuk repro tanpa tanya
  balik — khususnya bug TAMPILAN/UX:
  1. **Error console JS** — di-tangkap sedini mungkin via script di `<head>` (`window.onerror` +
     `unhandledrejection` + hook `console.error` → ring-buffer 30 `window.__errLog`). Untuk bug
     UI, ini sering kuncinya.
  2. **Versi BACKEND** — `backend_version` (commit nsa) di `/diagnostic`; di-stamp saat build via
     `ldflags "-X 'nsa/internal/version.Commit=<sha>'"` (di `compile.yml`/`release.yml`). Tanpa
     stamp = `"dev"`. WAJIB tahu binary mana (backend lokal bisa BASI — kasus balqis).
  3. **Versi FRONTEND** — fingerprint deploy (`version.json` bila ada + `Last-Modified`/`ETag`
     situs), karena slr Pages "deploy from branch" (tanpa SHA build).
  Kalau menambah fitur yang bisa gagal di UI, pastikan jejaknya tertangkap salah satu di atas.
- Bot **auto-reply "✅ diterima"** lewat poller di bawah.

### Cara DEVELOPER/Claude baca laporan
- Token bot di `/home/adb/awangga/.env` → `BUGLAPOR_BOT_TOKEN` (JANGAN bocorkan; redact `***`,
  `set +x` sebelum export). TIDAK di-hardcode di kode (frontend hanya pakai username publik).
- Poller: `/home/adb/awangga/bugbot/poll.sh` — **SATU-SATUNYA konsumen `getUpdates`**. JANGAN
  panggil `getUpdates` manual di tempat lain (offset bentrok → laporan hilang/ke-skip).
- Dipakai **ON-DEMAND**: saat user bilang "cek inbox", jalankan `bash /home/adb/awangga/bugbot/poll.sh`.
  Ia: (1) balas "diterima" tiap pesan, (2) unduh file lampiran ke `bugbot/files/`, (3) catat
  ringkas ke `bugbot/inbox.jsonl`, (4) majukan `bugbot/offset`. Lalu BACA `inbox.jsonl` + file
  di `files/` → perbaiki. (Real-time opsional: user sendiri pasang cron `* * * * * .../poll.sh`.)
- **Balas pelapor:** `bash /home/adb/awangga/bugbot/reply.sh <chat_id|last> "<pesan>"` (kirim via
  @BugLaporBot; `last` = pengirim laporan TERAKHIR di inbox.jsonl). **SEBELUM membalas (bila pesan
  mengklaim fix sudah live): WAJIB verifikasi deploy** dulu — binary `if.co.id/download` = commit
  terbaru + Pages SUKSES + pesan commit download memuat SHA `nsa` terbaru (lihat seksi "Deploy &
  distribusi"). Jangan janjikan fix yang belum benar-benar ter-deploy. CATATAN: mengirim pesan ke
  USER NYATA adalah aksi yang butuh OTORISASI manusia — assistant tak boleh kirim sendiri tanpa
  izin eksplisit; sodorkan perintahnya agar user yang menjalankan (atau user menambah permission).
- **Skrip bugbot di-version di repo PRIVATE `github.com/ifcoid/bugbot`** (`poll.sh`, `reply.sh`,
  README, `.gitignore`, `.env.example`). **Data user & rahasia TIDAK di-commit** (di-`.gitignore`):
  `inbox.jsonl`, `offset`, `files/`, `poll.log`, `.lock`, `.env` — tetap LOKAL di folder
  `/home/adb/awangga/bugbot/` (berisi laporan user = privasi; token di `.env`). Jalankan tetap
  dari folder lokal itu; repo hanya untuk versioning + backup skripnya. Ingat: poll.sh = satu
  konsumen `getUpdates` → **jalankan di SATU mesin saja**.

## Validitas metodologi SLR (publikasi Q1): protokol STABIL, preserve ≠ reset

SLR yang defensible (PRISMA + reproducibility) menuntut: **protokol ditetapkan a priori
dan diterapkan SERAGAM ke semua studi.** Maka untuk navigasi-mundur / koreksi keputusan,
DEFAULT adalah **PRESERVE**, bukan reset. Reset penuh justru **merusak validitas** kecuali
itu amendemen protokol yang disengaja & terdokumentasi.

- **Protokol ekstraksi (framework/data-items) WAJIB dipertahankan, JANGAN regenerate**
  saat set paper berubah. Studi yang diekstrak sebelum vs sesudah dengan form berbeda =
  inkonsisten = tidak valid; protokol yang berubah mengikuti data = HARKing (red flag Q1).
  → `runFrameworkL1` TIDAK boleh memanggil `RecommendFramework` bila `FrameworkSelection`
  sudah ada DAN tak ada feedback revisi framework eksplisit. Regenerasi diam-diam = BUG
  metodologis, bukan fitur.
- **Data ekstraksi yang sudah terkumpul WAJIB dipertahankan**; ekstrak HANYA paper yang
  baru ter-include (LLM non-deterministik → re-ekstraksi paper tak berubah menggeser nilai
  → merusak reproducibility). Filter `finalIncludedPapers` memang dinamis (baca keputusan
  M6 live) — manfaatkan itu untuk ekstraksi inkremental, jangan wipe lalu re-ekstrak semua.
- **`ResetModul7` (wipe penuh + regenerate) BUKAN tombol "balik sedikit".** Ia hanya untuk
  **amendemen protokol yang disengaja** (tipe studi / data-item baru) dan saat itu WAJIB
  re-ekstrak SEMUA studi uniform. Beri label + konfirmasi eksplisit yang membedakannya dari
  koreksi biasa. Koreksi include/exclude biasa → jalur PRESERVE (protokol+data tetap).
- **Catat ALASAN setiap perubahan include/exclude pasca-ekstraksi** ke audit trail
  (provenance/xAI) + update hitungan PRISMA. "Terasa sedikit" BUKAN justifikasi — melonggarkan
  kriteria post-hoc demi menggelembungkan N = selection bias. Bila kriteria memang direvisi,
  itu amendemen terdokumentasi + **re-screening SIMETRIS** (terapkan ke SEMUA record), bukan
  menambah beberapa paper yang kebetulan.

## UX WAJIB untuk operasi AI yang lama (progress, toast, atribusi model)

Setiap aksi yang memanggil LLM dan butuh waktu (screening, audit, saran, sintesis) HARUS
memberi umpan-balik agar user tahu prosesnya jalan dan bisa mengevaluasi modelnya:

- **JANGAN sinkron-lama di handler HTTP** (risiko timeout proxy fly.io). Pola: handler
  memulai **job background** (goroutine), balas segera `{started,total}`, frontend **poll**
  endpoint hasil tiap ~2 dtk. Pola sama dengan pipeline screening (`ExecuteAsync`).
- **Progress per-item ke Live Log**: `logger.Logf(sessionID, ...)` → tersiar via WebSocket
  `/ws/logs/{id}` ke panel Live Log. Log mulai, **tiap item** (mis. `Paper i/N: <judul>`),
  dan selesai. Inilah cara user melihat "lagi di paper A".
- **Toast** (frontend `showToast` di `js/ui.js`) saat **mulai** ("AI menganalisis N…") dan
  **selesai** ("✅ N saran via <model>"). Bukan `alert()`.
- **Atribusi MODEL (xAI)**: SELALU sertakan **provider + NAMA MODEL asli** di output AI
  (di hasil + log) — bukan hanya nama provider, karena **satu provider bisa beberapa model**.
  Ambil dari `client.ModelName()` (mis. `"openai/<model>"` → ambil bagian setelah `/`) dan
  gabung dengan provider role, mis. `groq / llama-3.3-70b-versatile`. Provider berasal dari
  **role configurable** (`LLMRoles`: Reviewer/Supervisor/Brain/**Auditor**), bukan hardcode.
- **Tombol di-disable saat diklik** (anti dobel-klik), tampilkan progres di teks tombol
  (mis. `🤖 3/16…`), aktifkan lagi hanya saat selesai/error. Tambahkan **guard server**
  (job in-flight → jangan mulai job baru) sebagai jaring kedua.

## Model pengujian (penting)

Korektnya **perilaku** TIDAK bisa diklaim hanya dari unit test + build hijau. **User =
tester manusia nyata** yang menjalankan alur sungguhan lalu mengirim output untuk
dievaluasi (AI tak bisa menguji interaktif tanpa user).

- Setelah implementasi: jalankan build + unit test + (bila relevan) verifikasi mekanis
  (mis. `pdflatex`), lalu nyatakan **"siap diuji"** — BUKAN "selesai/matang".
- **"Selesai/matang" hanya setelah user menjalankan test nyata** dan hasilnya sesuai.
- Bedakan unit test (komponen) vs verifikasi perilaku end-to-end (butuh user).
