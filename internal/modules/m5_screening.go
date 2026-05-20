package modules
import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/agent"
	"nsa/internal/model"
)
type M5Screening struct { deps *ModuleDeps }
func NewM5Screening(deps *ModuleDeps) *M5Screening { return &M5Screening{deps: deps} }
func (m *M5Screening) Name() string { return "M5_SCREENING" }
func (m *M5Screening) Execute(ctx context.Context, session *model.SLRSession) error {
	switch session.Status {
	// =========================================================================
	// LANGKAH 1: SCREENER BRIEFING
	// =========================================================================
	case "M5_INIT", "M5_STEP1_BRIEFING":
		fmt.Println("   [Langkah 5.1] Mengevaluasi kriteria & menyusun Screener Briefing...")

		picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")
		reasonCodesBytes, _ := json.MarshalIndent(session.ScreeningSetup.ReasonCodes, "", "  ")

		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		scAgent := agent.NewScreeningAgent(llmBrain)
		briefing, err := scAgent.GenerateBriefing(ctx, string(picoBytes), string(reasonCodesBytes))
		if err != nil { return err }

		session.ScreenerBriefing = briefing
		session.Status = "M5_STEP1_WAITING_APPROVAL"
		
		fmt.Println("   [System] Screener Briefing berhasil di-generate. DIJEDA.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M5_STEP1_WAITING_APPROVAL":
		fmt.Println("   [System] Sesi dikunci. Silakan buka MongoDB Compass:")
		fmt.Println("   1. Periksa dokumen 'screener_briefing'.")
		fmt.Println("   2. Baca 'validation_gap' dan 'briefing_doc'.")
		fmt.Println("   3a. Jika 'decision' merekomendasikan 'REVISE_M2', ubah status ke 'M5_STEP1_NEEDS_REVISION'.")
		fmt.Println("   3b. Jika Anda setuju untuk lanjut, ubah status ke 'M5_STEP1_APPROVED'.")
		return nil

	case "M5_STEP1_NEEDS_REVISION":
		fmt.Println("   [System] Kriteria tidak memadai. Mengembalikan riset ke Modul 2 Langkah 3 (PICO Definitions).")
		session.Status = "M2_STEP3_NEEDS_REVISION" 
		session.Feedback = fmt.Sprintf("Revisi Kriteria PICO dari Modul 5: %s", session.ScreenerBriefing.Recommendation)
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M5_STEP1_APPROVED":
		fmt.Println("   [Langkah 5.1] Screener Briefing disetujui! Lanjut ke Kalibrasi Dual-Review...")
		session.Status = "M5_STEP2_CALIBRATION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M5_STEP2_CALIBRATION":
		fmt.Println("   [Langkah 5.2] Kalibrasi Dual-Review (Belum diimplementasikan).")
		return nil

	default:
		fmt.Printf("   [Modul 5] Sub-status %s tidak dikenali atau belum diimplementasikan.\n", session.Status)
	}
	return nil
}
