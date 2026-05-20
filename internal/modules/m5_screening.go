package modules
import (
	"context"
	"encoding/json"
	"fmt"
	"time"
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

		if session.ScreeningSetup == nil {
			return fmt.Errorf("[ERROR] ScreeningSetup belum disiapkan. Pastikan Modul 4 Langkah 3 telah selesai.")
		}

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
		fmt.Println("   [Langkah 5.2] Menjalankan Kalibrasi Dual-Review (20 Sample) dengan API Z-AI GLM & Groq...")

		// Inisialisasi LLM 
		// (Fallback ke gemini jika z-ai atau groq tidak ada di DB, tapi kita coba panggil ID tersebut)
		llmR1, err := m.deps.LLMFactory.CreateClient(ctx, "z-ai")
		if err != nil { 
			fmt.Printf("   [WARNING] LLM z-ai gagal dimuat (%v). Fallback ke gemini.\n", err)
			llmR1, _ = m.deps.LLMFactory.CreateClient(ctx, "gemini")
		}
		llmR2, err := m.deps.LLMFactory.CreateClient(ctx, "groq")
		if err != nil { 
			fmt.Printf("   [WARNING] LLM groq gagal dimuat (%v). Fallback ke gemini.\n", err)
			llmR2, _ = m.deps.LLMFactory.CreateClient(ctx, "gemini")
		}

		scAgent1 := agent.NewScreeningAgent(llmR1)
		scAgent2 := agent.NewScreeningAgent(llmR2)

		briefingDoc := ""
		if session.ScreenerBriefing != nil {
			briefingDoc = session.ScreenerBriefing.BriefingDoc
		}

		papers, err := m.deps.MongoRepo.GetRandomScreeningPapers(ctx, session.ID, 20)
		if err != nil || len(papers) == 0 {
			fmt.Println("   [ERROR] Gagal mengambil 20 sampel atau collection kosong.")
			return err
		}

		fmt.Printf("   [System] Berhasil mengambil %d sampel. Memulai review paralel...\n", len(papers))

		var agreeCount, total int
		var pO, pE, kappa float64
		// Tabel 2x2 (Baris: R1, Kolom: R2)
		var bothInclude, bothExclude, r1IncR2Exc, r1ExcR2Inc int

		for i, p := range papers {
			fmt.Printf("      -> Reviewing sampel %d/%d...\n", i+1, len(papers))
			
			title := ""
			if val, ok := p["Title"].(string); ok { title = val }
			abs := ""
			if val, ok := p["Abstract"].(string); ok { abs = val }
			kwd := ""
			if val, ok := p["Keywords"].(string); ok { kwd = val }

			// R1 Review
			res1, _ := scAgent1.ReviewPaper(ctx, briefingDoc, title, abs, kwd)
			if res1 == nil { res1 = &agent.ScreeningDecision{Decision: "UNCERTAIN"} }
			
			// R2 Review
			res2, _ := scAgent2.ReviewPaper(ctx, briefingDoc, title, abs, kwd)
			if res2 == nil { res2 = &agent.ScreeningDecision{Decision: "UNCERTAIN"} }

			agreement := "DISAGREE"
			if res1.Decision == res2.Decision {
				agreement = "AGREE"
				agreeCount++
			}

			// Update dokumen
			updateDoc := map[string]interface{}{
				"Screener_1_Decision": res1.Decision,
				"Screener_1_Reason_Code": res1.ReasonCode,
				"Screener_1_Notes": res1.Notes,
				"Screener_2_Decision": res2.Decision,
				"Screener_2_Reason_Code": res2.ReasonCode,
				"Screener_2_Notes": res2.Notes,
				"Agreement": agreement,
			}
			m.deps.MongoRepo.UpdateScreeningPaper(ctx, p["_id"], updateDoc)

			// Hitung matriks (Abaikan UNCERTAIN untuk perhitungan dasar kappa 2x2 INCLUDE/EXCLUDE)
			d1 := res1.Decision
			d2 := res2.Decision
			if d1 == "INCLUDE" && d2 == "INCLUDE" { bothInclude++ }
			if d1 == "EXCLUDE" && d2 == "EXCLUDE" { bothExclude++ }
			if d1 == "INCLUDE" && d2 == "EXCLUDE" { r1IncR2Exc++ }
			if d1 == "EXCLUDE" && d2 == "INCLUDE" { r1ExcR2Inc++ }
			total++
		}

		// Kalkulasi Cohen's Kappa
		pO = float64(bothInclude + bothExclude) / float64(total)
		probR1Inc := float64(bothInclude + r1IncR2Exc) / float64(total)
		probR2Inc := float64(bothInclude + r1ExcR2Inc) / float64(total)
		probR1Exc := float64(bothExclude + r1ExcR2Inc) / float64(total)
		probR2Exc := float64(bothExclude + r1IncR2Exc) / float64(total)
		
		pE = (probR1Inc * probR2Inc) + (probR1Exc * probR2Exc)
		if 1-pE > 0 {
			kappa = (pO - pE) / (1 - pE)
		} else {
			kappa = 1.0 // Perfect agreement jika pE = 1 dan pO = 1
		}

		passed := kappa >= 0.60
		iter := len(session.KalibrasiLog) + 1
		logEntry := model.KalibrasiIteration{
			Iterasi: iter,
			Tanggal: time.Now().Format("2006-01-02"),
			Kappa: kappa,
			AgreementPct: float64(agreeCount) / float64(total) * 100,
			Passed: passed,
			Revisi: "Initial Review",
		}
		session.KalibrasiLog = append(session.KalibrasiLog, logEntry)

		fmt.Printf("   [Hasil Kalibrasi] Iterasi %d: Agreement %.1f%% | Kappa: %.3f\n", iter, logEntry.AgreementPct, kappa)

		if passed {
			fmt.Println("   [System] KAPPA >= 0.60 (PASSED). Kalibrasi berhasil!")
			session.Status = "M5_STEP2_APPROVED"
		} else {
			fmt.Println("   [System] KAPPA < 0.60 (FAILED). Membutuhkan analisis Root-Cause.")
			session.Status = "M5_STEP2_WAITING_APPROVAL"
		}

		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M5_STEP2_WAITING_APPROVAL":
		fmt.Println("   [System] Kalibrasi GAGAL (Kappa < 0.60). Silakan buka MongoDB Compass:")
		fmt.Println("   1. Buka collection 'slr_screening', filter data dengan 'Agreement: DISAGREE'.")
		fmt.Println("   2. Lakukan Root-Cause Analysis (lihat notes dari AI).")
		fmt.Println("   3. Jika kriteria salah, perbaiki 'screener_briefing' Anda.")
		fmt.Println("   4. Ubah status kembali ke 'M5_STEP2_CALIBRATION' untuk me-rerun kalibrasi 20 sample baru.")
		return nil

	case "M5_STEP2_APPROVED":
		fmt.Println("   [Langkah 5.2] Kalibrasi disetujui! Lanjut ke Batch Screening Massal...")
		session.Status = "M5_STEP3_BATCH_SCREENING"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	default:
		fmt.Printf("   [Modul 5] Sub-status %s tidak dikenali atau belum diimplementasikan.\n", session.Status)
	}
	return nil
}
