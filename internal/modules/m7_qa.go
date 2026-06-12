package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"nsa/internal/agent"
	"nsa/internal/logger"
	"nsa/internal/model"
)

const qaBatchSize = 6
const qaCalibrationPilotSize = 5
const qaCalibrationKappaThreshold = 0.6
const qaCalibrationMaxAttempts = 3

// knownToolCutoffs provides literature-grounded default thresholds and references
// for common critical appraisal tools.
var knownToolCutoffs = map[string]struct {
	DefaultThreshold float64
	References       []string
}{
	"MMAT": {
		DefaultThreshold: 70,
		References: []string{
			"Hong QN et al. (2018). Mixed Methods Appraisal Tool (MMAT) v2018 User Guide.",
			"Pace R et al. (2012). Testing the reliability and efficiency of the pilot MMAT. Int J Nurs Stud, 49(1):47-53.",
		},
	},
	"NOS": {
		DefaultThreshold: 70,
		References: []string{
			"Wells GA et al. The Newcastle-Ottawa Scale (NOS) for assessing the quality of nonrandomised studies.",
			"Lo CK et al. (2014). Newcastle-Ottawa Scale: comparing reviewers' to authors' assessments. BMC Med Res Methodol, 14:45.",
		},
	},
	"COCHRANE_ROB2": {
		DefaultThreshold: 75,
		References: []string{
			"Sterne JAC et al. (2019). RoB 2: a revised tool for assessing risk of bias in randomised trials. BMJ, 366:l4898.",
			"Higgins JPT et al. (2011). The Cochrane Collaboration's tool for assessing risk of bias. BMJ, 343:d5928.",
		},
	},
	"CASP": {
		DefaultThreshold: 70,
		References: []string{
			"Critical Appraisal Skills Programme (2018). CASP Qualitative Checklist.",
			"Long HA et al. (2020). Optimising the value of the CASP. BMC Med Res Methodol, 20:36.",
		},
	},
	"JBI": {
		DefaultThreshold: 70,
		References: []string{
			"Aromataris E, Munn Z (Eds). (2020). JBI Manual for Evidence Synthesis. JBI.",
			"Munn Z et al. (2015). Methodological quality of case series studies. JBI Database System Rev Implement Rep, 13(1):118-33.",
		},
	},
	"ROBINS_I": {
		DefaultThreshold: 70,
		References: []string{
			"Sterne JA et al. (2016). ROBINS-I: a tool for assessing risk of bias in non-randomised studies. BMJ, 355:i4919.",
		},
	},
	"CLAIM": {
		DefaultThreshold: 65,
		References: []string{
			"Mongan J et al. (2020). Checklist for AI in Medical Imaging (CLAIM). Radiology: AI, 2(2):e200029.",
		},
	},
	"TRIPOD_AI": {
		DefaultThreshold: 65,
		References: []string{
			"Collins GS et al. (2024). TRIPOD+AI statement: updated guidance for reporting clinical prediction models. BMJ, 385:e078378.",
			"Collins GS et al. (2015). Transparent Reporting of a multivariable prediction model. Ann Intern Med, 162(1):55-63.",
		},
	},
	"PROBAST": {
		DefaultThreshold: 65,
		References: []string{
			"Wolff RF et al. (2019). PROBAST: A Tool to Assess the Risk of Bias and Applicability. Ann Intern Med, 170(1):51-58.",
		},
	},
}

