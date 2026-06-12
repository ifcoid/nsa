package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"nsa/internal/llm"
	"nsa/internal/model"
)

// Exported system prompt constants for xAI transparency (M8 L2).
const DecidePathSystemPrompt = `Anda metodolog Systematic Literature Review.
Tentukan SYNTHESIS PATH. DEFAULT = "JALUR A" (narrative). UPGRADE ke "JALUR B" (meta-analysis)
HANYA jika SEMUA 5 syarat lolos:
1. Heterogeneity LOW atau MODERATE
2. >=3 studi outcome comparable (konstruk & measurement sama)
3. Effect size data tersedia & ekstraktabel konsisten
4. Design studi sebanding
5. Operational def outcome >=80% mirip

Jika ada NO -> "JALUR A". Subset homogen saja -> "HYBRID" (jarang). Jika ambigu -> pilih JALUR A.

Keluarkan HANYA JSON MURNI tanpa markdown:
{ "verdict": "JALUR A", "criteria_check": "checklist per-syarat (ya/tidak + alasan singkat)", "rationale": "3-4 kalimat untuk Methods" }`

const NarrativeSynthesisSystemPromptTemplate = `Anda penyintesis Systematic Literature Review (JALUR A — NARRATIVE/THEMATIC).
Susun sintesis naratif (Markdown, Bahasa Indonesia) berbasis framework %s.

PERINGATAN BAHASA (JALUR A): DILARANG "pooled effect"/"overall effect size"/"d = X across studies".
BOLEH: "synthesis", "pola tematik", "evidence suggests", "konsisten lintas studi", indicative range studi individual.

Cakup: (A) Theory synthesis, (B) Context synthesis (sebut dominasi geografis), (C) Characteristics,
(D) Methodology, (E) Pattern: consistent findings + contradictory findings + emerging/under-researched,
(F) Vote counting qualified, (G) Quality-stratified (HIGH vs MOD vs LOW), (H) Narrative integration PER RQ.

Keluarkan HANYA Markdown (tanpa code fence).`

const MetaScaffoldSystemPrompt = `Anda advisor meta-analysis Systematic Literature Review (JALUR B).
PENTING: statistik dihitung di software eksternal (R metafor / Stata / RevMan). JANGAN mengarang pooled effect.
Tugas Anda: scaffold pelaporan (Markdown) + skrip R (metafor) siap-jalan untuk forest plot.

Markdown cakup: model selection (RE/REML default), effect size standardization, rencana heterogeneity
(Q, I2, tau2, PI), publication bias (jika >=10 studi), subgroup/meta-regression plan, catatan PRISMA 2020.
forest_plot_script: skrip R metafor lengkap (placeholder data) yang menghasilkan forest plot SVG+PNG.

Keluarkan HANYA JSON MURNI tanpa markdown blok:
{ "markdown": "scaffold pelaporan...", "forest_plot_script": "library(metafor)\n..." }`

// SynthesisAgent menangani Modul 8 (analysis + synthesis + GRADE + interpretation).
type SynthesisAgent struct {
	client llm.LLMClient
}

func NewSynthesisAgent(client llm.LLMClient) *SynthesisAgent {
	return &SynthesisAgent{client: client}
}

// ===== L1: heterogeneity deep-dive =====

type HeterogeneityResult struct {
	Verdict   string `json:"verdict"`
	Narrative string `json:"narrative"`
}

func (a *SynthesisAgent) Heterogeneity(ctx context.Context, descriptiveJSON, priorVerdict string) (*HeterogeneityResult, error) {
	systemPrompt := `Anda metodolog Systematic Literature Review.
Lakukan HETEROGENEITY DEEP-DIVE (klinis/kontekstual + metodologis + statistik bila ada).
Tetapkan verdict: "LOW", "MODERATE", "HIGH", atau "VERY HIGH".
- LOW: homogen → meta-analysis kuat.
- MODERATE: meta feasible random-effects.
- HIGH: meta berisiko, prefer narrative.
- VERY HIGH: narrative only.

Keluarkan HANYA JSON MURNI tanpa markdown:
{ "verdict": "MODERATE", "narrative": "2-4 kalimat ringkas untuk Discussion (sebut bias geografis bila ada)" }`
	userPrompt := fmt.Sprintf("Verdict awal (M7): %s\n\n=== DESCRIPTIVE DATA ===\n%s", priorVerdict, descriptiveJSON)
	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("Heterogeneity LLM: %w", err)
	}
	var res HeterogeneityResult
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &res); err != nil {
		return nil, fmt.Errorf("parse Heterogeneity (%w). Raw: %s", err, raw)
	}
	return &res, nil
}

// ===== L2: synthesis path decision =====

