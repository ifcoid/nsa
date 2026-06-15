package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/agent"
	"nsa/internal/llm"
	"nsa/internal/logger"
	"nsa/internal/model"
)

type M3Search struct {
	deps *ModuleDeps
}

func NewM3Search(deps *ModuleDeps) *M3Search {
	return &M3Search{deps: deps}
}

func (m *M3Search) Name() string {
	return "M3_Search"
}

func (m *M3Search) Execute(ctx context.Context, session *model.SLRSession) error {
	ctx = llm.WithXAIContext(ctx, session.ID, session.Status, "M3Search")
	switch session.Status {
	// =========================================================================
	// LANGKAH 1: DATABASE SELECTION + JUSTIFICATION
	// =========================================================================
	case "M3_INIT", "M3_STEP1_DATABASE_SELECTION":
		logger.Log(session.ID, "   [Langkah 3.1] Menganalisis lanskap Database Selection...")
		llmBrain, err := m.deps.LLMFactory.BrainClient(ctx)
		if err != nil { return err }

		picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")
		scopeBytes, _ := json.MarshalIndent(session.ScopeJustifications, "", "  ")

		logger.Log(session.ID, "   [API] Memanggil Gemini (gemini-pro-latest) dengan Google Search Grounding...")
		dbAgent := agent.NewDBSelectionAgent(llmBrain)
		dbResult, rawOutput, err := dbAgent.Analyze(ctx, string(picoBytes), string(scopeBytes))
		if err != nil { return err }

		logger.Log(session.ID, "   [LLM Raw Output]:\n"+rawOutput)

		session.DatabaseSelection = dbResult
		session.Status = "M3_STEP1_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] Rekomendasi Database berhasil dibuat. DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP1_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi dikunci. Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Buka document sesi Anda, cari 'database_selection'.")
		logger.Log(session.ID, "   2. Verifikasi matriks database dan justifikasi finalnya.")
		logger.Log(session.ID, "   3a. Jika SUDAH sesuai, ubah 'status' menjadi 'M3_STEP1_APPROVED'.")
		logger.Log(session.ID, "   3b. Jika BUTUH REVISI, ubah 'status' ke 'M3_STEP1_NEEDS_REVISION' dan isi 'feedback'.")
		return nil

	case "M3_STEP1_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 3.1] Memperbaiki Database Selection berdasarkan feedback: '%s'\n", session.Feedback)

		picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")
		scopeBytes, _ := json.MarshalIndent(session.ScopeJustifications, "", "  ")
		
		scopeContext := string(scopeBytes) + fmt.Sprintf("\n\n[INSTRUKSI REVISI DARI PENELITI]:\n%s", session.Feedback)

		llmBrain, err := m.deps.LLMFactory.BrainClient(ctx)
		if err != nil { return err }

		logger.Log(session.ID, "   [API] Memanggil Gemini (gemini-pro-latest) dengan Google Search Grounding...")
		dbAgent := agent.NewDBSelectionAgent(llmBrain)
		dbResult, rawOutput, err := dbAgent.Analyze(ctx, string(picoBytes), scopeContext)
		if err != nil { return err }

		logger.Log(session.ID, "   [LLM Raw Output]:\n"+rawOutput)

		session.DatabaseSelection = dbResult
		session.Feedback = ""
		session.Status = "M3_STEP1_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] Database Selection direvisi. DIJEDA kembali menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP1_APPROVED":
		logger.Log(session.ID, "   [Langkah 3.1] Database Selection disetujui! Lanjut ke Keywords Development...")
		session.Status = "M3_STEP2_KEYWORDS"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 2: KEYWORDS DEVELOPMENT
	// =========================================================================
	case "M3_STEP2_KEYWORDS":
		logger.Log(session.ID, "   [Langkah 3.2] Mengembangkan Keywords (PICO + Avoid List)...")
		llmBrain, err := m.deps.LLMFactory.BrainClient(ctx)
		if err != nil { return err }

		picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")

		kwAgent := agent.NewKeywordsAgent(llmBrain)
		kwResult, err := kwAgent.DevelopKeywords(ctx, string(picoBytes))
		if err != nil { return err }

		session.Keywords = kwResult
		session.Status = "M3_STEP2_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] Dokumen Keywords berhasil dibuat. DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP2_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi dikunci. Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Buka document sesi Anda, cari array 'keywords'.")
		logger.Log(session.ID, "   2. Verifikasi kesesuaian main_synonyms dan AVOID list untuk setiap P, I, C, dan O.")
		logger.Log(session.ID, "   3a. Jika SUDAH sesuai, ubah 'status' menjadi 'M3_STEP2_APPROVED'.")
		logger.Log(session.ID, "   3b. Jika BUTUH REVISI, ubah 'status' ke 'M3_STEP2_NEEDS_REVISION' dan isi 'feedback'.")
		return nil

	case "M3_STEP2_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 3.2] Memperbaiki Keywords berdasarkan feedback: '%s'\n", session.Feedback)

		picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")
		
		picoContext := string(picoBytes) + fmt.Sprintf("\n\n[INSTRUKSI REVISI DARI PENELITI]:\n%s", session.Feedback)

		llmBrain, err := m.deps.LLMFactory.BrainClient(ctx)
		if err != nil { return err }

		kwAgent := agent.NewKeywordsAgent(llmBrain)
		kwResult, err := kwAgent.DevelopKeywords(ctx, picoContext)
		if err != nil { return err }

		session.Keywords = kwResult
		session.Feedback = ""
		session.Status = "M3_STEP2_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] Keywords direvisi. DIJEDA kembali menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP2_APPROVED":
		logger.Log(session.ID, "   [Langkah 3.2] Keywords disetujui! Lanjut ke Search String + Filter Specifications...")
		session.Status = "M3_STEP3_SEARCH_STRING"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 3: SEARCH STRING + FILTER SPECIFICATIONS
	// =========================================================================
	case "M3_STEP3_SEARCH_STRING":
		logger.Log(session.ID, "   [Langkah 3.3] Merangkai Search String & Filter Specifications...")
		llmBrain, err := m.deps.LLMFactory.BrainClient(ctx)
		if err != nil { return err }

		kwBytes, _ := json.MarshalIndent(session.Keywords, "", "  ")
		scopeBytes, _ := json.MarshalIndent(session.ScopeJustifications, "", "  ")

		ssAgent := agent.NewSearchStringAgent(llmBrain)
		ssResult, err := ssAgent.BuildSearchString(ctx, string(kwBytes), string(scopeBytes))
		if err != nil { return err }

		session.SearchString = ssResult
		session.Status = "M3_STEP3_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] Search String berhasil dirangkai. DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP3_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi dikunci. Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Buka document sesi Anda, cari objek 'search_string'.")
		logger.Log(session.ID, "   2. Verifikasi sintaks query Scopus (wildcard, quotation, kurung, OR/AND).")
		logger.Log(session.ID, "   3. Verifikasi array 'filters' memiliki justifikasi.")
		logger.Log(session.ID, "   4a. Jika SUDAH tepat, ubah 'status' menjadi 'M3_STEP3_APPROVED'.")
		logger.Log(session.ID, "   4b. Jika BUTUH REVISI, ubah 'status' ke 'M3_STEP3_NEEDS_REVISION' dan isi 'feedback'.")
		return nil

	case "M3_STEP3_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 3.3] Memperbaiki Search String berdasarkan feedback: '%s'\n", session.Feedback)

		kwBytes, _ := json.MarshalIndent(session.Keywords, "", "  ")
		scopeBytes, _ := json.MarshalIndent(session.ScopeJustifications, "", "  ")
		
		scopeContext := string(scopeBytes) + fmt.Sprintf("\n\n[INSTRUKSI REVISI DARI PENELITI]:\n%s", session.Feedback)

		llmBrain, err := m.deps.LLMFactory.BrainClient(ctx)
		if err != nil { return err }

		ssAgent := agent.NewSearchStringAgent(llmBrain)
		ssResult, err := ssAgent.BuildSearchString(ctx, string(kwBytes), scopeContext)
		if err != nil { return err }

		session.SearchString = ssResult
		session.Feedback = ""
		session.Status = "M3_STEP3_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] Search String direvisi. DIJEDA kembali menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP3_APPROVED":
		logger.Log(session.ID, "   [Langkah 3.3] Search String disetujui! Lanjut ke Pre-Validasi & Eksekusi...")
		// Clear feedback from M4 revision to avoid polluting M3_STEP4_EVALUATION
		session.Feedback = ""
		session.Status = "M3_STEP4_PRE_VALIDATION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 4: PRE-VALIDASI + EKSEKUSI
	// =========================================================================
	case "M3_STEP4_PRE_VALIDATION":
		logger.Log(session.ID, "   [Langkah 3.4 Fase 1] Melakukan Pre-Validasi Search String...")
		llmBrain, err := m.deps.LLMFactory.BrainClient(ctx)
		if err != nil { return err }

		ssBytes, _ := json.MarshalIndent(session.SearchString, "", "  ")

		execAgent := agent.NewExecutionAgent(llmBrain)
		analysis, err := execAgent.PreValidate(ctx, string(ssBytes))
		if err != nil { return err }

		logger.Log(session.ID, "\n=== HASIL PRE-VALIDASI ===")
		logger.Log(session.ID, analysis)
		logger.Log(session.ID, "==========================\n")

		if session.SearchString != nil {
			session.SearchString.PreValidation = analysis
		}
		session.Status = "M3_STEP4_WAITING_EXECUTION"
		
		logger.Log(session.ID, "   [System] FASE 2: INSTRUKSI EKSEKUSI MANUAL DI SCOPUS")
		logger.Log(session.ID, "   1. Buka Scopus Advanced Search.")
		logger.Log(session.ID, "   2. Masukkan search_string final Anda dan aplikasikan filter.")
		logger.Log(session.ID, "   3. Buka MongoDB Compass, isi field 'feedback' dengan hasil pencarian Anda.")
		logger.Log(session.ID, "      (Contoh isi: 'Scopus pre-filter: 500, post-filter: 150')")
		logger.Log(session.ID, "   4. Ubah status menjadi 'M3_STEP4_EVALUATION' lalu klik Update.")
		
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP4_WAITING_EXECUTION":
		logger.Log(session.ID, "   [System] Menunggu Anda memasukkan hasil eksekusi Scopus di field 'feedback'.")
		logger.Log(session.ID, "   Jika sudah, ubah 'status' menjadi 'M3_STEP4_EVALUATION' dan Update.")
		return nil

	case "M3_STEP4_EVALUATION":
		logger.Log(session.ID, "   [Langkah 3.4 Fase 3] Mengevaluasi hasil dan menyusun Output Akhir Modul 3...")
		
		if session.Feedback == "" {
			logger.Log(session.ID, "   [ERROR] Field 'feedback' kosong! Anda harus memasukkan total hits hasil eksekusi manual.")
			session.Status = "M3_STEP4_WAITING_EXECUTION"
			return m.deps.MongoRepo.UpdateSession(ctx, session)
		}

		userHits := session.Feedback

		llmBrain, err := m.deps.LLMFactory.BrainClient(ctx)
		if err != nil { return err }

		ssBytes, _ := json.MarshalIndent(session.SearchString, "", "  ")
		kwBytes, _ := json.MarshalIndent(session.Keywords, "", "  ")
		dbBytes, _ := json.MarshalIndent(session.DatabaseSelection, "", "  ")

		execAgent := agent.NewExecutionAgent(llmBrain)
		result, err := execAgent.EvaluateAndSummarize(ctx, string(ssBytes), string(kwBytes), string(dbBytes), userHits)
		if err != nil { return err }

		session.SearchLog = &result.SearchLog
		session.Modul3Summary = &result.Summary
		session.Feedback = "" // bersihkan feedback
		session.Status = "M3_STEP4_WAITING_APPROVAL"

		logger.Log(session.ID, "   [System] Dokumen 'search_log' dan 'modul3_summary' berhasil disusun.")
		logger.Log(session.ID, "   [System] DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP4_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi dikunci. Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Cari 'search_log' dan 'modul3_summary'.")
		logger.Log(session.ID, "   2. Verifikasi update policy dan total hits di search_log.")
		logger.Log(session.ID, "   3a. Jika SUDAH tepat, ubah 'status' menjadi 'M3_STEP4_APPROVED'.")
		logger.Log(session.ID, "   3b. Jika BUTUH REVISI, ubah status ke 'M3_STEP4_NEEDS_REVISION' dan ketik feedback.")
		return nil

	case "M3_STEP4_EVALUATION_ERROR":
		logger.Log(session.ID, "   [ERROR] Evaluasi gagal (LLM Parsing/Timeout). Silakan klik tombol 'Retry' di UI atau ubah status ke 'M3_STEP4_EVALUATION' secara manual.")
		return nil

	case "M3_STEP4_NEEDS_REVISION_ERROR":
		logger.Log(session.ID, "   [ERROR] Revisi gagal (LLM Parsing/Timeout). Silakan klik tombol 'Retry' di UI atau ubah status ke 'M3_STEP4_NEEDS_REVISION' secara manual.")
		return nil

	case "M3_STEP4_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 3.4] Memperbaiki Search Log & Summary berdasarkan feedback: '%s'\n", session.Feedback)
		
		userHits := fmt.Sprintf("KOREKSI DARI PENELITI: %s", session.Feedback)

		llmBrain, err := m.deps.LLMFactory.BrainClient(ctx)
		if err != nil { return err }

		ssBytes, _ := json.MarshalIndent(session.SearchString, "", "  ")
		kwBytes, _ := json.MarshalIndent(session.Keywords, "", "  ")
		dbBytes, _ := json.MarshalIndent(session.DatabaseSelection, "", "  ")

		execAgent := agent.NewExecutionAgent(llmBrain)
		result, err := execAgent.EvaluateAndSummarize(ctx, string(ssBytes), string(kwBytes), string(dbBytes), userHits)
		if err != nil { return err }

		session.SearchLog = &result.SearchLog
		session.Modul3Summary = &result.Summary
		session.Feedback = ""
		session.Status = "M3_STEP4_WAITING_APPROVAL"

		logger.Log(session.ID, "   [System] Search Log & Summary direvisi. DIJEDA kembali menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP4_APPROVED":
		logger.Log(session.ID, "   [Langkah 3.4] EKSEKUSI SELESAI! MODUL 3 RAMPUNG.")
		logger.Log(session.ID, "   [System] Mentransfer data ke Modul 4 (Data Mining).")
		session.Status = "M4_INIT" // Transisi ke modul 4
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	default:
		logger.Logf(session.ID, "   [Modul 3] Sub-status %s tidak dikenali atau belum diimplementasikan.\n", session.Status)
	}

	return nil
}
