package modules

import (
	"context"
	"encoding/json"
	"fmt"

	"nsa/internal/agent"
	"nsa/internal/logger"
	"nsa/internal/model"
)

// ===== L3: GRADE evidence grading + robustness =====

func (m *M8Synthesis) runGradeL3(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 8.3] GRADE evidence grading + robustness checks...")
	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return fmt.Errorf("brain (M8 grade) gagal dimuat: %w", err)
	}

	synthJSON, _ := json.Marshal(map[string]interface{}{
		"path_decision":     session.SynthesisPathDecision,
		"synthesis_results": session.SynthesisResults,
	})
	qaJSON, _ := json.Marshal(map[string]interface{}{
		"qa_threshold":         session.QAThreshold,
		"sensitivity_analysis": session.SensitivityAnalysis,
	})

	grade, err := agent.NewSynthesisAgent(brain).Grade(ctx, string(synthJSON), string(qaJSON))
	if err != nil {
		return err
	}

	// Capture model name for xAI transparency.
	brainPrimary, _ := m.deps.LLMFactory.RoleProviders(ctx, "brain")
	cfgBrain, _ := m.deps.MongoRepo.GetLLMConfig(ctx, brainPrimary)
	var modelName string
	if cfgBrain != nil {
		modelName = cfgBrain.ProviderName
		if cfgBrain.DefaultModel != "" {
			modelName += " (" + cfgBrain.DefaultModel + ")"
		}
	} else {
		modelName = brainPrimary
	}
	grade.ModelUsed = modelName
	grade.SystemPrompt = agent.GradeSystemPrompt

	session.GradeEvidence = grade
	logger.Logf(session.ID, "   [System] GRADE selesai. Robustness: %s.\n", grade.RobustnessVerdict)
	session.Status = "M8_STEP3_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// ===== L4: interpretation package + modul8_summary =====

func (m *M8Synthesis) runInterpretationL4(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 8.4] Interpretation package + summary...")
	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return fmt.Errorf("brain (M8 interpretation) gagal dimuat: %w", err)
	}

	rqJSON, _ := json.Marshal(session.ResearchQuestions)
	bundle := map[string]interface{}{
		"research_questions": json.RawMessage(rqJSON),
		"synthesis_results":  session.SynthesisResults,
		"grade_evidence":     session.GradeEvidence,
		"data_summary":       json.RawMessage([]byte(m.dataSummaryJSON(ctx, session))),
	}
	if session.DescriptiveAnalysis != nil {
		bundle["heterogeneity"] = map[string]string{
			"verdict":   session.DescriptiveAnalysis.HeterogeneityVerdict,
			"narrative": session.DescriptiveAnalysis.HeterogeneityNarrative,
		}
	}
	bundleJSON, _ := json.Marshal(bundle)

	pkg, err := agent.NewSynthesisAgent(brain).Interpretation(ctx, string(bundleJSON))
	if err != nil {
		return err
	}

	// Capture model name for xAI transparency.
	brainPrimary, _ := m.deps.LLMFactory.RoleProviders(ctx, "brain")
	cfgBrain, _ := m.deps.MongoRepo.GetLLMConfig(ctx, brainPrimary)
	var modelName string
	if cfgBrain != nil {
		modelName = cfgBrain.ProviderName
		if cfgBrain.DefaultModel != "" {
			modelName += " (" + cfgBrain.DefaultModel + ")"
		}
	} else {
		modelName = brainPrimary
	}
	session.InterpretationPackage = &model.InterpretationPackage{
		Markdown:     pkg,
		ModelUsed:    modelName,
		SystemPrompt: agent.InterpretationSystemPrompt,
	}

	// modul8_summary
	het, robust, path := "-", "-", "-"
	if session.DescriptiveAnalysis != nil {
		het = session.DescriptiveAnalysis.HeterogeneityVerdict
	}
	if session.GradeEvidence != nil {
		robust = session.GradeEvidence.RobustnessVerdict
	}
	if session.SynthesisPathDecision != nil {
		path = session.SynthesisPathDecision.Verdict
	}
	gradeTable := ""
	if session.GradeEvidence != nil {
		gradeTable = session.GradeEvidence.TableMarkdown
	}
	descMd := ""
	if session.DescriptiveAnalysis != nil {
		descMd = session.DescriptiveAnalysis.Markdown
	}
	summary := fmt.Sprintf("=== ANALYSIS + SYNTHESIS PACKAGE (SLR) ===\n\n"+
		"%s\n\nHETEROGENEITY VERDICT: %s\n\nSYNTHESIS PATH: %s\n\n"+
		"GRADE EVIDENCE TABLE:\n%s\n\nROBUSTNESS VERDICT: %s\n\n"+
		"FORWARD ARTIFACTS (→ Modul 9): interpretation_package, grade_evidence_table, synthesis_results, figures (SVG), heterogeneity/geographic-bias disclosure.\n\n"+
		"NEXT: Manuscript Writing (Modul 9)",
		descMd, het, path, gradeTable, robust)
	session.Modul8Summary = &model.Modul8Summary{Markdown: summary}

	logger.Log(session.ID, "   [System] interpretation_package + modul8_summary tersimpan.")
	session.Status = "M8_STEP4_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}
