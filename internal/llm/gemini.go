package llm

import (
	"context"
	"fmt"
	"os"
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
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: g.APIKey})
	if err != nil {
		return "", fmt.Errorf("gagal inisialisasi Gemini Client: %w", err)
	}

	temp := float32(0.1)
	config := &genai.GenerateContentConfig{
		Temperature: &temp,
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: systemPrompt}},
		},
	}

	// Google Search Grounding OPT-IN (default OFF). Untuk pipeline SLR, web-grounding
	// tidak diinginkan: model harus menalar dari konteks RAG/ekstraksi yang diberikan,
	// bukan mencari web (cegah kontaminasi & jaga output JSON terstruktur tetap bersih).
	// Aktifkan hanya bila EMBED-independen flag GEMINI_GROUNDING=true.
	grounding := strings.EqualFold(strings.TrimSpace(os.Getenv("GEMINI_GROUNDING")), "true")
	if grounding {
		fmt.Printf("[API] Memanggil Gemini (%s) + Google Search Grounding...\n", g.Model)
		config.Tools = []*genai.Tool{{GoogleSearch: &genai.GoogleSearch{}}}
	} else {
		fmt.Printf("[API] Memanggil Gemini (%s)...\n", g.Model)
	}

	res, err := client.Models.GenerateContent(ctx, g.Model, genai.Text(userPrompt), config)
	if err != nil {
		return "", fmt.Errorf("gemini API error: %w", err)
	}

	if len(res.Candidates) == 0 {
		return "", fmt.Errorf("gemini tidak mengembalikan kandidat jawaban")
	}

	candidate := res.Candidates[0]
	if candidate.Content == nil {
		return "", fmt.Errorf("gemini tidak mengembalikan content (finish reason: %s)", candidate.FinishReason)
	}

	// Gabungkan semua part teks dari response
	var output strings.Builder
	for _, part := range candidate.Content.Parts {
		output.WriteString(part.Text)
	}

	// Ekstrak referensi Google Search Grounding (jika ada)
	if candidate.GroundingMetadata != nil && len(candidate.GroundingMetadata.GroundingChunks) > 0 {
		output.WriteString("\n\n=== GROUNDING REFERENCES ===\n\n")
		for i, chunk := range candidate.GroundingMetadata.GroundingChunks {
			if chunk.Web != nil {
				output.WriteString(fmt.Sprintf("- **[%d]** [%s](%s)\n", i+1, chunk.Web.Title, chunk.Web.URI))
			}
		}
	}

	return output.String(), nil
}
