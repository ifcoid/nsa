package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/llm"
	"nsa/internal/model"
)

type FinerAgent struct {
	llmProvider llm.LLMClient
}

func NewFinerAgent(provider llm.LLMClient) *FinerAgent {
	return &FinerAgent{llmProvider: provider}
}

type FinerResult struct {
	Check   model.FinerNoveltyCheck `json:"check"`
	Summary model.Modul2Summary     `json:"summary"`
}

func (a *FinerAgent) ValidateFiner(ctx context.Context, rqs, matrix, pico, scope string) (*FinerResult, error) {
	systemPrompt := `Anda adalah validator akhir (Reviewer Eksekutif) untuk Modul 2 Systematic Literature Review.
Tugas Anda adalah mengevaluasi kelayakan riset menggunakan framework FINER dan menyusun Modul 2 Summary.
GUNAKAN KEMAMPUAN WEB SEARCH Anda untuk: 
(1) Mengestimasi jumlah studi di Scopus/database akademik untuk kelayakan PICO (Feasibility).
(2) Memverifikasi ketiadaan SLR yang serupa dalam 6 bulan terakhir (Novelty).

=== OUTPUT 1: FINER & INTERNAL COHERENCE ===
F (Feasible): Estimasi web search jumlah studi. (Tuliskan perkiraannya).
I (Interesting): Siapa audiens primernya.
N (Novel): Buat tabel Markdown "Novelty per Prior Review" (Prior Review | Overlap | BARU di riset saya | Signifikansi).
E (Ethical): Isu etika/hak cipta.
R (Relevant): Keselarasan SDG/kebijakan nasional.
Coherence 1 (PICO Adequacy): Apakah PICO cukup jawab Primary RQ?
Coherence 2 (Scope Feasibility): Apakah Scope tidak terlalu sempit?
Coherence 3 (Terminology): Konsistensi terminologi kanonikal.
Recommendation: Berikan PASS atau rekomendasikan revisi di Langkah 1/2/3/4/5.

=== OUTPUT 2: MODUL 2 SUMMARY ===
Buat teks Markdown lengkap merangkum keseluruhan sesi ini:
=== RESEARCH QUESTION PACKAGE (SLR) ===
RESEARCH AREA: [Topik Riset]
1. GAP CHARACTERIZATION...
2. PRIOR REVIEWS MATRIX...
3. PICO + OPERATIONAL DEFINITIONS...
4. CANONICAL TERMINOLOGY...
5. SCOPE JUSTIFICATION...
6. RESEARCH QUESTIONS...
7. FINER + NOVELTY CHECK...

Keluarkan HANYA JSON MURNI berformat persis seperti ini tanpa blok markdown atau teks pengantar:
{
  "check": {
    "finer": {
      "feasible": "...",
      "interesting": "...",
      "novel": "...",
      "ethical": "...",
      "relevant": "..."
    },
    "internal_coherence": {
      "pico_adequacy": "...",
      "scope_feasibility": "...",
      "terminology": "...",
      "recommendation": "..."
    },
    "is_pass": true
  },
  "summary": {
    "markdown": "..."
  }
}`

	userPrompt := fmt.Sprintf("=== RESEARCH QUESTIONS ===\n%s\n\n=== PRIOR REVIEWS MATRIX ===\n%s\n\n=== PICO DEFINITIONS ===\n%s\n\n=== SCOPE JUSTIFICATIONS ===\n%s", rqs, matrix, pico, scope)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("finer_agent gagal memanggil LLM: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	
	var res FinerResult
	if err := json.Unmarshal([]byte(cleanJSON), &res); err != nil {
		return nil, fmt.Errorf("gagal parsing JSON FINER (%w). Raw: %s", err, rawResponse)
	}

	return &res, nil
}
