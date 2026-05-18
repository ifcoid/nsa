package agent

import (
	"context"
	"nsa/internal/llm"
)

type ScreenerAgent struct {
	llmProvider llm.LLMClient // Menggunakan Interface, bukan struct kaku
}

func NewScreenerAgent(provider llm.LLMClient) *ScreenerAgent {
	return &ScreenerAgent{llmProvider: provider}
}

func (a *ScreenerAgent) Screen(ctx context.Context, abstract string) string {
	sysPrompt := "Kamu adalah agen penyaring jurnal..."

	// Agen tinggal pakai tanpa peduli ini Gemini atau Claude
	result, _ := a.llmProvider.Generate(ctx, sysPrompt, abstract)
	return result
}
