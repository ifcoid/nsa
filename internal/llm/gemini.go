package llm

import (
	"context"
	"fmt"
)

type GeminiClient struct {
	APIKey string
	Model  string
}

func NewGeminiClient(apiKey, model string) *GeminiClient {
	return &GeminiClient{APIKey: apiKey, Model: model}
}

func (g *GeminiClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// -> Di sini logika asli menggunakan SDK resmi google (google.golang.org/genai)
	fmt.Printf("[API] Memanggil Gemini (%s)...\n", g.Model)
	
	// Mengembalikan JSON valid sebagai respon dummy agar agen PICO / Kriteria tidak crash saat parsing
	return `{"P": "Populasi Dummy", "I": "Intervensi Dummy", "C": "Pembanding Dummy", "O": "Hasil Dummy", "inclusion_criteria": ["Kriteria 1"], "exclusion_criteria": ["Kriteria 2"]}`, nil
}
