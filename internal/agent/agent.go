package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"nsa/internal/llm"
	"nsa/internal/logger"
)

var thinkRegex = regexp.MustCompile(`(?s)<(?:think|thought)>.*?(?:</(?:think|thought)>|$)`)

// jsonNoisePatterns berisi prefix/kalimat umum yang sering dihasilkan LLM sebelum JSON.
var jsonNoisePatterns = []string{
	"Here is the JSON",
	"Here's the JSON",
	"Sure, here",
	"Berikut adalah JSON",
	"Berikut JSON",
	"Certainly",
	"Of course",
	"Baik, berikut",
	"Tentu, berikut",
}

// Info: File ini menampung utilitas bersama yang digunakan oleh seluruh agen cerdas (Pico, Criteria, Screener, dll)

// CleanJSONResponse adalah fungsi pembantu universal untuk membersihkan
// bungkusan markdown (triple backticks) yang sering ikut keluar dari output LLM.
// Fungsi ini diekspor (huruf kapital di awal) agar bisa dipakai oleh semua file di package agent.
// ClipRaw memangkas raw response LLM untuk pesan error/log agar tidak membanjiri Live Log
// (output ekstraksi bisa ribuan karakter). Cukup untuk diagnosa tanpa membuat log tak terbaca.
func ClipRaw(s string) string {
	s = strings.TrimSpace(s)
	const max = 220
	if len(s) > max {
		return s[:max] + " …[dipotong " + fmt.Sprintf("%d", len(s)-max) + " char]"
	}
	return s
}

// CleanJSONResponse mengekstrak blok JSON dari output LLM LALU meng-escape karakter kontrol
// MENTAH di dalam string literal. Langkah kedua penting: LLM sering menaruh tabel markdown
// multi-baris LANGSUNG di dalam nilai string → newline mentah → "invalid character '\n' in
// string literal" saat json.Unmarshal (tokenizer gagal SEBELUM UnmarshalJSON kustom jalan;
// kasus SLNAIntegration M8B_STEP4). Terpusat di sini → menutup SEMUA parsing JSON LLM.
func CleanJSONResponse(rawResponse string) string {
	return escapeRawControlCharsInStrings(cleanJSONResponseExtract(rawResponse))
}

// escapeRawControlCharsInStrings mengubah karakter kontrol MENTAH (<0x20: newline/tab/CR/dsb)
// yang muncul DI DALAM string literal JSON menjadi bentuk ter-escape yang valid (\n, \t, \r,
// \uXXXX). Karakter kontrol di LUAR string (whitespace struktural antar-token) DIBIARKAN.
// String yang sudah valid (escaped) tak berubah — `\n` (backslash+n) bukan newline mentah.
func escapeRawControlCharsInStrings(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 16)
	inString, escaped := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !inString {
			b.WriteByte(c)
			if c == '"' {
				inString = true
			}
			continue
		}
		if escaped { // char apa pun setelah backslash ditulis apa adanya
			b.WriteByte(c)
			escaped = false
			continue
		}
		switch {
		case c == '\\':
			b.WriteByte(c)
			escaped = true
		case c == '"':
			b.WriteByte(c)
			inString = false
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20:
			b.WriteString(fmt.Sprintf(`\u%04x`, c))
		default:
			b.WriteByte(c) // termasuk byte UTF-8 multibyte (>=0x80) → utuh
		}
	}
	return b.String()
}

func cleanJSONResponseExtract(rawResponse string) string {
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

	// 0b. Hapus kalimat naratif preamble sebelum JSON (LLM kadang nulis "Here is the JSON:" di awal)
	for _, prefix := range jsonNoisePatterns {
		lowerResp := strings.ToLower(rawResponse)
		lowerPrefix := strings.ToLower(prefix)
		if strings.HasPrefix(lowerResp, lowerPrefix) {
			// Potong sampai habis kalimat (akhir baris atau titik dua)
			idx := strings.IndexAny(rawResponse[len(prefix):], ":\n")
			if idx != -1 {
				rawResponse = strings.TrimSpace(rawResponse[len(prefix)+idx+1:])
			}
		}
	}

	// 0c. Hapus trailing text setelah JSON valid (misal "Note: ..." atau "Catatan: ...")
	// Ini hanya dilakukan jika ada brace/bracket penutup diikuti teks non-JSON
	rawResponse = strings.TrimSpace(rawResponse)

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
			// Jika tidak ada penutup ```, ambil sisa teks (truncated output)
			candidate := strings.TrimSpace(rawResponse[startIdx:])
			if strings.HasPrefix(candidate, "{") || strings.HasPrefix(candidate, "[") {
				return candidate
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

// GenerateJSON adalah helper retry yang memanggil LLM, membersihkan output,
// dan melakukan unmarshal ke target. Jika parsing gagal, ia akan re-call LLM
// dengan instruksi koreksi (maks maxRetries kali).
// Parameter target harus berupa pointer ke struct yang ingin diisi.
func GenerateJSON(ctx context.Context, client llm.LLMClient, systemPrompt, userPrompt string, target interface{}, maxRetries int) (string, error) {
	if maxRetries <= 0 {
		maxRetries = 2
	}

	rawResponse, err := client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("LLM generate gagal: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	if err := json.Unmarshal([]byte(cleanJSON), target); err == nil {
		return rawResponse, nil
	}

	// Retry loop: re-call LLM with correction prompt
	for attempt := 1; attempt <= maxRetries; attempt++ {
		logger.Logf("", "   [GenerateJSON] Retry %d/%d - JSON parsing gagal, meminta koreksi ke LLM...", attempt, maxRetries)

		correctionSystem := `Anda HARUS mengembalikan HANYA objek JSON yang valid tanpa teks tambahan apapun.
JANGAN gunakan blok markdown (jangan awali dengan ` + "```" + `).
JANGAN tambahkan penjelasan, komentar, atau narasi.
JANGAN gunakan backslash line continuation.
Output Anda harus dimulai dengan { dan diakhiri dengan } (atau [ dan ]).
Pastikan semua string di-escape dengan benar sesuai standar JSON.`

		correctionUser := fmt.Sprintf(`Respons Anda sebelumnya BUKAN JSON yang valid. Berikut instruksi awal yang harus Anda jawab dalam format JSON murni:

=== SYSTEM PROMPT AWAL ===
%s

=== USER PROMPT AWAL ===
%s

Sekarang berikan HANYA output JSON yang valid, tanpa teks lain.`, systemPrompt, userPrompt)

		rawResponse, err = client.Generate(ctx, correctionSystem, correctionUser)
		if err != nil {
			return "", fmt.Errorf("LLM retry %d gagal: %w", attempt, err)
		}

		cleanJSON = CleanJSONResponse(rawResponse)
		if err := json.Unmarshal([]byte(cleanJSON), target); err == nil {
			logger.Logf("", "   [GenerateJSON] Retry %d berhasil - JSON valid.", attempt)
			return rawResponse, nil
		}
	}

	// Semua retry gagal
	return rawResponse, fmt.Errorf("gagal parsing JSON setelah %d retry. Raw terakhir: %s", maxRetries, truncateForLog(rawResponse, 500))
}

// truncateForLog memotong string untuk keperluan log agar tidak terlalu panjang.
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}
