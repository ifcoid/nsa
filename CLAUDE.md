## Interaksi via Telegram (cocote)

Project ini terhubung ke Telegram lewat MCP server **cocote**. Pertanyaan keputusan
harus bisa dijawab dari Telegram MAUPUN dari Claude Code.

### Aturan keputusan & approval
- Setiap kali butuh **keputusan / approval / pilihan** dari user, **WAJIB panggil
  `ask_approval`** (jangan bertanya sebagai teks akhir giliran).
  - Sediakan `options` sebagai tombol (mis. `["Lanjut Modul 9", "M8b dulu"]`).
  - Set `allow_freetext: true` agar user bisa mengetik instruksi bebas (field `reply`).
  - `columns: 1` kalau label panjang. `timeout_seconds` wajar (mis. 600).
- Untuk pertanyaan terbuka, panggil **`wait_for_reply`**.
- Tulis pertanyaannya **jelas dan lengkap di dalam `question`/`prompt`** supaya
  tetap terbaca di Claude Code (bukan cuma di Telegram).

### Dua kanal jawaban (penting)
- Pertanyaan via `ask_approval`/`wait_for_reply` otomatis muncul di Telegram DAN
  sebagai tool call di Claude Code. User boleh menjawab dari mana saja:
  - Telegram: tap tombol / balas teks.
  - Claude Code: user menekan Esc lalu mengetik jawaban → perlakukan teks itu
    sebagai jawabannya dan lanjutkan (jangan ulang pertanyaan).
- Kalau hasil tool `timed_out: true` ATAU tool dibatalkan (user menjawab langsung
  di Claude Code), gunakan jawaban yang user berikan dan lanjutkan; jangan ngambek
  atau mengulang.

### Progres & status
- Update progres/selesai/error → `notify`. Clone tampilan → `mirror_screen`.

### Catatan
- Tool interaktif hanya jalan jika sesi ini memegang **booking** cocote (mode
  `active`). Jika tool mengembalikan "not booked / send-only", tanyakan langsung
  di Claude Code seperti biasa.

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