func (a *SynthesisAgent) DecidePath(ctx context.Context, heterogeneityVerdict, synthesisPrepJSON string) (*model.SynthesisPathDecision, error) {
	systemPrompt := DecidePathSystemPrompt
	userPrompt := fmt.Sprintf("Heterogeneity verdict (L1): %s\n\n=== SYNTHESIS PREP (M7) ===\n%s", heterogeneityVerdict, synthesisPrepJSON)
	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("DecidePath LLM: %w", err)
	}
	// criteria_check sering dikembalikan LLM sebagai objek -> terima fleksibel.
	var p struct {
		Verdict       string          `json:"verdict"`
		CriteriaCheck json.RawMessage `json:"criteria_check"`
		Rationale     string          `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &p); err != nil {
		return nil, fmt.Errorf("parse SynthesisPathDecision (%w). Raw: %s", err, raw)
	}
	return &model.SynthesisPathDecision{Verdict: p.Verdict, CriteriaCheck: rawToString(p.CriteriaCheck), Rationale: p.Rationale}, nil
}

// ===== L2: Jalur A narrative synthesis =====

func (a *SynthesisAgent) NarrativeSynthesis(ctx context.Context, framework, dataSummaryJSON, rqsJSON string) (string, error) {
	systemPrompt := fmt.Sprintf(NarrativeSynthesisSystemPromptTemplate, framework)
	userPrompt := fmt.Sprintf("=== DATA SUMMARY (ekstraksi+QA) ===\n%s\n\n=== RESEARCH QUESTIONS ===\n%s", dataSummaryJSON, rqsJSON)
	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("NarrativeSynthesis LLM: %w", err)
	}
	return stripMarkdownFence(raw), nil
}

// ===== L2: Jalur B meta-analysis scaffold =====

type MetaScaffoldResult struct {
	Markdown         string `json:"markdown"`
	ForestPlotScript string `json:"forest_plot_script"`
}

func (a *SynthesisAgent) MetaScaffold(ctx context.Context, dataSummaryJSON string) (*MetaScaffoldResult, error) {
	systemPrompt := MetaScaffoldSystemPrompt
	raw, err := a.client.Generate(ctx, systemPrompt, fmt.Sprintf("=== DATA SUMMARY ===\n%s", dataSummaryJSON))
	if err != nil {
		return nil, fmt.Errorf("MetaScaffold LLM: %w", err)
	}
	var res MetaScaffoldResult
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &res); err != nil {
		return nil, fmt.Errorf("parse MetaScaffold (%w). Raw: %s", err, raw)
	}
	return &res, nil
}

// ===== L3: GRADE evidence grading =====

func (a *SynthesisAgent) Grade(ctx context.Context, synthesisJSON, qaJSON string) (*model.GradeEvidence, error) {
	systemPrompt := `Anda penilai GRADE Systematic Literature Review.
Grade confidence evidence per outcome/RQ via 5 domain GRADE: study limitations (RoB), inconsistency,
indirectness, imprecision, publication bias. Level: HIGH/MODERATE/LOW/VERY LOW.

Lalu ROBUSTNESS summary (sensitivity, subgroup, publication bias, missing studies) -> verdict
"ROBUST"/"CONDITIONALLY ROBUST"/"NOT ROBUST". Lalu confidence statements untuk Discussion.

Keluarkan HANYA JSON MURNI tanpa markdown blok:
{
  "table_markdown": "| Outcome | Studies | RoB | Inconsistency | Indirectness | Imprecision | Pub Bias | Overall GRADE |\n|...|",
  "robustness_verdict": "CONDITIONALLY ROBUST",
  "robustness_summary": "ringkas robustness",
  "confidence_statements": "1-2 kalimat confidence per temuan utama"
}`
	userPrompt := fmt.Sprintf("=== SYNTHESIS RESULTS ===\n%s\n\n=== QA / SENSITIVITY ===\n%s", synthesisJSON, qaJSON)
	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("Grade LLM: %w", err)
	}
	// table_markdown/summary/statements bisa dikembalikan sebagai objek/array -> fleksibel.
	var p struct {
		TableMarkdown        json.RawMessage `json:"table_markdown"`
		RobustnessVerdict    string          `json:"robustness_verdict"`
		RobustnessSummary    json.RawMessage `json:"robustness_summary"`
		ConfidenceStatements json.RawMessage `json:"confidence_statements"`
	}
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &p); err != nil {
		return nil, fmt.Errorf("parse GradeEvidence (%w). Raw: %s", err, raw)
	}
	return &model.GradeEvidence{
		TableMarkdown:        rawToString(p.TableMarkdown),
		RobustnessVerdict:    p.RobustnessVerdict,
		RobustnessSummary:    rawToString(p.RobustnessSummary),
		ConfidenceStatements: rawToString(p.ConfidenceStatements),
	}, nil
}

// ===== L4: interpretation package =====

func (a *SynthesisAgent) Interpretation(ctx context.Context, bundleJSON string) (string, error) {
	systemPrompt := `Anda penyintesis akhir Systematic Literature Review.
Susun INTERPRETATION PACKAGE (Markdown, Bahasa Indonesia) untuk Modul 9, mencakup:
1. Answers to RQs (primary + secondary, grounded + GRADE confidence)
2. 3-5 key findings (headline)
3. Surprising/unexpected findings
4. Contradictions worth discussing
5. Dialog dengan teori (konfirmasi/perluas/menantang; teori under-utilized)
6. Heterogeneity narrative (incl. bias geografis)
7. Gaps untuk Future Research (3 HIGH + 2 MEDIUM + 1 LONG-TERM, tiap gap: RQ + metodologi)
8. Limitations self-audit 3-tier (review/study/synthesis level)

Keluarkan HANYA Markdown (tanpa code fence).`
	raw, err := a.client.Generate(ctx, systemPrompt, fmt.Sprintf("=== BUNDLE (descriptive+synthesis+grade) ===\n%s", bundleJSON))
	if err != nil {
		return "", fmt.Errorf("Interpretation LLM: %w", err)
	}
	return stripMarkdownFence(raw), nil
}
