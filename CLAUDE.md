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
