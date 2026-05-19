package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/agent"
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
	switch session.Status {
	// =========================================================================
	// LANGKAH 1: DATABASE SELECTION + JUSTIFICATION
	// =========================================================================
	case "M3_INIT", "M3_STEP1_DATABASE_SELECTION":
		fmt.Println("   [Langkah 3.1] Menganalisis lanskap Database Selection...")
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")
		scopeBytes, _ := json.MarshalIndent(session.ScopeJustifications, "", "  ")

		dbAgent := agent.NewDBSelectionAgent(llmBrain)
		dbResult, err := dbAgent.Analyze(ctx, string(picoBytes), string(scopeBytes))
		if err != nil { return err }

		session.DatabaseSelection = dbResult
		session.Status = "M3_STEP1_WAITING_APPROVAL"
		
		fmt.Println("   [System] Rekomendasi Database berhasil dibuat. DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP1_WAITING_APPROVAL":
		fmt.Println("   [System] Sesi dikunci. Silakan buka MongoDB Compass:")
		fmt.Println("   1. Buka document sesi Anda, cari 'database_selection'.")
		fmt.Println("   2. Verifikasi matriks database dan justifikasi finalnya.")
		fmt.Println("   3a. Jika SUDAH sesuai, ubah 'status' menjadi 'M3_STEP1_APPROVED'.")
		fmt.Println("   3b. Jika BUTUH REVISI, ubah 'status' ke 'M3_STEP1_NEEDS_REVISION' dan isi 'feedback'.")
		return nil

	case "M3_STEP1_NEEDS_REVISION":
		fmt.Printf("   [Revisi 3.1] Memperbaiki Database Selection berdasarkan feedback: '%s'\n", session.Feedback)

		picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")
		scopeBytes, _ := json.MarshalIndent(session.ScopeJustifications, "", "  ")
		
		scopeContext := string(scopeBytes) + fmt.Sprintf("\n\n[INSTRUKSI REVISI DARI PENELITI]:\n%s", session.Feedback)

		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		dbAgent := agent.NewDBSelectionAgent(llmBrain)
		dbResult, err := dbAgent.Analyze(ctx, string(picoBytes), scopeContext)
		if err != nil { return err }

		session.DatabaseSelection = dbResult
		session.Feedback = ""
		session.Status = "M3_STEP1_WAITING_APPROVAL"
		
		fmt.Println("   [System] Database Selection direvisi. DIJEDA kembali menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP1_APPROVED":
		fmt.Println("   [Langkah 3.1] Database Selection disetujui! Lanjut ke Keywords Development...")
		session.Status = "M3_STEP2_KEYWORDS"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 2: KEYWORDS DEVELOPMENT
	// =========================================================================
	case "M3_STEP2_KEYWORDS":
		fmt.Println("   [Langkah 3.2] Mengembangkan Keywords (PICO + Avoid List)...")
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")

		kwAgent := agent.NewKeywordsAgent(llmBrain)
		kwResult, err := kwAgent.DevelopKeywords(ctx, string(picoBytes))
		if err != nil { return err }

		session.Keywords = kwResult
		session.Status = "M3_STEP2_WAITING_APPROVAL"
		
		fmt.Println("   [System] Dokumen Keywords berhasil dibuat. DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP2_WAITING_APPROVAL":
		fmt.Println("   [System] Sesi dikunci. Silakan buka MongoDB Compass:")
		fmt.Println("   1. Buka document sesi Anda, cari array 'keywords'.")
		fmt.Println("   2. Verifikasi kesesuaian main_synonyms dan AVOID list untuk setiap P, I, C, dan O.")
		fmt.Println("   3a. Jika SUDAH sesuai, ubah 'status' menjadi 'M3_STEP2_APPROVED'.")
		fmt.Println("   3b. Jika BUTUH REVISI, ubah 'status' ke 'M3_STEP2_NEEDS_REVISION' dan isi 'feedback'.")
		return nil

	case "M3_STEP2_NEEDS_REVISION":
		fmt.Printf("   [Revisi 3.2] Memperbaiki Keywords berdasarkan feedback: '%s'\n", session.Feedback)

		picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")
		
		picoContext := string(picoBytes) + fmt.Sprintf("\n\n[INSTRUKSI REVISI DARI PENELITI]:\n%s", session.Feedback)

		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		kwAgent := agent.NewKeywordsAgent(llmBrain)
		kwResult, err := kwAgent.DevelopKeywords(ctx, picoContext)
		if err != nil { return err }

		session.Keywords = kwResult
		session.Feedback = ""
		session.Status = "M3_STEP2_WAITING_APPROVAL"
		
		fmt.Println("   [System] Keywords direvisi. DIJEDA kembali menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP2_APPROVED":
		fmt.Println("   [Langkah 3.2] Keywords disetujui! Lanjut ke Search String + Filter Specifications...")
		session.Status = "M3_STEP3_SEARCH_STRING"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 3: SEARCH STRING + FILTER SPECIFICATIONS
	// =========================================================================
	case "M3_STEP3_SEARCH_STRING":
		fmt.Println("   [Langkah 3.3] Merangkai Search String & Filter Specifications...")
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		kwBytes, _ := json.MarshalIndent(session.Keywords, "", "  ")
		scopeBytes, _ := json.MarshalIndent(session.ScopeJustifications, "", "  ")

		ssAgent := agent.NewSearchStringAgent(llmBrain)
		ssResult, err := ssAgent.BuildSearchString(ctx, string(kwBytes), string(scopeBytes))
		if err != nil { return err }

		session.SearchString = ssResult
		session.Status = "M3_STEP3_WAITING_APPROVAL"
		
		fmt.Println("   [System] Search String berhasil dirangkai. DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP3_WAITING_APPROVAL":
		fmt.Println("   [System] Sesi dikunci. Silakan buka MongoDB Compass:")
		fmt.Println("   1. Buka document sesi Anda, cari objek 'search_string'.")
		fmt.Println("   2. Verifikasi sintaks query Scopus (wildcard, quotation, kurung, OR/AND).")
		fmt.Println("   3. Verifikasi array 'filters' memiliki justifikasi.")
		fmt.Println("   4a. Jika SUDAH tepat, ubah 'status' menjadi 'M3_STEP3_APPROVED'.")
		fmt.Println("   4b. Jika BUTUH REVISI, ubah 'status' ke 'M3_STEP3_NEEDS_REVISION' dan isi 'feedback'.")
		return nil

	case "M3_STEP3_NEEDS_REVISION":
		fmt.Printf("   [Revisi 3.3] Memperbaiki Search String berdasarkan feedback: '%s'\n", session.Feedback)

		kwBytes, _ := json.MarshalIndent(session.Keywords, "", "  ")
		scopeBytes, _ := json.MarshalIndent(session.ScopeJustifications, "", "  ")
		
		scopeContext := string(scopeBytes) + fmt.Sprintf("\n\n[INSTRUKSI REVISI DARI PENELITI]:\n%s", session.Feedback)

		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		ssAgent := agent.NewSearchStringAgent(llmBrain)
		ssResult, err := ssAgent.BuildSearchString(ctx, string(kwBytes), scopeContext)
		if err != nil { return err }

		session.SearchString = ssResult
		session.Feedback = ""
		session.Status = "M3_STEP3_WAITING_APPROVAL"
		
		fmt.Println("   [System] Search String direvisi. DIJEDA kembali menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M3_STEP3_APPROVED":
		fmt.Println("   [Langkah 3.3] Search String disetujui! Lanjut ke Pre-Validasi & Eksekusi...")
		session.Status = "M3_STEP4_PRE_VALIDATION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 4: PRE-VALIDASI + EKSEKUSI
	// =========================================================================
	case "M3_STEP4_PRE_VALIDATION":
		fmt.Println("   [Langkah 3.4] Pre-Validasi & Eksekusi (Belum diimplementasikan).")
		return nil

	default:
		fmt.Printf("   [Modul 3] Sub-status %s tidak dikenali atau belum diimplementasikan.\n", session.Status)
	}

	return nil
}
