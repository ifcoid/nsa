// Package notify mengirim notifikasi HITL ke Telegram saat pipeline butuh
// tindakan user (gate WAITING / error / selesai). Dipakai untuk alur web-based:
// user tak perlu memantau terus — backend memberi tahu kapan harus buka web.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// ColabEmbedURL membuka notebook server embedding langsung di Google Colab dari
// repo publik ifcoid/pede (bisa di desktop maupun Chrome Android).
const ColabEmbedURL = "https://colab.research.google.com/github/ifcoid/pede/blob/main/notebooks/embed_server_colab.ipynb"

// ColabIngestURL membuka notebook PEDE (vektorisasi PDF -> Qdrant) di Google Colab.
const ColabIngestURL = "https://colab.research.google.com/github/ifcoid/pede/blob/main/notebooks/pede_colab.ipynb"

// webBaseURL untuk tautan "buka di web" pada pesan. Bisa dioverride via env.
func webBaseURL() string {
	if u := strings.TrimSpace(os.Getenv("WEB_BASE_URL")); u != "" {
		return u
	}
	return "https://if.co.id/slr"
}

// Telegram mengirim satu pesan ke CHAT_ID via TELEGRAM_BOT_TOKEN. No-op (aman)
// bila env belum diset. Fire-and-forget: kegagalan tak menghentikan pipeline.
func Telegram(text string) {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	chatID := strings.TrimSpace(os.Getenv("CHAT_ID"))
	if token == "" || chatID == "" {
		return
	}
	body, _ := json.Marshal(map[string]interface{}{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token), bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		log.Printf("[notify] Telegram gagal: %v", err)
		return
	}
	resp.Body.Close()
}

// GateMessage menyusun pesan HITL ramah berdasar status sesi.
func GateMessage(sessionID, status string) string {
	web := webBaseURL()
	desc := describe(status)
	if desc == "" {
		return "" // status bukan gate yang perlu notif
	}
	return fmt.Sprintf("🔔 <b>Sesi %s butuh tindakan Anda</b>\n%s\n\nBuka: %s", sessionID, desc, web)
}

func describe(status string) string {
	switch {
	case status == "COMPLETED":
		return "🎉 Pipeline SELESAI — manuskrip final siap diunduh (.md/.tex/.bib)."
	case status == "M6_STEP2_WAITING_EMBED":
		return "⏸️ Screening dijeda: endpoint embedding (BGE-M3) mati.\n" +
			"▶️ Nyalakan Colab (bisa di Chrome Android): " + ColabEmbedURL + "\n" +
			"Lalu Run all → salin EMBED_ENDPOINT → masukkan di web."
	case status == "M6_STEP2_WAITING_RESOLUTION":
		return "🧪 Batch full-text screening selesai. Resolusi konflik / Setuju & Lanjut di web."
	case status == "M6_STEP1_WAITING_SYNC":
		return "🔗 Modul 6 L1: akuisisi siap. Vektorisasi PDF → Qdrant lewat Colab PEDE: " + ColabIngestURL +
			"\nSetelah selesai, tekan 'Sinkronkan dengan Qdrant' di web."
	case status == "M5_STEP3_WAITING_RESOLUTION":
		return "🧪 Batch screening abstrak selesai. Resolusi konflik di web."
	case strings.HasSuffix(status, "_NEEDS_REVISION"):
		return fmt.Sprintf("✏️ Perlu revisi (%s). Tinjau di web.", status)
	case strings.HasSuffix(status, "_ERROR"):
		return fmt.Sprintf("⚠️ Error pada tahap %s. Cek & retry di web.", strings.TrimSuffix(status, "_ERROR"))
	case strings.HasSuffix(status, "_WAITING_APPROVAL"):
		return fmt.Sprintf("✅ Menunggu persetujuan: %s. Tinjau & approve di web.", strings.TrimSuffix(status, "_WAITING_APPROVAL"))
	default:
		return ""
	}
}
