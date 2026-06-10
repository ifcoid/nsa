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
	
	// Hapus halusinasi line continuation pada string JSON (misal: "string" \ \n "lanjutan")
	// Regex ini mencocokkan: tanda kutip penutup, spasi opsional, backslash, spasi/newline, tanda kutip pembuka
	lineContinuationRegex := regexp.MustCompile(`"\s*\\\s*\n\s*"`)
	rawResponse = lineContinuationRegex.ReplaceAllString(rawResponse, "")

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
		// 2. Cari blok markdown ```json ... ``` TERAKHIR
		// Ini penting untuk mengatasi kasus dimana LLM halusinasi, terpotong, lalu meminta maaf dan membuat blok json baru di akhir.
		startIdx := strings.LastIndex(rawResponse, "```json")
		if startIdx != -1 {
			startIdx += 7
			endIdx := strings.Index(rawResponse[startIdx:], "```")
			if endIdx != -1 {
				return strings.TrimSpace(rawResponse[startIdx : startIdx+endIdx])
			}
		}

		// 3. Coba cari blok markdown ``` ... ``` TERAKHIR jika tanpa tag json
		// Kita iterasi saja semua blok dan ambil yang terakhir
		var lastBlock string
		temp := rawResponse
		for {
			s := strings.Index(temp, "```")
			if s == -1 {
				break
			}
			e := strings.Index(temp[s+3:], "```")
			if e == -1 {
				break
			}
			lastBlock = temp[s+3 : s+3+e]
			if strings.HasPrefix(strings.TrimSpace(lastBlock), "json") {
				lastBlock = strings.TrimSpace(lastBlock)[4:]
			}
			temp = temp[s+3+e+3:]
		}
		
		if lastBlock != "" {
			return strings.TrimSpace(lastBlock)
		}
	}

	// 4. Fallback: Ekstrak blok JSON terakhir (menggunakan brace matching)
	// Berguna jika LLM melakukan self-correction: json {...} Wait... json {...}
	endIdx := strings.LastIndexAny(rawResponse, "}]")
	if endIdx != -1 {
		charEnd := rawResponse[endIdx]
		charStart := byte('{')
		if charEnd == ']' {
			charStart = '['
		}

		braceCount := 0
		startIdx := -1
		for i := endIdx; i >= 0; i-- {
			if rawResponse[i] == charEnd {
				braceCount++
			} else if rawResponse[i] == charStart {
				braceCount--
				if braceCount == 0 {
					startIdx = i
					break
				}
			}
		}

		if startIdx != -1 {
			return strings.TrimSpace(rawResponse[startIdx : endIdx+1])
		}
	}

	// 5. Fallback terakhir jika logika brace gagal
	startIdx := strings.IndexAny(rawResponse, "{[")
	if startIdx == -1 {
		return rawResponse
	}
	endIdx = strings.LastIndexAny(rawResponse, "}]")
	if endIdx == -1 || endIdx < startIdx {
		return rawResponse
	}

	return strings.TrimSpace(rawResponse[startIdx : endIdx+1])
}
