package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nsa/internal/llm"
	"nsa/internal/model"
)

// CriteriaResult menampung struktur data hasil rumusan kriteria
type CriteriaResult struct {
	Inclusion []string `json:"inclusion_criteria"`
	Exclusion []string `json:"exclusion_criteria"`
}

// CriteriaAgent bertanggung jawab membuat dan merevisi kriteria SLR
type CriteriaAgent struct {
	llmProvider llm.LLMClient
}

// NewCriteriaAgent adalah constructor untuk membuat CriteriaAgent
func NewCriteriaAgent(provider llm.LLMClient) *CriteriaAgent {
	return &CriteriaAgent{
		llmProvider: provider,
	}
}

// GenerateCriteria merumuskan kriteria inklusi dan eksklusi awal berdasarkan komponen PICO dan Scope Filters
func (a *CriteriaAgent) GenerateCriteria(ctx context.Context, pico *model.PICODefinitions, filters *model.ScopeFilters) (*CriteriaResult, error) {
	systemPrompt := `Kamu adalah agen AI akademis spesialis metodologi Systematic Literature Review (SLR) dan protokol PRISMA.
Tugasmu adalah menyusun Kriteria Inklusi (Inclusion Criteria) dan Kriteria Eksklusi (Exclusion Criteria) yang ketat, jelas, dan objektif.

Kamu harus mendasarkan kriteria pada 2 hal:
1. Komponen PICO (Population, Intervention, Comparison, Outcome) yang menargetkan batasan ilmiah/topikal riset.
2. Parameter Batasan Eksternal (Filter) yang telah ditentukan secara eksplisit oleh peneliti (seperti rentang tahun, geografi, bahasa).

Format Output WAJIB berupa JSON objek murni dengan struktur seperti contoh di bawah ini, tanpa teks pembuka/penutup, dan tanpa markdown code blocks.

Contoh output JSON yang benar:
{
  "inclusion_criteria": [
    "Artikel berfokus pada populasi X",
    "Diterbitkan antara tahun 2018-2023",
    "Ditulis dalam bahasa Inggris"
  ],
  "exclusion_criteria": [
    "Artikel berupa survey, review, atau meta-analisis",
    "Penelitian dilakukan di luar wilayah Asia Tenggara"
  ]
}`

	userPrompt := fmt.Sprintf("Susun kriteria berdasarkan data berikut:\n\n=== DATA PICO ===\nP: %s\nI: %s\nC: %s\nO: %s\n\n=== BATASAN / FILTER PENELITI ===\nRentang Tahun: %s\nGeografis: %s\nSektor: %s\nBahasa: %s\nLainnya: %s", 
		pico.P.Value, pico.I.Value, pico.C.Value, pico.O.Value,
		filters.RentangTahun, filters.Geografis, filters.Sektor, filters.Bahasa, filters.Lainnya)

	return a.callLLMAndParse(ctx, systemPrompt, userPrompt)
}

// RefineCriteria memperbaiki kriteria yang sudah ada berdasarkan umpan balik (feedback) dari manusia
func (a *CriteriaAgent) RefineCriteria(ctx context.Context, currentInclusion, currentExclusion []string, feedback string) (*CriteriaResult, error) {
	systemPrompt := `Kamu adalah agen AI akademis. Tugasmu adalah memperbaiki Kriteria Inklusi dan Eksklusi SLR yang sudah ada berdasarkan instruksi feedback revisi dari peneliti utama (manusia).
Pertahankan kriteria lama yang sudah bagus, dan ubah atau tambahkan kriteria baru sesuai arahan feedback secara spesifik dan presisi.

Format Output WAJIB berupa JSON objek murni tanpa teks tambahan dan tanpa markdown code blocks (jangan gunakan triple backticks markdown json).
Struktur JSON harus tepat seperti ini:
{
  "inclusion_criteria": ["kriteria 1", "kriteria 2"],
  "exclusion_criteria": ["kriteria 1", "kriteria 2"]
}`

	userPrompt := fmt.Sprintf("KRITERIA SAAT INI:\nInclusion: %v\nExclusion: %v\n\nFEEDBACK REVISI MANUSIA:\n\"%s\"\n\nSempurnakan kriteria di atas berdasarkan feedback tersebut.",
		currentInclusion, currentExclusion, feedback)

	return a.callLLMAndParse(ctx, systemPrompt, userPrompt)
}

// Handler internal untuk eksekusi ke LLM dan parsing JSON (DRY Principle)
func (a *CriteriaAgent) callLLMAndParse(ctx context.Context, systemPrompt, userPrompt string) (*CriteriaResult, error) {
	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("criteria_agent gagal memanggil LLM: %w", err)
	}

	// Bersihkan bungkusan markdown jika LLM keras kepala menyertakannya
	cleanJSON := strings.TrimSpace(rawResponse)
	cleanJSON = strings.TrimPrefix(cleanJSON, "```json")
	cleanJSON = strings.TrimPrefix(cleanJSON, "```")
	cleanJSON = strings.TrimSuffix(cleanJSON, "```")
	cleanJSON = strings.TrimSpace(cleanJSON)

	var result CriteriaResult
	err = json.Unmarshal([]byte(cleanJSON), &result)
	if err != nil {
		return nil, fmt.Errorf("criteria_agent gagal unmarshal JSON. Raw: %s, Error: %w", ClipRaw(rawResponse), err)
	}

	return &result, nil
}
