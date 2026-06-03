package modules

import (
	"context"
	"encoding/json"
	"fmt"

	"nsa/internal/agent"
	"nsa/internal/logger"
	"nsa/internal/model"
)

type M9Manuscript struct {
	deps *ModuleDeps
}

func NewM9Manuscript(deps *ModuleDeps) *M9Manuscript { return &M9Manuscript{deps: deps} }

func (m *M9Manuscript) Name() string { return "M9_MANUSCRIPT" }

func (m *M9Manuscript) Execute(ctx context.Context, session *model.SLRSession) error {
	logger.Logf(session.ID, ">> [MODUL 9: MANUSCRIPT] State: %s\n", session.Status)

	switch session.Status {
	case "M9_MANUSCRIPT", "M9_INIT":
		if session.Manuscript == nil {
			session.Manuscript = &model.Manuscript{}
		}
		session.Status = "M9_GROUPA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- GROUP A: Methods + Results + Discussion + Future Research ----
	case "M9_GROUPA":
		return m.generateGroupA(ctx, session)
	case "M9_GROUPA_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau Methods/Results/Discussion/Future Research. Approve / revisi.")
		return nil
	case "M9_GROUPA_NEEDS_REVISION":
		session.Feedback = ""
		session.Status = "M9_GROUPA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M9_GROUPA_APPROVED":
		session.Status = "M9_GROUPB"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- GROUP B: Introduction + Conclusions + Abstract + Title ----
	case "M9_GROUPB":
		return m.generateGroupB(ctx, session)
	case "M9_GROUPB_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau Introduction/Conclusions/Abstract/Title. Approve / revisi.")
		return nil
	case "M9_GROUPB_NEEDS_REVISION":
		session.Feedback = ""
		session.Status = "M9_GROUPB"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M9_GROUPB_APPROVED":
		session.Status = "M9_COMPILE"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- COMPILE: references + audit + prisma + final + latex + summary ----
	case "M9_COMPILE":
		return m.runCompile(ctx, session)
	case "M9_COMPILE_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau manuscript_final (+.tex/.bib), coherence_audit, PRISMA checklist. Approve untuk menutup pipeline.")
		return nil
	case "M9_COMPILE_NEEDS_REVISION":
		session.Feedback = ""
		session.Status = "M9_COMPILE"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M9_COMPILE_APPROVED":
		session.Status = "COMPLETED"
		logger.Log(session.ID, "   [System] 🎉 MODUL 9 SELESAI — Manuskrip SLR final siap. Pipeline COMPLETED.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	default:
		return nil
	}
}

// ===== Group generators =====

func (m *M9Manuscript) generateGroupA(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [L1-L4] Menulis Methods, Results, Discussion, Future Research...")
	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return fmt.Errorf("brain (M9) gagal dimuat: %w", err)
	}
	ag := agent.NewManuscriptAgent(brain)
	if session.Manuscript == nil {
		session.Manuscript = &model.Manuscript{}
	}
	bundle := m.artifactBundle(session)

	methods, err := ag.Write(ctx, promptMethods, bundle)
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Methods")
	results, err := ag.Write(ctx, promptResults, bundle+sectionCtx("METHODS (sudah ditulis)", methods))
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Results")
	discussion, err := ag.Write(ctx, promptDiscussion, bundle+sectionCtx("RESULTS (sudah ditulis — jangan diulang)", results))
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Discussion")
	future, err := ag.Write(ctx, promptFuture, bundle+sectionCtx("DISCUSSION (limitations — agenda harus beda/actionable)", discussion))
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Future Research")

	session.Manuscript.Methods = methods
	session.Manuscript.Results = results
	session.Manuscript.Discussion = discussion
	session.Manuscript.FutureResearch = future
	session.Status = "M9_GROUPA_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

