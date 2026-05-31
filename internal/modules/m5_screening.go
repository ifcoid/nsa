package modules
import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"
	"nsa/internal/agent"
	"nsa/internal/logger"
	"nsa/internal/model"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
		logger.Log(session.ID, "   [Langkah 5.1] Mengevaluasi kriteria & menyusun Screener Briefing...")

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
		
		logger.Log(session.ID, "   [System] Screener Briefing berhasil di-generate. DIJEDA.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M5_STEP1_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi dikunci. Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Periksa dokumen 'screener_briefing'.")
		logger.Log(session.ID, "   2. Baca 'validation_gap' dan 'briefing_doc'.")
		logger.Log(session.ID, "   3a. Jika 'decision' merekomendasikan 'REVISE_M2', ubah status ke 'M5_STEP1_NEEDS_REVISION'.")
		logger.Log(session.ID, "   3b. Jika Anda setuju untuk lanjut, ubah status ke 'M5_STEP1_APPROVED'.")
		return nil

	case "M5_STEP1_NEEDS_REVISION":
		logger.Log(session.ID, "   [System] Kriteria tidak memadai. Mengembalikan riset ke Modul 2 Langkah 3 (PICO Definitions).")
		session.Status = "M2_STEP3_NEEDS_REVISION" 
		session.Feedback = fmt.Sprintf("Revisi Kriteria PICO dari Modul 5: %s", session.ScreenerBriefing.Recommendation)
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M5_STEP1_APPROVED":
		logger.Log(session.ID, "   [Langkah 5.1] Screener Briefing disetujui! Lanjut ke Kalibrasi Dual-Review...")
		session.Status = "M5_STEP2_CALIBRATION"
		// Reset hasil screening sebelumnya saat pertama kali masuk kalibrasi
		_ = m.deps.MongoRepo.ResetCalibrationScreenings(ctx, session.ID)
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M5_STEP2_CALIBRATION":
		logger.Log(session.ID, "   [Langkah 5.2] Menjalankan Kalibrasi Dual-Review (20 Sample) dengan API Z-AI GLM & Groq...")
		
		// Inisialisasi LLM 
		// (WAJIB menggunakan z-ai dan groq untuk dual-review, hentikan proses jika gagal)
		llmR1, err := m.deps.LLMFactory.CreateClient(ctx, "zhipu")
		if err != nil { 
			logger.Logf(session.ID, "   [INFO] Zhipu gagal dimuat (%v). Fallback awal ke Xiaomi MiMo...\n", err)
			llmR1, err = m.deps.LLMFactory.CreateClient(ctx, "xiaomi")
			if err != nil {
				return fmt.Errorf("Reviewer 1 (Zhipu maupun Xiaomi) gagal dimuat. Harap konfigurasi API di Pengaturan")
			}
		}
		
		llmR2, err := m.deps.LLMFactory.CreateClient(ctx, "groq")
		if err != nil { 
			logger.Logf(session.ID, "   [ERROR] LLM groq gagal dimuat (%v). Harap konfigurasi API groq terlebih dahulu di halaman Pengaturan!\n", err)
			return fmt.Errorf("groq LLM configuration missing or invalid. Please configure the groq API key first")
		}

		scAgent1 := agent.NewScreeningAgent(llmR1)
		scAgent2 := agent.NewScreeningAgent(llmR2)

		briefingDoc := ""
		if session.ScreenerBriefing != nil {
			briefingDoc = session.ScreenerBriefing.BriefingDoc
		}

		papers, err := m.deps.MongoRepo.GetRandomScreeningPapers(ctx, session.ID, 20)
		if err != nil || len(papers) == 0 {
			logger.Log(session.ID, "   [ERROR] Gagal mengambil 20 sampel atau collection kosong.")
			return err
		}

		logger.Logf(session.ID, "   [System] Berhasil mengambil %d sampel. Memulai review paralel...\n", len(papers))

		var agreeCount, total int
		var pO, pE, kappa float64
		// Tabel 2x2 (Baris: R1, Kolom: R2)
		var bothInclude, bothExclude, r1IncR2Exc, r1ExcR2Inc int

		for i, p := range papers {
			logger.Logf(session.ID, "      -> Reviewing sampel %d/%d...\n", i+1, len(papers))
			
			title := ""
			if val, ok := p["Title"].(string); ok { title = val }
			abs := ""
			if val, ok := p["Abstract"].(string); ok { abs = val }
			kwd := ""
			if val, ok := p["Keywords"].(string); ok { kwd = val }

			var res1 *agent.ScreeningDecision
			var raw1 string
			var err1 error
			
			var res2 *agent.ScreeningDecision
			var raw2 string
			var err2 error

			// Cek apakah paper ini SUDAH dievaluasi sebelumnya (Resume logic)
			d1Str, hasD1 := p["Screener_1_Decision"].(string)
			d2Str, hasD2 := p["Screener_2_Decision"].(string)
			
			if hasD1 && hasD2 && d1Str != "" && d2Str != "" {
				// Paper sudah dievaluasi, langsung baca keputusannya!
				logger.Logf(session.ID, "      -> [Resume] Paper ini sudah dievaluasi sebelumnya (R1: %s, R2: %s). Melompati pemanggilan LLM...", d1Str, d2Str)
				res1 = &agent.ScreeningDecision{Decision: d1Str, ReasonCode: p["Screener_1_Reason_Code"].(string), Notes: p["Screener_1_Notes"].(string)}
				res2 = &agent.ScreeningDecision{Decision: d2Str, ReasonCode: p["Screener_2_Reason_Code"].(string), Notes: p["Screener_2_Notes"].(string)}
			} else {
				// Belum dievaluasi, panggil API!
				
				// R1 Review (dengan mekanisme Retry 4x & Backoff cepat karena Zhipu)
				backoffDelays := []int{10, 30, 60, 120} // detik
				for retry := 0; retry < 4; retry++ {
					res1, raw1, err1 = scAgent1.ReviewPaper(ctx, briefingDoc, title, abs, kwd)
					if err1 == nil && res1 != nil { break }
					
					baseDelaySec := float64(backoffDelays[retry])
					jitter := (rand.Float64()*0.4 - 0.2) * baseDelaySec 
					finalDelaySec := baseDelaySec + jitter
					backoff := time.Duration(finalDelaySec * float64(time.Second))
					
					logger.Logf(session.ID, "      [R1 Retry %d] Error LLM: %v. Menunggu %v sebelum mencoba lagi...", retry+1, err1, backoff)
					time.Sleep(backoff)
				}
				
				if res1 == nil || err1 != nil { 
					logger.Log(session.ID, "      [!] R1 Utama (Zhipu) gagal merespons. Melakukan Fallback on-the-fly ke Xiaomi MiMo...")
					llmR1Fallback, errF := m.deps.LLMFactory.CreateClient(ctx, "xiaomi")
					if errF != nil {
						logger.Logf(session.ID, "      [!] Gagal memuat Xiaomi MiMo untuk fallback R1: %v", errF)
					} else {
						scAgent1Fallback := agent.NewScreeningAgent(llmR1Fallback)
						fallbackBackoff := []int{1, 3, 5} // menit
						for retryFb := 0; retryFb < 3; retryFb++ {
							res1, raw1, err1 = scAgent1Fallback.ReviewPaper(ctx, briefingDoc, title, abs, kwd)
							if err1 == nil && res1 != nil { break }
							
							baseDelaySec := float64(fallbackBackoff[retryFb])
							jitter := (rand.Float64()*0.4 - 0.2) * baseDelaySec 
							finalDelaySec := baseDelaySec + jitter
							backoff := time.Duration(finalDelaySec * float64(time.Minute))
							
							logger.Logf(session.ID, "      [R1 Fallback Retry %d] Error LLM: %v. Menunggu %v...", retryFb+1, err1, backoff)
							time.Sleep(backoff)
						}
						if err1 != nil {
							logger.Logf(session.ID, "      [!] R1 Fallback (Xiaomi) juga gagal: %v", err1)
						}
					}
				}

				if res1 == nil || err1 != nil { 
					return fmt.Errorf("Kalibrasi dibatalkan: R1 gagal merespons setelah fallback: %v", err1)
				}
				logger.Logf(session.ID, "      [RAW R1 Zhipu] %s", raw1)
				
				// Jeda Antar Agen (Sequential Micro-Throttling) agar tidak burst request
				time.Sleep(3 * time.Second)
	
				// R2 Review (dengan mekanisme Retry 6x & Backoff wajar)
				backoffDelaysR2 := []int{1, 3, 5, 10, 15, 30}
				for retry := 0; retry < 6; retry++ {
					res2, raw2, err2 = scAgent2.ReviewPaper(ctx, briefingDoc, title, abs, kwd)
					if err2 == nil && res2 != nil { break }
					
					baseDelaySec := float64(backoffDelaysR2[retry])
					jitter := (rand.Float64()*0.4 - 0.2) * baseDelaySec
					finalDelaySec := baseDelaySec + jitter
					backoff := time.Duration(finalDelaySec * float64(time.Minute))
	
					logger.Logf(session.ID, "      [R2 Retry %d] Error LLM: %v. Menunggu %v sebelum mencoba lagi...", retry+1, err2, backoff)
					time.Sleep(backoff)
				}
				if res2 == nil || err2 != nil { 
					return fmt.Errorf("Kalibrasi dibatalkan: R2 (Groq) gagal merespons setelah 6x percobaan: %v", err2)
				}
				logger.Logf(session.ID, "      [RAW R2 Groq] %s", raw2)
			}

			agreement := "DISAGREE"
			if res1.Decision == res2.Decision {
				agreement = "AGREE"
				agreeCount++
			}

			// Update dokumen jika ini adalah evaluasi baru
			if !(hasD1 && hasD2 && d1Str != "" && d2Str != "") {
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
			}

			// Hitung matriks (Abaikan UNCERTAIN untuk perhitungan dasar kappa 2x2 INCLUDE/EXCLUDE)
			d1 := res1.Decision
			d2 := res2.Decision
			if d1 == "INCLUDE" && d2 == "INCLUDE" { bothInclude++ }
			if d1 == "EXCLUDE" && d2 == "EXCLUDE" { bothExclude++ }
			if d1 == "INCLUDE" && d2 == "EXCLUDE" { r1IncR2Exc++ }
			if d1 == "EXCLUDE" && d2 == "INCLUDE" { r1ExcR2Inc++ }
			total++

			// Jeda (Pacing/Throttling) agar tidak terjadi Rate Limit
			// Zhipu free tier max 3-10 RPM. 8 detik memastikan ~6 RPM.
			time.Sleep(8 * time.Second)
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

		// Mengatasi fenomena "Kappa Paradox" (di mana Kappa anjlok menjadi 0 jika prevalensi sangat skewed meskipun Agreement sangat tinggi)
		passed := kappa >= 0.60 || (float64(agreeCount)/float64(total) >= 0.90)
		iter := len(session.KalibrasiLog) + 1
		logEntry := model.KalibrasiIteration{
			Iterasi:       iter,
			Tanggal:       time.Now().Format("2006-01-02"),
			TotalSample:   total,
			AgreeCount:    agreeCount,
			DisagreeCount: total - agreeCount,
			BothInclude:   bothInclude,
			BothExclude:   bothExclude,
			R1IncR2Exc:    r1IncR2Exc,
			R1ExcR2Inc:    r1ExcR2Inc,
			PO:            pO,
			PE:            pE,
			Kappa:         kappa,
			AgreementPct:  float64(agreeCount) / float64(total) * 100,
			Passed:        passed,
			Revisi:        "Initial Review",
		}
		session.KalibrasiLog = append(session.KalibrasiLog, logEntry)

		logger.Logf(session.ID, "   [Hasil Kalibrasi] Iterasi %d: Agreement %.1f%% | Kappa: %.3f\n", iter, logEntry.AgreementPct, kappa)

		if passed {
			logger.Log(session.ID, "   [System] KAPPA >= 0.60 (PASSED). Kalibrasi berhasil!")
			session.Status = "M5_STEP2_APPROVED"
		} else {
			logger.Log(session.ID, "   [System] KAPPA < 0.60 (FAILED). Membutuhkan analisis Root-Cause.")
			session.Status = "M5_STEP2_WAITING_APPROVAL"
		}

		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M5_STEP2_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Kalibrasi GAGAL (Kappa < 0.60). Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Buka collection 'slr_screening', filter data dengan 'Agreement: DISAGREE'.")
		logger.Log(session.ID, "   2. Lakukan Root-Cause Analysis (lihat notes dari AI).")
		logger.Log(session.ID, "   3. Jika kriteria salah, perbaiki 'screener_briefing' Anda.")
		logger.Log(session.ID, "   4. Ubah status kembali ke 'M5_STEP2_CALIBRATION' untuk me-rerun kalibrasi 20 sample baru.")
		return nil

	case "M5_STEP2_CALIBRATION_ERROR":
		logger.Log(session.ID, "   [System] Agen sedang ditangguhkan akibat error LLM.")
		logger.Log(session.ID, "   [System] Menunggu instruksi 'Retry' (Coba Lagi) dari Anda melalui layar UI...")
		return nil

	case "M5_STEP2_NEEDS_REVISION":
		logger.Log(session.ID, "   [System] Menerima feedback revisi untuk Screener Briefing. Memproses pembaruan...")
		
		// Hapus flag in_calibration_batch agar iterasi kalibrasi berikutnya mengambil 20 sampel baru!
		updateFilter := bson.M{"session_id": session.ID, "in_calibration_batch": true}
		updateDoc := bson.M{"$unset": bson.M{"in_calibration_batch": ""}}
		m.deps.MongoRepo.GetScreeningCollection().UpdateMany(ctx, updateFilter, updateDoc)

		// Reset hasil screening sebelumnya karena briefing diubah (menghindari hasil evaluasi usang/stale)
		_ = m.deps.MongoRepo.ResetCalibrationScreenings(ctx, session.ID)

		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		scAgent := agent.NewScreeningAgent(llmBrain)
		currentBriefing := ""
		if session.ScreenerBriefing != nil {
			currentBriefing = session.ScreenerBriefing.BriefingDoc
		}

		revisedBriefing, err := scAgent.ReviseBriefing(ctx, currentBriefing, session.Feedback)
		if err != nil {
			logger.Logf(session.ID, "   [ERROR] Gagal merevisi briefing: %v\n", err)
			return err
		}

		session.ScreenerBriefing = revisedBriefing
		session.Status = "M5_STEP2_CALIBRATION" // Kembalikan ke kalibrasi lagi
		logger.Log(session.ID, "   [System] Screener Briefing berhasil direvisi. Melanjutkan ulang kalibrasi...")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M5_STEP2_APPROVED":
		logger.Log(session.ID, "   [Langkah 5.2] Kalibrasi disetujui! Lanjut ke Batch Screening Massal...")
		session.Status = "M5_STEP3_BATCH_SCREENING"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M5_STEP3_BATCH_SCREENING":
		logger.Log(session.ID, "   [Langkah 5.3] Memulai Batch Screening Massal (Max 20 per batch)...")

		// 1. Pengecekan Unevaluated Papers (State Machine)
		unevaluatedPapers, err := m.deps.MongoRepo.GetUnevaluatedPapers(ctx, session.ID)
		if err != nil {
			return fmt.Errorf("gagal mengambil unevaluated papers: %w", err)
		}

		totalPapers, screenedPapers, _ := m.deps.MongoRepo.GetScreeningProgress(ctx, session.ID)
		isFinished := totalPapers > 0 && screenedPapers == totalPapers
		
		// Jika kuota evaluasi penuh (>= 20) ATAU semua paper di DB sudah habis discreening
		if len(unevaluatedPapers) >= 20 || (isFinished && len(unevaluatedPapers) > 0) {
			logger.Logf(session.ID, "   [System] Mengevaluasi %d paper yang telah selesai di-screening...\n", len(unevaluatedPapers))
			
			var agreeCount, bothInclude, bothExclude, r1IncR2Exc, r1ExcR2Inc int
			var paperIDs []primitive.ObjectID

			for _, p := range unevaluatedPapers {
				if oid, ok := p["_id"].(primitive.ObjectID); ok {
					paperIDs = append(paperIDs, oid)
				}
				d1 := ""
				if val, ok := p["Screener_1_Decision"].(string); ok { d1 = val }
				d2 := ""
				if val, ok := p["Screener_2_Decision"].(string); ok { d2 = val }
				agreement := ""
				if val, ok := p["Agreement"].(string); ok { agreement = val }

				if agreement == "AGREE" { agreeCount++ }
				if d1 == "INCLUDE" && d2 == "INCLUDE" { bothInclude++ }
				if d1 == "EXCLUDE" && d2 == "EXCLUDE" { bothExclude++ }
				if d1 == "INCLUDE" && d2 == "EXCLUDE" { r1IncR2Exc++ }
				if d1 == "EXCLUDE" && d2 == "INCLUDE" { r1ExcR2Inc++ }
			}

			totalEval := len(unevaluatedPapers)
			kappa := 0.0
			if totalEval > 0 {
				pO := float64(bothInclude + bothExclude) / float64(totalEval)
				probR1Inc := float64(bothInclude + r1IncR2Exc) / float64(totalEval)
				probR2Inc := float64(bothInclude + r1ExcR2Inc) / float64(totalEval)
				probR1Exc := float64(bothExclude + r1ExcR2Inc) / float64(totalEval)
				probR2Exc := float64(bothExclude + r1IncR2Exc) / float64(totalEval)
				pE := (probR1Inc * probR2Inc) + (probR1Exc * probR2Exc)
				if 1-pE > 0 { kappa = (pO - pE) / (1 - pE) } else { kappa = 1.0 }
			}

			batchNum := len(session.ScreeningResultsLog) + 1
			drift := kappa < 0.60 && totalEval >= 10

			logEntry := model.ScreeningResultsLog{
				BatchNumber: batchNum,
				ProcessedRecords: totalEval,
				CurrentKappa: kappa,
				DisagreementCases: totalEval - agreeCount,
				DriftDetected: drift,
				Tanggal: time.Now().Format("2006-01-02"),
			}
			session.ScreeningResultsLog = append(session.ScreeningResultsLog, logEntry)

			m.deps.MongoRepo.MarkPapersAsEvaluated(ctx, session.ID, paperIDs)

			logger.Logf(session.ID, "   [Batch Result] Batch %d | Processed: %d | Kappa: %.3f | Disagreements: %d\n", batchNum, totalEval, kappa, logEntry.DisagreementCases)

			if drift {
				logger.Log(session.ID, "   [WARNING] Drift interpretasi terdeteksi (Kappa < 0.60)! Wajib resolusi.")
			}
			session.Status = "M5_STEP3_WAITING_RESOLUTION"
			return m.deps.MongoRepo.UpdateSession(ctx, session)
		}

		if isFinished && len(unevaluatedPapers) == 0 {
			logger.Log(session.ID, "   [System] Semua paper telah di-screening dan dievaluasi! Lanjut ke Langkah 4.")
			session.Status = "M5_STEP4_REVIEW_HASIL"
			return m.deps.MongoRepo.UpdateSession(ctx, session)
		}

		// 2. Persiapan LLM untuk proses screening sisa kuota
		llmR1, err := m.deps.LLMFactory.CreateClient(ctx, "zhipu")
		if err != nil { 
			logger.Logf(session.ID, "   [INFO] Zhipu gagal dimuat (%v). Fallback awal ke Xiaomi MiMo...\n", err)
			llmR1, err = m.deps.LLMFactory.CreateClient(ctx, "xiaomi")
			if err != nil {
				return fmt.Errorf("Reviewer 1 (Zhipu maupun Xiaomi) gagal dimuat. Harap konfigurasi API di Pengaturan")
			}
		}
		
		llmR2, err := m.deps.LLMFactory.CreateClient(ctx, "groq")
		if err != nil { 
			logger.Logf(session.ID, "   [ERROR] LLM groq gagal dimuat (%v). Harap konfigurasi API groq terlebih dahulu di halaman Pengaturan!\n", err)
			return fmt.Errorf("groq LLM configuration missing or invalid. Please configure the groq API key first")
		}

		var scAgentSupervisor *agent.ScreeningAgent
		var primarySupervisorName string

		llmSupervisor, err := m.deps.LLMFactory.CreateClient(ctx, "xiaomi")
		if err == nil { 
			scAgentSupervisor = agent.NewScreeningAgent(llmSupervisor)
			primarySupervisorName = "Xiaomi MiMo"
		} else {
			logger.Logf(session.ID, "   [INFO] Xiaomi MiMo gagal dimuat (%v). Fallback awal ke OpenRouter...\n", err)
			llmSupervisor, err = m.deps.LLMFactory.CreateClient(ctx, "openrouter")
			if err != nil {
				return fmt.Errorf("AI Supervisor (Xiaomi maupun OpenRouter) gagal dimuat. Harap konfigurasi API di Pengaturan")
			}
			scAgentSupervisor = agent.NewScreeningAgent(llmSupervisor)
			primarySupervisorName = "OpenRouter"
		}

		scAgent1 := agent.NewScreeningAgent(llmR1)
		scAgent2 := agent.NewScreeningAgent(llmR2)

		briefingDoc := ""
		if session.ScreenerBriefing != nil {
			briefingDoc = session.ScreenerBriefing.BriefingDoc
		}

		fetchLimit := 20 - len(unevaluatedPapers)
		papers, err := m.deps.MongoRepo.GetUnscreenedPapers(ctx, session.ID, fetchLimit)
		if err != nil {
			return fmt.Errorf("gagal mengambil unscreened papers: %w", err)
		}

		progressPercent := 0.0
		if totalPapers > 0 {
			progressPercent = float64(screenedPapers) / float64(totalPapers) * 100
		}
		logger.Logf(session.ID, "   📊 [Progress] %d dari %d papers telah discreening (%.1f%%). Ada %d tersimpan di batch ini.\n", screenedPapers, totalPapers, progressPercent, len(unevaluatedPapers))
		logger.Logf(session.ID, "   [System] Memproses %d papers tambahan (untuk melengkapi batch)...\n", len(papers))

		var total int

		for i, p := range papers {
			logger.Logf(session.ID, "      -> Screening [%d/%d] ID: %v\n", i+1, len(papers), p["_id"])
			
			title := ""
			if val, ok := p["Title"].(string); ok { title = val }
			abs := ""
			if val, ok := p["Abstract"].(string); ok { abs = val }
			kwd := ""
			if val, ok := p["Keywords"].(string); ok { kwd = val }

			paperID := ""
			if oid, ok := p["_id"].(primitive.ObjectID); ok {
				paperID = oid.Hex()
			}

			// R1 Review
			backoffDelays := []int{10, 30, 60, 120} // detik, karena Zhipu jarang 429 parah
			var res1 *model.ScreeningPerspective
			var err1 error
			for retry := 0; retry < 4; retry++ {
				res1, err1 = scAgent1.BatchReviewPaper(ctx, briefingDoc, title, abs, kwd)
				if err1 == nil && res1 != nil { break }
				
				baseDelaySec := float64(backoffDelays[retry])
				jitter := (rand.Float64()*0.4 - 0.2) * baseDelaySec 
				finalDelaySec := baseDelaySec + jitter
				backoff := time.Duration(finalDelaySec * float64(time.Second))
				logger.Logf(session.ID, "      [R1 Batch Retry %d] Error LLM: %v. Menunggu %v...", retry+1, err1, backoff)
				time.Sleep(backoff)
			}

			if res1 == nil || err1 != nil { 
				logger.Log(session.ID, "      [!] R1 Utama (Zhipu) gagal memberikan evaluasi. Melakukan Fallback on-the-fly ke Xiaomi MiMo...")
				llmR1Fallback, errF := m.deps.LLMFactory.CreateClient(ctx, "xiaomi")
				if errF != nil {
					logger.Logf(session.ID, "      [!] Gagal memuat Xiaomi MiMo untuk fallback R1: %v", errF)
				} else {
					scAgent1Fallback := agent.NewScreeningAgent(llmR1Fallback)
					fallbackBackoff := []int{1, 3, 5} // menit
					for retryFb := 0; retryFb < 3; retryFb++ {
						res1, err1 = scAgent1Fallback.BatchReviewPaper(ctx, briefingDoc, title, abs, kwd)
						if err1 == nil && res1 != nil { break }
						
						baseDelaySec := float64(fallbackBackoff[retryFb])
						jitter := (rand.Float64()*0.4 - 0.2) * baseDelaySec 
						finalDelaySec := baseDelaySec + jitter
						backoff := time.Duration(finalDelaySec * float64(time.Minute))
						logger.Logf(session.ID, "      [R1 Fallback Retry %d] Error LLM: %v. Menunggu %v...", retryFb+1, err1, backoff)
						time.Sleep(backoff)
					}
					if err1 != nil {
						logger.Logf(session.ID, "      [!] R1 Fallback (Xiaomi) juga gagal: %v", err1)
					}
				}
			}

			if res1 == nil || err1 != nil { 
				return fmt.Errorf("API R1 gagal merespons setelah fallback (%v). Batch terhenti, %d paper berhasil disimpan.", err1, total)
			}
			res1.PaperID = paperID
			res1.Title = title

			time.Sleep(3 * time.Second)

			// R2 Review
			backoffDelaysR2 := []int{1, 3, 5, 10, 15, 30}
			var res2 *model.ScreeningPerspective
			var err2 error
			for retry := 0; retry < 6; retry++ {
				res2, err2 = scAgent2.BatchReviewPaper(ctx, briefingDoc, title, abs, kwd)
				if err2 == nil && res2 != nil { break }
				
				baseDelaySec := float64(backoffDelaysR2[retry])
				jitter := (rand.Float64()*0.4 - 0.2) * baseDelaySec 
				finalDelaySec := baseDelaySec + jitter
				backoff := time.Duration(finalDelaySec * float64(time.Minute))
				logger.Logf(session.ID, "      [R2 Batch Retry %d] Error LLM: %v. Menunggu %v...", retry+1, err2, backoff)
				time.Sleep(backoff)
			}
			if res2 == nil || err2 != nil { 
				return fmt.Errorf("API R2 gagal merespons setelah 6x percobaan (%v). Batch terhenti, %d paper berhasil disimpan.", err2, total)
			}
			res2.PaperID = paperID
			res2.Title = title

			session.Reviewer1Perspectives = append(session.Reviewer1Perspectives, *res1)
			session.Reviewer2Perspectives = append(session.Reviewer2Perspectives, *res2)

			agreement := "DISAGREE"
			if res1.Recommend == res2.Recommend {
				agreement = "AGREE"
			}

			notes1 := fmt.Sprintf("Strict: %s | Liberal: %s | Evidence: %s", res1.Strict, res1.Liberal, res1.Evidence)
			notes2 := fmt.Sprintf("Strict: %s | Liberal: %s | Evidence: %s", res2.Strict, res2.Liberal, res2.Evidence)

			conflictRes := ""
			if agreement == "DISAGREE" || res1.Recommend == "UNCERTAIN" || res2.Recommend == "UNCERTAIN" {
				logger.Logf(session.ID, "      [*] Disagreement terdeteksi! Mengambil saran resolusi dari AI Supervisor (%s)...\n", primarySupervisorName)
				
				var advice *agent.ResolutionAdvice
				var errAdv error
				for retry := 0; retry < 4; retry++ {
					advice, errAdv = scAgentSupervisor.AnalyzeDisagreement(ctx, briefingDoc, title, abs, notes1, notes2)
					if errAdv == nil && advice != nil { break }
					
					baseDelaySec := float64(backoffDelays[retry])
					jitter := (rand.Float64()*0.4 - 0.2) * baseDelaySec 
					finalDelaySec := baseDelaySec + jitter
					backoff := time.Duration(finalDelaySec * float64(time.Minute))
					logger.Logf(session.ID, "      [Supervisor Retry %d] Error LLM: %v. Menunggu %v...", retry+1, errAdv, backoff)
					time.Sleep(backoff)
				}
				
				// Fallback on-the-fly jika provider utama adalah Xiaomi dan dia gagal (token habis/error)
				if (errAdv != nil || advice == nil) && primarySupervisorName == "Xiaomi MiMo" {
					logger.Logf(session.ID, "      [!] %s gagal memberikan saran setelah 4 percobaan. Melakukan Fallback on-the-fly ke OpenRouter...\n", primarySupervisorName)
					llmFallback, errFb := m.deps.LLMFactory.CreateClient(ctx, "openrouter")
					if errFb == nil {
						fallbackAgent := agent.NewScreeningAgent(llmFallback)
						for retry := 0; retry < 3; retry++ {
							advice, errAdv = fallbackAgent.AnalyzeDisagreement(ctx, briefingDoc, title, abs, notes1, notes2)
							if errAdv == nil && advice != nil { 
								logger.Logf(session.ID, "      [V] Fallback OpenRouter berhasil memberikan saran resolusi.\n")
								break 
							}
							
							baseDelaySec := float64(backoffDelays[retry])
							jitter := (rand.Float64()*0.4 - 0.2) * baseDelaySec 
							finalDelaySec := baseDelaySec + jitter
							backoff := time.Duration(finalDelaySec * float64(time.Minute))
							logger.Logf(session.ID, "      [Fallback Retry %d] Error LLM OpenRouter: %v. Menunggu %v...", retry+1, errAdv, backoff)
							time.Sleep(backoff)
						}
					} else {
						logger.Logf(session.ID, "      [!] Gagal memuat OpenRouter untuk fallback: %v\n", errFb)
					}
				}
				
				if errAdv == nil && advice != nil {
					conflictRes = fmt.Sprintf("[AI_SUGGESTION: %s] %s", advice.Advice, advice.Analysis)
				} else {
					logger.Logf(session.ID, "      [!] Supervisor gagal memberikan saran resolusi (baik Utama maupun Fallback).\n")
					conflictRes = "[AI_SUGGESTION: ERROR] Supervisor gagal merespons akibat error koneksi pada provider."
				}
			}

			updateDoc := map[string]interface{}{
				"Screener_1_Decision": res1.Recommend,
				"Screener_1_Reason_Code": res1.ReasonCode,
				"Screener_1_Notes": notes1,
				"Screener_2_Decision": res2.Recommend,
				"Screener_2_Reason_Code": res2.ReasonCode,
				"Screener_2_Notes": notes2,
				"Agreement": agreement,
				"Conflict_Resolution": conflictRes,
				"Batch_Evaluated": false,
			}
			m.deps.MongoRepo.UpdateScreeningPaper(ctx, p["_id"], updateDoc)

			total++
		}

		// Jika sukses menyelesaikan kuota tanpa error (crash) API, 
		// kita set status agar di loop ExecuteAsync selanjutnya masuk kembali ke M5_STEP3_BATCH_SCREENING 
		// untuk dievaluasi oleh State Machine di awal fungsi.
		session.Status = "M5_STEP3_BATCH_SCREENING"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M5_STEP3_WAITING_RESOLUTION":
		logger.Log(session.ID, "   [System] Sesi dijeda untuk evaluasi batch (HitL).")
		logger.Log(session.ID, "   1. Periksa bagian 'Tindakan Anda' di UI web.")
		logger.Log(session.ID, "   2. Jika ada konflik (Disagreements), baca rekomendasi AI Arbitrator.")
		logger.Log(session.ID, "   3. Tentukan keputusan akhir (INCLUDE/EXCLUDE) dan klik 'Simpan Keputusan & Lanjutkan'.")
		logger.Log(session.ID, "   4. Jika tidak ada konflik, sistem memunculkan tombol 'Lanjut Batch Berikutnya / Selesai'.")
		return nil

	case "M5_STEP4_REVIEW_HASIL":
		logger.Log(session.ID, "   [Langkah 5.4] Menyusun Exclusion Table dan Modul 5 Summary...")

		papers, err := m.deps.MongoRepo.GetAllScreeningPapers(ctx, session.ID)
		if err != nil { return fmt.Errorf("gagal get all papers: %w", err) }

		totalIdentified := 0
		if session.DataMiningLog != nil && session.DataMiningLog.QualityAudit != nil { totalIdentified = session.DataMiningLog.QualityAudit.TotalRecords }
		duplicatesRemoved := 0
		if session.DataMiningLog != nil && session.DataMiningLog.Dedup != nil { duplicatesRemoved = session.DataMiningLog.Dedup.TotalDuplicates }
		recordsScreened := len(papers)

		var included, excluded []map[string]interface{}
		reasonCounts := make(map[string]int)
		var deferredCount int
		var resolvedDiscussCount int

		for _, p := range papers {
			finalDec := ""
			if val, ok := p["Final_Decision"].(string); ok && val != "" {
				finalDec = val
			} else if val, ok := p["Screener_1_Decision"].(string); ok {
				finalDec = val
			}
			
			if finalDec == "INCLUDE" {
				included = append(included, p)
			} else if finalDec == "EXCLUDE" {
				excluded = append(excluded, p)
				reason := "OTHER"
				if r1rc, ok := p["Screener_1_Reason_Code"].(string); ok && r1rc != "" && r1rc != "-" {
					reason = r1rc
				}
				reasonCounts[reason]++
			} else {
				deferredCount++ // Uncertain
			}

			if cr, ok := p["Conflict_Resolution"].(string); ok && cr != "" {
				resolvedDiscussCount++
			}
		}

		// 1. FLOW NUMBERS
		flowNumbers := fmt.Sprintf("- Total records identified: %d\n- Duplicates removed: %d\n- Records screened: %d\n- Records excluded: %d\n- Records included for full-text: %d", 
			totalIdentified, duplicatesRemoved, recordsScreened, len(excluded), len(included))

		// 2. EXCLUSION REASONS TABLE
		exclusionReasons := "| Reason Code | Count | % | Deskripsi |\n|---|---|---|---|\n"
		for code, count := range reasonCounts {
			pct := 0.0
			if len(excluded) > 0 { pct = float64(count) / float64(len(excluded)) * 100 }
			
			desc := "Tidak memenuhi kriteria"
			codeUpper := strings.ToUpper(code)
			if strings.Contains(codeUpper, "P-NOMATCH") {
				desc = "Populasi/Subjek tidak sesuai kriteria"
			} else if strings.Contains(codeUpper, "I-NOMATCH") {
				desc = "Intervensi/Teknologi tidak sesuai kriteria"
			} else if strings.Contains(codeUpper, "C-NOMATCH") {
				desc = "Pembanding (Comparator) tidak sesuai"
			} else if strings.Contains(codeUpper, "O-NOMATCH") {
				desc = "Luaran (Outcome) tidak relevan"
			} else if strings.Contains(codeUpper, "S-NOMATCH") {
				desc = "Tipe/Desain Studi tidak sesuai kriteria"
			} else if strings.Contains(codeUpper, "DATE-NOMATCH") {
				desc = "Tahun publikasi di luar rentang"
			} else if strings.Contains(codeUpper, "LANG-NOMATCH") {
				desc = "Bahasa publikasi tidak didukung"
			} else if strings.Contains(codeUpper, "OTHER") {
				desc = "Alasan lainnya"
			}
			
			if idx := strings.Index(code, "("); idx != -1 {
				if endIdx := strings.Index(code, ")"); endIdx != -1 && endIdx > idx {
					desc += ": " + code[idx+1:endIdx]
				}
			}

			exclusionReasons += fmt.Sprintf("| %s | %d | %.1f%% | %s |\n", code, count, pct, desc)
		}

		// 3. KAPPA REPORT
		iter1Kappa := 0.0
		finalKappa := 0.0
		if len(session.KalibrasiLog) > 0 {
			iter1Kappa = session.KalibrasiLog[0].Kappa
			finalKappa = session.KalibrasiLog[len(session.KalibrasiLog)-1].Kappa
		}
		
		batchKappa := 0.0
		if len(session.ScreeningResultsLog) > 0 {
			batchKappa = session.ScreeningResultsLog[len(session.ScreeningResultsLog)-1].CurrentKappa
		}
		
		kappaClass := "Fair"
		if finalKappa >= 0.80 { kappaClass = "Almost Perfect" } else if finalKappa >= 0.60 { kappaClass = "Substantial" }
		
		kappaReport := fmt.Sprintf("- Kalibrasi iterasi 1: %.3f\n- Jumlah iterasi kalibrasi: %d\n- Kalibrasi final: %.3f\n- Batch massal final: %.3f\n- Klasifikasi: %s\n- Disagreements resolved: %d\n- Deferred ke full-text: %d",
			iter1Kappa, len(session.KalibrasiLog), finalKappa, batchKappa, kappaClass, resolvedDiscussCount, deferredCount)

		// Initialize Agent
		llm, _ := m.deps.LLMFactory.CreateClient(ctx, "xiaomi")
		scAgent := agent.NewScreeningAgent(llm)

		// 4. PICO AUDIT (10% Random INCLUDE)
		picoAuditText := "Audit dilewati (Tidak ada paper INCLUDE)"
		if len(included) > 0 {
			logger.Log(session.ID, "      -> Menjalankan PICO-Consistency Audit (10% Sample)...")
			sampleSize := len(included) / 10
			if sampleSize < 1 { sampleSize = 1 }
			if sampleSize > 10 { sampleSize = 10 } // limit token

			rand.Seed(time.Now().UnixNano())
			rand.Shuffle(len(included), func(i, j int) { included[i], included[j] = included[j], included[i] })
			
			sampleData, _ := json.Marshal(included[:sampleSize])
			
			picoDef := ""
			if session.PICODefinitions != nil {
				picoBytes, _ := json.Marshal(session.PICODefinitions)
				picoDef = string(picoBytes)
			}

			auditRes, err := scAgent.AuditPICO(ctx, picoDef, string(sampleData))
			if err != nil {
				logger.Log(session.ID, "      [!] Gagal Audit PICO dgn Xiaomi. Mencoba Fallback ke Zhipu...")
				llmFallback, errFb := m.deps.LLMFactory.CreateClient(ctx, "zhipu")
				if errFb == nil {
					scAgentFallback := agent.NewScreeningAgent(llmFallback)
					auditRes, err = scAgentFallback.AuditPICO(ctx, picoDef, string(sampleData))
				}
				
				// Secondary fallback to groq if Zhipu also fails
				if err != nil {
					logger.Log(session.ID, "      [!] Gagal Audit PICO dgn Zhipu. Mencoba Fallback ke Groq...")
					llmFallback2, errFb2 := m.deps.LLMFactory.CreateClient(ctx, "groq")
					if errFb2 == nil {
						scAgentFallback2 := agent.NewScreeningAgent(llmFallback2)
						auditRes, err = scAgentFallback2.AuditPICO(ctx, picoDef, string(sampleData))
					}
				}
			}

			if err == nil && auditRes != nil {
				picoAuditText = fmt.Sprintf("Slipped-through: %d\nAction: %s\nAnalysis: %s", auditRes.SlippedThroughCount, auditRes.Action, auditRes.Analysis)
			} else {
				picoAuditText = "Gagal menjalankan audit: " + err.Error()
			}
		}

		// 5. FULL-TEXT PRIORITIZATION
		fullTextPrep := "Tidak ada paper untuk diprioritaskan."
		if len(included) > 0 {
			logger.Log(session.ID, "      -> Menjalankan Full-Text Prioritization...")
			// Batasi jumlah yang dikirim ke LLM jika terlalu banyak (max 50)
			limit := len(included)
			if limit > 50 { limit = 50 }
			sampleData, _ := json.Marshal(included[:limit])
			res, err := scAgent.PrioritizeFullText(ctx, string(sampleData))
			if err != nil {
				logger.Log(session.ID, "      [!] Gagal Prioritize dgn Xiaomi. Mencoba Fallback ke Zhipu...")
				llmFallback, errFb := m.deps.LLMFactory.CreateClient(ctx, "zhipu")
				if errFb == nil {
					scAgentFallback := agent.NewScreeningAgent(llmFallback)
					res, err = scAgentFallback.PrioritizeFullText(ctx, string(sampleData))
				}
				
				// Secondary fallback to groq if Zhipu also fails
				if err != nil {
					logger.Log(session.ID, "      [!] Gagal Prioritize dgn Zhipu. Mencoba Fallback ke Groq...")
					llmFallback2, errFb2 := m.deps.LLMFactory.CreateClient(ctx, "groq")
					if errFb2 == nil {
						scAgentFallback2 := agent.NewScreeningAgent(llmFallback2)
						res, err = scAgentFallback2.PrioritizeFullText(ctx, string(sampleData))
					}
				}
			}
			if err == nil { fullTextPrep = res }
		}

		session.ExclusionTable = &model.ExclusionTable{
			FlowNumbers: flowNumbers,
			ExclusionReasons: exclusionReasons,
			KappaReport: kappaReport,
			PICOAudit: picoAuditText,
			FullTextPrep: fullTextPrep,
		}

		// OUTPUT 2: MODUL 5 SUMMARY
		summaryMd := fmt.Sprintf("=== TITLE/ABSTRACT SCREENING SUMMARY ===\n\n"+
			"FLOW NUMBERS (PRISMA):\n%s\n\n"+
			"KALIBRASI:\n- Sample 20 records, %d iterasi\n- Kappa iter 1 -> final: %.3f -> %.3f\n\n"+
			"BATCH SCREENING:\n- Total screened: %d\n- R1 + R2 complete: ✓\n- Final kappa: %.3f\n\n"+
			"DECISIONS:\n- INCLUDE for full-text: %d\n- EXCLUDE: %d\n- UNCERTAIN deferred: %d\n\n"+
			"EXCLUSION TABLE:\n%s\n\n"+
			"DISAGREEMENT RESOLUTION:\n- Total resolved/discussed: %d\n\n"+
			"PICO-CONSISTENCY AUDIT:\n%s\n\n"+
			"FULL-TEXT PREP:\n%s", 
			flowNumbers, len(session.KalibrasiLog), iter1Kappa, finalKappa, recordsScreened, batchKappa, 
			len(included), len(excluded), deferredCount, exclusionReasons, resolvedDiscussCount, picoAuditText, fullTextPrep)

		session.Modul5Summary = &model.Modul5Summary{Markdown: summaryMd}

		session.Status = "M5_STEP4_WAITING_APPROVAL"
		logger.Log(session.ID, "   [System] Exclusion Table & Modul 5 Summary berhasil di-generate!")
		logger.Log(session.ID, "   [System] Menunggu Persetujuan Anda (HITL) sebelum Modul 5 ditutup sepenuhnya.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M5_STEP4_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi dijeda. Menunggu persetujuan Anda atas hasil akhir (Modul 5 Summary).")
		logger.Log(session.ID, "   Tekan 'Approve & Selesai' di UI untuk mengakhiri Modul 5.")
		return nil

	case "M5_STEP4_APPROVED":
		session.Status = "M6_INIT"
		logger.Log(session.ID, "   [System] Modul 5 SELESAI. Memulai Modul 6 (Full-Text Acquisition).")

		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// M5_DONE is replaced by M6_INIT transition

	default:
		logger.Logf(session.ID, "[Modul 5] Sub-status %s tidak dikenali atau belum diimplementasikan.", session.Status)
		return nil
	}
}
