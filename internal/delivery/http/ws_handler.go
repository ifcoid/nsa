package http

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"nsa/internal/logger"
)

// Upgrader untuk menaikkan koneksi HTTP menjadi WebSocket
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Izinkan koneksi dari sembarang asal (CORS frontend)
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// LogStreamHandler menangani koneksi WebSocket untuk streaming log agen.
func LogStreamHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	// Upgrade koneksi
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to websocket: %v\n", err)
		return
	}
	defer conn.Close()

	// Berlangganan (Subscribe) ke log manager
	logChan := logger.Subscribe(sessionID)
	defer logger.Unsubscribe(sessionID, logChan)

	// Goroutine untuk mengirim log ke klien saat log tersedia di channel
	go func() {
		for msg := range logChan {
			err := conn.WriteMessage(websocket.TextMessage, []byte(msg))
			if err != nil {
				// Klien terputus
				return
			}
		}
	}()

	// Tunggu pesan dari klien atau putusnya koneksi.
	// Klien web hanya membaca log, jadi kita blokir fungsi ini sampai error/tutup.
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			// Keluar jika terjadi error baca (contoh: tab browser ditutup)
			break
		}
	}
}
