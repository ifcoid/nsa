package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/llm"
	"nsa/internal/model"
)

type SearchStringAgent struct {
	llmProvider llm.LLMClient
}

func NewSearchStringAgent(provider llm.LLMClient) *SearchStringAgent {
	return &SearchStringAgent{llmProvider: provider}
}

func (a *SearchStringAgent) BuildSearchString(ctx context.Context, keywords, scope string) (*model.SearchStringData, error) {
	systemPrompt := `Anda adalah ahli merumuskan Search String (Information Specialist) tingkat lanjut untuk Systematic Literature Review.
Tugas Anda adalah merangkai Keywords PICO menjadi Search String formal dan menyusun Spesifikasi Filter berdasarkan Batasan Scope.

ATURAN SEARCH STRING (SCOPUS):
1. Format: TITLE-ABS-KEY((P1 OR P2) AND (I1 OR I2) AND (O1 OR O2)).
2. Gunakan OR untuk sinonim, AND antar komponen.
3. Gunakan wildcard (*) untuk variasi (misal: educat*).
4. Gunakan quotation marks untuk frasa ("machine learning").
5. HINDARI sepenuhnya kata-kata yang ada di "avoid_list" dari Keywords (jangan masukkan ke dalam query).
6. Buat query yang komprehensif (sensitive) tapi tidak terlalu broad (specific).
7. Jika ada indikasi Multi-DB, sertakan query adaptasi untuk database relevan (WoS, PubMed).

ATURAN FILTER TABLE:
Setiap filter (Tahun, Bahasa, Tipe Dokumen, Area Subjek, dll) WAJIB memiliki justifikasi yang jelas dari Scope Justifications.
Jika sebuah filter TIDAK memiliki justifikasi dari Scope, JANGAN masukkan ke dalam daftar filter.

Keluarkan HANYA JSON MURNI tanpa markdown blok awalan/akhiran:
{
  "scopus_query": "TITLE-ABS-KEY(...)",
  "adapted_strings": [
    {
      "database": "Web of Science",
      "query": "TS=(...)"
    }
  ],
  "filters": [
    {
      "filter": "Publication year",
      "value": "2018-2023",
      "justification": "Masa pasca-pandemi mengubah tren..."
    }
  ]
}`

	userPrompt := fmt.Sprintf("=== KEYWORDS ===\n%s\n\n=== SCOPE JUSTIFICATIONS ===\n%s", keywords, scope)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("search_string_agent gagal memanggil LLM: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	var result model.SearchStringData
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("gagal parsing JSON SearchString (%w). Raw: %s", err, rawResponse)
	}

	return &result, nil
}
