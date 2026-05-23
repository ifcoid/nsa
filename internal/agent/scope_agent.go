package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/llm"
	"nsa/internal/model"
)

type ScopeAgent struct {
	llmProvider llm.LLMClient
}

func NewScopeAgent(provider llm.LLMClient) *ScopeAgent {
	return &ScopeAgent{
		llmProvider: provider,
	}
}

func (a *ScopeAgent) GenerateJustifications(ctx context.Context, picoContext, filtersContext string) ([]model.ScopeJustification, error) {
	systemPrompt := `Anda adalah asisten peneliti akademik profesional dengan akses Internet (Web Search).
Tugas Anda adalah membuat justifikasi 3-Lapis untuk SETIAP batasan/filter riset (Rentang Tahun, Geografi, Sektor, Bahasa, dll).

Untuk SETIAP filter, bangun 3 lapis justifikasi:
1. TEORETIS: landasan konseptual (GUNAKAN KEMAMPUAN WEB SEARCH ANDA untuk memverifikasi klaim. WAJIB tuliskan Judul/URL sumber secara LANGSUNG di dalam teks kalimat, hindari sitasi angka kurung siku [1] karena metadata link-nya akan terhapus).
2. METODOLOGIS: mengapa batasan ini memperbaiki atau menjaga kualitas review.
3. PRAKTIS: relevansi kebijakan atau praktik nyata di lapangan (misal: SDG, ILO, Bappenas).

Jika suatu batasan tidak memiliki justifikasi teoretis/ilmiah yang kuat sama sekali, isi status dengan "Perlu Diubah/Dihapus". Jika argumentasinya kuat, isi dengan "Valid".

Output WAJIB berupa blok markdown JSON murni (diapit oleh ` + "```json" + ` dan ` + "```" + `).
Contoh Output:
` + "```json" + `
[
  {
    "name": "Rentang Tahun: 2020-2024",
    "theoretical": "Era pasca-COVID secara struktural mengubah lanskap pendidikan jarak jauh (Smith et al., 2021).",
    "methodological": "Memastikan literatur merefleksikan teknologi pasca-disrupsi sehingga tidak usang.",
    "practical": "Relevan dengan agenda transformasi digital Bappenas 2024.",
    "status": "Valid"
  }
]
` + "```"

	userPrompt := fmt.Sprintf("=== PICO DEFINITIONS ===\n%s\n\n=== SCOPE FILTERS (BATASAN YANG AKAN DIJUSTIFIKASI) ===\n%s", picoContext, filtersContext)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("scope_agent gagal memanggil LLM: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)

	var result []model.ScopeJustification
	err = json.Unmarshal([]byte(cleanJSON), &result)
	if err != nil {
		return nil, fmt.Errorf("gagal parsing JSON dari LLM (%w). Raw response: %s", err, rawResponse)
	}

	return result, nil
}
