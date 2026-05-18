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
	return "Hasil text dari Gemini", nil
}
