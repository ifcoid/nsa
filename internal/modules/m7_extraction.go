package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"nsa/internal/agent"
	"nsa/internal/logger"
	"nsa/internal/model"
)

type M7Extraction struct {
	deps *ModuleDeps
}

func NewM7Extraction(deps *ModuleDeps) *M7Extraction {
	return &M7Extraction{deps: deps}
}

func (m *M7Extraction) Name() string { return "M7_EXTRACTION" }

const extractionBatchSize = 6

func (m *M7Extraction) Execute(ctx context.Context, session *model.SLRSession) error {
	logger.Logf(session.ID, ">> [MODUL 7: EXTRACTION + QA] State: %s\n", session.Status)

	switch session.Status {
	case "M7_EXTRACTION", "M7_INIT":
		session.Status = "M7_STEP1_FRAMEWORK"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L1: Framework selection + template + pre-populate extraction ----
	case "M7_STEP1_FRAMEWORK":
		return m.runFrameworkL1(ctx, session)
	case "M7_STEP1_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau 'framework_selection' (framework + kolom template). Approve / revisi.")
		return nil
	case "M7_STEP1_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 7.1] Menyusun ulang framework (feedback: '%s')\n", session.Feedback)
		session.Feedback = ""
		return m.runFrameworkL1(ctx, session)
	case "M7_STEP1_APPROVED":
		session.Status = "M7_STEP2_EXTRACTION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L2: Systematic extraction (RAG) + 20% spot-verification ----
	case "M7_STEP2_EXTRACTION":
		return m.runExtractionL2(ctx, session)
	case "M7_STEP2_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau hasil ekstraksi (collection slr_extraction) + 'extraction_log'. Approve / revisi.")
		return nil
	case "M7_STEP2_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 7.2] Ekstraksi ulang (feedback: '%s')\n", session.Feedback)
		_, _ = m.deps.MongoRepo.GetExtractionCollection().UpdateMany(ctx,
			bson.M{"session_id": session.ID}, bson.M{"$set": bson.M{"extracted": false, "verified": false}})
		session.ExtractionLog = nil
		session.Feedback = ""
		session.Status = "M7_STEP2_EXTRACTION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP2_APPROVED":
		session.Status = "M7_STEP3_QA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L3: Quality appraisal (tool + threshold + dual-rater kappa + sensitivity) ----
	case "M7_STEP3_QA":
		return m.runQAL3(ctx, session)
	case "M7_STEP3_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau 'qa_threshold_justification' + 'sensitivity_analysis'. Approve / revisi.")
		return nil
	case "M7_STEP3_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 7.3] QA ulang (feedback: '%s')\n", session.Feedback)
		_, _ = m.deps.MongoRepo.GetExtractionCollection().UpdateMany(ctx,
			bson.M{"session_id": session.ID}, bson.M{"$set": bson.M{"qa_rated": false}})
		session.QAThreshold = nil
		session.SensitivityAnalysis = nil
		session.Feedback = ""
		session.Status = "M7_STEP3_QA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP3_APPROVED":
		session.Status = "M7_STEP4_SYNTHESIS_PREP"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L4: Synthesis prep + meta-analysis feasibility + summary ----
	case "M7_STEP4_SYNTHESIS_PREP":
		return m.runSynthesisL4(ctx, session)
	case "M7_STEP4_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau 'synthesis_prep' + 'modul7_summary'. Approve untuk lanjut ke Modul 8.")
		return nil
	case "M7_STEP4_NEEDS_REVISION":
		session.Feedback = ""
		session.Status = "M7_STEP4_SYNTHESIS_PREP"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP4_APPROVED":
		session.Status = "M8_SYNTHESIS"
		logger.Log(session.ID, "   [System] Modul 7 SELESAI. Lanjut ke Modul 8 (Analysis + Synthesis).")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	default:
		return nil
	}
}

// ===== L1 =====

