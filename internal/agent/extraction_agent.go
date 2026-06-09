package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"nsa/internal/llm"
	"nsa/internal/model"
)

// ExtractionAgent menangani Modul 7 (framework, ekstraksi, QA, synthesis prep).
type ExtractionAgent struct {
	client llm.LLMClient
}

func NewExtractionAgent(client llm.LLMClient) *ExtractionAgent {
	return &ExtractionAgent{client: client}
}

// ===== L1: Framework recommendation + extraction template =====

func (a *ExtractionAgent) RecommendFramework(ctx context.Context, pico, rqs, designBreakdown string) (*model.FrameworkSelection, error) {
	systemPrompt := `Anda adalah metodolog Systematic Literature Review.
Pilih FRAMEWORK ekstraksi paling sesuai lalu turunkan TEMPLATE kolom ekstraksi.

Opsi framework:
- TCCM (Theory-Context-Characteristics-Methodology): management/social science; gap Tipe C; RQ "bagaimana konsep X beroperasi di konteks Y".
- ADO (Antecedents-Decisions-Outcomes): decision/consumer/organizational; RQ "apa pemicu, keputusan, hasil"; studi causal/process.
- PICO-BASED: health/medical/intervention; RQ efektivitas intervensi; studi eksperimental/kuasi.
- TEMA (Technical-Evaluation-Methodology-Applicability): computer science/engineering; sistem teknis, performa algoritma.
- D-A-V-E-C (Dataset-Architecture-Validation-Efficiency-Context): AI/Machine Learning/Deep Learning; fokus pada arsitektur model, dataset, komputasi.
- CUSTOM: tidak ada yang fit (wajib justifikasi).

Turunkan kolom template dari framework terpilih. Sertakan kolom Meta (ID, Author, Year, Journal, DOI), kolom inti framework (beri category T/C/Ch/M atau A/D/O dst), Key_Findings (Output), Quality_Score (QA), Notes (Manual).

Keluarkan HANYA JSON MURNI tanpa markdown:
{
  "framework": "TCCM",
  "justification": "3-4 kalimat alasan, siap dipakai di Methods",
  "columns": [
    {"key": "Theory", "category": "T", "desc": "Teori/framework studi (nama + ref)"}
  ]
}`
	userPrompt := fmt.Sprintf("=== PICO DEFINITIONS ===\n%s\n\n=== RESEARCH QUESTIONS ===\n%s\n\n=== STUDY DESIGN BREAKDOWN ===\n%s", pico, rqs, designBreakdown)

	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("RecommendFramework LLM: %w", err)
	}
	var res model.FrameworkSelection
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &res); err != nil {
		return nil, fmt.Errorf("parse FrameworkSelection (%w). Raw: %s", err, raw)
	}
	res.SystemPrompt = systemPrompt
	res.UserPrompt = userPrompt
	return &res, nil
}

// ===== L2: Systematic extraction (RAG) =====

type ExtractedField struct {
	Key      string `json:"key" bson:"key"`
	Value    string `json:"value" bson:"value"`
	Evidence string `json:"evidence" bson:"evidence"` // kutipan + section ref
	Status   string `json:"status" bson:"status"`     // REPORTED / NOT_REPORTED / AMBIGUOUS
}

type ExtractionResult struct {
	Fields      []ExtractedField `json:"fields"`
	KeyFindings string           `json:"key_findings"`
	QARedFlags  string           `json:"qa_red_flags"`
	Ambiguous   []string         `json:"ambiguous"`
	Coverage    string           `json:"coverage"` // COMPLETE / PARTIAL
}

