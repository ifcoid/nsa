package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/llm"
	"nsa/internal/model"
	"time"
)

type ExecutionAgent struct {
	llmProvider llm.LLMClient
}

func NewExecutionAgent(provider llm.LLMClient) *ExecutionAgent {
	return &ExecutionAgent{llmProvider: provider}
}

// Fase 1: Pre-Validasi
func (a *ExecutionAgent) PreValidate(ctx context.Context, searchString string) (string, error) {
	systemPrompt := `Anda adalah validator Search String. Lakukan Fase 1: Pre-Validasi:
1. Syntax check (TITLE-ABS-KEY format, Boolean operators, wildcard, quotation).
2. Verifikasi trap keywords (homonim/ambigu) via web search bila perlu.
3. Estimasi kelayakan (sempit/luas).
Berikan analisis Anda dalam teks markdown ringkas (bullet points). Beri pesan peringatan jika sintaks salah.`

	userPrompt := fmt.Sprintf("=== SEARCH STRING ===\n%s", searchString)

	return a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
}

type ExecutionResult struct {
	SearchLog model.SearchLog     `json:"search_log"`
	Summary   model.Modul3Summary `json:"summary"`
}

// Fase 3: Evaluasi & Summarize
func (a *ExecutionAgent) EvaluateAndSummarize(ctx context.Context, ss, kw, db, userHits string) (*ExecutionResult, error) {
	currentDate := time.Now().Format("2006-01-02")
	systemPrompt := fmt.Sprintf(`Anda adalah eksekutor akhir Modul 3. Tugas Anda melakukan Fase 3 (Evaluasi) dan membuat 2 Dokumen Output.

DATA INPUT USER HASIL EKSEKUSI MANUAL:
[USER_HITS] (termasuk total hit pre/post filter)

TUGAS ANDA:
1. Lakukan Sanity Check (jumlah masuk akal? relevan dengan PICO?).
2. Susun dokumen SEARCH LOG (JSON). Wajib cantumkan DATE STAMP ("%s") dan UPDATE POLICY (Trigger re-run: 6 bulan belum submit, dll).
3. Susun dokumen MODUL 3 SUMMARY (Markdown utuh "=== COMPLETE SEARCH STRATEGY (SLR) ===").

Output WAJIB berupa JSON MURNI tanpa blok markdown awalan/akhiran:
PENTING: JANGAN memecah baris (line continuation) menggunakan backslash (\) untuk string panjang. Tuliskan teks dalam satu baris untuk tiap key JSON.
{
  "search_log": {
    "search_string_final": "[GABUNGKAN SEMUA SEARCH STRING KE DALAM 1 TEKS STRING, JANGAN GUNAKAN OBJECT JSON/DICTIONARY]",
    "filters_applied": [ {"filter": "...", "value": "...", "justification": "..."} ],
    "databases": ["Scopus", "Web of Science"],
    "date_executed": {"Scopus": "%s"},
    "total_hits": {"Scopus": "150 post-filter"},
    "update_policy": "TRIGGER WAJIB RE-RUN:..."
  },
  "summary": {
    "markdown": "=== COMPLETE SEARCH STRATEGY (SLR) ===\n..."
  }
}`, currentDate, currentDate)

	userPrompt := fmt.Sprintf("=== SEARCH STRING ===\n%s\n\n=== KEYWORDS ===\n%s\n\n=== DB SELECTION ===\n%s\n\n=== USER HITS INPUT ===\n%s", ss, kw, db, userHits)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("execution_agent gagal memanggil LLM: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	var result ExecutionResult
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("gagal parsing JSON ExecutionResult (%w). Raw: %s", err, rawResponse)
	}

	return &result, nil
}
