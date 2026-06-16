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

func (a *ScreeningAgent) ReviseBriefing(ctx context.Context, currentBriefing string, feedback string) (*model.ScreenerBriefing, error) {
	systemPrompt := `Anda adalah Manajer Sistematic Literature Review.
Tugas Anda mengeksekusi revisi terhadap SCREENER BRIEFING berdasarkan feedback dari pengguna.

Gunakan feedback untuk memperbaiki instruksi. Keluarkan HANYA JSON MURNI (tanpa markdown blok):
{
  "validation_gap": "Update analisis kelengkapan...",
  "decision": "PROCEED",
  "recommendation": "Saran revisi diterapkan...",
  "briefing_doc": "--- SCREENER BRIEFING ..."
}`

	userPrompt := fmt.Sprintf("=== CURRENT BRIEFING ===\n%s\n\n=== FEEDBACK/REVISION REQUEST ===\n%s", currentBriefing, feedback)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("screening_agent gagal memanggil LLM untuk revisi briefing: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	var result model.ScreenerBriefing
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("gagal parsing JSON ScreenerBriefing revisi (%w). Raw: %s", err, rawResponse)
	}

	return &result, nil
}

type ScreeningDecision struct {
	Decision   string `json:"decision"`
	ReasonCode string `json:"reason_code"`
	Strict     string `json:"strict_perspective"`
	Liberal    string `json:"liberal_perspective"`
	VerdictAid string `json:"verdict_aid"`
	Notes      string `json:"-"`
}

func (a *ScreeningAgent) ReviewPaper(ctx context.Context, briefing, title, abstract, keywords string) (*ScreeningDecision, string, error) {
	systemPrompt := fmt.Sprintf(`Anda adalah Reviewer Independen untuk Systematic Literature Review.
Berikut adalah SCREENER BRIEFING yang WAJIB Anda patuhi:
%s

Tugas Anda adalah meninjau Title, Abstract, dan Keywords dari paper yang diberikan, lalu tentukan keputusan Anda:
"INCLUDE", "EXCLUDE", atau "UNCERTAIN".

ATURAN:
1. Jika keputusan "EXCLUDE", Anda WAJIB mengisi field "reason_code" dengan salah satu dari REASON CODES di briefing. Jika keputusan "INCLUDE" atau "UNCERTAIN", field "reason_code" harus dikosongkan (isi "-").
2. Field "strict_perspective", "liberal_perspective", dan "verdict_aid" WAJIB SELALU DIISI terlepas dari apapun keputusannya.

CRITICAL INSTRUCTION: You must respond ONLY with a valid JSON object. Do not include any markdown blocks (like '''json), conversational text, or explanations outside the JSON.
Gunakan urutan berikut di mana perspektif berada di awal agar Anda dapat berpikir (Chain-of-Thought) sebelum menetapkan "decision":
{
  "strict_perspective": "Tuliskan analisis jika Anda bersikap sangat kaku/ketat...",
  "liberal_perspective": "Tuliskan analisis jika Anda bersikap longgar/suportif...",
  "verdict_aid": "Kesimpulan penengah dari kedua perspektif di atas...",
  "decision": "EXCLUDE",
  "reason_code": "STUDY-DESIGN"
}`, briefing)

	userPrompt := fmt.Sprintf("Title: %s\nKeywords: %s\nAbstract: %s", title, keywords, abstract)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, "", fmt.Errorf("gagal review paper: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	var result ScreeningDecision
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, "", fmt.Errorf("gagal parsing JSON ReviewPaper (%w). Raw: %s", err, rawResponse)
	}
	
	result.Notes = fmt.Sprintf("<b>Perspektif Strict:</b> %s<br><br><b>Perspektif Liberal:</b> %s<br><br><b>Verdict-Aid:</b> %s", result.Strict, result.Liberal, result.VerdictAid)
	return &result, rawResponse, nil
}

