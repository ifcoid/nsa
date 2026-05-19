package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/llm"
	"nsa/internal/model"
)

type PicoAgent struct {
	llmProvider llm.LLMClient
}

func NewPicoAgent(provider llm.LLMClient) *PicoAgent {
	return &PicoAgent{
		llmProvider: provider,
	}
}

func (a *PicoAgent) Analyze(ctx context.Context, topicContext string, priorMatrixContext string) (*model.PICODefinitions, error) {
	systemPrompt := `Anda adalah asisten peneliti akademik profesional.
Tugas Anda adalah merumuskan PICO Framework 3-Lapis berdasarkan Konteks Topik dan Sintesis Matriks Prior Reviews yang diberikan.

=== LAPISAN 1: PICO ===
P (Population): siapa yang diteliti?
I (Intervention/Exposure): apa yang diteliti?
C (Comparison): pembanding (atau "no comparison" jika SLR deskriptif)
O (Outcome): hasil yang diukur

=== LAPISAN 2: OPERATIONAL DEFINITIONS (per komponen P/I/C/O) ===
- WHAT COUNTS: kriteria eksplisit yang MEMBUAT studi memenuhi syarat inklusi.
- WHAT DOESN'T COUNT: kriteria eksplisit yang MENGGUGURKAN studi (eksklusi).
- EDGE CASES: Kasus borderline beserta keputusan default (include/exclude + alasan).

=== LAPISAN 3: TERMINOLOGI KANONIKAL ===
Pilih komponen yang paling kompleks (P atau I) dan tentukan Terminologi Kanonikalnya:
- term: Terminologi baku secara internasional.
- definition: Definisi baku dalam 1 kalimat jelas.
- rejected_alternatives: Alternatif istilah yang umum namun DITOLAK beserta alasan penolakannya.

Output HARUS dalam format JSON MURNI dengan struktur persis seperti ini:
{
  "p": {
    "value": "...",
    "operational_def": {"what_counts": "...", "what_doesnt_count": "...", "edge_cases": "..."}
  },
  "i": {
    "value": "...",
    "operational_def": {"what_counts": "...", "what_doesnt_count": "...", "edge_cases": "..."}
  },
  "c": {
    "value": "...",
    "operational_def": {"what_counts": "...", "what_doesnt_count": "...", "edge_cases": "..."}
  },
  "o": {
    "value": "...",
    "operational_def": {"what_counts": "...", "what_doesnt_count": "...", "edge_cases": "..."}
  },
  "canonical_term": {
    "term": "...",
    "definition": "...",
    "rejected_alternatives": "..."
  }
}
Keluarkan HANYA JSON tanpa blok markdown atau teks pengantar.`

	userPrompt := fmt.Sprintf("Konteks Topik & Gap:\n%s\n\nPrior Reviews Matrix (Literatur Terdahulu):\n%s", topicContext, priorMatrixContext)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("pico_agent gagal berkomunikasi dengan LLM: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)

	var definitions model.PICODefinitions
	err = json.Unmarshal([]byte(cleanJSON), &definitions)
	if err != nil {
		return nil, fmt.Errorf("gagal parsing JSON dari LLM (%w). Raw response: %s", err, rawResponse)
	}

	return &definitions, nil
}
