package agent

import (
	"context"
	"fmt"

	"nsa/internal/llm"
)

// ManuscriptAgent menulis section manuskrip (Modul 9). Prompt per-section disediakan modul.
type ManuscriptAgent struct {
	client llm.LLMClient
}

func NewManuscriptAgent(client llm.LLMClient) *ManuscriptAgent {
	return &ManuscriptAgent{client: client}
}

// Write menghasilkan satu section (Markdown). systemPrompt = instruksi+aturan section,
// userPrompt = bundle artefak (data dari modul sebelumnya).
func (a *ManuscriptAgent) Write(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("manuscript LLM: %w", err)
	}
	return stripMarkdownFence(raw), nil
}