// normalizeToolKey normalizes a tool name for lookup by uppercasing and replacing
// spaces, hyphens, and other separators with underscores.
func normalizeToolKey(name string) string {
	name = strings.ToUpper(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return name
}

// lookupToolCutoff finds a matching entry in knownToolCutoffs using normalized key matching.
func lookupToolCutoff(toolName string) (struct {
	DefaultThreshold float64
	References       []string
}, bool) {
	normalized := normalizeToolKey(toolName)
	if ref, ok := knownToolCutoffs[normalized]; ok {
		return ref, true
	}
	// Try partial match: check if any known key is contained in the normalized name or vice versa.
	for key, ref := range knownToolCutoffs {
		if strings.Contains(normalized, key) || strings.Contains(key, normalized) {
			return ref, true
		}
	}
	return struct {
		DefaultThreshold float64
		References       []string
	}{}, false
}

// abs64 returns the absolute value of a float64.
func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// ===== QA Calibration: anchor examples + pilot batch + kappa check =====

func (m *M7Extraction) runQACalibration(ctx context.Context, session *model.SLRSession) error {
	coll := m.deps.MongoRepo.GetExtractionCollection()
	tool := session.QAThreshold.Tool
	cat := session.QAThreshold.Categorization
	justification := session.QAThreshold.ToolJustification
	thr := session.QAThreshold.Threshold

	// Initialize calibration state if needed.
	if session.QACalibration == nil {
		session.QACalibration = &model.QACalibration{
			MaxAttempts: qaCalibrationMaxAttempts,
			Attempts:    0,
		}
	}
	cal := session.QACalibration
	cal.Attempts++

	// Phase 1: Generate anchor examples (only on first attempt or if no anchors yet).
	if len(cal.Anchors) == 0 {
		logger.Log(session.ID, "   [Kalibrasi QA] Menghasilkan anchor examples (HIGH/MODERATE/LOW)...")
		brain, err := m.deps.LLMFactory.BrainClient(ctx)
		if err != nil {
			return fmt.Errorf("brain client for QA anchors: %w", err)
		}
		anchors, err := agent.NewExtractionAgent(brain).GenerateQAAnchors(ctx, tool, cat, justification)
		if err != nil {
			return fmt.Errorf("GenerateQAAnchors: %w", err)
		}
		cal.Anchors = anchors
		logger.Logf(session.ID, "   [System] %d anchor examples dihasilkan.\n", len(anchors))
	}

	// Phase 2: Pilot batch - pick up to qaCalibrationPilotSize papers with fulltext.
	logger.Log(session.ID, "   [Kalibrasi QA] Menjalankan pilot batch rating...")

	ftIndex, _, _ := BuildFulltextIndex(ctx)
	if ftIndex == nil {
		ftIndex = map[string]string{}
	}

	// Find papers that have fulltext available for pilot.
	cur, err := coll.Find(ctx, bson.M{"session_id": session.ID}, options.Find().SetLimit(50))
	if err != nil {
		return fmt.Errorf("find papers for pilot: %w", err)
	}
	var allPapers []bson.M
	_ = cur.All(ctx, &allPapers)

	var pilotPapers []bson.M
	for _, p := range allPapers {
		if len(pilotPapers) >= qaCalibrationPilotSize {
			break
		}
		title := getStr(p, "Title")
		doi := getStr(p, "DOI", "doi")
		var ft string
		if nd := normalizeDOIForRAG(doi); nd != "" && ftIndex[nd] != "" {
			ft = ftIndex[nd]
		} else if nt := NormTitle(title); nt != "" && ftIndex["title:"+nt] != "" {
			ft = ftIndex["title:"+nt]
		}
		if ft != "" {
			pilotPapers = append(pilotPapers, p)
		}
	}

	if len(pilotPapers) == 0 {
		// No fulltext available for pilot, pass calibration automatically.
		logger.Log(session.ID, "   [Kalibrasi QA] Tidak ada paper dengan fulltext untuk pilot. Kalibrasi dilewati.")
		cal.CalibrationPassed = true
		cal.PilotKappa = 1.0
		session.Status = "M7_STEP3_QA_CALIBRATION_WAITING_APPROVAL"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	// Set up dual raters.
	qp1, qf1 := m.deps.LLMFactory.RoleProviders(ctx, "reviewer1")
	r1, err := m.agentWithFallback(ctx, qp1, qf1)
	if err != nil {
		return fmt.Errorf("QA Calibration Rater 1 (%s/%s): %w", qp1, qf1, err)
	}
	qp2, qf2 := m.deps.LLMFactory.RoleProviders(ctx, "reviewer2")
	r2, err := m.agentWithFallback(ctx, qp2, qf2)
	if err != nil {
		return fmt.Errorf("QA Calibration Rater 2 (%s/%s): %w", qp2, qf2, err)
	}

	// Get model names for transparency.
	var r1Model, r2Model string
	cfg1, _ := m.deps.MongoRepo.GetLLMConfig(ctx, qp1)
	if cfg1 != nil {
		r1Model = cfg1.ProviderName
		if cfg1.DefaultModel != "" {
			r1Model += " (" + cfg1.DefaultModel + ")"
		}
	} else {
		r1Model = qp1
	}
	cfg2, _ := m.deps.MongoRepo.GetLLMConfig(ctx, qp2)
	if cfg2 != nil {
		r2Model = cfg2.ProviderName
		if cfg2.DefaultModel != "" {
			r2Model += " (" + cfg2.DefaultModel + ")"
		}
	} else {
		r2Model = qp2
	}
	cal.R1Model = r1Model
	cal.R2Model = r2Model

	// Get brain model name for transparency.
	brainPrimary, _ := m.deps.LLMFactory.RoleProviders(ctx, "brain")
	cfgBrain, _ := m.deps.MongoRepo.GetLLMConfig(ctx, brainPrimary)
	if cfgBrain != nil {
		cal.BrainModel = cfgBrain.ProviderName
		if cfgBrain.DefaultModel != "" {
			cal.BrainModel += " (" + cfgBrain.DefaultModel + ")"
		}
	} else {
		cal.BrainModel = brainPrimary
	}

	// Build anchor context string to include in appraisal prompt.
	anchorCtx := formatAnchorContext(cal.Anchors)

	// Rate pilot papers.
	var pilotResults []model.QACalibrationPilot
	var systemPromptCaptured bool
	for i, p := range pilotPapers {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		title := getStr(p, "Title")
		doi := getStr(p, "DOI", "doi")
		var ft string
		if nd := normalizeDOIForRAG(doi); nd != "" && ftIndex[nd] != "" {
			ft = ftIndex[nd]
		} else if nt := NormTitle(title); nt != "" && ftIndex["title:"+nt] != "" {
			ft = ftIndex["title:"+nt]
		}

		logger.Logf(session.ID, "      -> Pilot QA [%d/%d] %s\n", i+1, len(pilotPapers), doi)

		// Include anchor examples in justification context for calibration.
		calibJustification := justification + "\n\n" + anchorCtx
		if cal.RefinementNote != "" {
			calibJustification += "\n\n[RUBRIC REFINEMENT]: " + cal.RefinementNote
		}

		// Store the system prompt once (representative for all papers in this pilot batch).
		if !systemPromptCaptured {
			cal.SystemPrompt = calibJustification
			systemPromptCaptured = true
		}

		s1, e1 := r1.AppraiseQuality(ctx, tool, cat, calibJustification, title, ft)
		time.Sleep(3 * time.Second)
		s2, e2 := r2.AppraiseQuality(ctx, tool, cat, calibJustification, title, ft)

		pilot := model.QACalibrationPilot{
			PaperID: doi,
			Title:   title,
		}
		if e1 == nil && s1 != nil {
			pilot.R1Score = s1.TotalScore
			pilot.R1Category = s1.Category
		}
		if e2 == nil && s2 != nil {
			pilot.R2Score = s2.TotalScore
			pilot.R2Category = s2.Category
		}
		if s1 != nil && s2 != nil {
			avg := (s1.TotalScore + s2.TotalScore) / 2
			pilot.FinalCategory = categoryFor(avg, thr, cat)
			pilot.Disagreement = s1.Category != s2.Category
		} else {
			pilot.FinalCategory = "ERROR"
			pilot.Disagreement = true
		}
		pilotResults = append(pilotResults, pilot)

		// Mark paper as calibration pilot in MongoDB.
		_, _ = coll.UpdateByID(ctx, p["_id"], bson.M{"$set": bson.M{"qa_calibration_pilot": true}})
		time.Sleep(5 * time.Second)
	}

	cal.PilotResults = pilotResults

	// Phase 3: Compute pilot kappa.
	pilotKappa := computePilotKappa(pilotResults)
	cal.PilotKappa = pilotKappa

	logger.Logf(session.ID, "   [Kalibrasi QA] Pilot kappa: %.3f (threshold: %.2f, attempt %d/%d)\n",
		pilotKappa, qaCalibrationKappaThreshold, cal.Attempts, cal.MaxAttempts)

	if pilotKappa >= qaCalibrationKappaThreshold {
		cal.CalibrationPassed = true
		session.Status = "M7_STEP3_QA_CALIBRATION_WAITING_APPROVAL"
		logger.Log(session.ID, "   [System] Kalibrasi QA LULUS. Menunggu persetujuan untuk lanjut full rating.")
	} else {
		cal.CalibrationPassed = false
		if cal.Attempts >= cal.MaxAttempts {
			// Max retries reached, proceed with warning.
			cal.CalibrationPassed = true
			cal.RefinementNote += " [WARNING: Kalibrasi tidak tercapai setelah " +
				fmt.Sprintf("%d", cal.MaxAttempts) + " percobaan. Melanjutkan dengan peringatan.]"
			session.Status = "M7_STEP3_QA_CALIBRATION_WAITING_APPROVAL"
			logger.Log(session.ID, "   [System] Kalibrasi QA tidak tercapai setelah max attempts. Melanjutkan dengan peringatan.")
		} else {
			// Ask brain for rubric refinement suggestion.
			brain, err := m.deps.LLMFactory.BrainClient(ctx)
			if err == nil {
				note, rerr := agent.NewExtractionAgent(brain).SuggestRubricRefinement(ctx, tool, cat, pilotResults)
				if rerr == nil && note != "" {
					cal.RefinementNote = note
				}
			}
			// Generate clear action items so user knows what will happen on retry.
			if cal.RefinementNote != "" {
				cal.ActionItems = fmt.Sprintf(
					"Jika Anda klik 'Retry Kalibrasi':\n"+
						"1. Sistem akan mereset pilot papers dan mengambil %d sample baru\n"+
						"2. Rubrik refinement dari Brain akan ditambahkan ke prompt rater:\n   \"%s\"\n"+
						"3. Kedua rater (R1: %s, R2: %s) akan menilai ulang dengan panduan yang lebih ketat\n"+
						"4. Kappa dihitung ulang - target >= %.1f\n\n"+
						"Jika Anda klik 'Lanjutkan (Force Proceed)':\n"+
						"1. Sistem akan melanjutkan full rating TANPA kalibrasi ulang\n"+
						"2. Kappa rendah akan dicatat sebagai limitasi di manuskrip\n"+
						"3. Hasil QA tetap dihasilkan tapi dengan peringatan inter-rater agreement rendah",
					qaCalibrationPilotSize, cal.RefinementNote, cal.R1Model, cal.R2Model, qaCalibrationKappaThreshold)
			}
			session.Status = "M7_STEP3_QA_CALIBRATION_LOW_KAPPA"
			logger.Log(session.ID, "   [System] Kalibrasi QA kappa rendah. Menunggu keputusan user (retry/proceed).")
		}
	}

	session.QACalibration = cal
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// computePilotKappa calculates Cohen's kappa from pilot calibration results.
// Uses pass/fail binary (HIGH/MODERATE = pass, LOW = fail).
func computePilotKappa(pilots []model.QACalibrationPilot) float64 {
	var total, bothPass, bothFail, r1PassR2Fail, r1FailR2Pass int
	pass := func(cat string) bool { return cat == "HIGH" || cat == "MODERATE" }
	for _, p := range pilots {
		if p.R1Category == "" || p.R2Category == "" {
			continue
		}
		total++
		switch {
		case pass(p.R1Category) && pass(p.R2Category):
			bothPass++
		case !pass(p.R1Category) && !pass(p.R2Category):
			bothFail++
		case pass(p.R1Category) && !pass(p.R2Category):
			r1PassR2Fail++
		default:
			r1FailR2Pass++
		}
	}
	return cohensKappa(total, bothPass, bothFail, r1PassR2Fail, r1FailR2Pass)
}

// formatAnchorContext formats anchor examples into a string for inclusion in prompts.
func formatAnchorContext(anchors []model.QAAnchorExample) string {
	if len(anchors) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[ANCHOR EXAMPLES FOR CALIBRATION]:\n")
	for _, a := range anchors {
		sb.WriteString(fmt.Sprintf("- %s (score ~%.0f): %s | Reasoning: %s\n",
			a.Category, a.Score, a.Description, a.Reasoning))
	}
	return sb.String()
}

// ===== L3: Quality appraisal (tool + threshold + dual-rater kappa + sensitivity) =====

func (m *M7Extraction) runQAL3(ctx context.Context, session *model.SLRSession) error {
	coll := m.deps.MongoRepo.GetExtractionCollection()

	// Fase 1+2: tool selection + threshold 3-lapis (sekali).
	if session.QAThreshold == nil {
		logger.Log(session.ID, "   [Langkah 7.3] Tool selection + threshold justification...")
		brain, err := m.deps.LLMFactory.BrainClient(ctx)
		if err != nil {
			return fmt.Errorf("gemini (brain QA) gagal: %w", err)
		}
		designBreakdown := m.designBreakdownFromExtraction(ctx, session)
		qt, err := agent.NewExtractionAgent(brain).SelectQATool(ctx, designBreakdown, session.Feedback)
		if err != nil {
			return err
		}
		session.QAThreshold = qt
		session.Feedback = "" // Bersihkan feedback setelah dipakai

		// Generate operational scoring rubric for the selected tool.
		rubric, rubricErr := agent.NewExtractionAgent(brain).GenerateQARubric(ctx, qt.Tool, qt.Categorization, qt.ToolJustification)
		if rubricErr != nil {
			logger.Logf(session.ID, "   [Warning] GenerateQARubric gagal: %v\n", rubricErr)
		} else {
			session.QAThreshold.QARubric = rubric
		}

		// Point 4: Enrich with literature grounding from known tool cutoffs.
		if ref, ok := lookupToolCutoff(qt.Tool); ok {
			qt.LiteratureReferences = ref.References
			// Flag if threshold deviates significantly from known default.
			if abs64(qt.Threshold-ref.DefaultThreshold) > 15 {
				qt.ThresholdDeviationNote = fmt.Sprintf("Threshold %.0f%% menyimpang >15 poin dari default umum tool (%s: %.0f%%). Pastikan justifikasi 3-lapis mendukung deviasi ini.",
					qt.Threshold, qt.Tool, ref.DefaultThreshold)
			}
		}

		logger.Logf(session.ID, "   [System] QA tool: %s, threshold %.0f%%.\n", qt.Tool, qt.Threshold)
		session.Status = "M7_STEP3_QA_TOOL_WAITING_APPROVAL"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	// Fase 3: dual-rater QA per paper (batch).
	cur, err := coll.Find(ctx, bson.M{"session_id": session.ID, "qa_rated": bson.M{"$ne": true}},
		options.Find().SetLimit(int64(qaBatchSize)))
	if err != nil {
		return err
	}
	var batch []bson.M
	_ = cur.All(ctx, &batch)

	if len(batch) > 0 {
		ftIndex, _, _ := BuildFulltextIndex(ctx)
		if ftIndex == nil {
			ftIndex = map[string]string{}
		}
		qp1, qf1 := m.deps.LLMFactory.RoleProviders(ctx, "reviewer1")
		r1, err := m.agentWithFallback(ctx, qp1, qf1)
		if err != nil {
			return fmt.Errorf("QA Rater 1 (%s/%s) gagal: %w", qp1, qf1, err)
		}
		qp2, qf2 := m.deps.LLMFactory.RoleProviders(ctx, "reviewer2")
		r2, err := m.agentWithFallback(ctx, qp2, qf2)
		if err != nil {
			return fmt.Errorf("QA Rater 2 (%s/%s) gagal: %w", qp2, qf2, err)
		}

		var r1Model, r2Model string
		cfg1, _ := m.deps.MongoRepo.GetLLMConfig(ctx, qp1)
		if cfg1 != nil {
			r1Model = cfg1.ProviderName
			if cfg1.DefaultModel != "" {
				r1Model += " (" + cfg1.DefaultModel + ")"
			}
		} else {
			r1Model = qp1
		}

		cfg2, _ := m.deps.MongoRepo.GetLLMConfig(ctx, qp2)
		if cfg2 != nil {
			r2Model = cfg2.ProviderName
			if cfg2.DefaultModel != "" {
				r2Model += " (" + cfg2.DefaultModel + ")"
			}
		} else {
			r2Model = qp2
		}
		tool := session.QAThreshold.Tool
		cat := session.QAThreshold.Categorization
		thr := session.QAThreshold.Threshold

		for i, p := range batch {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			title := getStr(p, "Title")
			doi := getStr(p, "DOI", "doi")
			
			var ft string
			if nd := normalizeDOIForRAG(doi); nd != "" && ftIndex[nd] != "" {
				ft = ftIndex[nd]
			} else if nt := NormTitle(title); nt != "" && ftIndex["title:"+nt] != "" {
				ft = ftIndex["title:"+nt]
			}
			
			logger.Logf(session.ID, "      -> QA [%d/%d] %s\n", i+1, len(batch), getStr(p, "DOI", "doi"))
			upd := bson.M{"qa_rated": true}
			if ft == "" {
				upd["qa_final_category"] = "UNRATED"
				upd["qa_total_score"] = 0
			} else {
				justification := session.QAThreshold.ToolJustification

				// Add scoring rubric
				if session.QAThreshold.QARubric != "" {
					justification += "\n\n[RUBRIK OPERASIONAL PENILAIAN]:\n" + session.QAThreshold.QARubric
				}

				// Add anchor examples from calibration
				if session.QACalibration != nil && len(session.QACalibration.Anchors) > 0 {
					justification += "\n\n" + formatAnchorContext(session.QACalibration.Anchors)
				}

				s1, e1 := r1.AppraiseQuality(ctx, tool, cat, justification, title, ft)
				time.Sleep(3 * time.Second)
				s2, e2 := r2.AppraiseQuality(ctx, tool, cat, justification, title, ft)
				
				isFatal := false
				var fatalMsg string
				if e1 != nil && strings.Contains(e1.Error(), "provider merespons dengan error") {
					isFatal = true
					fatalMsg = e1.Error()
				} else if e2 != nil && strings.Contains(e2.Error(), "provider merespons dengan error") {
					isFatal = true
					fatalMsg = e2.Error()
				}

				if isFatal {
					session.Status = "M7_STEP3_NEEDS_REVISION"
					session.SystemError = fatalMsg
					logger.Logf(session.ID, "      [FATAL] %s\n", fatalMsg)
					return m.deps.MongoRepo.UpdateSession(ctx, session)
				}

				if e1 != nil || e2 != nil || s1 == nil || s2 == nil {
					// Save any successful rater's data even if the other failed.
					if s1 != nil {
						upd["qa_r1_score"] = s1.TotalScore
						upd["qa_r1_category"] = s1.Category
						upd["qa_r1_reasoning"] = s1.Reasoning
						upd["qa_r1_evidence"] = s1.Evidence
						upd["qa_r1_model"] = r1Model
					}
					if s2 != nil {
						upd["qa_r2_score"] = s2.TotalScore
						upd["qa_r2_category"] = s2.Category
						upd["qa_r2_reasoning"] = s2.Reasoning
						upd["qa_r2_evidence"] = s2.Evidence
						upd["qa_r2_model"] = r2Model
					}

					// Try to compute final from available data (current run + existing doc).
					var r1Score, r2Score float64
					var hasR1, hasR2 bool
					if s1 != nil {
						r1Score = s1.TotalScore
						hasR1 = true
					} else if v, ok := toFloat(p["qa_r1_score"]); ok && v > 0 {
						r1Score = v
						hasR1 = true
					}
					if s2 != nil {
						r2Score = s2.TotalScore
						hasR2 = true
					} else if v, ok := toFloat(p["qa_r2_score"]); ok && v > 0 {
						r2Score = v
						hasR2 = true
					}

					if hasR1 && hasR2 {
						avg := (r1Score + r2Score) / 2
						upd["qa_total_score"] = avg
						upd["qa_final_category"] = categoryFor(avg, thr, cat)
					} else {
						upd["qa_final_category"] = "ERROR"
						upd["qa_total_score"] = 0
					}
				} else {
					avg := (s1.TotalScore + s2.TotalScore) / 2
					upd["qa_r1_score"] = s1.TotalScore
					upd["qa_r1_category"] = s1.Category
					upd["qa_r1_reasoning"] = s1.Reasoning
					upd["qa_r1_evidence"] = s1.Evidence
					upd["qa_r1_model"] = r1Model
					upd["qa_r2_score"] = s2.TotalScore
					upd["qa_r2_category"] = s2.Category
					upd["qa_r2_reasoning"] = s2.Reasoning
					upd["qa_r2_evidence"] = s2.Evidence
					upd["qa_r2_model"] = r2Model
					upd["qa_total_score"] = avg
					upd["qa_final_category"] = categoryFor(avg, thr, cat)
				}
				time.Sleep(5 * time.Second)
			}
			_, _ = coll.UpdateByID(ctx, p["_id"], bson.M{"$set": upd})
		}
		session.Status = "M7_STEP3_QA" // loop
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	// Fase 3b + 4: kappa + sensitivity (semua sudah dirating).

	// Recalculation pass: fix papers stuck in ERROR that have valid R1 AND R2 data.
	tool := session.QAThreshold.Tool
	cat := session.QAThreshold.Categorization
	thr := session.QAThreshold.Threshold
	_ = tool // used for context only; categoryFor uses thr and cat

	errCur, errFindErr := coll.Find(ctx, bson.M{
		"session_id":        session.ID,
		"qa_final_category": "ERROR",
		"qa_r1_score":       bson.M{"$gt": 0},
		"qa_r2_score":       bson.M{"$gt": 0},
	})
	if errFindErr == nil {
		var errDocs []bson.M
		_ = errCur.All(ctx, &errDocs)
		for _, ep := range errDocs {
			r1sc, r1ok := toFloat(ep["qa_r1_score"])
			r2sc, r2ok := toFloat(ep["qa_r2_score"])
			if r1ok && r2ok && r1sc > 0 && r2sc > 0 {
				avg := (r1sc + r2sc) / 2
				fixUpd := bson.M{
					"qa_total_score":    avg,
					"qa_final_category": categoryFor(avg, thr, cat),
				}
				_, _ = coll.UpdateByID(ctx, ep["_id"], bson.M{"$set": fixUpd})
				logger.Logf(session.ID, "      [Recalc] Fixed ERROR paper %s: avg=%.1f -> %s\n",
					getStr(ep, "DOI", "doi"), avg, categoryFor(avg, thr, cat))
			}
		}
	}

	logger.Log(session.ID, "   [Langkah 7.3] Hitung kappa QA + sensitivity analysis...")
	rated := m.allRated(ctx, session)
	kappa, details := qaKappa(rated)
	session.QAThreshold.Kappa = kappa
	session.QAThreshold.KappaDetails = details

	// Point 1: Calculate actual feasibility from rated data.
	threshold := session.QAThreshold.Threshold
	totalRateable := 0
	actualPass := 0
	for _, p := range rated {
		cat := getStr(p, "qa_final_category")
		if cat == "UNRATED" || cat == "ERROR" || cat == "" {
			continue
		}
		totalRateable++
		if sc, ok := toFloat(p["qa_total_score"]); ok && sc >= threshold {
			actualPass++
		}
	}
	if totalRateable > 0 {
		session.QAThreshold.ActualFeasibility = float64(actualPass) / float64(totalRateable) * 100
		session.QAThreshold.ActualFeasibilityNote = fmt.Sprintf("Dari %d studi yang berhasil dinilai, %d (%.1f%%) memenuhi threshold %.0f%%.",
			totalRateable, actualPass, session.QAThreshold.ActualFeasibility, threshold)
	}

	// Point 2: Pass categorization to buildSensitivity for band-based scenarios.
	session.SensitivityAnalysis = buildSensitivity(rated, threshold, session.QAThreshold.Categorization)

	logger.Logf(session.ID, "   [System] QA kappa %.3f; sensitivity verdict %s.\n", kappa, session.SensitivityAnalysis.Verdict)
	session.Status = "M7_STEP3_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

func (m *M7Extraction) allRated(ctx context.Context, session *model.SLRSession) []bson.M {
	cur, _ := m.deps.MongoRepo.GetExtractionCollection().Find(ctx, bson.M{"session_id": session.ID})
	var all []bson.M
	_ = cur.All(ctx, &all)
	return all
}

func categoryFor(score, threshold float64, categorization ...string) string {
	if len(categorization) > 0 && categorization[0] != "" {
		if bands := parseCategorization(categorization[0]); bands != nil {
			if score >= bands.highMin {
				return "HIGH"
			}
			if score >= bands.moderateMin {
				return "MODERATE"
			}
			return "LOW"
		}
	}
	// Fallback: hardcoded +10 band
	if score >= threshold+10 {
		return "HIGH"
	}
	if score >= threshold {
		return "MODERATE"
	}
	return "LOW"
}

// categorizationBands holds parsed numeric boundaries from categorization string.
type categorizationBands struct {
	highMin     float64 // score >= highMin => HIGH
	moderateMin float64 // score >= moderateMin => MODERATE
}

// parseCategorization parses dynamic categorization strings generated by the brain LLM.
// Supported formats:
//   - "HIGH >=80% | MODERATE 70-79% | LOW <70%"
//   - "HIGH>=80 MODERATE 70-79 LOW<70"
//   - "HIGH >=85% | MODERATE 60-84% | LOW <60%"
//
// Returns nil if parsing fails.
func parseCategorization(s string) *categorizationBands {
	if s == "" {
		return nil
	}

	// Normalize: remove %, trim spaces
	s = strings.ReplaceAll(s, "%", "")

	var highMin, moderateMin float64
	var foundHigh, foundModerate bool

	// Strategy 1: look for "HIGH >=X" or "HIGH>=X" pattern
	reHigh := regexp.MustCompile(`(?i)HIGH\s*>=?\s*(\d+(?:\.\d+)?)`)
	if m := reHigh.FindStringSubmatch(s); len(m) > 1 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			highMin = v
			foundHigh = true
		}
	}

	// Strategy 2: look for "MODERATE X-Y" or "MODERATE >=X" pattern
	reModRange := regexp.MustCompile(`(?i)MODERATE\s+(\d+(?:\.\d+)?)\s*-\s*(\d+(?:\.\d+)?)`)
	reModGte := regexp.MustCompile(`(?i)MODERATE\s*>=?\s*(\d+(?:\.\d+)?)`)
	if m := reModRange.FindStringSubmatch(s); len(m) > 1 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			moderateMin = v
			foundModerate = true
		}
	} else if m := reModGte.FindStringSubmatch(s); len(m) > 1 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			moderateMin = v
			foundModerate = true
		}
	}

	// If MODERATE not found, try extracting from LOW <X (moderateMin = X)
	if !foundModerate {
		reLow := regexp.MustCompile(`(?i)LOW\s*<\s*(\d+(?:\.\d+)?)`)
		if m := reLow.FindStringSubmatch(s); len(m) > 1 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				moderateMin = v
				foundModerate = true
			}
		}
	}

	// Need at least HIGH boundary to be useful; derive MODERATE from HIGH if needed
	if !foundHigh && !foundModerate {
		return nil
	}
	if foundHigh && !foundModerate {
		// Cannot determine moderate boundary, fall back
		return nil
	}
	if !foundHigh && foundModerate {
		// Cannot determine high boundary, fall back
		return nil
	}

	// Sanity check: highMin should be > moderateMin
	if highMin <= moderateMin {
		return nil
	}

	return &categorizationBands{
		highMin:     highMin,
		moderateMin: moderateMin,
	}
}

