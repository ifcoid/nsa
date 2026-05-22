package agent

import (
	"strings"
)

// Info: File ini menampung utilitas bersama yang digunakan oleh seluruh agen cerdas (Pico, Criteria, Screener, dll)

// CleanJSONResponse adalah fungsi pembantu universal untuk membersihkan
// bungkusan markdown (triple backticks) yang sering ikut keluar dari output LLM.
// Fungsi ini diekspor (huruf kapital di awal) agar bisa dipakai oleh semua file di package agent.
func CleanJSONResponse(rawResponse string) string {
	// Temukan indeks karakter awal JSON (array '[' atau object '{')
	startIdx := strings.IndexAny(rawResponse, "[{")
	if startIdx == -1 {
		return strings.TrimSpace(rawResponse) // Kembalikan apa adanya jika tidak ditemukan
	}

	// Temukan indeks karakter akhir JSON (array ']' atau object '}')
	endIdx := strings.LastIndexAny(rawResponse, "]}")
	if endIdx == -1 || endIdx < startIdx {
		return strings.TrimSpace(rawResponse)
	}

	// Ekstrak hanya bagian JSON-nya saja, membuang semua teks pengantar/penutup
	return rawResponse[startIdx : endIdx+1]
}
