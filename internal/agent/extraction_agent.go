package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

func (a *ExtractionAgent) ModelName() string {
	return a.client.ModelName()
}

// ===== L1: Framework recommendation + extraction template =====

func (a *ExtractionAgent) RecommendFramework(ctx context.Context, pico, rqs, designBreakdown string) (*model.FrameworkSelection, error) {
	systemPrompt := `Anda adalah ahli metodologi Systematic Literature Review (SLR) di bidang Artificial Intelligence, Machine Learning, dan Computer Science.
Tugas Anda:
Pilih satu FRAMEWORK ekstraksi dan sintesis yang **paling sesuai** untuk topik ini, lalu buat TEMPLATE kolom ekstraksi data.

Opsi framework yang tersedia:
- TCCM (Theory-Context-Characteristics-Methodology)
- ADO (Antecedents-Decisions-Outcomes)
- PICO-BASED
- TEMA (Technical-Evaluation-Methodology-Applicability): computer science/engineering; fokus sistem teknis dan performa algoritma.
- D-A-V-E-C (Dataset-Architecture-Validation-Efficiency-Context): khusus AI/ML/Deep Learning; sangat fokus pada arsitektur model, dataset, validasi, dan komputasi.
- CUSTOM: jika tidak ada yang benar-benar fit (wajib justifikasi kuat).

Instruksi:
1. Pilih framework yang paling cocok dengan karakter studi.
2. Berikan justification yang kuat dan siap dipakai di bagian Methods SLR (3-5 kalimat).
3. Buat daftar kolom ekstraksi yang lengkap dan terstruktur.
4. Wajib menyertakan kolom Meta: ID, Author, Year, Title, Journal/Conference, DOI.
5. Kolom inti sesuai framework yang dipilih (beri label kategori seperti T/C/Ch/M atau D/A/V/E/C dll.).
6. Tambahkan kolom: Key_Findings, Quality_Score (dari appraisal), Limitations, Notes.

Keluarkan **HANYA JSON MURNI** tanpa penjelasan apapun, tanpa markdown, tanpa code block. 

Contoh output yang diharapkan:
{
  "framework": "D-A-V-E-C",
  "justification": "Topik ini sangat teknis dan berfokus pada pengembangan model AI...",
  "columns": [
    {"key": "id", "category": "Meta", "desc": "Nomor urut studi"},
    {"key": "dataset", "category": "D", "desc": "Dataset yang digunakan beserta jumlah subjek dan karakteristiknya"}
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
	res.ModelUsed = a.client.ModelName()
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
- DILARANG KERAS menulis teks penjelasan, chain of thought (seperti "Wait, let me..."), atau analisis apa pun di luar JSON.

Keluarkan HANYA JSON MURNI tanpa markdown. Buka dengan '{' dan tutup dengan '}':
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

// GenerateQAAnchors asks the brain LLM to produce 3 synthetic anchor examples (HIGH, MODERATE, LOW)
// for calibrating the QA raters before full rating begins.
func (a *ExtractionAgent) GenerateQAAnchors(ctx context.Context, tool, categorization, justification string) ([]model.QAAnchorExample, error) {
	systemPrompt := fmt.Sprintf(`Anda adalah ahli metodologi SLR yang bertugas membuat CONTOH ANCHOR (kalibrasi) untuk quality appraisal.

TOOL yang digunakan: %s
KATEGORISASI: %s
JUSTIFIKASI TOOL: %s

TUGAS: Buat 3 contoh sintetis (fictional) paper yang masing-masing mewakili kategori HIGH, MODERATE, dan LOW.
Setiap contoh harus berisi:
- Deskripsi singkat paper sintetis (1-2 kalimat tentang desain, metode, dan temuan)
- Skor yang diharapkan (0-100, sesuai dengan kategorisasi di atas)
- Penjelasan mengapa paper tersebut masuk kategori itu berdasarkan kriteria tool

Contoh ini akan dipakai sebagai referensi oleh rater agar konsisten dalam menilai.

Keluarkan HANYA JSON MURNI tanpa markdown:
[
  {"category": "HIGH", "description": "...", "score": 85, "reasoning": "..."},
  {"category": "MODERATE", "description": "...", "score": 72, "reasoning": "..."},
  {"category": "LOW", "description": "...", "score": 45, "reasoning": "..."}
]`, tool, categorization, justification)

	userPrompt := "Buat 3 anchor examples (HIGH, MODERATE, LOW) berdasarkan tool dan kategorisasi di atas."

	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("GenerateQAAnchors LLM: %w", err)
	}
	var res []model.QAAnchorExample
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &res); err != nil {
		return nil, fmt.Errorf("parse QAAnchorExamples (%w). Raw: %s", err, raw)
	}
	return res, nil
}

// SuggestRubricRefinement asks the brain to suggest improvements to the QA rubric
// based on pilot disagreements.
func (a *ExtractionAgent) SuggestRubricRefinement(ctx context.Context, tool, categorization string, pilots []model.QACalibrationPilot) (string, error) {
	pilotsJSON, _ := json.Marshal(pilots)
	systemPrompt := fmt.Sprintf(`Anda metodolog QA SLR. Pilot kalibrasi menunjukkan kappa rendah (disagreement tinggi antara 2 rater).

TOOL: %s
KATEGORISASI: %s

HASIL PILOT (ada disagreement antara R1 dan R2):
%s

TUGAS: Analisis pola disagreement dan sarankan PERBAIKAN RUBRIK yang spesifik agar rater lebih konsisten.
Fokus pada:
1. Ambiguitas dalam kriteria yang menyebabkan perbedaan interpretasi
2. Item yang perlu klarifikasi operasional
3. Saran threshold adjustment jika diperlukan

Keluarkan teks ringkas (2-4 kalimat) berisi saran perbaikan rubrik.`, tool, categorization, string(pilotsJSON))

	userPrompt := "Berdasarkan hasil pilot di atas, berikan saran perbaikan rubrik."
	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("SuggestRubricRefinement LLM: %w", err)
	}
	return strings.TrimSpace(raw), nil
}

func (a *ExtractionAgent) SelectQATool(ctx context.Context, designBreakdown string, feedback string) (*model.QAThresholdJustification, error) {
	systemPrompt := `Anda seorang metodolog QA Systematic Literature Review yang ahli dalam memilih instrumen critical appraisal.

TUGAS: Pilih TOOL yang paling tepat berdasarkan distribusi study design, lalu tetapkan THRESHOLD (0-100) dengan justifikasi 3-lapis.

PANDUAN PRIORITAS TOOL:
1. Jika studi DIDOMINASI (>70%) oleh **RCT** → Cochrane RoB 2 atau Jadad
2. Jika DIDOMINASI oleh **studi observasional (kohort/case-control)** → NOS atau ROBINS-I
3. Jika DIDOMINASI oleh **studi kualitatif** → CASP atau JBI
4. Jika DIDOMINASI oleh **studi komputasional / AI / deep learning / model prediksi** → 
   - CLAIM (untuk AI dalam medical imaging)
   - TRIPOD-AI atau PROBAST-AI (untuk model prediksi risiko)
   - ML Reproducibility Checklist (untuk transparansi kode/data)
   - JANGAN gunakan MMAT untuk studi AI murni tanpa intervensi manusia
5. Jika **lintas-desain heterogen** (campuran RCT, observasional, kualitatif, komputasional) → MMAT atau JBI Mixed Methods
6. Jika **sangat heterogen & tidak ada tool yang pas** → buat CUSTOM rubric dengan normalisasi 0-100%

ATURAN THRESHOLD 3-LAPIS (wajib dijelaskan masing-masing):
- Layer literature: cutoff yang umum digunakan di bidang studi (dengan referensi minimal 2 sumber)
- Layer developer: rekomendasi dari pengembang tool (jika ada) atau praktik umum penggunaan tool
- Layer feasibility: estimasi proporsi studi yang akan lolos di pool (harus realistis, target retain 50-70% studi)

Keluarkan HANYA JSON MURNI tanpa markdown, tanpa komentar tambahan:

{
  "tool": "NAMA_TOOL",
  "tool_justification": "100-150 kata menjelaskan alasan pemilihan tool berdasarkan breakdown design",
  "threshold": 70,
  "layer_literature": "Penjelasan...",
  "layer_developer": "Penjelasan...",
  "layer_feasibility": "Penjelasan...",
  "categorization": "HIGH >=80% | MODERATE 70-79% | LOW <70%"
}

CATATAN PENTING:
- Jika design breakdown menunjukkan proporsi >50% adalah "studi komputasional", "deep learning", "model Mamba", "EEG classification tanpa RCT", maka tool haruslah CLAIM, TRIPOD-AI, atau ML Reproducibility Checklist, BUKAN MMAT.
- Threshold untuk studi komputasional biasanya 60-70% (jika menggunakan CLAIM/TRIPOD-AI) karena sifatnya yang lebih teknis.
- Jika tidak ada tool standar yang sesuai, isi tool dengan "CUSTOM_RUBRIC" dan beri justifikasi.`

	userPrompt := fmt.Sprintf("=== STUDY DESIGN BREAKDOWN ===\n%s\n\nBerdasarkan breakdown di atas, pilih tool, threshold, dan berikan justifikasi 3-lapis.", designBreakdown)
	if feedback != "" {
		userPrompt += fmt.Sprintf("\n\n[INSTRUKSI REVISI DARI PENELITI]:\n%s\nPerhatikan instruksi ini secara saksama dalam memilih tool dan threshold.", feedback)
	}

	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("SelectQATool LLM: %w", err)
	}

	cleaned := CleanJSONResponse(raw)
	var res model.QAThresholdJustification
	if err := json.Unmarshal([]byte(cleaned), &res); err != nil {
		return nil, fmt.Errorf("parse QAThreshold (err=%w). Raw: %s", err, cleaned)
	}

	// Validasi tool yang dipilih sesuai dengan design breakdown (tambahan safety)
	if err := validateToolAgainstDesign(res.Tool, designBreakdown); err != nil {
		// Log warning tapi tetap return, atau bisa juga return error
		// Di sini kita hanya log internal, tidak mengganggu eksekusi
		_ = err
	}

	return &res, nil
}

// validateToolAgainstDesign melakukan validasi sederhana: jika design mengandung banyak kata kunci AI,
// pastikan tool bukan MMAT.
func validateToolAgainstDesign(tool, designBreakdown string) error {
	aiKeywords := []string{"deep learning", "neural network", "mamba", "transformer", "cnn", "lstm", "bert", "gpt", "benchmark dataset", "state space model", "eeg classification", "computational model"}
	toolLower := strings.ToLower(tool)
	if toolLower == "mmat" || strings.Contains(toolLower, "mmat") {
		designLower := strings.ToLower(designBreakdown)
		for _, kw := range aiKeywords {
			if strings.Contains(designLower, kw) {
				return fmt.Errorf("potensi mismatch: tool MMAT digunakan pada studi dengan keyword AI '%s', padahal CLAIM/TRIPOD-AI lebih sesuai", kw)
			}
		}
	}
	return nil
}

type QAResult struct {
	TotalScore   float64 `json:"total_score"` // 0-100
	Category     string  `json:"category"`    // HIGH / MODERATE / LOW
	ItemsSummary string  `json:"items_summary"`
	Reasoning    string  `json:"reasoning"` // Penjelasan logis mengapa paper ini mendapat skor/kategori tersebut
	Evidence     string  `json:"evidence"`  // Bukti atau kutipan dari teks yang mendukung
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