// qaKappa: Cohen's kappa 2-kelas (pass=HIGH/MODERATE, fail=LOW) atas keputusan R1 vs R2.
func qaKappa(docs []bson.M) (float64, *model.QAKappaDetails) {
	var total, bothPass, bothFail, r1PassR2Fail, r1FailR2Pass int
	pass := func(cat string) bool { return cat == "HIGH" || cat == "MODERATE" }
	for _, p := range docs {
		c1 := getStr(p, "qa_r1_category")
		c2 := getStr(p, "qa_r2_category")
		if c1 == "" || c2 == "" {
			continue
		}
		total++
		switch {
		case pass(c1) && pass(c2):
			bothPass++
		case !pass(c1) && !pass(c2):
			bothFail++
		case pass(c1) && !pass(c2):
			r1PassR2Fail++
		default:
			r1FailR2Pass++
		}
	}
	details := &model.QAKappaDetails{
		TotalRated:   total,
		BothPass:     bothPass,
		BothFail:     bothFail,
		R1PassR2Fail: r1PassR2Fail,
		R1FailR2Pass: r1FailR2Pass,
	}
	return cohensKappa(total, bothPass, bothFail, r1PassR2Fail, r1FailR2Pass), details
}

func buildSensitivity(docs []bson.M, threshold float64, categorization string) *model.SensitivityAnalysis {
	countAtLeast := func(t float64) int {
		n := 0
		for _, p := range docs {
			if sc, ok := toFloat(p["qa_total_score"]); ok && sc >= t {
				n++
			}
		}
		return n
	}

	// Count total rateable (exclude UNRATED/ERROR).
	totalRateable := 0
	for _, p := range docs {
		cat := getStr(p, "qa_final_category")
		if cat != "UNRATED" && cat != "ERROR" && cat != "" {
			totalRateable++
		}
	}

	// Point 2: Determine strict/loose thresholds from categorization bands.
	var strictThreshold, looseThreshold float64
	bands := parseCategorization(categorization)
	if bands != nil {
		strictThreshold = bands.highMin                                    // only HIGH passes
		looseThreshold = bands.moderateMin - (bands.highMin - bands.moderateMin) // symmetrical loosening
		if looseThreshold < 0 {
			looseThreshold = 0
		}
	} else {
		// Fallback to fixed +/-10.
		strictThreshold = threshold + 10
		looseThreshold = threshold - 10
		if looseThreshold < 0 {
			looseThreshold = 0
		}
	}

	base := countAtLeast(threshold)
	strict := countAtLeast(strictThreshold)
	loose := countAtLeast(looseThreshold)

	verdict := "ROBUST"
	if abs(strict-base) > 1 || abs(loose-base) > 1 {
		verdict = "CONDITIONALLY ROBUST"
	}
	if (base > 0 && abs(strict-base)*100/maxInt(base, 1) > 30) || abs(loose-base) > maxInt(base/2, 2) {
		verdict = "SENSITIVE"
	}

	// Point 3: Dynamic reasoning based on actual numbers.
	pctChangeStrict := 0.0
	pctChangeLoose := 0.0
	if totalRateable > 0 {
		pctChangeStrict = float64(abs(base-strict)) / float64(totalRateable) * 100
		pctChangeLoose = float64(abs(loose-base)) / float64(totalRateable) * 100
	}

	var reasoning string
	switch verdict {
	case "ROBUST":
		reasoning = fmt.Sprintf("Dari %d studi, pergeseran threshold dari %.0f%% (baseline) ke %.0f%% (ketat) hanya mengeksklusi %d studi (%.1f%%), dan pelonggaran ke %.0f%% hanya menambah %d studi (%.1f%%). Variasi ini tidak material terhadap kesimpulan.",
			totalRateable, threshold, strictThreshold, base-strict, pctChangeStrict, looseThreshold, loose-base, pctChangeLoose)
	case "CONDITIONALLY ROBUST":
		reasoning = fmt.Sprintf("Terdapat pergeseran moderat: threshold ketat (%.0f%%) mengeksklusi %d dari %d studi (%.1f%%), sementara pelonggaran (%.0f%%) menambah %d (%.1f%%). Pool masih representatif namun kesimpulan perlu dicatat dengan batasan ini.",
			strictThreshold, base-strict, totalRateable, pctChangeStrict, looseThreshold, loose-base, pctChangeLoose)
	case "SENSITIVE":
		reasoning = fmt.Sprintf("Analisis menunjukkan sensitivitas tinggi: threshold ketat (%.0f%%) mengeksklusi %d dari %d studi (%.1f%%), mengubah pool secara substansial. Ini mengindikasikan bahwa kesimpulan sangat bergantung pada nilai cutoff yang dipilih. Pembaca harus mempertimbangkan implikasi ini.",
			strictThreshold, base-strict, totalRateable, pctChangeStrict)
	}

	// Build scenarios including the 4th "Exclude all LOW" scenario.
	sc := []model.SensitivityScenario{
		{Name: "Baseline", Threshold: fmt.Sprintf("%.0f%%", threshold), NIncluded: base, Findings: "set studi acuan"},
		{Name: "Ketat", Threshold: fmt.Sprintf("%.0f%%", strictThreshold), NIncluded: strict, Findings: fmt.Sprintf("%+d studi vs baseline", strict-base)},
		{Name: "Longgar", Threshold: fmt.Sprintf("%.0f%%", looseThreshold), NIncluded: loose, Findings: fmt.Sprintf("%+d studi vs baseline", loose-base)},
	}

	// 4th scenario: "Exclude all LOW" (only HIGH+MODERATE pass, using moderateMin if available).
	if bands != nil {
		excludeLow := countAtLeast(bands.moderateMin)
		sc = append(sc, model.SensitivityScenario{
			Name:      "Exclude all LOW",
			Threshold: fmt.Sprintf("%.0f%%", bands.moderateMin),
			NIncluded: excludeLow,
			Findings:  fmt.Sprintf("hanya HIGH+MODERATE; %+d studi vs baseline", excludeLow-base),
		})
	}

	md := fmt.Sprintf("## Sensitivity Analysis\n\n| Skenario | Threshold | n included | Catatan |\n|---|---|---|---|\n"+
		"| Baseline | %.0f%% | %d | acuan |\n| Ketat | %.0f%% | %d | %+d |\n| Longgar | %.0f%% | %d | %+d |\n",
		threshold, base, strictThreshold, strict, strict-base, looseThreshold, loose, loose-base)
	if bands != nil {
		excludeLow := countAtLeast(bands.moderateMin)
		md += fmt.Sprintf("| Exclude all LOW | %.0f%% | %d | %+d |\n", bands.moderateMin, excludeLow, excludeLow-base)
	}
	md += fmt.Sprintf("\n**Verdict:** %s\n\n**Penjelasan (xAI):** %s", verdict, reasoning)

	return &model.SensitivityAnalysis{Scenarios: sc, Verdict: verdict, Reasoning: reasoning, Markdown: md}
}

