package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nsa/internal/agent"
	"nsa/internal/llm"
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
	ctx = llm.WithXAIContext(ctx, session.ID, session.Status, "M9Manuscript")

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
	lang := langDirective(session)

	methods, err := ag.Write(ctx, promptMethods+lang, bundle)
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Methods")
	results, err := ag.Write(ctx, promptResults+lang, bundle+sectionCtx("METHODS (sudah ditulis)", methods))
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Results")
	discussion, err := ag.Write(ctx, promptDiscussion+lang, bundle+sectionCtx("RESULTS (sudah ditulis — jangan diulang)", results))
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Discussion")
	future, err := ag.Write(ctx, promptFuture+lang, bundle+sectionCtx("DISCUSSION (limitations — agenda harus beda/actionable)", discussion))
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
	lang := langDirective(session)

	intro, err := ag.Write(ctx, promptIntro+lang, bundle+sectionCtx("RESULTS (untuk tune preview, JANGAN bocorkan angka spesifik di Intro)", trim(ms.Results, 4000)))
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Introduction")
	conclusions, err := ag.Write(ctx, promptConclusions+lang, bundle+sectionCtx("DISCUSSION", trim(ms.Discussion, 5000))+sectionCtx("FUTURE RESEARCH", trim(ms.FutureResearch, 2500)))
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Conclusions")
	abstract, err := ag.Write(ctx, promptAbstract+lang, bundle+sectionCtx("METHODS", trim(ms.Methods, 3000))+sectionCtx("RESULTS", trim(ms.Results, 4000))+sectionCtx("DISCUSSION", trim(ms.Discussion, 3000)))
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Abstract")
	title, err := ag.Write(ctx, promptTitle+lang, sectionCtx("ABSTRACT", abstract)+m.geoFrameworkHint(session))
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

// langDirective menentukan bahasa output manuskrip (default Bahasa Indonesia untuk draft).
func langDirective(session *model.SLRSession) string {
	lang := strings.ToLower(strings.TrimSpace(session.ManuscriptLang))
	if lang == "en" || lang == "english" || lang == "inggris" {
		return "\n\nLANGUAGE: Write the entire section in formal, publication-quality academic English."
	}
	return "\n\nBAHASA: Tulis SELURUH section dalam Bahasa Indonesia akademik formal (kualitas jurnal). Istilah metodologi baku boleh tetap Inggris ('systematic review', 'PRISMA 2020', 'PICO', 'meta-analysis', nama tool RoB seperti RoB 2/NOS/MMAT). Judul karya & daftar referensi JANGAN diterjemahkan."
}

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

