package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/llm"
	"nsa/internal/model"
)

type GapAgent struct {
	client llm.LLMClient
}

func NewGapAgent(client llm.LLMClient) *GapAgent {
	return &GapAgent{client: client}
}

func (a *GapAgent) GenerateSuggestedTopics(ctx context.Context, initialTopic string) ([]model.SuggestedTopic, error) {
	systemPrompt := `Anda adalah agen riset akademik tingkat lanjut yang ahli dalam Systematic Literature Review (SLR).
Tugas Anda adalah mengambil gagasan topik riset awal dari pengguna, mensimulasikan pencarian literatur terbaru (3 tahun terakhir), dan mengidentifikasi 'Research Gap' yang belum terjawab.

Klasifikasikan setiap gap ke dalam salah satu tipe berikut:
- TIPE A: FRAGMENTASI LITERATUR (studi tersebar tanpa sintesis)
- TIPE B: KONTRADIKSI ANTAR STUDI (temuan primer bertentangan)
- TIPE C: KETIADAAN INTEGRATIVE FRAMEWORK (konsep belum terikat framework)

Kriteria Topik yang disarankan:
- Gap jelas + terverifikasi dari literatur terbaru
- Cocok untuk SLR
- Relevan dengan praktik saat ini

Keluarkan output HANYA dalam bentuk array JSON dengan struktur berikut:
[
  {
    "name": "Judul Topik",
    "gap": "Penjelasan spesifik mengenai gap yang ada",
    "type": "TIPE A / TIPE B / TIPE C",
    "type_reason": "Alasan mengapa masuk tipe ini",
    "evidence": "Contoh literatur/DOI/URL/fenomena terbaru",
    "importance": "Mengapa topik ini krusial diteliti sekarang"
  }
]
Pastikan mengembalikan tepat 3 saran topik berbeda. Output HANYA array JSON murni.`

	userPrompt := fmt.Sprintf("Topik mentah awal: %s\nBuatkan 3 saran topik SLR terbaik beserta Gap Characterization-nya.", initialTopic)

	response, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	cleaned := CleanJSONResponse(response)

	var suggestions []model.SuggestedTopic
	err = json.Unmarshal([]byte(cleaned), &suggestions)
	if err != nil {
		return nil, fmt.Errorf("gagal parsing JSON dari LLM (%w). Raw response: %s", err, response)
	}

	return suggestions, nil
}