func (a *ExtractionAgent) ExtractPaper(ctx context.Context, columnsJSON, opDefs, title, fulltext string) (*ExtractionResult, error) {
	systemPrompt := fmt.Sprintf(`Anda Extractor utama untuk Systematic Literature Review.
Ekstrak data per kolom TEMPLATE dari FULL-TEXT artikel (konteks RAG).

TEMPLATE KOLOM (JSON):
%s

OPERATIONAL DEFINITIONS:
%s

ATURAN ANTI-HALUSINASI (WAJIB):
- Simpulkan HANYA dari full-text yang diberikan. Dilarang memakai pengetahuan luar.
- Per field: kutip kalimat pendukung + section ref di "evidence" (mis. "Methods p.5: We surveyed 234...").
- Jika tidak ada di teks: value "[NOT REPORTED]", status "NOT_REPORTED" (JANGAN mengira).
- Borderline: status "AMBIGUOUS" + alasan di evidence.
- Konsisten dengan canonical terminology.
- RED FLAGS QA (sample kecil tanpa power analysis, missing data tak dijelaskan, confounder tak ditangani, outcome tak validated) → ringkas di "qa_red_flags" (awali tiap item 'QA_RED:').

Keluarkan HANYA JSON MURNI tanpa markdown:
{
  "fields": [{"key": "Theory", "value": "...", "evidence": "Intro p.2: ...", "status": "REPORTED"}],
  "key_findings": "1-2 kalimat temuan utama",
  "qa_red_flags": "QA_RED: ... ; QA_RED: ...",
  "ambiguous": ["nama field yang ambiguous"],
  "coverage": "COMPLETE"
}`, columnsJSON, opDefs)

	userPrompt := fmt.Sprintf("Title: %s\n\n=== FULL-TEXT (KONTEKS RAG, satu-satunya sumber) ===\n%s", title, fulltext)

	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("ExtractPaper LLM: %w", err)
	}
	var res ExtractionResult
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &res); err != nil {
		return nil, fmt.Errorf("parse ExtractionResult (%w). Raw: %s", err, raw)
	}
	return &res, nil
}

type VerifyResult struct {
	Disagree bool   `json:"disagree"`
	Notes    string `json:"notes"`
}

// VerifyExtraction = spot-verification 20% oleh Extractor 2 (cek entry vs full-text asli).
func (a *ExtractionAgent) VerifyExtraction(ctx context.Context, opDefs, title, fulltext, priorJSON string) (*VerifyResult, error) {
	systemPrompt := fmt.Sprintf(`Anda Extractor verifikator (kedua) untuk SLR.
Bandingkan EXTRACTION ENTRY (hasil extractor 1) dengan FULL-TEXT asli.
Operational defs: %s
Tentukan apakah ada ketidaksesuaian material (value salah/menyimpang dari teks).

Keluarkan HANYA JSON MURNI:
{ "disagree": false, "notes": "ringkas perbedaan material jika ada" }`, opDefs)

	userPrompt := fmt.Sprintf("Title: %s\n\n=== EXTRACTION ENTRY ===\n%s\n\n=== FULL-TEXT (RAG) ===\n%s", title, priorJSON, fulltext)
	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("VerifyExtraction LLM: %w", err)
	}
	var res VerifyResult
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &res); err != nil {
		return nil, fmt.Errorf("parse VerifyResult (%w). Raw: %s", err, raw)
	}
	return &res, nil
}

