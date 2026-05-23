package agent

import (
	"regexp"
	"strings"
)

// Info: File ini menampung utilitas bersama yang digunakan oleh seluruh agen cerdas (Pico, Criteria, Screener, dll)

// CleanJSONResponse adalah fungsi pembantu universal untuk membersihkan
// bungkusan markdown (triple backticks) yang sering ikut keluar dari output LLM.
// Fungsi ini diekspor (huruf kapital di awal) agar bisa dipakai oleh semua file di package agent.
func CleanJSONResponse(rawResponse string) string {
	// 0. Hapus blok referensi grounding jika ada, agar tidak mengacaukan parser
	if refIdx := strings.Index(rawResponse, "=== GROUNDING REFERENCES ==="); refIdx != -1 {
		rawResponse = rawResponse[:refIdx]
	}

	// 1. Coba cari blok markdown ` + "`" + `json ... ` + "`" + `
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

	// 3. Fallback: Gunakan regex untuk mencari awal struktur JSON yang valid
	// Mencari '[' atau '{' yang diikuti oleh spasi/newline/tab lalu karakter '"' atau '{' atau '['
	re := regexp.MustCompile(`([\[\{])\s*["\{\[]`)
	loc := re.FindStringIndex(rawResponse)
	startIdx := -1
	if loc != nil {
		startIdx = loc[0]
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
