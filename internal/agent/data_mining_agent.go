package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/llm"
	"nsa/internal/model"
)

type DataMiningAgent struct {
	llmProvider llm.LLMClient
}

func NewDataMiningAgent(provider llm.LLMClient) *DataMiningAgent {
	return &DataMiningAgent{llmProvider: provider}
}

func (a *DataMiningAgent) SanityCheck(ctx context.Context, sampleData, scope string) (*model.SanityCheckVerdict, error) {
	systemPrompt := `Anda adalah evaluator Eksekusi Data Mining pencarian SLR.
Tugas Anda melakukan SANITY CHECK terhadap hasil pencarian awal user.

ATURAN EVALUASI:
1. PAPER-KUNCI CHECK: Lihat data "key_papers_missing". Jika ada paper kunci yang krusial hilang, berarti search string gagal menangkapnya. 
2. KONFIRMASI VOLUME: Analisis "total_hits_post_filter". Apakah jumlah tersebut reasonable untuk lingkup riset SLR berdasarkan Scope Justification (biasanya ratusan hingga ribuan awal, ideal disaring jadi puluhan/ratusan)? Jika terlalu sedikit (<50) = filter terlalu ketat. Jika terlalu banyak (>5000) = mungkin ada trap keyword.
3. GO/NO-GO DECISION:
   - "PROCEED": Jika volume reasonable dan paper kunci mayoritas ditemukan.
   - "REVISE": Jika volume tidak masuk akal ATAU paper kunci penting hilang. Saran harus spesifik (misal: "Kembali ke Modul 3, tambahkan sinonim X" atau "Hapus trap keyword Y").

Keluarkan HANYA JSON MURNI tanpa blok awalan/akhiran:
{
  "key_papers_missing": ["paper A", "paper B"],
  "volume_analysis": "Analisis volume hits...",
  "decision": "PROCEED",
  "recommendation": "Lanjut ke eksport / Kembali ke Modul 3 Langkah 3 karena..."
}`

	userPrompt := fmt.Sprintf("=== INITIAL SAMPLE DATA ===\n%s\n\n=== SCOPE JUSTIFICATIONS ===\n%s", sampleData, scope)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("data_mining_agent gagal memanggil LLM: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	var result model.SanityCheckVerdict
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("gagal parsing JSON SanityCheck (%w). Raw: %s", err, rawResponse)
	}

	return &result, nil
}
