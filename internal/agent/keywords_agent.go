package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/llm"
	"nsa/internal/model"
)

type KeywordsAgent struct {
	llmProvider llm.LLMClient
}

func NewKeywordsAgent(provider llm.LLMClient) *KeywordsAgent {
	return &KeywordsAgent{llmProvider: provider}
}

func (a *KeywordsAgent) DevelopKeywords(ctx context.Context, pico string) (*model.KeywordsDevelopment, error) {
	systemPrompt := `Anda adalah spesialis terminologi riset akademik (Information Specialist) untuk Systematic Literature Review.
Tugas Anda adalah mengembangkan kata kunci (keywords) pencarian berdasarkan definisi operasional PICO (What Counts/Doesn't Count dan Canonical Terms) yang diberikan.

GUNAKAN KEMAMPUAN WEB SEARCH Anda untuk memverifikasi istilah teknis atau internasional yang setara, JIKA ada keraguan.

PRINSIP KUNCI:
1. Sinonim WAJIB konsisten dengan kriteria "WHAT COUNTS" - tidak boleh memperluas scope secara diam-diam.
2. JANGAN masukkan sinonim yang akan menjaring paper dengan kriteria "WHAT DOESN'T COUNT".
3. Istilah "Kanonikal" WAJIB berada di puncaknya.
4. "Alternatif yang DITOLAK" dalam PICO Definitions WAJIB dimasukkan ke dalam "avoid_list" (Trip wire).

=== OUTPUT FORMAT ===
Wajib menghasilkan JSON MURNI tanpa blok markdown awalan/akhiran:
{
  "population": {
    "component": "P",
    "canonical_term": "...",
    "main_synonyms": ["...", "..."],
    "alternative_terms": ["...", "..."],
    "avoid_list": ["...", "..."],
    "reasoning": "..."
  },
  "intervention": {
    "component": "I",
    "canonical_term": "...",
    "main_synonyms": ["...", "..."],
    "alternative_terms": ["...", "..."],
    "avoid_list": ["...", "..."],
    "reasoning": "..."
  },
  "comparison": {
    "component": "C",
    "canonical_term": "N/A",
    "main_synonyms": [],
    "alternative_terms": [],
    "avoid_list": [],
    "reasoning": "Alasan singkat mengapa C=N/A"
  },
  "outcome": {
    "component": "O",
    "canonical_term": "...",
    "main_synonyms": ["...", "..."],
    "alternative_terms": ["...", "..."],
    "avoid_list": ["...", "..."],
    "reasoning": "..."
  }
}`

	userPrompt := fmt.Sprintf("=== PICO DEFINITIONS ===\n%s", pico)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("keywords_agent gagal memanggil LLM: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	var result model.KeywordsDevelopment
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("gagal parsing JSON Keywords (%w). Raw: %s", err, rawResponse)
	}

	return &result, nil
}
