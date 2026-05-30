package agent

import (
	"regexp"
	"strings"
)

var thinkRegex = regexp.MustCompile(`(?s)<(?:think|thought)>.*?(?:</(?:think|thought)>|$)`)

// Info: File ini menampung utilitas bersama yang digunakan oleh seluruh agen cerdas (Pico, Criteria, Screener, dll)

// CleanJSONResponse adalah fungsi pembantu universal untuk membersihkan
// bungkusan markdown (triple backticks) yang sering ikut keluar dari output LLM.
// Fungsi ini diekspor (huruf kapital di awal) agar bisa dipakai oleh semua file di package agent.
func CleanJSONResponse(rawResponse string) string {
	rawResponse = strings.TrimSpace(rawResponse)

	// Hapus tag <think>...</think> secara utuh (mendukung unclosed tag akibat token limit)
	rawResponse = thinkRegex.ReplaceAllString(rawResponse, "")
	rawResponse = strings.TrimSpace(rawResponse)

	// 0. Hapus blok referensi grounding jika ada, agar tidak mengacaukan parser
	if refIdx := strings.Index(rawResponse, "=== GROUNDING REFERENCES ==="); refIdx != -1 {
		rawResponse = strings.TrimSpace(rawResponse[:refIdx])
	}

	// 1. Jika rawResponse sudah berbentuk JSON murni, langsung gunakan fallback extraction
	// untuk memastikan kita memotong string yang diapit {} atau [] (berjaga-jaga jika ada teks ekstra di akhir)
	if (strings.HasPrefix(rawResponse, "{") || strings.HasPrefix(rawResponse, "[")) && 
	   (strings.HasSuffix(rawResponse, "}") || strings.HasSuffix(rawResponse, "]")) {
		// Do nothing, proceed to fallback extractor below
	} else {
		// 2. Coba cari blok markdown ```json ... ```
		startBlock := strings.Index(rawResponse, "```json")
		if startBlock != -1 {
			startBlock += 7
			// Gunakan LastIndex agar tidak terpotong oleh ``` di dalam JSON
			endBlock := strings.LastIndex(rawResponse, "```")
			if endBlock != -1 && endBlock > startBlock {
				return strings.TrimSpace(rawResponse[startBlock:endBlock])
			}
		}

		// 3. Coba cari blok markdown ``` ... ``` secara umum
		startBlock = strings.Index(rawResponse, "```")
		if startBlock != -1 {
			startBlock += 3
			// Jika baris pertama adalah nama bahasa (misal: json), kita bisa skip
			if strings.HasPrefix(rawResponse[startBlock:], "json") {
				startBlock += 4
			}
			endBlock := strings.LastIndex(rawResponse, "```")
			if endBlock != -1 && endBlock > startBlock {
				return strings.TrimSpace(rawResponse[startBlock:endBlock])
			}
		}
	}

	// 4. Fallback: Gunakan strings.IndexAny untuk mengekstrak hanya bagian JSON
	
	// Kita cari index pertama dari { atau [
	startIdx := strings.IndexAny(rawResponse, "{[")
	if startIdx == -1 {
		return rawResponse
	}
	
	// Kita cari index terakhir dari } atau ]
	endIdx := strings.LastIndexAny(rawResponse, "}]")
	if endIdx == -1 || endIdx < startIdx {
		return rawResponse
	}

	return strings.TrimSpace(rawResponse[startIdx : endIdx+1])
}
