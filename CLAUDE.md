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