func (m *M7Extraction) runFrameworkL1(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 7.1] Rekomendasi framework + template ekstraksi...")

	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return fmt.Errorf("gemini (brain M7) gagal dimuat: %w", err)
	}
	ag := agent.NewExtractionAgent(brain)

	picoJSON := "(tidak tersedia)"
	if session.PICODefinitions != nil {
		b, _ := json.Marshal(session.PICODefinitions)
		picoJSON = string(b)
	}
	rqJSON := "(tidak tersedia)"
	if len(session.ResearchQuestions) > 0 {
		b, _ := json.Marshal(session.ResearchQuestions)
		rqJSON = string(b)
	}

	included := m.finalIncludedPapers(ctx, session)
	designBreakdown := docTypeBreakdown(included)

	fw, err := ag.RecommendFramework(ctx, picoJSON, rqJSON, designBreakdown)
	if err != nil {
		return err
	}
	session.FrameworkSelection = fw

	// Pre-populate koleksi extraction (idempotent untuk sesi ini).
	coll := m.deps.MongoRepo.GetExtractionCollection()
	_, _ = coll.DeleteMany(ctx, bson.M{"session_id": session.ID})
	var docs []interface{}
	for _, p := range included {
		paperID := ""
		if oid, ok := p["_id"].(interface{ Hex() string }); ok {
			paperID = oid.Hex()
		}
		docs = append(docs, bson.M{
			"session_id": session.ID,
			"paper_id":   paperID,
			"Title":      getStr(p, "Title", "title"),
			"Author":     getStr(p, "Authors", "authors"),
			"Year":       getStr(p, "Year", "year"),
			"Journal":    getStr(p, "Journal", "journal"),
			"DOI":        getStr(p, "DOI", "doi"),
			"extracted":  false,
			"qa_rated":   false,
		})
	}
	if len(docs) > 0 {
		_, _ = coll.InsertMany(ctx, docs)
	}
	logger.Logf(session.ID, "   [System] Framework '%s' dipilih, %d paper INCLUDE di-prepopulate.\n", fw.Framework, len(docs))

	session.Status = "M7_STEP1_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// ===== L2 =====

