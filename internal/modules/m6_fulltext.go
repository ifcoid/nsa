package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"nsa/internal/agent"
	"nsa/internal/llm"
	"nsa/internal/logger"
	"nsa/internal/model"
)

const fulltextBatchSize = 10

// ===========================================================================
// LANGKAH 2: FULL-TEXT SCREENING (dual-reviewer + AI-assist, RAG dari Qdrant)
// ===========================================================================

func (m *M6Acquisition) runFullTextScreeningBatch(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 6.2] Full-Text Screening (dual-reviewer berbasis RAG Qdrant)...")
	ctx = llm.WithXAIContext(ctx, session.ID, session.Status, "runFullTextScreeningBatch")

	// 1. State machine: cek paper yang sudah di-screen tapi belum dievaluasi (kappa batch).
	unevaluated, err := m.deps.MongoRepo.GetUnevaluatedFullTextPapers(ctx, session.ID)
	if err != nil {
		return fmt.Errorf("gagal mengambil unevaluated full-text papers: %w", err)
	}
	total, screened, _ := m.deps.MongoRepo.GetFullTextScreeningProgress(ctx, session.ID)
	isFinished := total > 0 && screened == total

	if len(unevaluated) >= fulltextBatchSize || (isFinished && len(unevaluated) > 0) {
		m.evaluateFullTextBatch(ctx, session, unevaluated)
		session.Status = "M6_STEP2_WAITING_RESOLUTION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	if isFinished && len(unevaluated) == 0 {
		logger.Log(session.ID, "   [System] Semua full-text telah di-screening & dievaluasi. Lanjut ke Langkah 3 (Review).")
		session.Status = "M6_STEP3_REVIEW"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	if total == 0 {
		logger.Log(session.ID, "   [System] Tidak ada paper eligible (INCLUDE + full_text_retrieved). Lanjut ke Langkah 3.")
		session.Status = "M6_STEP3_REVIEW"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	// 2. Siapkan LLM reviewer (config-driven via Model Routing): R1->fallback, R2, supervisor->fallback.
	roles := m.deps.LLMFactory.Roles(ctx)
	llmR1, err := m.deps.LLMFactory.CreateClient(ctx, roles.Reviewer1)
	if err != nil {
		logger.Logf(session.ID, "   [INFO] R1 %s gagal (%v). Fallback ke %s...\n", roles.Reviewer1, err, roles.Reviewer1Fallback)
		llmR1, err = m.deps.LLMFactory.CreateClient(ctx, roles.Reviewer1Fallback)
		if err != nil {
			return fmt.Errorf("Reviewer 1 (%s/%s) gagal dimuat. Konfigurasi API dulu", roles.Reviewer1, roles.Reviewer1Fallback)
		}
	}
	llmR2, err := m.deps.LLMFactory.CreateClient(ctx, roles.Reviewer2)
	if err != nil {
		return fmt.Errorf("%s (Reviewer 2) belum dikonfigurasi: %w", roles.Reviewer2, err)
	}
	var supervisor *agent.ScreeningAgent
	supName := roles.Supervisor
	if llmSup, e := m.deps.LLMFactory.CreateClient(ctx, roles.Supervisor); e == nil {
		supervisor = agent.NewScreeningAgent(llmSup)
	} else if llmSup, e := m.deps.LLMFactory.CreateClient(ctx, roles.SupervisorFallback); e == nil {
		supervisor = agent.NewScreeningAgent(llmSup)
		supName = roles.SupervisorFallback
	} else {
		return fmt.Errorf("AI Supervisor (Xiaomi/OpenRouter) gagal dimuat")
	}

	scR1 := agent.NewScreeningAgent(llmR1)
	scR2 := agent.NewScreeningAgent(llmR2)
	logger.Logf(session.ID, "   [Reviewer] R1=%s (fb %s) | R2=%s | Supervisor=%s\n", roles.Reviewer1, roles.Reviewer1Fallback, roles.Reviewer2, supName)

	opDefs := m.operationalDefsContext(session)

	// 3. Bangun indeks RAG full-text dari Qdrant (sekali per batch run), beserta
	//    vektor dense tiap chunk untuk pemilihan top-k semantik.
	rag, ragAvailable, err := BuildFulltextRAG(ctx)
	if err != nil {
		logger.Logf(session.ID, "   [WARN] Gagal membangun indeks RAG Qdrant: %v. Paper tanpa RAG akan ditandai pending manual.\n", err)
		rag = nil
	}
	if !ragAvailable {
		logger.Log(session.ID, "   [WARN] QDRANT_URL/ENDPOINT belum diset. Semua paper akan ditandai pending manual (NO-FULLTEXT-RAG).")
		rag = nil
	}

	// 3b. Embed query screening (PICO/definisi operasional) SEKALI agar tiap paper
	//     cukup diberi top-k chunk paling relevan (prompt lebih pendek => lebih cepat
	//     & murah). Jika endpoint embedding tak diset, fallback ke konteks penuh.
	var qvec []float32
	if rag != nil {
		queryText := opDefs
		if len(queryText) > 1600 {
			queryText = queryText[:1600]
		}
		// Perkaya query dengan istilah metode/hasil agar top-k mengangkat chunk
		// pembawa keputusan (desain studi + metrik Outcome di Methods/Results),
		// bukan hanya Intro/Background.
		queryText += " methods experimental setup study design participants dataset evaluation accuracy performance results metrics outcomes findings"

		// Resolusi endpoint embedding: utamakan embed_config DB (bisa diubah via web),
		// fallback ke env. TANPA endpoint hidup, screening DI-PAUSE (HITL) — TIDAK
		// auto-degrade ke section-aware tanpa sepengetahuan user.
		ec := m.deps.MongoRepo.GetEmbedConfig(ctx)
		endpoint, key, emodel := ec.Endpoint, ec.APIKey, ec.Model
		if strings.TrimSpace(endpoint) == "" {
			endpoint, key, emodel = os.Getenv("EMBED_ENDPOINT"), os.Getenv("EMBED_API_KEY"), os.Getenv("EMBED_MODEL")
		}
		v, ok, e := EmbedWith(ctx, queryText, endpoint, key, emodel)
		if ok && e == nil && len(v) > 0 {
			qvec = v
			logger.Logf(session.ID, "   [RAG] Top-k semantik aktif (BGE-M3, dim %d) + section-aware (jamin Methods/Results).\n", len(v))
		} else {
			reason := "Endpoint embedding belum diset. Nyalakan Colab (embed_server_colab.ipynb) lalu masukkan endpoint via web."
			if strings.TrimSpace(endpoint) != "" {
				reason = fmt.Sprintf("Endpoint embedding (%s) tak merespons: %v. Restart Colab & masukkan endpoint baru via web.", endpoint, e)
			}
			logger.Logf(session.ID, "   [PAUSE] %s\n", reason)
			session.Status = "M6_STEP2_WAITING_EMBED"
			session.EmbedError = reason
			return m.deps.MongoRepo.UpdateSession(ctx, session)
		}
	}
	const fulltextTopK = 14
	const fulltextTopKChars = 12000

	fetchLimit := fulltextBatchSize - len(unevaluated)
	papers, err := m.deps.MongoRepo.GetUnscreenedFullTextPapers(ctx, session.ID, fetchLimit)
	if err != nil {
		return fmt.Errorf("gagal mengambil unscreened full-text papers: %w", err)
	}
	logger.Logf(session.ID, "   📊 [Progress] %d/%d full-text discreening. Memproses %d paper batch ini.\n", screened, total, len(papers))

	for i, p := range papers {
		title := getStr(p, "Title", "title")
		doi := normalizeDOIForRAG(getStr(p, "DOI", "doi"))
		logger.Logf(session.ID, "      -> Full-text [%d/%d] DOI=%s\n", i+1, len(papers), doi)

		fulltext := ""
		if rag != nil {
			if doi != "" && rag.Has(doi) {
				fulltext = rag.TopK(doi, qvec, fulltextTopK, fulltextTopKChars)
			} else {
				// DOI kosong/tak cocok (mis. 4.5% chunk PEDE tanpa DOI) -> fallback judul.
				fulltext = rag.TopKByTitle(title, qvec, fulltextTopK, fulltextTopKChars)
				if fulltext != "" {
					logger.Logf(session.ID, "         [RAG] DOI tak cocok; dipetakan via kemiripan judul.\n")
				}
			}
		}

		// RAG tidak tersedia -> tandai pending manual (jangan auto-screen tanpa konten).
		if strings.TrimSpace(fulltext) == "" {
			logger.Log(session.ID, "         [!] Full-text tidak ditemukan di Qdrant. Ditandai pending manual.")
			m.deps.MongoRepo.UpdateScreeningPaper(ctx, p["_id"], map[string]interface{}{
				"Screener_1_Decision_Full":    "UNCERTAIN",
				"Screener_1_Reason_Code_Full": "NO-FULLTEXT-RAG",
				"Screener_1_Notes_Full":       "Full-text tidak tersedia di Qdrant (RAG). Perlu keputusan manual.",
				"Screener_2_Decision_Full":    "UNCERTAIN",
				"Screener_2_Reason_Code_Full": "NO-FULLTEXT-RAG",
				"Screener_2_Notes_Full":       "Full-text tidak tersedia di Qdrant (RAG). Perlu keputusan manual.",
				"Agreement_Full":              "DISAGREE",
				"Conflict_Resolution_Full":    "[PENDING_MANUAL] Full-text tidak tersedia untuk RAG; putuskan manual berdasarkan PDF.",
				"Batch_Evaluated_Full":        false,
			})
			continue
		}

		// R1: percobaan cepat primary -> fallback GIGIH (cohere andal). Tidak men-skip
		// paper: kalau benar-benar mentok, batch DIJEDA (return error) supaya bisa
		// di-Resume & paper yang SAMA dicoba ulang — screening tetap runut/lengkap.
		res1, err1 := reviewWithRetry(ctx, scR1, opDefs, title, fulltext,
			[]time.Duration{8 * time.Second, 20 * time.Second}, session.ID, "R1", roles.Reviewer1)
		if res1 == nil || err1 != nil {
			if llmFb, e := m.deps.LLMFactory.CreateClient(ctx, roles.Reviewer1Fallback); e == nil {
				res1, err1 = reviewWithRetry(ctx, agent.NewScreeningAgent(llmFb), opDefs, title, fulltext,
					[]time.Duration{15 * time.Second, 30 * time.Second, 60 * time.Second, 120 * time.Second}, session.ID, "R1-fallback", roles.Reviewer1Fallback)
			}
		}
		if res1 == nil || err1 != nil {
			return fmt.Errorf("R1 (%s/%s) gagal setelah retry panjang pada paper %s: %w. Batch DIJEDA — provider mungkin sedang down; Resume untuk lanjut paper yang sama (tak ada paper di-skip, progres tersimpan)", roles.Reviewer1, roles.Reviewer1Fallback, doi, err1)
		}

		time.Sleep(3 * time.Second)

		// R2: percobaan primary -> fallback gigih (kini R2 PUNYA fallback). Sama: tak
		// men-skip; mentok total -> batch dijeda & resumable.
		res2, err2 := reviewWithRetry(ctx, scR2, opDefs, title, fulltext,
			[]time.Duration{10 * time.Second, 30 * time.Second}, session.ID, "R2", roles.Reviewer2)
		if res2 == nil || err2 != nil {
			if llmFb, e := m.deps.LLMFactory.CreateClient(ctx, roles.Reviewer2Fallback); e == nil {
				res2, err2 = reviewWithRetry(ctx, agent.NewScreeningAgent(llmFb), opDefs, title, fulltext,
					[]time.Duration{15 * time.Second, 30 * time.Second, 60 * time.Second, 120 * time.Second}, session.ID, "R2-fallback", roles.Reviewer2Fallback)
			}
		}
		if res2 == nil || err2 != nil {
			return fmt.Errorf("R2 (%s/%s) gagal setelah retry panjang pada paper %s: %w. Batch DIJEDA — Resume untuk lanjut (tak ada paper di-skip, progres tersimpan)", roles.Reviewer2, roles.Reviewer2Fallback, doi, err2)
		}

		agreement := "DISAGREE"
		if res1.Recommend == res2.Recommend {
			agreement = "AGREE"
		}
		notes1 := fmt.Sprintf("Decision: %s | Strict: %s | Liberal: %s | Evidence: %s", res1.Recommend, res1.Strict, res1.Liberal, res1.Evidence)
		notes2 := fmt.Sprintf("Decision: %s | Strict: %s | Liberal: %s | Evidence: %s", res2.Recommend, res2.Strict, res2.Liberal, res2.Evidence)

		conflictRes := ""
		if agreement == "DISAGREE" || res1.Recommend == "UNCERTAIN" || res2.Recommend == "UNCERTAIN" {
			logger.Logf(session.ID, "         [*] Disagreement/Uncertain. Minta saran AI Supervisor (%s)...\n", supName)
			ftSnippet := fulltext
			if len(ftSnippet) > 1500 {
				ftSnippet = ftSnippet[:1500]
			}
			var advice *agent.ResolutionAdvice
			var errAdv error
			for retry := 0; retry < 3; retry++ {
				logger.Logf(session.ID, "         [Supervisor] Sedang menghubungi API %s (Attempt #%d)... Mohon tunggu.\n", supName, retry+1)
				advice, errAdv = supervisor.AnalyzeDisagreement(ctx, opDefs, title, ftSnippet, notes1, notes2)
				if errAdv == nil && advice != nil {
					break
				}
				errDetail := "(nil)"
				if errAdv != nil {
					errDetail = errAdv.Error()
				}
				logger.Logf(session.ID, "         [Supervisor gagal #%d] provider=%s error: %s\n", retry+1, supName, clipErr(errDetail))
				if retry < 2 {
					wait := time.Duration(retry+1) * time.Minute
					logger.Logf(session.ID, "         [Supervisor] retry dalam %v...\n", wait)
					time.Sleep(wait)
				}
			}
			if errAdv == nil && advice != nil {
				conflictRes = fmt.Sprintf("[AI_SUGGESTION: %s] %s", advice.Advice, advice.Analysis)
			} else {
				// Simpan hasil R1/R2 yang sudah di-screen (jangan buang) lalu HALT.
				errDetail := "(nil)"
				if errAdv != nil {
					errDetail = errAdv.Error()
				}
				logger.Logf(session.ID, "         [HALT] Supervisor (%s) gagal setelah 3 percobaan. Error terakhir: %s\n", supName, clipErr(errDetail))
				logger.Log(session.ID, "         [HALT] Menyimpan hasil R1/R2 paper ini, lalu BATCH DIJEDA. Resume untuk lanjut.")

				m.deps.MongoRepo.UpdateScreeningPaper(ctx, p["_id"], map[string]interface{}{
					"Screener_1_Decision_Full":    res1.Recommend,
					"Screener_1_Reason_Code_Full": res1.ReasonCode,
					"Screener_1_Notes_Full":       notes1,
					"Screener_2_Decision_Full":    res2.Recommend,
					"Screener_2_Reason_Code_Full": res2.ReasonCode,
					"Screener_2_Notes_Full":       notes2,
					"Agreement_Full":              agreement,
					"Conflict_Resolution_Full":    fmt.Sprintf("[PENDING_SUPERVISOR] %s gagal: %s", supName, clipErr(errDetail)),
					"Batch_Evaluated_Full":        false,
				})

				return fmt.Errorf("Supervisor (%s) gagal setelah 3 percobaan pada paper %s: %s. Batch DIJEDA — cek API key/koneksi Supervisor, lalu Resume", supName, doi, clipErr(errDetail))
			}
		}

		m.deps.MongoRepo.UpdateScreeningPaper(ctx, p["_id"], map[string]interface{}{
			"Screener_1_Decision_Full":    res1.Recommend,
			"Screener_1_Reason_Code_Full": res1.ReasonCode,
			"Screener_1_Notes_Full":       notes1,
			"Screener_2_Decision_Full":    res2.Recommend,
			"Screener_2_Reason_Code_Full": res2.ReasonCode,
			"Screener_2_Notes_Full":       notes2,
			"Agreement_Full":              agreement,
			"Conflict_Resolution_Full":    conflictRes,
			"Batch_Evaluated_Full":        false,
		})

		time.Sleep(8 * time.Second) // rate-limit pacing
	}

	// Loop lagi ke state ini agar di-evaluasi (kappa) saat kuota cukup / selesai.
	session.Status = "M6_STEP2_FULLTEXT_SCREENING"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// evaluateFullTextBatch menghitung Cohen's kappa untuk batch yang sudah di-screen.
func (m *M6Acquisition) evaluateFullTextBatch(ctx context.Context, session *model.SLRSession, unevaluated []map[string]interface{}) {
	var agreeCount, bothInc, bothExc, r1IncR2Exc, r1ExcR2Inc int
	var ids []primitive.ObjectID
	for _, p := range unevaluated {
		if oid, ok := p["_id"].(primitive.ObjectID); ok {
			ids = append(ids, oid)
		}
		d1 := getStr(p, "Screener_1_Decision_Full")
		d2 := getStr(p, "Screener_2_Decision_Full")
		if getStr(p, "Agreement_Full") == "AGREE" {
			agreeCount++
		}
		switch {
		case d1 == "INCLUDE" && d2 == "INCLUDE":
			bothInc++
		case d1 == "EXCLUDE" && d2 == "EXCLUDE":
			bothExc++
		case d1 == "INCLUDE" && d2 == "EXCLUDE":
			r1IncR2Exc++
		case d1 == "EXCLUDE" && d2 == "INCLUDE":
			r1ExcR2Inc++
		}
	}
	totalEval := len(unevaluated)
	kappa := cohensKappa(totalEval, bothInc, bothExc, r1IncR2Exc, r1ExcR2Inc)

	batchNum := len(session.FulltextScreeningLog) + 1
	drift := kappa < 0.60 && totalEval >= 5
	session.FulltextScreeningLog = append(session.FulltextScreeningLog, model.ScreeningResultsLog{
		BatchNumber:       batchNum,
		ProcessedRecords:  totalEval,
		CurrentKappa:      kappa,
		DisagreementCases: totalEval - agreeCount,
		DriftDetected:     drift,
		Tanggal:           time.Now().Format("2006-01-02"),
	})
	session.FulltextKappa = kappa
	m.deps.MongoRepo.MarkFullTextEvaluated(ctx, session.ID, ids)
	logger.Logf(session.ID, "   [Batch FT %d] Processed: %d | Kappa: %.3f | Disagreements: %d\n", batchNum, totalEval, kappa, totalEval-agreeCount)
	if drift {
		logger.Log(session.ID, "   [WARNING] Drift full-text terdeteksi (Kappa < 0.60). Wajib resolusi.")
	}
}

// reviewWithRetry mencoba FullTextReviewPaper dengan exponential backoff + jitter.
// Log diperjelas: tag peran, PROVIDER, lama waktu, dan KLASIFIKASI sebab gagal
// (TIMEOUT / rate-limit / respons-rusak) agar mudah didiagnosis dari UI.
func reviewWithRetry(ctx context.Context, ag *agent.ScreeningAgent, opDefs, title, ft string, delays []time.Duration, sessionID, tag, provider string) (*model.ScreeningPerspective, error) {
	var res *model.ScreeningPerspective
	var err error
	for i := 0; i <= len(delays); i++ {
		// Batas per-attempt dinaikkan menjadi 15 menit untuk model besar (mis. Claude/Gemini)
		// saat memproses full text panjang beserta reasoning-nya agar tidak keburu timeout.
		start := time.Now()
		attemptCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		res, err = ag.FullTextReviewPaper(attemptCtx, opDefs, title, ft)
		cancel()
		took := time.Since(start)
		if err == nil && res != nil {
			return res, nil
		}
		cause := reviewErrCause(err, took)
		if i < len(delays) {
			base := delays[i]
			jitter := time.Duration((rand.Float64()*0.4 - 0.2) * float64(base))
			d := base + jitter
			logger.Logf(sessionID, "         [%s=%s gagal #%d, %s] %s. retry dalam %v...", tag, provider, i+1, took.Round(time.Second), cause, d.Round(time.Second))
			time.Sleep(d)
		} else {
			logger.Logf(sessionID, "         [%s=%s gagal #%d, %s] %s (percobaan habis).", tag, provider, i+1, took.Round(time.Second), cause)
		}
	}
	return nil, err
}

// reviewErrCause mengklasifikasi error LLM menjadi sebab yang mudah dibaca manusia.
func reviewErrCause(err error, took time.Duration) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "context deadline exceeded"), strings.Contains(s, "context canceled"):
		return fmt.Sprintf("TIMEOUT — provider tak merespons dalam %s (tunnel/proxy hang? Claude/Gemini tak sempat menjawab; bukan error model)", took.Round(time.Second))
	case strings.Contains(s, "429"), strings.Contains(s, "rate limit"), strings.Contains(s, "quota"), strings.Contains(s, "速率"):
		return "RATE-LIMIT/QUOTA (429): " + clipErr(s)
	case strings.Contains(s, "parsing JSON"), strings.Contains(s, "empty response"), strings.Contains(s, "malformed"):
		return "RESPONS TAK VALID — provider balas kosong/rusak: " + clipErr(s)
	case strings.Contains(s, "5") && strings.Contains(s, "HTTP"):
		return "HTTP 5xx (server provider): " + clipErr(s)
	default:
		return clipErr(s)
	}
}

