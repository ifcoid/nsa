package llm

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

type GeminiClient struct {
	APIKey string
	Model  string
}

func NewGeminiClient(apiKey, model string) *GeminiClient {
	return &GeminiClient{APIKey: apiKey, Model: model}
}

func (g *GeminiClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	fmt.Printf("[API] Memanggil Gemini (%s) dengan Google Search Grounding...\n", g.Model)

	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: g.APIKey})
	if err != nil {
		return "", fmt.Errorf("gagal inisialisasi Gemini Client: %w", err)
	}

	// Mengaktifkan GoogleSearchRetrieval
	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: systemPrompt}},
		},
		Tools: []*genai.Tool{
			{GoogleSearch: &genai.GoogleSearch{}},
		},
	}

	res, err := client.Models.GenerateContent(ctx, g.Model, genai.Text(userPrompt), config)
	if err != nil {
		return "", fmt.Errorf("gemini API error: %w", err)
	}

	if len(res.Candidates) == 0 {
		return "", fmt.Errorf("gemini tidak mengembalikan kandidat jawaban")
	}

	// Gabungkan semua part teks dari response
	var output strings.Builder
	for _, part := range res.Candidates[0].Content.Parts {
		output.WriteString(part.Text)
	}

	return output.String(), nil
}
