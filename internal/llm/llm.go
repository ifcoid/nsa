package llm

import "context"

// LLMClient adalah kontrak universal untuk semua provider LLM
type LLMClient interface {
	Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error)
	ModelName() string
}