// artifactBundle merangkai artefak M2-M8 sebagai konteks TEKS RINGKAS (bukan JSON penuh)
// agar prompt kecil/fokus (penting untuk backend lambat seperti rprompt/claude headless).
func (m *M9Manuscript) artifactBundle(session *model.SLRSession) string {
	var b strings.Builder
	w := func(label, val string) {
		if strings.TrimSpace(val) != "" {
			fmt.Fprintf(&b, "## %s\n%s\n\n", label, val)
		}
	}
	b.WriteString("=== ARTEFAK SLR (sumber data; pakai angka & kutipan APA ADANYA) ===\n\n")
	w("TOPIC", session.Topic)

	if t := session.SelectedTopic; t != nil {
		w("GAP", fmt.Sprintf("%s | Tipe %s | %s", t.Name, t.Type, t.Gap))
	}
	if p := session.PICODefinitions; p != nil {
		w("PICO", fmt.Sprintf("P: %s\nI: %s\nC: %s\nO: %s\nCanonical term: %s — %s",
			p.P.Value, p.I.Value, p.C.Value, p.O.Value, p.CanonicalTerm.Term, p.CanonicalTerm.Definition))
	}
	if len(session.ResearchQuestions) > 0 {
		var rq strings.Builder
		for _, q := range session.ResearchQuestions {
			fmt.Fprintf(&rq, "- [%s] %s\n", q.Type, q.Question)
		}
		w("RESEARCH QUESTIONS", rq.String())
	}
	if pr := session.PriorReviewsMatrix; pr != nil {
		var s strings.Builder
		for _, r := range pr.Reviews {
			fmt.Fprintf(&s, "- %s | Scope: %s | Method: %s | Findings: %s | Limitations: %s | Selisih: %s\n",
				r.AuthorYear, r.Scope, r.Methodology, r.KeyFindings, r.Limitations, r.Selisih)
		}
		w("PRIOR REVIEWS", s.String())
	}
	if len(session.ScopeJustifications) > 0 {
		var s strings.Builder
		for _, sj := range session.ScopeJustifications {
			fmt.Fprintf(&s, "- %s: T=%s; M=%s; P=%s\n", sj.Name, sj.Theoretical, sj.Methodological, sj.Practical)
		}
		w("SCOPE JUSTIFICATIONS", s.String())
	}
	if sl := session.SearchLog; sl != nil {
		dates, _ := json.Marshal(sl.DateExecuted)
		hits, _ := json.Marshal(sl.TotalHits)
		w("SEARCH", fmt.Sprintf("String: %s\nDatabases: %s\nDate: %s | Hits: %s\nUpdate policy: %s",
			sl.SearchStringFinal, strings.Join(sl.Databases, ", "), string(dates), string(hits), sl.UpdatePolicy))
	}
	if et := session.ExclusionTable; et != nil {
		w("PRISMA FLOW + EXCLUSION + KAPPA", et.FlowNumbers+"\n\n"+et.KappaReport+"\n\n"+et.ExclusionReasons)
	}
	kappaTA := 0.0
	if n := len(session.KalibrasiLog); n > 0 {
		kappaTA = session.KalibrasiLog[n-1].Kappa
	}
	extK, robK := 0.0, 0.0
	if session.ExtractionLog != nil {
		extK = session.ExtractionLog.ExtractionKappa
	}
	if session.QAThreshold != nil {
		robK = session.QAThreshold.Kappa
	}
	w("KAPPA VALUES", fmt.Sprintf("κ_TA=%.3f | κ_FT=%.3f | κ_extract=%.3f | κ_rob=%.3f", kappaTA, session.FulltextKappa, extK, robK))

	if fw := session.FrameworkSelection; fw != nil {
		w("FRAMEWORK", fw.Framework+" — "+fw.Justification)
	}
	if el := session.ExtractionLog; el != nil {
		w("EXTRACTION", fmt.Sprintf("Total %d | verifikasi %d | disagreement %.1f%%", el.TotalExtracted, el.VerifiedSample, el.DisagreementRate))
	}
	if qa := session.QAThreshold; qa != nil {
		w("QUALITY APPRAISAL", fmt.Sprintf("Tool %s | threshold %.0f%% | %s | tool justification: %s", qa.Tool, qa.Threshold, qa.Categorization, qa.ToolJustification))
	}
	if s := session.SensitivityAnalysis; s != nil {
		w("SENSITIVITY", s.Verdict+" — "+s.Markdown)
	}
	if sp := session.SynthesisPathDecision; sp != nil {
		w("SYNTHESIS PATH", sp.Verdict+" — "+sp.Rationale)
	}
	if sr := session.SynthesisResults; sr != nil {
		w("SYNTHESIS RESULTS", sr.Markdown)
	}
	if g := session.GradeEvidence; g != nil {
		w("GRADE EVIDENCE", g.TableMarkdown+"\nRobustness: "+g.RobustnessVerdict+"\n"+g.ConfidenceStatements)
	}
	if d := session.DescriptiveAnalysis; d != nil {
		w("DESCRIPTIVE", d.Markdown+"\nHeterogeneity: "+d.HeterogeneityVerdict+"\n"+d.HeterogeneityNarrative)
	}
	if ip := session.InterpretationPackage; ip != nil {
		w("INTERPRETATION PACKAGE", ip.Markdown)
	}
	if ac := session.AcquisitionLog; ac != nil {
		w("ACQUISITION", fmt.Sprintf("Included %d | inaccessible %d (%.1f%%)", ac.TotalInclude, ac.InaccessibleCount, ac.InaccessiblePct))
	}
	if si := session.SLNAIntegration; si != nil {
		w("SLNA INTEGRATION", si.Markdown+"\nConvergent gaps: "+si.ConvergentGaps)
	}
	return b.String()
}
