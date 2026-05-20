package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/llm"
	"nsa/internal/model"
)

type ScreeningAgent struct {
	llmProvider llm.LLMClient
}

func NewScreeningAgent(provider llm.LLMClient) *ScreeningAgent {
	return &ScreeningAgent{llmProvider: provider}
}

func (a *ScreeningAgent) GenerateBriefing(ctx context.Context, pico, reasonCodes string) (*model.ScreenerBriefing, error) {
	systemPrompt := `Anda adalah Manajer Sistematic Literature Review.
Tugas Anda mengeksekusi TASK 1 (Validasi Kriteria) dan TASK 2 (Generate Briefing).

=== TASK 1: VALIDASI KELENGKAPAN KRITERIA ===
Evaluasi apakah PICO Definitions dan Reason Codes yang ada sudah cukup testable dan komprehensif (tidak ada celah interpretasi besar). Jika ada celah/gap besar (misal Edge Cases tidak terjawab atau What Counts tumpang tindih), berikan decision "REVISE_M2". Jika cukup solid, berikan "PROCEED".

=== TASK 2: GENERATE SCREENER BRIEFING ===
Buat dokumen instruksi baku untuk 2 reviewer. Wajib menggunakan struktur persis berikut:

---
SCREENER BRIEFING
Date: [YYYY-MM-DD]
Reviewers: R1 & R2

1. CANONICAL TERMINOLOGY: [Ekstrak dari PICO Definitions]
2. OPERATIONAL DEFINITIONS (quick reference):
   P/I/C/O: [WHAT COUNTS | WHAT DOESN'T | EDGE CASES]
3. DECISION TREE (kasus ambigu):
   If [kondisi X] AND [Y] -> INCLUDE
   If [X] BUT NOT [Y] -> UNCERTAIN, flag diskusi
   If NOT [X] -> EXCLUDE
4. REASON CODES: [Tampilkan dari data REASON CODES yang diberikan]
5. UNCERTAIN PROTOCOL:
   - Cukup info di abstract tapi sulit decide -> UNCERTAIN + notes
   - Abstract tidak cukup info -> "pending full-text" di Notes
   - JANGAN decide INCLUDE/EXCLUDE tanpa grounded operational def
6. AI-ASSISTANT WORKFLOW:
   - Cowork berikan DUAL PERSPECTIVE (Strict + Liberal) untuk record sulit
   - Reviewer baca, decide independen
   - Decision/Reason/Notes = ditulis HUMAN
7. REPORTING:
   - Cohen's kappa = R1 vs R2 (HUMAN, bukan AI)
---

Keluarkan HANYA JSON MURNI (tanpa markdown blok):
{
  "validation_gap": "Analisis kelengkapan PICO...",
  "decision": "PROCEED",
  "recommendation": "Saran jika ada...",
  "briefing_doc": "--- SCREENER BRIEFING ..."
}`

	userPrompt := fmt.Sprintf("=== PICO DEFINITIONS ===\n%s\n\n=== REASON CODES ===\n%s", pico, reasonCodes)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("screening_agent gagal memanggil LLM: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	var result model.ScreenerBriefing
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("gagal parsing JSON ScreenerBriefing (%w). Raw: %s", err, rawResponse)
	}

	return &result, nil
}

type ScreeningDecision struct {
	Decision   string `json:"decision"`
	ReasonCode string `json:"reason_code"`
	Notes      string `json:"notes"`
}

func (a *ScreeningAgent) ReviewPaper(ctx context.Context, briefing, title, abstract, keywords string) (*ScreeningDecision, error) {
	systemPrompt := fmt.Sprintf(`Anda adalah Reviewer Independen untuk Systematic Literature Review.
Berikut adalah SCREENER BRIEFING yang WAJIB Anda patuhi:
%s

Tugas Anda adalah meninjau Title, Abstract, dan Keywords dari paper yang diberikan, lalu tentukan keputusan Anda:
"INCLUDE", "EXCLUDE", atau "UNCERTAIN".

ATURAN:
1. Jika keputusan "EXCLUDE", Anda WAJIB mengisi field "reason_code" dengan salah satu dari REASON CODES di briefing. Jika "INCLUDE" atau "UNCERTAIN", kosongkan atau isi "-".
2. Dalam field "notes", tuliskan analisis Anda secara komprehensif sebagai VERDICT-AID (termasuk pandangan STRICT dan LIBERAL).

Keluarkan HANYA JSON MURNI tanpa blok markdown:
{
  "decision": "EXCLUDE",
  "reason_code": "STUDY-DESIGN",
  "notes": "Perspektif Strict: ... Perspektif Liberal: ... Verdict-Aid: ..."
}`, briefing)

	userPrompt := fmt.Sprintf("Title: %s\nKeywords: %s\nAbstract: %s", title, keywords, abstract)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("gagal review paper: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	var result ScreeningDecision
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("gagal parsing JSON ReviewPaper (%w). Raw: %s", err, rawResponse)
	}
	return &result, nil
}

func (a *ScreeningAgent) BatchReviewPaper(ctx context.Context, briefing, title, abstract, keywords string) (*model.ScreeningPerspective, error) {
	systemPrompt := fmt.Sprintf(`Anda adalah Reviewer Independen untuk Systematic Literature Review.
Berikut adalah SCREENER BRIEFING yang WAJIB Anda patuhi:
%s

Tugas Anda adalah meninjau Title, Abstract, dan Keywords dari paper yang diberikan.

Keluarkan HANYA JSON MURNI tanpa blok markdown dengan struktur berikut:
{
  "strict": "Perspektif jika Anda bersikap STRICT (bias EXCLUDE)",
  "liberal": "Perspektif jika Anda bersikap LIBERAL (bias INCLUDE)",
  "recommend": "INCLUDE" atau "EXCLUDE" atau "UNCERTAIN",
  "reason_code": "WAJIB DIISI DARI REASON CODES JIKA EXCLUDE, '-' JIKA INCLUDE/UNCERTAIN",
  "evidence": "Kalimat bukti dari abstract...",
  "confidence": "HIGH" atau "MEDIUM" atau "LOW"
}`, briefing)

	userPrompt := fmt.Sprintf("Title: %s\nKeywords: %s\nAbstract: %s", title, keywords, abstract)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("gagal batch review paper: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	var result model.ScreeningPerspective
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("gagal parsing JSON BatchReviewPaper (%w). Raw: %s", err, rawResponse)
	}
	return &result, nil
}
