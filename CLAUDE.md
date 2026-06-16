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

## Model pengujian (penting)

Korektnya **perilaku** TIDAK bisa diklaim hanya dari unit test + build hijau. **User =
tester manusia nyata** yang menjalankan alur sungguhan lalu mengirim output untuk
dievaluasi (AI tak bisa menguji interaktif tanpa user).

- Setelah implementasi: jalankan build + unit test + (bila relevan) verifikasi mekanis
  (mis. `pdflatex`), lalu nyatakan **"siap diuji"** — BUKAN "selesai/matang".
- **"Selesai/matang" hanya setelah user menjalankan test nyata** dan hasilnya sesuai.
- Bedakan unit test (komponen) vs verifikasi perilaku end-to-end (butuh user).
