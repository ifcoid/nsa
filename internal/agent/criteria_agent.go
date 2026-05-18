package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nsa/internal/llm"
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

// GenerateCriteria merumuskan kriteria inklusi dan eksklusi awal berdasarkan komponen PICO
func (a *CriteriaAgent) GenerateCriteria(ctx context.Context, pico map[string]string) (*CriteriaResult, error) {
	systemPrompt := `Kamu adalah agen AI akademis spesialis metodologi Systematic Literature Review (SLR) dan protokol PRISMA.
Tugasmu adalah menyusun Kriteria Inklusi (Inclusion Criteria) dan Kriteria Eksklusi (Exclusion Criteria) yang ketat, jelas, dan objektif berdasarkan komponen PICO yang diberikan.

Kriteria harus mencakup aspek:
1. Batasan topik riset (berdasarkan Population & Intervention).
2. Jenis publikasi (misal: hanya peer-reviewed journal/proceedings).
3. Batasan bahasa (misal: hanya bahasa Inggris).
4. Rentang tahun (misal: 5 tahun terakhir).

Format Output WAJIB berupa JSON objek murni dengan struktur seperti contoh di bawah ini, tanpa teks pembuka/penutup, dan tanpa markdown code blocks (jangan gunakan bungkusan triple backticks markdown json).

Contoh output JSON yang benar:
{
  "inclusion_criteria": [
    "Artikel berfokus pada populasi X",
    "Artikel menerapkan metode Y"
  ],
  "exclusion_criteria": [
    "Artikel berupa survey, review, atau meta-analisis",
    "Studi yang dilakukan bukan pada subjek manusia"
  ]
}`

	userPrompt := fmt.Sprintf("Susun kriteria berdasarkan data PICO berikut:\n"+
		"P: %s\nI: %s\nC: %s\nO: %s", pico["P"], pico["I"], pico["C"], pico["O"])

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
		return nil, fmt.Errorf("criteria_agent gagal unmarshal JSON. Raw: %s, Error: %w", rawResponse, err)
	}

	return &result, nil
}
