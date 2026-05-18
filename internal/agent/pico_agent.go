package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nsa/internal/llm"
)

// PicoAgent bertugas untuk menganalisis topik riset menjadi komponen PICO
type PicoAgent struct {
	llmProvider llm.LLMClient
}

// NewPicoAgent adalah constructor untuk menyuntikkan otak LLM ke PicoAgent
func NewPicoAgent(provider llm.LLMClient) *PicoAgent {
	return &PicoAgent{
		llmProvider: provider,
	}
}

// Analyze menerima topik mentah dan mengembalikan map berisi komponen P, I, C, O
func (a *PicoAgent) Analyze(ctx context.Context, topic string) (map[string]string, error) {
	// Instruksi bersih tanpa karakter backtick literal untuk menghindari error compiler Go
	systemPrompt := `Kamu adalah agen AI akademis spesialis Systematic Literature Review (SLR).
Tugasmu adalah membedah topik penelitian yang diberikan menjadi komponen PICO secara akademis dan tajam:
- P (Population): Populasi, subjek, jenis sinyal, atau masalah spesifik yang diteliti.
- I (Intervention): Metode, teknik, arsitektur, algoritma, atau perlakuan utama yang diterapkan.
- C (Comparison): Alternatif metode, algoritma pembanding, atau kondisi kontrol standar di ranah tersebut (jika tidak ada di topik, rumuskan pembanding yang paling relevan).
- O (Outcome): Hasil akhir, metrik keberhasilan, target akurasi, atau dampak yang ingin diukur.

Format Output WAJIB berupa JSON objek murni tanpa hiasan teks pembuka/penutup dan tanpa markdown code blocks (jangan pakai bungkusan triple backticks markdown json).
Contoh output yang benar:
{"P": "deskripsi populasi", "I": "deskripsi intervensi", "C": "deskripsi pembanding", "O": "deskripsi hasil"}`

	userPrompt := fmt.Sprintf("Analisislah topik penelitian berikut ke dalam bentuk PICO:\n\"%s\"", topic)

	// Panggil LLM via Interface universal
	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("pico_agent gagal berkomunikasi dengan LLM: %w", err)
	}

	// Defense Mechanism: Bersihkan string jika LLM nakal tetap memberikan bungkusan markdown
	cleanJSON := strings.TrimSpace(rawResponse)
	cleanJSON = strings.TrimPrefix(cleanJSON, "```json")
	cleanJSON = strings.TrimPrefix(cleanJSON, "```")
	cleanJSON = strings.TrimSuffix(cleanJSON, "```")
	cleanJSON = strings.TrimSpace(cleanJSON)

	// Parse hasil teks JSON dari LLM ke dalam map Go
	var picoMap map[string]string
	err = json.Unmarshal([]byte(cleanJSON), &picoMap)
	if err != nil {
		return nil, fmt.Errorf("gagal mengonversi teks LLM ke JSON map. Raw response: %s, Error: %w", rawResponse, err)
	}

	// Validasi Guardrail: Pastikan seluruh kunci P, I, C, O tersedia di dalam map
	requiredKeys := []string{"P", "I", "C", "O"}
	for _, key := range requiredKeys {
		if _, exists := picoMap[key]; !exists {
			picoMap[key] = "Tidak disebutkan secara spesifik oleh LLM"
		}
	}

	return picoMap, nil
}