// ===== L4: Synthesis prep + meta-analysis feasibility + summary =====

func (m *M7Extraction) runSynthesisL4(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 7.4] Synthesis preparation + meta-analysis feasibility...")
	docs := m.allRated(ctx, session)

	// Ringkasan untuk LLM.
	designs := tallyExtField(docs, "design")
	geo := tallyExtField(docs, "geographic")
	years := tallyMeta(docs, "Year")
	qaDist := map[string]int{}
	for _, p := range docs {
		qaDist[getStr(p, "qa_final_category")]++
	}
	summary := map[string]interface{}{
		"framework":          frameworkName(session),
		"total_included":     len(docs),
		"design_breakdown":   designs,
		"geographic":         geo,
		"year_distribution":  years,
		"quality_distribution": qaDist,
		"qa_threshold":       session.QAThreshold,
	}
	sumJSON, _ := json.Marshal(summary)

	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return fmt.Errorf("gemini (brain synthesis) gagal: %w", err)
	}
	sp, err := agent.NewExtractionAgent(brain).PrepareSynthesis(ctx, string(sumJSON))
	if err != nil {
		return err
	}

	// xAI transparency: record model used and system prompt.
	brainPrimary, _ := m.deps.LLMFactory.RoleProviders(ctx, "brain")
	cfgBrain, _ := m.deps.MongoRepo.GetLLMConfig(ctx, brainPrimary)
	if cfgBrain != nil {
		sp.ModelUsed = cfgBrain.ProviderName
		if cfgBrain.DefaultModel != "" {
			sp.ModelUsed += " (" + cfgBrain.DefaultModel + ")"
		}
	} else {
		sp.ModelUsed = brainPrimary
	}
	sp.SystemPrompt = agent.SynthesisPrepSystemPrompt

	session.SynthesisPrep = sp

	// modul7_summary
	fwLine := "-"
	if session.FrameworkSelection != nil {
		fwLine = fmt.Sprintf("%s — %s", session.FrameworkSelection.Framework, session.FrameworkSelection.Justification)
	}
	exLog := session.ExtractionLog
	exLine := "tidak tersedia"
	if exLog != nil {
		exLine = fmt.Sprintf("Total %d | verifikasi %d | disagreement %.1f%% | ambiguous %d",
			exLog.TotalExtracted, exLog.VerifiedSample, exLog.DisagreementRate, exLog.AmbiguousCount)
	}
	qa := session.QAThreshold
	qaLine := "tidak tersedia"
	if qa != nil {
		qaLine = fmt.Sprintf("Tool %s | threshold %.0f%% | kappa %.3f | kategori: HIGH %d / MODERATE %d / LOW %d",
			qa.Tool, qa.Threshold, qa.Kappa, qaDist["HIGH"], qaDist["MODERATE"], qaDist["LOW"])
	}
	sens := "tidak tersedia"
	if session.SensitivityAnalysis != nil {
		sens = session.SensitivityAnalysis.Verdict
	}
	md := fmt.Sprintf("=== EXTRACTION + QA SUMMARY (SLR) ===\n\n"+
		"FRAMEWORK: %s\n\n"+
		"EXTRACTION: %s\n\n"+
		"QUALITY ASSESSMENT: %s\n\n"+
		"SENSITIVITY: %s\n\n"+
		"HETEROGENEITY VERDICT: %s\n"+
		"META-ANALYSIS FEASIBILITY: %s\n\n"+
		"DESCRIPTIVE OVERVIEW:\n%s\n\n"+
		"FRAMEWORK-DRIVEN GROUPINGS:\n%s\n\n"+
		"NEXT: Data Analysis + Synthesis (Modul 8)",
		fwLine, exLine, qaLine, sens, sp.HeterogeneityVerdict, sp.MetaFeasibility, sp.DescriptiveOverview, sp.Groupings)
	session.Modul7Summary = &model.Modul7Summary{Markdown: md}

	session.Status = "M7_STEP4_WAITING_APPROVAL"
	logger.Log(session.ID, "   [System] synthesis_prep + modul7_summary tersimpan.")
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