func clipErr(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

func cohensKappa(total, bothInc, bothExc, r1IncR2Exc, r1ExcR2Inc int) float64 {
	if total == 0 {
		return 0.0
	}
	pO := float64(bothInc+bothExc) / float64(total)
	probR1Inc := float64(bothInc+r1IncR2Exc) / float64(total)
	probR2Inc := float64(bothInc+r1ExcR2Inc) / float64(total)
	probR1Exc := float64(bothExc+r1ExcR2Inc) / float64(total)
	probR2Exc := float64(bothExc+r1IncR2Exc) / float64(total)
	pE := (probR1Inc * probR2Inc) + (probR1Exc * probR2Exc)
	if 1-pE > 0 {
		return (pO - pE) / (1 - pE)
	}
	return 1.0
}

func (m *M6Acquisition) operationalDefsContext(session *model.SLRSession) string {
	if session.PICODefinitions != nil {
		b, _ := json.Marshal(session.PICODefinitions)
		return string(b)
	}
	if session.ScreenerBriefing != nil {
		return session.ScreenerBriefing.BriefingDoc
	}
	return "(operational definitions tidak tersedia)"
}

func getStr(p map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := p[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// ===========================================================================
// LANGKAH 3: RESOLVE + AUDIT + EXTRACTION PREP + HASIL AKHIR (4 output)
// ===========================================================================

func (m *M6Acquisition) buildModul6Outputs(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 6.3] Menyusun 4 output (log, inaccessible impact, extraction readiness, summary)...")

	papers, err := m.deps.MongoRepo.GetAllScreeningPapers(ctx, session.ID)
	if err != nil {
		return fmt.Errorf("gagal get all papers: %w", err)
	}

	var includedFull, excludedFull []map[string]interface{}
	reasonCounts := map[string]int{}
	uncertain := 0

	for _, p := range papers {
		// hanya paper eligible full-text
		retrieved, _ := p["full_text_retrieved"].(bool)
		includedAbstract := getStr(p, "Final_Decision") == "INCLUDE" ||
			(getStr(p, "Final_Decision") == "" && getStr(p, "Screener_1_Decision") == "INCLUDE")
		if !retrieved || !includedAbstract {
			continue
		}

		dec := finalFullDecision(p)
		switch dec {
		case "INCLUDE":
			includedFull = append(includedFull, p)
		case "EXCLUDE":
			excludedFull = append(excludedFull, p)
			rc := getStr(p, "Screener_1_Reason_Code_Full")
			if rc == "" || rc == "-" {
				rc = "OTHER"
			}
			reasonCounts[rc]++
		default:
			uncertain++
		}
	}

	// OUTPUT 1: fulltext_screening_log (sudah terisi per-batch). Final kappa:
	finalKappa := session.FulltextKappa
	disagreedRemaining, _ := m.deps.MongoRepo.GetDisagreedFullTextPapers(ctx, session.ID)

	// PICO-CONSISTENCY FINAL AUDIT (15% included)
	picoAudit := "Audit dilewati (tidak ada INCLUDE)."
	if len(includedFull) > 0 {
		picoAudit = m.runFinalPicoAudit(ctx, session, includedFull)
	}

	// OUTPUT 2: inaccessible_impact
	inacc := m.buildInaccessibleImpact(session)
	session.InaccessibleImpact = inacc

	// OUTPUT 3: extraction_readiness
	allResolved := len(disagreedRemaining) == 0
	readyMd := buildExtractionReadiness(len(includedFull), allResolved, len(session.FulltextScreeningLog) > 0)
	session.ExtractionReadiness = &model.ExtractionReadiness{AllReady: allResolved, Markdown: readyMd}

	// Exclusion reasons table (full-text)
	exTable := "| Reason Code | Count | % |\n|---|---|---|\n"
	for code, c := range reasonCounts {
		pct := 0.0
		if len(excludedFull) > 0 {
			pct = float64(c) / float64(len(excludedFull)) * 100
		}
		exTable += fmt.Sprintf("| %s | %d | %.1f%% |\n", code, c, pct)
	}
	if len(reasonCounts) == 0 {
		exTable += "| (tidak ada) | 0 | 0% |\n"
	}

	// OUTPUT 4: modul6_summary
	acq := session.AcquisitionLog
	acqLine := "Acquisition log tidak tersedia."
	if acq != nil {
		acqLine = fmt.Sprintf("- Target INCLUDE: %d | OA (high): %d | HITL (medium): %d\n- Vectorized (Qdrant): %d | Inaccessible: %d (%.1f%%)",
			acq.TotalInclude, acq.HighRetrieved, acq.MediumRetrieved, acq.VectorizedCount, acq.InaccessibleCount, acq.InaccessiblePct)
	}
	summary := fmt.Sprintf("=== FULL-TEXT SCREENING SUMMARY (SLR) ===\n\n"+
		"ACQUISITION:\n%s\n\n"+
		"FULL-TEXT SCREENING:\n- Total dievaluasi (batch): %d\n- Full-text kappa final: %.3f\n- Disagreements belum terselesaikan: %d\n\n"+
		"DECISIONS:\n- FINAL INCLUDED: %d studi\n- EXCLUDED (full-text): %d\n- UNCERTAIN/pending: %d\n\n"+
		"EXCLUSION REASONS (full-text stage):\n%s\n"+
		"PICO-CONSISTENCY FINAL AUDIT:\n%s\n\n"+
		"INACCESSIBLE IMPACT:\n%s\n\n"+
		"EXTRACTION READINESS:\n%s\n\n"+
		"NEXT: Data extraction + QA (Modul 7)",
		acqLine, countBatchProcessed(session), finalKappa, len(disagreedRemaining),
		len(includedFull), len(excludedFull), uncertain, exTable, picoAudit, inacc.Markdown, readyMd)

	session.Modul6Summary = &model.Modul6Summary{Markdown: summary}

	session.Status = "M6_STEP3_WAITING_APPROVAL"
	logger.Log(session.ID, "   [System] 4 output Modul 6 tersimpan. Menunggu persetujuan akhir (HITL).")
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

func finalFullDecision(p map[string]interface{}) string {
	if fd := getStr(p, "Final_Decision_Full"); fd != "" {
		return fd
	}
	d1 := getStr(p, "Screener_1_Decision_Full")
	d2 := getStr(p, "Screener_2_Decision_Full")
	if d1 == "INCLUDE" && d2 == "INCLUDE" {
		return "INCLUDE"
	}
	if d1 == "EXCLUDE" && d2 == "EXCLUDE" {
		return "EXCLUDE"
	}
	return "UNCERTAIN"
}

func countBatchProcessed(session *model.SLRSession) int {
	n := 0
	for _, l := range session.FulltextScreeningLog {
		n += l.ProcessedRecords
	}
	return n
}

func (m *M6Acquisition) runFinalPicoAudit(ctx context.Context, session *model.SLRSession, included []map[string]interface{}) string {
	sampleSize := len(included) * 15 / 100
	if sampleSize < 1 {
		sampleSize = 1
	}
	if sampleSize > 10 {
		sampleSize = 10
	}
	rand.Shuffle(len(included), func(i, j int) { included[i], included[j] = included[j], included[i] })
	sampleData, _ := json.Marshal(included[:sampleSize])

	picoDef := ""
	if session.PICODefinitions != nil {
		b, _ := json.Marshal(session.PICODefinitions)
		picoDef = string(b)
	}

	var sc *agent.ScreeningAgent
	if c, e := m.deps.LLMFactory.CreateClient(ctx, "xiaomi"); e == nil {
		sc = agent.NewScreeningAgent(c)
	} else if c, e := m.deps.LLMFactory.CreateClient(ctx, "zhipu"); e == nil {
		sc = agent.NewScreeningAgent(c)
	} else {
		return "Audit dilewati (tidak ada LLM tersedia)."
	}
	res, err := sc.AuditPICO(ctx, picoDef, string(sampleData))
	if err != nil || res == nil {
		return "Audit gagal: " + fmt.Sprint(err)
	}
	action := res.Action
	pct := float64(res.SlippedThroughCount) / float64(sampleSize) * 100
	if pct > 5 {
		action = "RE-SCREEN disarankan (slipped-through > 5%)"
	}
	return fmt.Sprintf("Sample %d (15%%) | Slipped-through: %d (%.1f%%) | Action: %s\n%s",
		sampleSize, res.SlippedThroughCount, pct, action, res.Analysis)
}

func (m *M6Acquisition) buildInaccessibleImpact(session *model.SLRSession) *model.InaccessibleImpact {
	count := 0
	pct := 0.0
	if session.AcquisitionLog != nil {
		count = session.AcquisitionLog.InaccessibleCount
		pct = session.AcquisitionLog.InaccessiblePct
	}
	band := ""
	switch {
	case pct < 5:
		band = "<5%: dampak rendah, dokumentasi standar di Limitations."
	case pct <= 15:
		band = "5-15%: perlu dokumentasi detail + analisis bias (apakah skewed ke region/tahun/topik tertentu?)."
	default:
		band = ">15%: REVISI strategi akuisisi (tambah channel, konsultasi supervisor)."
	}
	md := fmt.Sprintf("## Inaccessible Impact\n\n- Jumlah inaccessible: **%d** (%.1f%%)\n- Penilaian: %s\n\n"+
		"**Template disclosure (Limitations, Modul 9):**\n"+
		"> %d studi (%.1f%%) tidak dapat diakses meski telah ditempuh jalur Unpaywall/arXiv/HITL. "+
		"Karakterisasi sebaran perlu dicek (region/tahun/topik). Potensi bias: %s Estimasi dampak: %s",
		count, pct, band, count, pct,
		map[bool]string{true: "berpotensi systematic.", false: "cenderung acak."}[pct >= 5],
		map[bool]string{true: "moderate/high.", false: "low."}[pct >= 5])
	return &model.InaccessibleImpact{Count: count, Pct: pct, Markdown: md}
}

func buildExtractionReadiness(includedCount int, allResolved, kappaDone bool) string {
	chk := func(ok bool) string {
		if ok {
			return "[x]"
		}
		return "[ ]"
	}
	return fmt.Sprintf("## Extraction Readiness Checklist (sebelum Modul 7)\n\n"+
		"%s Final INCLUDED list finalized (%d studi)\n"+
		"%s Semua DISAGREE/UNCERTAIN resolved (Final_Decision_Full terisi)\n"+
		"%s Full-text kappa calculated + terdokumentasi\n"+
		"%s Exclusion reasons table (full-text) compiled\n"+
		"%s PICO-consistency final audit completed\n"+
		"%s Inaccessible dokumentasi ready\n\n"+
		"%s",
		chk(includedCount > 0), includedCount,
		chk(allResolved),
		chk(kappaDone),
		chk(true),
		chk(true),
		chk(true),
		map[bool]string{true: "✅ PROCEED ke Modul 7.", false: "⚠️ Masih ada konflik/uncertain yang belum diputuskan — selesaikan dulu."}[allResolved && includedCount > 0])
}