// AutoResolveField = LLM membaca ulang fulltext untuk menyimpulkan SATU field yang spesifik secara konklusif.
func (a *ExtractionAgent) AutoResolveField(ctx context.Context, opDefs, title, fulltext, fieldKey string) (*ExtractedField, error) {
	systemPrompt := fmt.Sprintf(`Anda AI Penengah Resolusi Ambiguitas untuk Systematic Literature Review.
Sebelumnya, ekstraksi data untuk atribut/kolom berikut dinilai AMBIGU. 
Tugas Anda: Baca ulang teks, hilangkan ambiguitas, dan berikan satu jawaban yang TEGAS (konklusif).

TARGET FIELD YANG HARUS DIRESOLUSI: "%s"

OPERATIONAL DEFINITIONS:
%s

ATURAN ANTI-HALUSINASI (WAJIB):
- Simpulkan HANYA dari full-text yang diberikan.
- Berikan kutipan kalimat pendukung di "evidence".
- Jika benar-benar tidak ada di teks, tulis value "[NOT REPORTED]" dan status "NOT_REPORTED". Dilarang mereka-reka.
- Jika ada informasi, status WAJIB "REPORTED". Jangan membuat status AMBIGUOUS lagi.

Keluarkan HANYA JSON MURNI tanpa markdown:
{
  "key": "%s",
  "value": "Nilai yang konklusif / pasti",
  "evidence": "Intro p.2: ...",
  "status": "REPORTED"
}`, fieldKey, opDefs, fieldKey)

	userPrompt := fmt.Sprintf("Title: %s\n\n=== FULL-TEXT (RAG) ===\n%s", title, fulltext)
	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("AutoResolveField LLM: %w", err)
	}
	var res ExtractedField
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &res); err != nil {
		return nil, fmt.Errorf("parse AutoResolveField (%w). Raw: %s", err, raw)
	}
	return &res, nil
}

// ===== L3: Quality appraisal =====

func (a *ExtractionAgent) SelectQATool(ctx context.Context, designBreakdown string) (*model.QAThresholdJustification, error) {
	systemPrompt := `Anda metodolog QA Systematic Literature Review.
Pilih TOOL critical appraisal berdasarkan distribusi study design, lalu tetapkan THRESHOLD dengan justifikasi 3-lapis.

Panduan tool:
- Machine Learning / AI / Computational Models: CLAIM, TRIPOD-AI, PROBAST-AI, atau ML Reproducibility Checklist.
- 1 design dominan >70%: RCT->Cochrane RoB 2/Jadad; Observational->NOS; Qualitative->CASP/JBI; SLR->AMSTAR 2.
- Lintas-desain: MMAT / JBI set / Kmet (score dinormalisasi 0-100%).
- Sangat heterogen: kombinasi (NOS + CASP), normalisasi 0-100%.

Threshold 3-lapis: (1) berbasis literatur bidang, (2) berbasis tool developer, (3) berbasis feasibility pool studi.

Keluarkan HANYA JSON MURNI tanpa markdown:
{
  "tool": "MMAT",
  "tool_justification": "100-150 kata untuk Methods",
  "threshold": 70,
  "layer_literature": "...",
  "layer_developer": "...",
  "layer_feasibility": "...",
  "categorization": "HIGH >=80% | MODERATE 70-79% | LOW <70%"
}`
	userPrompt := fmt.Sprintf("=== STUDY DESIGN BREAKDOWN ===\n%s", designBreakdown)
	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("SelectQATool LLM: %w", err)
	}
	var res model.QAThresholdJustification
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &res); err != nil {
		return nil, fmt.Errorf("parse QAThreshold (%w). Raw: %s", err, raw)
	}
	return &res, nil
}

type QAResult struct {
	TotalScore   float64 `json:"total_score"` // 0-100
	Category     string  `json:"category"`    // HIGH / MODERATE / LOW
	ItemsSummary string  `json:"items_summary"`
	Reasoning    string  `json:"reasoning"`   // Penjelasan logis mengapa paper ini mendapat skor/kategori tersebut
	Evidence     string  `json:"evidence"`    // Bukti atau kutipan dari teks yang mendukung
}

