package logger

import (
	"fmt"
	"sync"
)

var (
	subscribers = make(map[string]map[chan string]bool)
	mutex       sync.RWMutex
)

// Subscribe mendaftarkan channel baru untuk menerima log dari sesi tertentu.
func Subscribe(sessionID string) chan string {
	mutex.Lock()
	defer mutex.Unlock()

	ch := make(chan string, 100) // Buffer 100 pesan
	if subscribers[sessionID] == nil {
		subscribers[sessionID] = make(map[chan string]bool)
	}
	subscribers[sessionID][ch] = true
	return ch
}

// Unsubscribe menghapus channel dari daftar pendengar.
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

// Logf mencetak ke terminal (stdout) dan menyiarkan ke semua websocket subscriber sesi tersebut.
func Logf(sessionID string, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	
	// Tetap cetak di terminal backend
	fmt.Println(message)

	// Broadcast ke websocket
	mutex.RLock()
	defer mutex.RUnlock()

	if subs, ok := subscribers[sessionID]; ok {
		for ch := range subs {
			// Gunakan non-blocking send agar tidak macet jika klien lambat membaca
			select {
			case ch <- message:
			default:
				// Channel penuh, abaikan atau log error internal
			}
		}
	}
}

// Log adalah wrapper jika tidak butuh formatting
func Log(sessionID string, message string) {
	Logf(sessionID, "%s", message)
}
