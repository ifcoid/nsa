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

func (a *DataMiningAgent) PicoConsistencyPreview(ctx context.Context, papersJSON, pico string) (*model.PICOPreviewCheck, error) {
	systemPrompt := `Anda adalah validator PICO-Consistency untuk Systematic Literature Review.
Tugas Anda mengklasifikasikan 20 sampel paper yang diberikan berdasarkan definisi operasional PICO.

Untuk SETIAP paper, tentukan klasifikasi SALAH SATU dari:
- "MATCH WHAT COUNTS": Sangat relevan dengan PICO.
- "MATCH WHAT DOESN'T": Masuk ke kriteria eksklusi / AVOID list.
- "AMBIGU": Judul/abstrak tidak memberi cukup informasi.
- "OFF-TOPIC": Sama sekali tidak relevan dengan topik riset.

Setelah klasifikasi semua paper, hitung persentase "MATCH WHAT COUNTS" dan berikan Verdict:
- Jika >60% -> "PROCEED L3"
- Jika 30-60% -> "ACCEPTABLE_HIGH_WORKLOAD"
- Jika <30% -> "BACK_TO_MODUL_3"

Keluarkan HANYA JSON MURNI tanpa markdown blok awalan/akhiran:
{
  "samples_analyzed": [
    {"title": "Judul Paper 1", "classification": "MATCH WHAT COUNTS", "reasoning": "Alasan singkat..."}
  ],
  "match_counts_pct": 65.0,
  "verdict": "PROCEED L3",
  "recommendation": "Saran untuk langkah selanjutnya..."
}`

	userPrompt := fmt.Sprintf("=== PICO DEFINITIONS ===\n%s\n\n=== 20 SAMPEL PAPER ===\n%s", pico, papersJSON)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("data_mining_agent gagal preview PICO: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	var result model.PICOPreviewCheck
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("gagal parsing JSON PICOPreview (%w). Raw: %s", err, rawResponse)
	}

	return &result, nil
}

func (a *DataMiningAgent) GenerateModul4Summary(ctx context.Context, sessionData string) (*model.Modul4Summary, error) {
	systemPrompt := `Anda adalah asisten pembuat Modul 4 Summary.
Berdasarkan data mining log SLR, buat ringkasan Markdown dengan struktur persis seperti berikut:

=== DATA MINING SUMMARY (SLR) ===

SEARCH EXECUTION:
- Date: [...]
- Total hits per DB: [...]

SANITY CHECK:
- Paper-kunci: [...]
- Volume verdict: reasonable
- Go/no-go: PROCEED

EXPORT:
- Files: [...]
- Records per DB sources: [...]
- Fields preserved: [...]

DEDUPLICATION:
- Total → Unique slr_papers_post_dedup: [...]
- Duplicates breakdown: [...]
- Manual review flags: [...]

PICO-CONSISTENCY PREVIEW:
- MATCH "WHAT COUNTS": [...]%
- Verdict: PROCEED

SCREENING DATABASE:
- Collection: screening
- Embedded criteria Row 1-5: ✓
- Reason codes: ✓ (8 standard)
- Kappa sheet: ✓
- Dual-reviewer columns: ✓

Keluarkan HANYA JSON murni tanpa awalan/akhiran markdown blok:
{
  "markdown": "=== DATA MINING SUMMARY (SLR) ===\n..."
}`

	userPrompt := fmt.Sprintf("=== DATA MINING LOG ===\n%s", sessionData)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("data_mining_agent gagal membuat summary: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	var result model.Modul4Summary
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("gagal parsing JSON Modul4Summary (%w). Raw: %s", err, rawResponse)
	}

	return &result, nil
}