func (a *ExtractionAgent) AppraiseQuality(ctx context.Context, tool, categorization, justification, title, fulltext string) (*QAResult, error) {
	systemPrompt := fmt.Sprintf(`Anda penilai kualitas (rater) Systematic Literature Review.
Nilai kualitas metodologis artikel memakai tool: %s.
Detail/Framework/Justifikasi tool: %s
Kategorisasi ambang: %s

ATURAN: nilai HANYA dari full-text (konteks RAG). Skor 0-100 (dinormalisasi).
Tetapkan category sesuai ambang.

Keluarkan HANYA JSON MURNI tanpa markdown:
{ "total_score": 78, "category": "MODERATE", "items_summary": "ringkas penilaian per item utama", "reasoning": "alasan skor dan kategori", "evidence": "kutipan bukti teks" }`, tool, justification, categorization)

	userPrompt := fmt.Sprintf("Title: %s\n\n=== FULL-TEXT (RAG) ===\n%s", title, fulltext)
	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("AppraiseQuality LLM: %w", err)
	}
	var res QAResult
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &res); err != nil {
		return nil, fmt.Errorf("parse QAResult (%w). Raw: %s", err, raw)
	}
	return &res, nil
}

// ===== L4: Synthesis preparation =====

func (a *ExtractionAgent) PrepareSynthesis(ctx context.Context, extractionSummaryJSON string) (*model.SynthesisPrep, error) {
	systemPrompt := `Anda metodolog sintesis Systematic Literature Review.
Berdasarkan ringkasan data ekstraksi + QA, susun PERSIAPAN SINTESIS untuk Modul 8.

Tentukan:
1. DESCRIPTIVE OVERVIEW (distribusi design/geografis/tahun/framework/kualitas).
2. HETEROGENEITY VERDICT: LOW / MODERATE / HIGH / VERY HIGH (klinis+metodologis+statistik).
3. META-ANALYSIS FEASIBILITY (5-criteria): heterogeneity LOW/MODERATE; >=3 studi outcome comparable; effect size tersedia; design sebanding; operational def outcome >=80% mirip.
   - SEMUA ya -> "JALUR B" (meta-analysis); ada yang tidak -> "JALUR A" (narrative, default); subset homogen -> "HYBRID".
4. FRAMEWORK-DRIVEN GROUPINGS untuk narrative (group per komponen framework).

PERINGATAN: jangan klaim pooled effect tanpa meta-analysis formal; keputusan jalur harus tegas.

Keluarkan HANYA JSON MURNI tanpa markdown:
{
  "descriptive_overview": "...",
  "heterogeneity_verdict": "MODERATE",
  "meta_feasibility": "JALUR A",
  "criteria_check": "breakdown 5 kriteria (ya/tidak + alasan)",
  "groupings": "Group 1: ...; Group 2: ...",
  "markdown": "ringkasan rapi markdown gabungan semua poin di atas"
}`
	userPrompt := fmt.Sprintf("=== RINGKASAN EKSTRAKSI + QA ===\n%s", extractionSummaryJSON)
	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("PrepareSynthesis LLM: %w", err)
	}
	// criteria_check & groupings kadang dikembalikan LLM sebagai objek/array, bukan string —
	// terima fleksibel via json.RawMessage lalu ratakan ke string.
	var p struct {
		DescriptiveOverview  string          `json:"descriptive_overview"`
		HeterogeneityVerdict string          `json:"heterogeneity_verdict"`
		MetaFeasibility      string          `json:"meta_feasibility"`
		CriteriaCheck        json.RawMessage `json:"criteria_check"`
		Groupings            json.RawMessage `json:"groupings"`
		Markdown             string          `json:"markdown"`
	}
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &p); err != nil {
		return nil, fmt.Errorf("parse SynthesisPrep (%w). Raw: %s", err, raw)
	}
	return &model.SynthesisPrep{
		DescriptiveOverview:  p.DescriptiveOverview,
		HeterogeneityVerdict: p.HeterogeneityVerdict,
		MetaFeasibility:      p.MetaFeasibility,
		CriteriaCheck:        rawToString(p.CriteriaCheck),
		Groupings:            rawToString(p.Groupings),
		Markdown:             p.Markdown,
	}, nil
}

// rawToString meratakan json.RawMessage: jika string JSON -> nilainya; selain itu -> JSON apa adanya.
func rawToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}
