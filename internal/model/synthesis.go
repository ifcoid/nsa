package model

// ===== Modul 8: Analysis + Synthesis =====

type Figure struct {
	Name string `bson:"name" json:"name"`
	SVG  string `bson:"svg" json:"svg"`
	URL  string `bson:"url,omitempty" json:"url,omitempty"` // URL GitHub Pages bila dipublikasi
}

// DescriptiveAnalysis = output L1 (descriptive_analysis + figures + heterogeneity).
type DescriptiveAnalysis struct {
	Markdown               string   `bson:"markdown" json:"markdown"`
	Figures                []Figure `bson:"figures" json:"figures"`
	HeterogeneityVerdict   string   `bson:"heterogeneity_verdict" json:"heterogeneity_verdict"` // LOW/MODERATE/HIGH/VERY HIGH
	HeterogeneityNarrative string   `bson:"heterogeneity_narrative" json:"heterogeneity_narrative"`
}

// SynthesisPathDecision = output L2 fase 1 (synthesis_path_decision).
type SynthesisPathDecision struct {
	Verdict       string `bson:"verdict" json:"verdict"` // JALUR A / JALUR B / HYBRID
	CriteriaCheck string `bson:"criteria_check" json:"criteria_check"`
	Rationale     string `bson:"rationale" json:"rationale"`
}

// SynthesisResults = output L2 fase 2/3 (synthesis_results).
type SynthesisResults struct {
	Path             string `bson:"path" json:"path"`
	Markdown         string `bson:"markdown" json:"markdown"`
	ForestPlotScript string `bson:"forest_plot_script,omitempty" json:"forest_plot_script,omitempty"`
}

// GradeEvidence = output L3 (grade_evidence_table + robustness).
type GradeEvidence struct {
	TableMarkdown        string `bson:"table_markdown" json:"table_markdown"`
	RobustnessVerdict    string `bson:"robustness_verdict" json:"robustness_verdict"` // ROBUST / CONDITIONALLY ROBUST / NOT ROBUST
	RobustnessSummary    string `bson:"robustness_summary" json:"robustness_summary"`
	ConfidenceStatements string `bson:"confidence_statements" json:"confidence_statements"`
}

// InterpretationPackage = output L4 (untuk Modul 9).
type InterpretationPackage struct {
	Markdown string `bson:"markdown" json:"markdown"`
}

type Modul8Summary struct {
	Markdown string `bson:"markdown" json:"markdown"`
}
