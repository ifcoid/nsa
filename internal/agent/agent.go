package agent

import (
	"strings"
)

// Info: File ini menampung utilitas bersama yang digunakan oleh seluruh agen cerdas (Pico, Criteria, Screener, dll)

// CleanJSONResponse adalah fungsi pembantu universal untuk membersihkan
// bungkusan markdown (triple backticks) yang sering ikut keluar dari output LLM.
// Fungsi ini diekspor (huruf kapital di awal) agar bisa dipakai oleh semua file di package agent.
func CleanJSONResponse(rawResponse string) string {
	// 1. Coba cari blok markdown ```json ... ```
	startBlock := strings.Index(rawResponse, "```json")
	if startBlock != -1 {
		startBlock += 7
		endBlock := strings.Index(rawResponse[startBlock:], "```")
		if endBlock != -1 {
			return strings.TrimSpace(rawResponse[startBlock : startBlock+endBlock])
		}
	}

	// 2. Coba cari blok markdown ``` ... ``` secara umum
	startBlock = strings.Index(rawResponse, "```")
	if startBlock != -1 {
		startBlock += 3
		// Jika baris pertama adalah nama bahasa (misal: json), kita bisa skip
		if strings.HasPrefix(rawResponse[startBlock:], "json") {
			startBlock += 4
		}
		endBlock := strings.Index(rawResponse[startBlock:], "```")
		if endBlock != -1 {
			return strings.TrimSpace(rawResponse[startBlock : startBlock+endBlock])
		}
	}

	// 3. Fallback: Temukan indeks karakter awal JSON (array '[' atau object '{')
	// Kita cari "[\n" atau "{\n" atau "[ " atau "{ " untuk menghindari terpotong di sitasi "[1]"
	startIdx := strings.Index(rawResponse, "[\n")
	if startIdx == -1 {
		startIdx = strings.Index(rawResponse, "{\n")
	}
	if startIdx == -1 {
		startIdx = strings.Index(rawResponse, "[ ")
	}
	if startIdx == -1 {
		startIdx = strings.Index(rawResponse, "{ ")
	}
	if startIdx == -1 {
		// Paling mentok, cari yang pertama
		startIdx = strings.IndexAny(rawResponse, "[{")
	}

	if startIdx == -1 {
		return strings.TrimSpace(rawResponse)
	}

	endIdx := strings.LastIndexAny(rawResponse, "]}")
	if endIdx == -1 || endIdx < startIdx {
		return strings.TrimSpace(rawResponse)
	}

	return strings.TrimSpace(rawResponse[startIdx : endIdx+1])
}
