package agent

import (
	"context"
	"fmt"
	"nsa/internal/llm"
	"nsa/internal/model"
)

type RQAgent struct {
	llmProvider llm.LLMClient
}

func NewRQAgent(provider llm.LLMClient) *RQAgent {
	return &RQAgent{
		llmProvider: provider,
	}
}

func (a *RQAgent) GenerateRQ(ctx context.Context, topic, matrix, pico, scope string) ([]model.ResearchQuestion, string, error) {
	systemPrompt := `Anda adalah ahli riset akademis spesialis merumuskan Research Questions (RQ) untuk Systematic Literature Review.
Tugas Anda adalah memformulasikan 1 PRIMARY RQ (utama) dan 3 SECONDARY RQs (pendukung) berdasarkan Konteks (Topik, Matriks Prior Reviews, PICO, dan Scope Justifications) yang diberikan.

ATURAN WAJIB:
Setiap RQ WAJIB memiliki "GAP-RQ TRACEABILITY":
a) gap: Sebutkan spesifik gap mana dari Topik yang dijawab.
b) prior_reviews: Sebutkan spesifik perbedaannya dari review terdahulu.
c) pico: Buktikan mengapa RQ ini bisa dijawab oleh kriteria PICO yang ada.
d) scope: Buktikan mengapa RQ ini sesuai dan selaras dengan Batasan/Filter (Scope Justifications) yang telah ditetapkan.

Jika ada RQ yang TIDAK BISA di-trace kuat ke keempat elemen tersebut, berikan nilai "is_orphan": true. Jika kuat, "is_orphan": false.

Output WAJIB berupa JSON array dengan struktur:
[
  {
    "type": "PRIMARY",
    "question": "...",
    "traceability": {
      "gap": "...",
      "prior_reviews": "...",
      "pico": "...",
      "scope": "..."
    },
    "is_orphan": false
  }
]`

	userPrompt := fmt.Sprintf("=== SELECTED TOPIC ===\n%s\n\n=== PRIOR REVIEWS MATRIX ===\n%s\n\n=== PICO DEFINITIONS ===\n%s\n\n=== SCOPE JUSTIFICATIONS ===\n%s", topic, matrix, pico, scope)

	var result []model.ResearchQuestion
	rawResponse, err := GenerateJSON(ctx, a.llmProvider, systemPrompt, userPrompt, &result, 2)
	if err != nil {
		return nil, rawResponse, fmt.Errorf("rq_agent gagal (LLM/parse JSON) (%w). Raw: %s", err, ClipRaw(rawResponse))
	}

	return result, rawResponse, nil
}
