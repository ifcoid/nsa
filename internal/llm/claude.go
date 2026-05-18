package llm

import (
	"context"
	"fmt"
)

type ClaudeClient struct {
	AuthToken string
	Model     string
}

func NewClaudeClient(token, model string) *ClaudeClient {
	return &ClaudeClient{AuthToken: token, Model: model}
}

func (c *ClaudeClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// -> Di sini logika asli menggunakan HTTP Client / SDK Anthropic
	fmt.Printf("[API] Memanggil Claude (%s)...\n", c.Model)
	return "Hasil text dari Claude", nil
}
