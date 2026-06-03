package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"nsa/internal/agent"
	"nsa/internal/logger"
	"nsa/internal/model"
)

type M8Synthesis struct {
	deps *ModuleDeps
}

func NewM8Synthesis(deps *ModuleDeps) *M8Synthesis { return &M8Synthesis{deps: deps} }

func (m *M8Synthesis) Name() string { return "M8_SYNTHESIS" }

func (m *M8Synthesis) Execute(ctx context.Context, session *model.SLRSession) error {
	logger.Logf(session.ID, ">> [MODUL 8: ANALYSIS + SYNTHESIS] State: %s\n", session.Status)

	switch session.Status {
	case "M8_SYNTHESIS", "M8_INIT":
		session.Status = "M8_STEP1_DESCRIPTIVE"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L1: descriptive analysis + figures + heterogeneity ----
	case "M8_STEP1_DESCRIPTIVE":
		return m.runDescriptiveL1(ctx, session)
	case "M8_STEP1_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau 'descriptive_analysis' (+figur SVG) & heterogeneity. Approve / revisi.")
		return nil
	case "M8_STEP1_NEEDS_REVISION":
		session.DescriptiveAnalysis = nil
		session.Feedback = ""
		session.Status = "M8_STEP1_DESCRIPTIVE"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M8_STEP1_APPROVED":
		session.Status = "M8_STEP2_SYNTHESIS"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L2: synthesis path decision + execution (A/B) ----
	case "M8_STEP2_SYNTHESIS":
		return m.runSynthesisL2(ctx, session)
	case "M8_STEP2_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau 'synthesis_path_decision' & 'synthesis_results'. Approve / revisi.")
		return nil
	case "M8_STEP2_NEEDS_REVISION":
		session.SynthesisPathDecision = nil
		session.SynthesisResults = nil
		session.Feedback = ""
		session.Status = "M8_STEP2_SYNTHESIS"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M8_STEP2_APPROVED":
		session.Status = "M8_STEP3_GRADE"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L3: GRADE + robustness ----
	case "M8_STEP3_GRADE":
		return m.runGradeL3(ctx, session)
	case "M8_STEP3_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau 'grade_evidence_table' & robustness. Approve / revisi.")
		return nil
	case "M8_STEP3_NEEDS_REVISION":
		session.GradeEvidence = nil
		session.Feedback = ""
		session.Status = "M8_STEP3_GRADE"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M8_STEP3_APPROVED":
		session.Status = "M8_STEP4_INTERPRETATION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L4: interpretation package + summary ----
	case "M8_STEP4_INTERPRETATION":
		return m.runInterpretationL4(ctx, session)
	case "M8_STEP4_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau 'interpretation_package' & 'modul8_summary'. Approve untuk lanjut ke Modul 9.")
		return nil
	case "M8_STEP4_NEEDS_REVISION":
		session.InterpretationPackage = nil
		session.Modul8Summary = nil
		session.Feedback = ""
		session.Status = "M8_STEP4_INTERPRETATION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M8_STEP4_APPROVED":
		session.Status = "M9_MANUSCRIPT"
		logger.Log(session.ID, "   [System] Modul 8 SELESAI. Lanjut ke Modul 9 (Manuscript Writing).")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	default:
		return nil
	}
}

// ===== L1 =====

func (m *M8Synthesis) runDescriptiveL1(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 8.1] Descriptive analysis + figur + heterogeneity deep-dive...")
	docs := m.extractionDocs(ctx, session)

	designs := tallyExtField(docs, "design")
	years := tallyMeta(docs, "Year")
	geo := tallyExtField(docs, "geographic")
	quality := map[string]int{}
	for _, p := range docs {
		c := getStr(p, "qa_final_category")
		if c == "" {
			c = "UNRATED"
		}
		quality[c]++
	}

	md := fmt.Sprintf("## Descriptive Analysis\n\n- Total studi: **%d**\n- Study design: %s\n- Distribusi tahun: %s\n- Geografis: %s\n- Kualitas: %s\n",
		len(docs), fmtCounts(designs), fmtCounts(years), fmtCounts(geo), fmtCounts(quality))

	figs := []model.Figure{
		figureFromCounts("fig_temporal", "Distribusi Tahun Publikasi", years, true),
		figureFromCounts("fig_geographic", "Distribusi Geografis", geo, false),
		figureFromCounts("fig_design", "Distribusi Study Design", designs, false),
		figureFromCounts("fig_quality", "Distribusi Kualitas (GRADE QA)", quality, false),
	}

	// Heterogeneity deep-dive via brain LLM.
	priorVerdict := ""
	if session.SynthesisPrep != nil {
		priorVerdict = session.SynthesisPrep.HeterogeneityVerdict
	}
	descJSON, _ := json.Marshal(map[string]interface{}{
		"total": len(docs), "designs": designs, "years": years, "geographic": geo, "quality": quality,
	})

	da := &model.DescriptiveAnalysis{Markdown: md, Figures: figs, HeterogeneityVerdict: priorVerdict}
	if brain, err := m.deps.LLMFactory.BrainClient(ctx); err == nil {
		if h, e := agent.NewSynthesisAgent(brain).Heterogeneity(ctx, string(descJSON), priorVerdict); e == nil && h != nil {
			da.HeterogeneityVerdict = h.Verdict
			da.HeterogeneityNarrative = h.Narrative
		} else if e != nil {
			logger.Logf(session.ID, "   [WARN] Heterogeneity LLM gagal: %v (pakai verdict M7).\n", e)
		}
	}

	session.DescriptiveAnalysis = da
	session.Status = "M8_STEP1_WAITING_APPROVAL"
	logger.Logf(session.ID, "   [System] Descriptive + %d figur tersusun. Heterogeneity: %s.\n", len(figs), da.HeterogeneityVerdict)
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// ===== L2 =====

