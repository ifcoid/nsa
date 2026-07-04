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

func (a *SearchStringAgent) BuildSearchString(ctx context.Context, keywords, scope, databases string) (*model.SearchStringData, error) {
	systemPrompt := `Anda adalah ahli merumuskan Search String (Information Specialist) tingkat lanjut untuk Systematic Literature Review.
Tugas Anda adalah merangkai Keywords PICO menjadi Search String formal dan menyusun Spesifikasi Filter berdasarkan Batasan Scope.

ATURAN SEARCH STRING (SCOPUS):
1. Format: TITLE-ABS-KEY((P1 OR P2) AND (I1 OR I2) AND (O1 OR O2)).
2. Gunakan OR untuk sinonim, AND antar komponen.
3. Gunakan wildcard (*) untuk variasi (misal: educat*).
4. Gunakan quotation marks untuk frasa ("machine learning").
5. HINDARI sepenuhnya kata-kata yang ada di "avoid_list" dari Keywords (jangan masukkan ke dalam query).
6. Buat query yang komprehensif (sensitive) tapi tidak terlalu broad (specific).
7. Query adaptasi WAJIB dibuat HANYA untuk database yang DIPILIH di sesi (lihat DATABASE TERPILIH pada input). JANGAN menambah database yang tidak dipilih, dan JANGAN melewatkan yang dipilih (protokol pencarian harus konsisten dengan keputusan pemilihan database). Bila hanya Scopus yang dipilih, cukup scopus_query dan adapted_strings boleh kosong.

ATURAN FILTER TABLE:
Setiap filter (Tahun, Bahasa, Tipe Dokumen, Area Subjek, dll) WAJIB memiliki justifikasi yang jelas dari Scope Justifications.
Jika sebuah filter TIDAK memiliki justifikasi dari Scope, JANGAN masukkan ke dalam daftar filter.

Keluarkan HANYA JSON MURNI tanpa markdown blok awalan/akhiran (STRUKTUR di bawah hanya contoh format; isi database WAJIB mengikuti DATABASE TERPILIH, sintaks disesuaikan tiap database):
{
  "scopus_query": "TITLE-ABS-KEY(...)",
  "adapted_strings": [
    { "database": "<salah satu DATABASE TERPILIH>", "query": "<sintaks query khas database itu>" }
  ],
  "filters": [
    { "filter": "<nama filter dari Scope>", "value": "<nilai dari filter peneliti>", "justification": "<justifikasi dari Scope>" }
  ]
}`

	userPrompt := fmt.Sprintf("=== KEYWORDS ===\n%s\n\n=== SCOPE JUSTIFICATIONS ===\n%s\n\n=== DATABASE TERPILIH (buat adaptasi HANYA untuk ini) ===\n%s", keywords, scope, databases)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("search_string_agent gagal memanggil LLM: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	var result model.SearchStringData
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("gagal parsing JSON SearchString (%w). Raw: %s", err, ClipRaw(rawResponse))
	}

	return &result, nil
}
