package agent

import (
	"strings"
)

// Info: File ini menampung utilitas bersama yang digunakan oleh seluruh agen cerdas (Pico, Criteria, Screener, dll)

// CleanJSONResponse adalah fungsi pembantu universal untuk membersihkan
// bungkusan markdown (triple backticks) yang sering ikut keluar dari output LLM.
// Fungsi ini diekspor (huruf kapital di awal) agar bisa dipakai oleh semua file di package agent.
func CleanJSONResponse(rawResponse string) string {
	cleanJSON := strings.TrimSpace(rawResponse)

	// Bersihkan prefix markdown jika ada
	cleanJSON = strings.TrimPrefix(cleanJSON, "```json")
	cleanJSON = strings.TrimPrefix(cleanJSON, "```JSON") // Antisipasi jika LLM menulis huruf besar
	cleanJSON = strings.TrimPrefix(cleanJSON, "```")

	// Bersihkan suffix markdown jika ada
	cleanJSON = strings.TrimSuffix(cleanJSON, "```")

	// Potong spasi/pindah baris yang tersisa di ujung-ujung string
	return strings.TrimSpace(cleanJSON)
}
