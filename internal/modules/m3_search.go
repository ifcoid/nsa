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
		fmt.Println("   [Langkah 3.2] Keywords Development (Belum diimplementasikan).")
		return nil

	default:
		fmt.Printf("   [Modul 3] Sub-status %s tidak dikenali atau belum diimplementasikan.\n", session.Status)
	}

	return nil
}