func (m *M7Extraction) runExtractionL2(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 7.2] Systematic extraction (RAG Qdrant)...")
	coll := m.deps.MongoRepo.GetExtractionCollection()

	totalCount, _ := coll.CountDocuments(ctx, bson.M{"session_id": session.ID})
	if totalCount == 0 {
		logger.Log(session.ID, "   [System] Tidak ada paper untuk diekstrak. Lanjut approval.")
		session.ExtractionLog = &model.ExtractionLog{}
		session.Status = "M7_STEP2_WAITING_APPROVAL"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	cur, err := coll.Find(ctx, bson.M{"session_id": session.ID, "extracted": bson.M{"$ne": true}},
		options.Find().SetLimit(int64(extractionBatchSize)))
	if err != nil {
		return err
	}
	var batch []bson.M
	_ = cur.All(ctx, &batch)

	// Semua sudah diekstrak -> spot-verify (20%) lalu approval.
	if len(batch) == 0 {
		if session.ExtractionLog == nil {
			return m.spotVerifyL2(ctx, session)
		}
		session.Status = "M7_STEP2_WAITING_APPROVAL"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	colsJSON := "[]"
	if session.FrameworkSelection != nil {
		b, _ := json.Marshal(session.FrameworkSelection.Columns)
		colsJSON = string(b)
	}
	opDefs := m.opDefs(session)

	ftIndex, _, _ := BuildFulltextIndex(ctx)
	if ftIndex == nil {
		ftIndex = map[string]string{}
	}

	rp1, rf1 := m.deps.LLMFactory.RoleProviders(ctx, "reviewer1")
	leadAg, err := m.agentWithFallback(ctx, rp1, rf1)
	if err != nil {
		return fmt.Errorf("extractor utama (%s/%s) gagal: %w", rp1, rf1, err)
	}

	for i, p := range batch {
		title := getStr(p, "Title")
		doi := normalizeDOIForRAG(getStr(p, "DOI", "doi"))
		logger.Logf(session.ID, "      -> Extract [%d/%d] %s\n", i+1, len(batch), doi)

		ft := ftIndex[doi]
		update := bson.M{"extracted": true}
		if ft == "" {
			update["coverage"] = "NO_FULLTEXT_RAG"
			update["notes"] = "Full-text tidak tersedia di Qdrant; perlu ekstraksi manual."
		} else {
			res, e := leadAg.ExtractPaper(ctx, colsJSON, opDefs, title, ft)
			if e != nil {
				logger.Logf(session.ID, "         [!] gagal extract: %v (ditandai ERROR)\n", e)
				update["coverage"] = "ERROR"
				update["notes"] = "Ekstraksi gagal: " + e.Error()
			} else {
				update["fields"] = res.Fields
				update["key_findings"] = res.KeyFindings
				update["qa_red_flags"] = res.QARedFlags
				update["ambiguous"] = res.Ambiguous
				update["coverage"] = res.Coverage
				update["nr_count"] = countNotReported(res.Fields)
			}
			time.Sleep(5 * time.Second)
		}
		_, _ = coll.UpdateByID(ctx, p["_id"], bson.M{"$set": update})
	}

	session.Status = "M7_STEP2_EXTRACTION" // loop batch berikutnya / verifikasi
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// spotVerifyL2 memverifikasi 20% sampel + field AMBIGUOUS (extractor 2).
func (m *M7Extraction) spotVerifyL2(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 7.2] Spot-verification 20% (extractor 2)...")
	coll := m.deps.MongoRepo.GetExtractionCollection()

	cur, _ := coll.Find(ctx, bson.M{"session_id": session.ID, "coverage": bson.M{"$nin": bson.A{"NO_FULLTEXT_RAG", "ERROR"}}})
	var all []bson.M
	_ = cur.All(ctx, &all)

	total := len(all)
	sample := total / 5 // 20%
	if sample < 1 && total > 0 {
		sample = 1
	}

	opDefs := m.opDefs(session)
	ftIndex, _, _ := BuildFulltextIndex(ctx)
	if ftIndex == nil {
		ftIndex = map[string]string{}
	}
	vp, vf := m.deps.LLMFactory.RoleProviders(ctx, "reviewer2")
	verAg, err := m.agentWithFallback(ctx, vp, vf)
	if err != nil {
		logger.Logf(session.ID, "   [WARN] Verifier (%s/%s) gagal: %v. Lewati verifikasi.\n", vp, vf, err)
	}

	disagree, checked, ambiguous := 0, 0, 0
	for i := 0; i < total; i++ {
		p := all[i]
		amb, _ := p["ambiguous"].(bson.A)
		isAmbiguous := len(amb) > 0
		ambiguous += len(amb)
		if verAg == nil || (i >= sample && !isAmbiguous) {
			continue
		}
		doi := normalizeDOIForRAG(getStr(p, "DOI", "doi"))
		ft := ftIndex[doi]
		if ft == "" {
			continue
		}
		priorJSON, _ := json.Marshal(p["fields"])
		res, e := verAg.VerifyExtraction(ctx, opDefs, getStr(p, "Title"), ft, string(priorJSON))
		checked++
		if e == nil && res != nil && res.Disagree {
			disagree++
			_, _ = coll.UpdateByID(ctx, p["_id"], bson.M{"$set": bson.M{"verify_disagree": true, "verify_notes": res.Notes}})
		}
		time.Sleep(4 * time.Second)
	}

	rate := 0.0
	if checked > 0 {
		rate = float64(disagree) / float64(checked) * 100
	}
	nrNote := "<5%: acceptable; dokumentasi Limitations."
	if rate > 15 {
		nrNote = ">15%: disarankan full dual-extraction untuk semua studi."
	} else if rate >= 5 {
		nrNote = "5-15%: refine protocol, re-do subset yang di-flag."
	}
	session.ExtractionLog = &model.ExtractionLog{
		TotalExtracted:   total,
		VerifiedSample:   checked,
		DisagreementRate: rate,
		AmbiguousCount:   ambiguous,
		NRNote:           nrNote,
	}
	logger.Logf(session.ID, "   [System] Ekstraksi %d paper; verifikasi %d; disagreement %.1f%%.\n", total, checked, rate)
	session.Status = "M7_STEP2_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// ===== Helpers (dipakai L1-L4) =====

func (m *M7Extraction) finalIncludedPapers(ctx context.Context, session *model.SLRSession) []map[string]interface{} {
	all, err := m.deps.MongoRepo.GetAllScreeningPapers(ctx, session.ID)
	if err != nil {
		return nil
	}
	var out []map[string]interface{}
	for _, p := range all {
		retrieved, _ := p["full_text_retrieved"].(bool)
		incAbs := getStr(p, "Final_Decision") == "INCLUDE" ||
			(getStr(p, "Final_Decision") == "" && getStr(p, "Screener_1_Decision") == "INCLUDE")
		if retrieved && incAbs && finalFullDecision(p) == "INCLUDE" {
			out = append(out, p)
		}
	}
	return out
}

func (m *M7Extraction) opDefs(session *model.SLRSession) string {
	if session.PICODefinitions != nil {
		b, _ := json.Marshal(session.PICODefinitions)
		return string(b)
	}
	return "(operational definitions tidak tersedia)"
}

func (m *M7Extraction) agentWithFallback(ctx context.Context, primary, fallback string) (*agent.ExtractionAgent, error) {
	c, err := m.deps.LLMFactory.CreateClient(ctx, primary)
	if err == nil {
		return agent.NewExtractionAgent(c), nil
	}
	if fallback != "" {
		if c2, e2 := m.deps.LLMFactory.CreateClient(ctx, fallback); e2 == nil {
			return agent.NewExtractionAgent(c2), nil
		}
	}
	return nil, err
}

func docTypeBreakdown(papers []map[string]interface{}) string {
	counts := map[string]int{}
	for _, p := range papers {
		dt := getStr(p, "Document_Type", "DocumentType", "document_type")
		if dt == "" {
			dt = "Unknown"
		}
		counts[dt]++
	}
	if len(counts) == 0 {
		return fmt.Sprintf("(belum tersedia; total %d paper)", len(papers))
	}
	s := fmt.Sprintf("Total %d paper. Document types: ", len(papers))
	for k, v := range counts {
		s += fmt.Sprintf("%s=%d; ", k, v)
	}
	return s
}

func countNotReported(fields []agent.ExtractedField) int {
	n := 0
	for _, f := range fields {
		if f.Status == "NOT_REPORTED" {
			n++
		}
	}
	return n
}