func (m *M8Synthesis) runSynthesisL2(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 8.2] Synthesis path decision + execution...")
	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return fmt.Errorf("brain (M8 synthesis) gagal dimuat: %w", err)
	}
	ag := agent.NewSynthesisAgent(brain)

	hetVerdict := ""
	if session.DescriptiveAnalysis != nil {
		hetVerdict = session.DescriptiveAnalysis.HeterogeneityVerdict
	}
	prepJSON := "{}"
	if session.SynthesisPrep != nil {
		b, _ := json.Marshal(session.SynthesisPrep)
		prepJSON = string(b)
	}

	decision, err := ag.DecidePath(ctx, hetVerdict, prepJSON)
	if err != nil {
		return err
	}
	session.SynthesisPathDecision = decision

	dataJSON := m.dataSummaryJSON(ctx, session)
	if strings.Contains(strings.ToUpper(decision.Verdict), "JALUR B") {
		scaf, err := ag.MetaScaffold(ctx, dataJSON)
		if err != nil {
			return err
		}
		session.SynthesisResults = &model.SynthesisResults{Path: decision.Verdict, Markdown: scaf.Markdown, ForestPlotScript: scaf.ForestPlotScript}
	} else {
		rqJSON, _ := json.Marshal(session.ResearchQuestions)
		md, err := ag.NarrativeSynthesis(ctx, frameworkName(session), dataJSON, string(rqJSON))
		if err != nil {
			return err
		}
		session.SynthesisResults = &model.SynthesisResults{Path: decision.Verdict, Markdown: md}
	}

	logger.Logf(session.ID, "   [System] Synthesis path: %s.\n", decision.Verdict)
	session.Status = "M8_STEP2_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// ===== helpers =====

func (m *M8Synthesis) extractionDocs(ctx context.Context, session *model.SLRSession) []bson.M {
	cur, err := m.deps.MongoRepo.GetExtractionCollection().Find(ctx, bson.M{"session_id": session.ID})
	if err != nil {
		return nil
	}
	var docs []bson.M
	_ = cur.All(ctx, &docs)
	return docs
}

func (m *M8Synthesis) dataSummaryJSON(ctx context.Context, session *model.SLRSession) string {
	docs := m.extractionDocs(ctx, session)
	quality := map[string]int{}
	keyFindings := []string{}
	for _, p := range docs {
		quality[getStr(p, "qa_final_category")]++
		if kf := getStr(p, "key_findings"); kf != "" {
			keyFindings = append(keyFindings, kf)
		}
	}
	if len(keyFindings) > 40 {
		keyFindings = keyFindings[:40]
	}
	summary := map[string]interface{}{
		"framework":          frameworkName(session),
		"total_included":     len(docs),
		"designs":            tallyExtField(docs, "design"),
		"geographic":         tallyExtField(docs, "geographic"),
		"years":              tallyMeta(docs, "Year"),
		"quality":            quality,
		"key_findings":       keyFindings,
		"qa_threshold":       session.QAThreshold,
		"synthesis_prep":     session.SynthesisPrep,
	}
	b, _ := json.Marshal(summary)
	return string(b)
}

func fmtCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "(tidak tersedia)"
	}
	parts := make([]string, 0, len(counts))
	for k, v := range counts {
		parts = append(parts, fmt.Sprintf("%s=%d", k, v))
	}
	return strings.Join(parts, "; ")
}
