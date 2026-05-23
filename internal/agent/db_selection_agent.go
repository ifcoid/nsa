package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/llm"
	"nsa/internal/model"
	"strings"
	"time"
)

type DBSelectionAgent struct {
	llmProvider llm.LLMClient
}

func NewDBSelectionAgent(provider llm.LLMClient) *DBSelectionAgent {
	return &DBSelectionAgent{llmProvider: provider}
}

func (a *DBSelectionAgent) Analyze(ctx context.Context, pico, scope string) (*model.DatabaseSelection, string, error) {
	currentDate := time.Now().Format("2006-01-02")
	systemPrompt := fmt.Sprintf(`Anda adalah ahli metodologi pencarian literatur (Information Specialist) untuk Systematic Literature Review.
Tugas Anda adalah memilih database akademik yang paling tepat berdasarkan PICO dan Batasan Scope riset, menggunakan bantuan WEB SEARCH.

=== TUGAS ANDA ===
1. CEK COVERAGE BIDANG: Lakukan web search untuk memastikan apakah Scopus sudah mencakup mayoritas jurnal inti di bidang ini, atau apakah ada literatur penting (regional/niche) yang tidak terindeks Scopus.
2. MATRIKS DATABASE: Evaluasi kecocokan beberapa database (Scopus, Web of Science, PubMed, IEEE, dll) terhadap topik riset ini.
3. DECISION: Putuskan apakah SCOPUS-ONLY (>80%% paper kunci ada di Scopus) atau MULTI-DATABASE (misal ditambah PubMed/IEEE), atau tambahan Google Scholar untuk grey literature.
4. JUSTIFIKASI FINAL: Tulis paragraf 200 kata bergaya "Methods section" bahasa Inggris. Wajib mencantumkan "Date of search: %s".

Format Output WAJIB berupa JSON MURNI tanpa markdown blok awalan/akhiran:
{
  "cek_coverage_bidang": "Penjelasan analitis hasil cek...",
  "matriks_database": [
    {
      "database": "Scopus",
      "coverage_strength": "...",
      "limitation": "...",
      "fit_dengan_topik": "..."
    }
  ],
  "decision": "SCOPUS-ONLY / MULTI-DATABASE...",
  "justifikasi_final": "We conducted primary search using..."
}`, currentDate)

	userPrompt := fmt.Sprintf("=== PICO DEFINITIONS ===\n%s\n\n=== SCOPE JUSTIFICATIONS ===\n%s", pico, scope)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, "", fmt.Errorf("db_selection_agent gagal memanggil LLM: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	var result model.DatabaseSelection
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, "", fmt.Errorf("gagal parsing JSON DB Selection (%w). Raw: %s", err, rawResponse)
	}

	// Sisipkan referensi grounding ke UI via JustifikasiFinal
	if refIdx := strings.Index(rawResponse, "=== GROUNDING REFERENCES ==="); refIdx != -1 {
		result.JustifikasiFinal += "\n\n" + rawResponse[refIdx:]
	}

	return &result, rawResponse, nil
}
