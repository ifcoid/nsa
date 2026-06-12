package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"nsa/internal/agent"
	"nsa/internal/llm"
	"nsa/internal/logger"
	"nsa/internal/model"
	"nsa/internal/repository"
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
	ctx = llm.WithXAIContext(ctx, session.ID, session.Status, "M7Extraction")

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
		_, _ = m.deps.MongoRepo.GetSessionCollection().UpdateOne(ctx,
			bson.M{"_id": session.ID}, bson.M{"$unset": bson.M{"extraction_log": ""}})
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
	case "M7_STEP3_QA_TOOL_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau pilihan QA Tool & Threshold. Approve / revisi.")
		return nil
	case "M7_STEP3_QA_TOOL_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 7.3] Pemilihan ulang QA Tool (feedback: '%s')\n", session.Feedback)
		_, _ = m.deps.MongoRepo.GetSessionCollection().UpdateOne(ctx,
			bson.M{"_id": session.ID}, bson.M{"$unset": bson.M{"qa_threshold_justification": ""}})
		session.QAThreshold = nil
		// Feedback JANGAN dikosongkan di sini agar bisa dibaca oleh runQAL3
		session.Status = "M7_STEP3_QA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP3_QA_TOOL_APPROVED":
		session.Status = "M7_STEP3_QA_CALIBRATION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L3 Calibration: anchor examples + pilot batch + kappa check ----
	case "M7_STEP3_QA_CALIBRATION":
		return m.runQACalibration(ctx, session)
	case "M7_STEP3_QA_CALIBRATION_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau hasil kalibrasi QA (anchors + pilot kappa). Approve untuk lanjut full rating.")
		return nil
	case "M7_STEP3_QA_CALIBRATION_APPROVED":
		// Reset pilot papers qa_rated flag so they get re-rated in full batch.
		coll := m.deps.MongoRepo.GetExtractionCollection()
		_, _ = coll.UpdateMany(ctx,
			bson.M{"session_id": session.ID, "qa_calibration_pilot": true},
			bson.M{"$set": bson.M{"qa_rated": false}})
		session.Status = "M7_STEP3_QA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP3_QA_CALIBRATION_LOW_KAPPA":
		logger.Log(session.ID, "   [System] Kalibrasi QA kappa rendah. Pilih: retry kalibrasi atau lanjutkan (force proceed).")
		return nil
	case "M7_STEP3_QA_CALIBRATION_RETRY":
		// User wants to retry calibration. Reset pilot papers and re-run.
		coll := m.deps.MongoRepo.GetExtractionCollection()
		_, _ = coll.UpdateMany(ctx,
			bson.M{"session_id": session.ID, "qa_calibration_pilot": true},
			bson.M{"$unset": bson.M{"qa_calibration_pilot": ""}, "$set": bson.M{"qa_rated": false}})
		session.Status = "M7_STEP3_QA_CALIBRATION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP3_QA_CALIBRATION_FORCE_PROCEED":
		// User wants to proceed despite low kappa.
		coll := m.deps.MongoRepo.GetExtractionCollection()
		_, _ = coll.UpdateMany(ctx,
			bson.M{"session_id": session.ID, "qa_calibration_pilot": true},
			bson.M{"$set": bson.M{"qa_rated": false}})
		session.Status = "M7_STEP3_QA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP3_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau 'qa_threshold_justification' + 'sensitivity_analysis'. Approve / revisi.")
		return nil
	case "M7_STEP3_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 7.3] QA ulang (feedback: '%s')\n", session.Feedback)
		_, _ = m.deps.MongoRepo.GetExtractionCollection().UpdateMany(ctx,
			bson.M{"session_id": session.ID}, bson.M{"$set": bson.M{"qa_rated": false}})
		_, _ = m.deps.MongoRepo.GetSessionCollection().UpdateOne(ctx,
			bson.M{"_id": session.ID}, bson.M{"$unset": bson.M{"qa_threshold_justification": "", "sensitivity_analysis": ""}})
		session.QAThreshold = nil
		session.SensitivityAnalysis = nil
		// Feedback JANGAN dikosongkan di sini agar bisa dibaca oleh runQAL3
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
		session.Status = "M7_STEP5_GRAPH_EXTRACTION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L5: Knowledge Graph Extraction (Neuro-Symbolic / GraphRAG) ----
	case "M7_STEP5_GRAPH_EXTRACTION":
		return m.runGraphExtractionL5(ctx, session)
	case "M7_STEP5_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau hasil ekstraksi Knowledge Graph Neo4j. Approve untuk lanjut ke Modul 8.")
		return nil
	case "M7_STEP5_NEEDS_REVISION":
		session.Feedback = ""
		session.Status = "M7_STEP5_GRAPH_EXTRACTION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP5_APPROVED":
		session.Status = "M8_SYNTHESIS"
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
		if ctx.Err() != nil {
			return ctx.Err()
		}
		title := getStr(p, "Title")
		doi := getStr(p, "DOI", "doi")
		logger.Logf(session.ID, "      -> Extract [%d/%d] %s\n", i+1, len(batch), doi)

		var ft string
		if nd := normalizeDOIForRAG(doi); nd != "" && ftIndex[nd] != "" {
			ft = ftIndex[nd]
		} else if nt := NormTitle(title); nt != "" && ftIndex["title:"+nt] != "" {
			ft = ftIndex["title:"+nt]
		}

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
				update["model_extraction"] = leadAg.ModelName()
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
		if ctx.Err() != nil {
			return ctx.Err()
		}
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
	rp1, _ := m.deps.LLMFactory.RoleProviders(ctx, "reviewer1")
	colsJSON := "[]"
	if session.FrameworkSelection != nil {
		b, _ := json.Marshal(session.FrameworkSelection.Columns)
		colsJSON = string(b)
	}
	systemPrompt := fmt.Sprintf(`Anda Extractor utama untuk Systematic Literature Review.
Ekstrak data per kolom TEMPLATE dari FULL-TEXT artikel (konteks RAG).

TEMPLATE KOLOM (JSON):
%s

OPERATIONAL DEFINITIONS:
%s

ATURAN ANTI-HALUSINASI (WAJIB):
- Simpulkan HANYA dari full-text yang diberikan. Dilarang memakai pengetahuan luar.
- Per field: kutip kalimat pendukung + section ref di "evidence" (mis. "Methods p.5: We surveyed 234...").
- Jika tidak ada di teks: value "[NOT REPORTED]", status "NOT_REPORTED" (JANGAN mengira).
- Borderline: status "AMBIGUOUS" + alasan di evidence.
- Konsisten dengan canonical terminology.
- RED FLAGS QA (sample kecil tanpa power analysis, missing data tak dijelaskan, confounder tak ditangani, outcome tak validated) → ringkas di "qa_red_flags" (awali tiap item 'QA_RED:').

Keluarkan HANYA JSON MURNI tanpa markdown:
{
  "fields": [{"key": "Theory", "value": "...", "evidence": "Intro p.2: ...", "status": "REPORTED"}],
  "key_findings": "1-2 kalimat temuan utama",
  "qa_red_flags": "QA_RED: ... ; QA_RED: ...",
  "ambiguous": ["nama field yang ambiguous"],
  "coverage": "COMPLETE"
}`, colsJSON, opDefs)

	session.ExtractionLog = &model.ExtractionLog{
		TotalExtracted:      total,
		VerifiedSample:      checked,
		DisagreementRate:    rate,
		AmbiguousCount:      ambiguous,
		NRNote:              nrNote,
		SystemPrompt:        systemPrompt,
		ModelExtraction:     rp1,
		ModelRefineProtocol: vp,
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

// agentWithFallback membuat ExtractionAgent dengan retry+fallback (primary -> fallback).
func (m *M7Extraction) agentWithFallback(ctx context.Context, primary, fallback string) (*agent.ExtractionAgent, error) {
	p, errP := m.deps.LLMFactory.CreateClient(ctx, primary)
	var fb llm.LLMClient
	if fallback != "" {
		if c, e := m.deps.LLMFactory.CreateClient(ctx, fallback); e == nil {
			fb = c
		}
	}
	if errP != nil {
		if fb != nil {
			return agent.NewExtractionAgent(llm.NewRetryingClient(nil, fb)), nil
		}
		return nil, errP
	}
	return agent.NewExtractionAgent(llm.NewRetryingClient(p, fb)), nil
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

// ---- L5: Graph Extraction (Neo4j) ----
type ExtractedNode struct {
	ID    string                 `json:"id"`
	Label string                 `json:"label"`
	Props map[string]interface{} `json:"props"`
}

type ExtractedEdge struct {
	SourceID    string                 `json:"source_id"`
	SourceLabel string                 `json:"source_label"`
	TargetID    string                 `json:"target_id"`
	TargetLabel string                 `json:"target_label"`
	Type        string                 `json:"type"`
	Props       map[string]interface{} `json:"props"`
}

type GraphExtractionResponse struct {
	Nodes []ExtractedNode `json:"nodes"`
	Edges []ExtractedEdge `json:"edges"`
}

func (m *M7Extraction) runGraphExtractionL5(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 7.5] Ekstraksi Knowledge Graph (Neo4j) berjalan...")

	if m.deps.Neo4jRepo == nil {
		// Coba reconnect: baca ulang env vars dan coba koneksi sekali lagi
		logger.Log(session.ID, "   [Neo4j] Koneksi nil saat startup, mencoba reconnect...")
		neo4jURI := os.Getenv("NEO4JURI")
		neo4jUser := os.Getenv("NEO4JUSER")
		neo4jPass := os.Getenv("NEO4JPASSWORD")

		if neo4jURI == "" {
			errMsg := fmt.Sprintf("NEO4JURI env var kosong. Pastikan secret NEO4JURI sudah di-set di environment (Fly.io secrets / .env). Error startup sebelumnya: %s", m.deps.Neo4jConnErr)
			logger.Logf(session.ID, "   [ERROR] %s", errMsg)
			return fmt.Errorf("neo4j: %s", errMsg)
		}

		maskedURI := neo4jURI
		if len(maskedURI) > 10 {
			maskedURI = maskedURI[:10] + "..."
		}
		logger.Logf(session.ID, "   [Neo4j] Reconnect attempt: uri=%s, user=%q, pass_len=%d", maskedURI, neo4jUser, len(neo4jPass))

		repo, err := repository.NewNeo4jRepository(neo4jURI, neo4jUser, neo4jPass)
		if err != nil {
			errDetail := fmt.Sprintf("Neo4j reconnect gagal (uri=%s, user=%q): %v. Error startup: %s", maskedURI, neo4jUser, err, m.deps.Neo4jConnErr)
			logger.Logf(session.ID, "   [ERROR] %s", errDetail)
			return fmt.Errorf("neo4j: %s", errDetail)
		}

		// Reconnect berhasil!
		m.deps.Neo4jRepo = repo
		logger.Log(session.ID, "   [Neo4j] Reconnect BERHASIL! Melanjutkan ekstraksi graph...")
	}

	collExt := m.deps.MongoRepo.GetExtractionCollection()
	filter := bson.M{
		"session_id": session.ID,
		"extracted":  true,
		"qa_rated":   true,
		"graph_extracted": bson.M{"$ne": true},
	}
	opts := options.Find().SetLimit(int64(extractionBatchSize))

	cursor, err := collExt.Find(ctx, filter, opts)
	if err != nil {
		return fmt.Errorf("find unextracted graph papers: %w", err)
	}
	var papers []map[string]interface{}
	if err := cursor.All(ctx, &papers); err != nil {
		return fmt.Errorf("cursor all: %w", err)
	}

	if len(papers) == 0 {
		logger.Log(session.ID, "   [Langkah 7.5] Seluruh ekstraksi Knowledge Graph selesai.")
		session.Status = "M7_STEP5_WAITING_APPROVAL"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	logger.Logf(session.ID, "   [Graph] Memproses batch %d dokumen untuk GraphRAG...\n", len(papers))

	rp1, _ := m.deps.LLMFactory.RoleProviders(ctx, "reviewer1")
	brain, err := m.deps.LLMFactory.CreateClient(ctx, rp1)
	if err != nil {
		return fmt.Errorf("llm init: %w", err)
	}

	for _, p := range papers {
		objID := p["_id"]
		title, _ := p["Title"].(string)
		if title == "" {
			title, _ = p["title"].(string)
		}
		doi, _ := p["DOI"].(string)
		if doi == "" {
			doi, _ = p["doi"].(string)
		}
		
		fields, _ := json.Marshal(p["m7_fields"])
		
		sysPrompt := `Anda adalah ahli neuro-symbolic AI yang bertugas membangun Knowledge Graph dari literatur ilmiah.
Tugas Anda adalah membaca hasil ekstraksi sebuah paper, dan mengubahnya menjadi Nodes (simpul) dan Edges (relasi).
ATURAN NODES:
- Wajib sertakan minimal node Paper.
  Node Paper: Label "Paper", ID (DOI atau Title yang di-slug), Props minimal {title, doi}.
- Buat Nodes untuk Entitas penting: "Author", "Method", "Dataset", "Metric", "Conclusion".
- Gunakan ID yang sangat konsisten untuk entitas yang sama (contoh: id="dataset-adni", label="Dataset", props={name: "ADNI"}).

ATURAN EDGES:
- Hubungkan Paper dengan entitas lain.
- Tipe Relasi valid contohnya: WRITTEN_BY, USES_METHOD, USES_DATASET, EVALUATES_METRIC, CONCLUDES.
- Tiap edge butuh source_id, target_id, source_label, target_label, type, dan props.

Hanya keluarkan JSON utuh dengan format:
{
  "nodes": [{"id":"...", "label":"...", "props":{}}],
  "edges": [{"source_id":"...", "source_label":"...", "target_id":"...", "target_label":"...", "type":"...", "props":{}}]
}
Jangan ada tambahan teks markdown (tanpa ` + "```json" + ` ... ` + "```" + `) atau penjelasan.`

		userPrompt := fmt.Sprintf("Ekstrak graf dari paper berikut:\nJudul: %s\nDOI: %s\nHasil Ekstraksi:\n%s", title, doi, string(fields))

		respText, err := brain.Generate(ctx, sysPrompt, userPrompt)
		if err != nil {
			logger.Logf(session.ID, "      [!] Gagal memanggil LLM untuk paper %s: %v\n", title, err)
			continue
		}

		// Bersihkan markdown markdown block jika ada
		respText = strings.TrimSpace(respText)
		if strings.HasPrefix(respText, "```json") {
			respText = strings.TrimPrefix(respText, "```json")
			respText = strings.TrimSuffix(respText, "```")
		}
		if strings.HasPrefix(respText, "```") {
			respText = strings.TrimPrefix(respText, "```")
			respText = strings.TrimSuffix(respText, "```")
		}

		var gResp GraphExtractionResponse
		if err := json.Unmarshal([]byte(respText), &gResp); err != nil {
			logger.Logf(session.ID, "      [!] Gagal mem-parsing JSON Graph untuk paper %s. Output LLM:\n%s\n", title, respText)
			continue
		}

		// Convert ke struktur Neo4jRepository
		var rNodes []repository.GraphNode
		var rEdges []repository.GraphEdge

		for _, n := range gResp.Nodes {
			if n.Props == nil {
				n.Props = make(map[string]interface{})
			}
			n.Props["id"] = n.ID
			rNodes = append(rNodes, repository.GraphNode{
				Label:      n.Label,
				Properties: n.Props,
			})
		}
		for _, e := range gResp.Edges {
			rEdges = append(rEdges, repository.GraphEdge{
				SourceNode: repository.GraphNode{Label: e.SourceLabel, Properties: map[string]interface{}{"id": e.SourceID}},
				TargetNode: repository.GraphNode{Label: e.TargetLabel, Properties: map[string]interface{}{"id": e.TargetID}},
				Type:       e.Type,
				Properties: e.Props,
			})
		}

		err = m.deps.Neo4jRepo.SaveKnowledgeGraph(ctx, rNodes, rEdges)
		if err != nil {
			logger.Logf(session.ID, "      [!] Gagal menyimpan Knowledge Graph Neo4j untuk paper %s: %v\n", title, err)
			continue
		}

		_, _ = collExt.UpdateByID(ctx, objID, bson.M{"$set": bson.M{"graph_extracted": true}})
		logger.Logf(session.ID, "      [+] Sukses menyimpan ke Neo4j: %s (%d nodes, %d edges)\n", title, len(rNodes), len(rEdges))
	}

	return nil
}