func (m *M9Manuscript) generateGroupB(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [L5-L9] Menulis Introduction, Conclusions, Abstract, Title...")
	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return fmt.Errorf("brain (M9) gagal dimuat: %w", err)
	}
	ag := agent.NewManuscriptAgent(brain)
	ms := session.Manuscript
	bundle := m.artifactBundle(session)

	intro, err := ag.Write(ctx, promptIntro, bundle+sectionCtx("RESULTS (untuk tune preview, JANGAN bocorkan angka spesifik di Intro)", trim(ms.Results, 4000)))
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Introduction")
	conclusions, err := ag.Write(ctx, promptConclusions, bundle+sectionCtx("DISCUSSION", trim(ms.Discussion, 5000))+sectionCtx("FUTURE RESEARCH", trim(ms.FutureResearch, 2500)))
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Conclusions")
	abstract, err := ag.Write(ctx, promptAbstract, bundle+sectionCtx("METHODS", trim(ms.Methods, 3000))+sectionCtx("RESULTS", trim(ms.Results, 4000))+sectionCtx("DISCUSSION", trim(ms.Discussion, 3000)))
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Abstract")
	title, err := ag.Write(ctx, promptTitle, sectionCtx("ABSTRACT", abstract)+m.geoFrameworkHint(session))
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Title")

	ms.Introduction = intro
	ms.Conclusions = conclusions
	ms.Abstract = abstract
	ms.Title = title
	session.Status = "M9_GROUPB_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// ===== context helpers =====

func sectionCtx(label, content string) string {
	if content == "" {
		return ""
	}
	return fmt.Sprintf("\n\n=== %s ===\n%s\n", label, content)
}

func trim(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func (m *M9Manuscript) geoFrameworkHint(session *model.SLRSession) string {
	fw := frameworkName(session)
	path := "JALUR A (narrative)"
	if session.SynthesisPathDecision != nil && session.SynthesisPathDecision.Verdict != "" {
		path = session.SynthesisPathDecision.Verdict
	}
	return fmt.Sprintf("\n\n=== KONTEKS TITLE ===\nFramework: %s | Synthesis path: %s | Topik: %s\n", fw, path, session.Topic)
}

// artifactBundle merangkai artefak M2-M8 (JSON) sebagai konteks; SVG figur dikecualikan.
func (m *M9Manuscript) artifactBundle(session *model.SLRSession) string {
	var descr interface{}
	if session.DescriptiveAnalysis != nil {
		descr = map[string]string{
			"markdown":  session.DescriptiveAnalysis.Markdown,
			"verdict":   session.DescriptiveAnalysis.HeterogeneityVerdict,
			"narrative": session.DescriptiveAnalysis.HeterogeneityNarrative,
		}
	}
	kappaTA := 0.0
	if n := len(session.KalibrasiLog); n > 0 {
		kappaTA = session.KalibrasiLog[n-1].Kappa
	}
	robKappa := 0.0
	if session.QAThreshold != nil {
		robKappa = session.QAThreshold.Kappa
	}
	extKappa := 0.0
	if session.ExtractionLog != nil {
		extKappa = session.ExtractionLog.ExtractionKappa
	}

	b := map[string]interface{}{
		"topic":                session.Topic,
		"selected_topic_gap":   session.SelectedTopic,
		"pico":                 session.PICODefinitions,
		"research_questions":   session.ResearchQuestions,
		"scope_justifications": session.ScopeJustifications,
		"prior_reviews":        session.PriorReviewsMatrix,
		"search_log":           session.SearchLog,
		"prisma_flow_and_kappa": session.ExclusionTable, // flow numbers + kappa report + exclusion reasons
		"kappa": map[string]float64{
			"kappa_TA": kappaTA, "fulltext": session.FulltextKappa, "extract": extKappa, "rob": robKappa,
		},
		"framework":            session.FrameworkSelection,
		"extraction_log":       session.ExtractionLog,
		"qa_threshold":         session.QAThreshold,
		"sensitivity":          session.SensitivityAnalysis,
		"synthesis_path":       session.SynthesisPathDecision,
		"synthesis_results":    session.SynthesisResults,
		"grade_evidence":       session.GradeEvidence,
		"descriptive":          descr,
		"interpretation":       session.InterpretationPackage,
		"acquisition":          session.AcquisitionLog,
		"inaccessible_impact":  session.InaccessibleImpact,
		"slna_integration":     session.SLNAIntegration, // nil bila M8b tak dijalankan
		"modul8_summary":       session.Modul8Summary,
	}
	j, _ := json.Marshal(b)
	return "=== ARTEFAK SLR (sumber data; pakai angka apa adanya) ===\n" + string(j)
}