func (m *M7Extraction) designBreakdownFromExtraction(ctx context.Context, session *model.SLRSession) string {
	docs := m.allRated(ctx, session)
	t := tallyExtField(docs, "design")
	if len(t) == 0 {
		return fmt.Sprintf("(design tidak terekstrak; total %d studi)", len(docs))
	}
	s := fmt.Sprintf("Total %d studi. Designs: ", len(docs))
	for k, v := range t {
		s += fmt.Sprintf("%s=%d; ", k, v)
	}
	return s
}

// ===== small helpers =====

func frameworkName(session *model.SLRSession) string {
	if session.FrameworkSelection != nil {
		return session.FrameworkSelection.Framework
	}
	return "CUSTOM"
}

// extFieldAliases maps a primary keySub to additional aliases that should also
// be tried when searching extraction fields. This handles variations in
// framework templates that may use different key names for the same concept.
var extFieldAliases = map[string][]string{
	"design": {
		"study_type", "study type", "methodology", "research_approach",
		"tipe_studi", "desain", "type_of_study", "research_design", "metode",
	},
	"geographic": {
		"country", "countries", "location", "region",
		"negara", "lokasi", "setting", "geographical",
	},
}

func extFieldValue(p bson.M, keySub string) string {
	arr, ok := p["fields"].(bson.A)
	if !ok {
		// Fallback: try as []interface{}
		if arr2, ok2 := p["fields"].([]interface{}); ok2 {
			arr = bson.A(arr2)
		}
	}
	// Jika "fields" tidak ada atau kosong, coba "m7_fields" sebagai fallback
	if len(arr) == 0 {
		if arr2, ok := p["m7_fields"].(bson.A); ok {
			arr = arr2
		} else if arr3, ok3 := p["m7_fields"].([]interface{}); ok3 {
			arr = bson.A(arr3)
		}
	}
	if len(arr) == 0 {
		return ""
	}

	// Build the list of substrings to try: primary keySub first, then aliases
	candidates := []string{strings.ToLower(keySub)}
	if aliases, ok := extFieldAliases[strings.ToLower(keySub)]; ok {
		for _, a := range aliases {
			candidates = append(candidates, strings.ToLower(a))
		}
	}

	for _, it := range arr {
		f, ok := it.(bson.M)
		if !ok {
			continue
		}
		k, _ := f["key"].(string)
		kLower := strings.ToLower(k)
		for _, candidate := range candidates {
			if strings.Contains(kLower, candidate) {
				v, _ := f["value"].(string)
				return v
			}
		}
	}
	return ""
}

func tallyExtField(docs []bson.M, keySub string) map[string]int {
	out := map[string]int{}
	for _, p := range docs {
		v := extFieldValue(p, keySub)
		if v == "" || v == "[NOT REPORTED]" {
			continue
		}
		if len(v) > 40 {
			v = v[:40]
		}
		out[v]++
	}
	return out
}

func tallyMeta(docs []bson.M, key string) map[string]int {
	out := map[string]int{}
	for _, p := range docs {
		v := getStr(p, key)
		if v == "" {
			continue
		}
		out[v]++
	}
	return out
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	}
	return 0, false
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
