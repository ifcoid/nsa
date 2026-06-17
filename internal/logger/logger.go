package logger

import (
	"fmt"
	"sync"
)

// maxHistoryPerSession membatasi jumlah baris log yang disimpan per sesi untuk replay.
// Cukup untuk konteks beberapa batch terakhir tanpa menahan memori berlebihan.
const maxHistoryPerSession = 800

var (
	subscribers = make(map[string]map[chan string]bool)
	// history menyimpan ring-buffer baris log terakhir per sesi. Tujuannya: klien WebSocket
	// yang baru connect (mis. setelah auto-resume saat startup, refresh tab, atau restart
	// server) bisa langsung me-REPLAY konteks terbaru — bukan panel Live Log kosong karena
	// log sudah terbit sebelum ia berlangganan. (Logger ini ephemeral; tanpa histori, log
	// yang disiarkan saat tak ada subscriber hilang selamanya.)
	history = make(map[string][]string)
	mutex   sync.Mutex
)

// Subscribe mendaftarkan channel baru untuk menerima log dari sesi tertentu, sekaligus
// mengembalikan SNAPSHOT histori (backlog) baris yang sudah terbit. Backlog diambil di
// bawah kunci yang sama dengan registrasi channel → tidak ada celah / duplikasi: backlog
// = semua yang ter-log SEBELUM titik ini, channel = semua yang SETELAHnya.
func Subscribe(sessionID string) (chan string, []string) {
	mutex.Lock()
	defer mutex.Unlock()

	ch := make(chan string, 100) // Buffer 100 pesan
	if subscribers[sessionID] == nil {
		subscribers[sessionID] = make(map[chan string]bool)
	}
	subscribers[sessionID][ch] = true

	var backlog []string
	if h := history[sessionID]; len(h) > 0 {
		backlog = make([]string, len(h))
		copy(backlog, h)
	}
	return ch, backlog
}

// Unsubscribe menghapus channel dari daftar pendengar. Histori SENGAJA tidak dihapus di
// sini agar tetap bisa di-replay ke klien berikutnya (mis. saat user refresh halaman).
func Unsubscribe(sessionID string, ch chan string) {
	mutex.Lock()
	defer mutex.Unlock()

	if _, ok := subscribers[sessionID]; ok {
		delete(subscribers[sessionID], ch)
		close(ch)
		if len(subscribers[sessionID]) == 0 {
			delete(subscribers, sessionID)
		}
	}
}

// Logf mencetak ke terminal (stdout), menyimpan ke histori, dan menyiarkan ke semua
// websocket subscriber sesi tersebut.
func Logf(sessionID string, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)

	// Tetap cetak di terminal backend
	fmt.Println(message)

	mutex.Lock()
	defer mutex.Unlock()

	// Simpan ke ring-buffer histori (untuk replay ke klien yang connect belakangan).
	h := append(history[sessionID], message)
	if len(h) > maxHistoryPerSession {
		trimmed := make([]string, maxHistoryPerSession)
		copy(trimmed, h[len(h)-maxHistoryPerSession:])
		h = trimmed
	}
	history[sessionID] = h

	// Broadcast ke subscriber yang sedang terhubung (non-blocking agar tak macet).
	if subs, ok := subscribers[sessionID]; ok {
		for ch := range subs {
			select {
			case ch <- message:
			default:
				// Channel penuh, abaikan
			}
		}
	}
}

// Log adalah wrapper jika tidak butuh formatting
func Log(sessionID string, message string) {
	Logf(sessionID, "%s", message)
}

// ClearHistory menghapus histori log sebuah sesi (mis. saat sesi dihapus). Opsional.
func ClearHistory(sessionID string) {
	mutex.Lock()
	defer mutex.Unlock()
	delete(history, sessionID)
}