func (a *ScreeningAgent) BatchReviewPaper(ctx context.Context, briefing, title, abstract, keywords string) (*model.ScreeningPerspective, error) {
	systemPrompt := fmt.Sprintf(`Anda adalah Reviewer Independen untuk Systematic Literature Review.
Berikut adalah SCREENER BRIEFING yang WAJIB Anda patuhi:
%s

Tugas Anda adalah meninjau Title, Abstract, dan Keywords dari paper yang diberikan.

Keluarkan HANYA JSON MURNI tanpa blok markdown dengan struktur berikut:
(PENTING: Semua value HARUS berupa string biasa, dilarang menggunakan nested object atau array!)
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
	result, err := parseLooseScreeningPerspective([]byte(cleanJSON))
	if err != nil {
		return nil, fmt.Errorf("gagal parsing JSON BatchReviewPaper (%w). Raw: %s", err, rawResponse)
	}
	return result, nil
}

// FullTextReviewPaper melakukan screening tahap FULL-TEXT (Modul 6 L2) berbasis RAG.
// `fulltext` adalah konten teks artikel yang diambil dari Qdrant (vektorisasi PEDE).
// Reviewer WAJIB hanya menyimpulkan dari konten RAG ini (anti-halusinasi).
func (a *ScreeningAgent) FullTextReviewPaper(ctx context.Context, operationalDefs, title, fulltext string) (*model.ScreeningPerspective, error) {
	systemPrompt := fmt.Sprintf(`Anda adalah Reviewer Independen untuk FULL-TEXT screening Systematic Literature Review.
Anda menilai berdasarkan ISI FULL-TEXT (bukan sekadar abstract).

OPERATIONAL DEFINITIONS (WHAT COUNTS / WHAT DOESN'T / EDGE CASES):
%s

ATURAN ANTI-HALUSINASI (WAJIB):
- Simpulkan HANYA dari kutipan teks full-text yang diberikan pengguna (konteks RAG).
- DILARANG memakai pengetahuan di luar teks. Jika informasi tidak ada di teks, jangan mengarang.
- Jika teks tidak cukup untuk memutuskan suatu komponen, gunakan "UNCERTAIN".

REASON CODES (12; pakai PERSIS salah satu jika EXCLUDE):
- 8 dari tahap abstrak: P-NOMATCH, I-NOMATCH, O-NOMATCH, STUDY-DESIGN, LANGUAGE, DUPLICATE, NO-ABSTRACT, OTHER
- 4 tambahan full-text: METHODS-UNCLEAR (deskripsi metodologi tak cukup), NO-EMPIRICAL-DATA (konseptual tanpa data empiris), DUPLICATE-POSTHOC (overlap dataset/konten), POOR-QUALITY (kualitas metodologis ekstrem rendah, mis. predatory)

Analisis tiap artikel: (1) STUDY DESIGN dari bagian Methods, (2) POPULATION vs WHAT COUNTS, (3) INTERVENTION/EXPOSURE, (4) OUTCOME + alat ukur, (5) RED FLAGS metodologis untuk QA Modul 7 (sample kecil tanpa power analysis? confounder tak ditangani? follow-up kurang? missing data tak dilaporkan?).

Keluarkan HANYA JSON MURNI tanpa blok markdown:
(PENTING: Semua value HARUS berupa string biasa, dilarang menggunakan nested object atau array!)
{
  "strict": "Perspektif STRICT (bias EXCLUDE) dengan kutipan dari full-text",
  "liberal": "Perspektif LIBERAL (bias INCLUDE) dengan kutipan dari full-text",
  "recommend": "INCLUDE" atau "EXCLUDE" atau "UNCERTAIN",
  "reason_code": "salah satu dari 12 reason code jika EXCLUDE, '-' jika INCLUDE/UNCERTAIN",
  "evidence": "Kutipan kalimat dari Methods/Results sebagai bukti. Awali red flags QA dengan 'QA_RED:' jika ada.",
  "confidence": "HIGH" atau "MEDIUM" atau "LOW"
}`, operationalDefs)

	userPrompt := fmt.Sprintf("Title: %s\n\n=== FULL-TEXT (KONTEKS RAG, satu-satunya sumber yang boleh dipakai) ===\n%s", title, fulltext)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("gagal full-text review paper: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	result, err := parseLooseScreeningPerspective([]byte(cleanJSON))
	if err != nil {
		return nil, fmt.Errorf("gagal parsing JSON FullTextReviewPaper (%w). Raw: %s", err, rawResponse)
	}
	return result, nil
}

type looseScreeningPerspective struct {
	PaperID    string      `json:"paper_id"`
	Title      string      `json:"title"`
	Strict     interface{} `json:"strict"`
	Liberal    interface{} `json:"liberal"`
	Recommend  string      `json:"recommend"`
	ReasonCode string      `json:"reason_code"`
	Evidence   interface{} `json:"evidence"`
	Confidence string      `json:"confidence"`
}

func parseLooseScreeningPerspective(cleanJSON []byte) (*model.ScreeningPerspective, error) {
	var loose looseScreeningPerspective
	if err := json.Unmarshal(cleanJSON, &loose); err != nil {
		return nil, err
	}

	stringify := func(v interface{}) string {
		if v == nil {
			return ""
		}
		if s, ok := v.(string); ok {
			return s
		}
		b, _ := json.Marshal(v)
		return string(b)
	}

	return &model.ScreeningPerspective{
		PaperID:    loose.PaperID,
		Title:      loose.Title,
		Strict:     stringify(loose.Strict),
		Liberal:    stringify(loose.Liberal),
		Recommend:  loose.Recommend,
		ReasonCode: loose.ReasonCode,
		Evidence:   stringify(loose.Evidence),
		Confidence: loose.Confidence,
	}, nil
}

type ResolutionAdvice struct {
	Analysis string `json:"analysis"`
	Advice   string `json:"advice"`
}

func (a *ScreeningAgent) AnalyzeDisagreement(ctx context.Context, briefing, title, abstract, r1Notes, r2Notes string) (*ResolutionAdvice, error) {
	systemPrompt := fmt.Sprintf(`Anda adalah Supervisor / Arbiter untuk Systematic Literature Review.
Berikut adalah SCREENER BRIEFING:
%s

Terdapat kasus DISAGREE atau UNCERTAIN antara Reviewer 1 dan Reviewer 2.
Berikan analisis 1-2 kalimat untuk mencari akar konflik dari 'notes' mereka, dan berikan saran resolusi.
Saran resolusi ("advice") HARUS salah satu dari: "DISCUSS", "DEFER-TO-FULLTEXT", atau "UPDATE-BRIEFING" (jika polanya sistematis).

Keluarkan HANYA JSON MURNI tanpa markdown:
{
  "analysis": "Reviewer 1 fokus pada X, sedangkan Reviewer 2 fokus pada Y...",
  "advice": "DISCUSS"
}`, briefing)

	userPrompt := fmt.Sprintf("Title: %s\nAbstract: %s\nR1 Notes: %s\nR2 Notes: %s", title, abstract, r1Notes, r2Notes)

	rawResponse, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("gagal analyze disagreement: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResponse)
	var result ResolutionAdvice
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("gagal parsing JSON ResolutionAdvice (%w). Raw: %s", err, rawResponse)
	}
	return &result, nil
}

// AuditSlipped identifies one paper that was labelled INCLUDE but violates PICO and
// should have been EXCLUDE. The caller assigns each paper a stable "index"; the model
// returns that index so identity matching is deterministic (no fragile DOI/title echo).
type AuditSlipped struct {
	Index      int    `json:"index"`       // index from the input list (PRIMARY identity)
	Title      string `json:"title"`       // optional, for human-readable cross-check
	ReasonCode string `json:"reason_code"` // P-NOMATCH / I-NOMATCH / C-NOMATCH / O-NOMATCH / S-NOMATCH / DATE-NOMATCH / OTHER
	Reason     string `json:"reason"`
}

type PICOAuditResult struct {
	SlippedThroughCount int            `json:"slipped_through_count"`
	Action              string         `json:"action"`
	Analysis            string         `json:"analysis"`
	Slipped             []AuditSlipped `json:"slipped"`
}

func (a *ScreeningAgent) AuditPICO(ctx context.Context, pico, includedPapersJSON string) (*PICOAuditResult, error) {
	systemPrompt := `Anda adalah Auditor Systematic Literature Review.
Periksa SETIAP paper berlabel "INCLUDE" di bawah terhadap PICO DEFINITIONS, dan temukan yang "lolos" padahal seharusnya EXCLUDE (slipped-through). Audit ketat: utamakan definisi operasional what_counts/what_doesnt_count.

Setiap paper diberi field "index" (angka). Untuk SETIAP paper yang salah-INCLUDE, kembalikan "index"-nya PERSIS (jangan dikarang), title (untuk verifikasi), reason_code (P-NOMATCH/I-NOMATCH/C-NOMATCH/O-NOMATCH/S-NOMATCH/DATE-NOMATCH/OTHER) dan reason ringkas berbasis kriteria.

Keluarkan HANYA JSON MURNI tanpa markdown:
{
  "slipped_through_count": <jumlah item di slipped>,
  "action": "none" (bila tidak ada) atau "re-screening" (bila ada >=1),
  "analysis": "Ringkasan audit batch ini...",
  "slipped": [
    { "index": 3, "title": "...", "reason_code": "I-NOMATCH", "reason": "..." }
  ]
}
Bila tidak ada yang slipped, kembalikan slipped: [] dan slipped_through_count: 0.`
	userPrompt := fmt.Sprintf("=== PICO ===\n%s\n\n=== INCLUDED PAPERS (audit SEMUA; pakai field index sbg identitas) ===\n%s", pico, includedPapersJSON)
	rawResp, err := a.llmProvider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil { return nil, err }

	var res PICOAuditResult
	if err := json.Unmarshal([]byte(CleanJSONResponse(rawResp)), &res); err != nil { return nil, err }
	// Keep the count consistent with the returned list regardless of model arithmetic.
	res.SlippedThroughCount = len(res.Slipped)
	if res.SlippedThroughCount > 0 {
		res.Action = "re-screening"
	} else if res.Action == "" {
		res.Action = "none"
	}
	return &res, nil
}

type PrioritizationResult struct {
	Report string `json:"report"`
}

func (a *ScreeningAgent) PrioritizeFullText(ctx context.Context, includedPapersJSON string) (string, error) {
	systemPrompt := `Anda adalah Asisten Peneliti. Kelompokkan paper INCLUDE berikut menjadi prioritas full-text (HIGH, MEDIUM, LOW) berdasarkan abstract.
Keluarkan output dalam bentuk teks Markdown murni (tanpa awalan markdown blok code).`
	return a.llmProvider.Generate(ctx, systemPrompt, includedPapersJSON)
}
