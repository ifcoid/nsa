package model

import "time"

type SuggestedTopic struct {
	Name       string `bson:"name" json:"name"`
	Gap        string `bson:"gap" json:"gap"`
	Type       string `bson:"type" json:"type"` // A, B, atau C
	TypeReason string `bson:"type_reason" json:"type_reason"`
	Evidence   string `bson:"evidence" json:"evidence"` // DOI / URL
	Importance string `bson:"importance" json:"importance"`
}

type PriorReview struct {
	AuthorYear       string `bson:"author_year" json:"author_year"`
	Scope            string `bson:"scope" json:"scope"`
	Methodology      string `bson:"methodology" json:"methodology"`
	KeyFindings      string `bson:"key_findings" json:"key_findings"`
	Limitations      string `bson:"limitations" json:"limitations"`
	Selisih          string `bson:"selisih" json:"selisih"` // BEDA POPULASI, dll
	SynthesisNovelty string `bson:"synthesis_novelty" json:"synthesis_novelty"`
}

type PriorReviewsMatrix struct {
	Reviews []PriorReview `bson:"reviews" json:"reviews"`
}

type OperationalDef struct {
	WhatCounts      string `bson:"what_counts" json:"what_counts"`
	WhatDoesntCount string `bson:"what_doesnt_count" json:"what_doesnt_count"`
	EdgeCases       string `bson:"edge_cases" json:"edge_cases"`
}

type PICOElement struct {
	Value          string         `bson:"value" json:"value"`
	OperationalDef OperationalDef `bson:"operational_def" json:"operational_def"`
}

type CanonicalTerm struct {
	Term         string `bson:"term" json:"term"`
	Definition   string `bson:"definition" json:"definition"`
	RejectedAlts string `bson:"rejected_alternatives" json:"rejected_alternatives"`
}

type PICODefinitions struct {
	P             PICOElement   `bson:"p" json:"p"`
	I             PICOElement   `bson:"i" json:"i"`
	C             PICOElement   `bson:"c" json:"c"`
	O             PICOElement   `bson:"o" json:"o"`
	CanonicalTerm CanonicalTerm `bson:"canonical_term" json:"canonical_term"`
}

type ScopeFilters struct {
	RentangTahun string `bson:"rentang_tahun" json:"rentang_tahun"`
	Geografis    string `bson:"geografis" json:"geografis"`
	Sektor       string `bson:"sektor" json:"sektor"`
	Bahasa       string `bson:"bahasa" json:"bahasa"`
	Lainnya      string `bson:"lainnya" json:"lainnya"`
}

type ScopeJustification struct {
	Name           string `bson:"name" json:"name"`
	Theoretical    string `bson:"theoretical" json:"theoretical"`
	Methodological string `bson:"methodological" json:"methodological"`
	Practical      string `bson:"practical" json:"practical"`
	Status         string `bson:"status" json:"status"`
}

type RQTraceability struct {
	Gap          string `bson:"gap" json:"gap"`
	PriorReviews string `bson:"prior_reviews" json:"prior_reviews"`
	PICO         string `bson:"pico" json:"pico"`
	Scope        string `bson:"scope" json:"scope"`
}

type ResearchQuestion struct {
	Type         string         `bson:"type" json:"type"` // "PRIMARY" or "SECONDARY"
	Question     string         `bson:"question" json:"question"`
	Traceability RQTraceability `bson:"traceability" json:"traceability"`
	IsOrphan     bool           `bson:"is_orphan" json:"is_orphan"`
}

type FINERCriteria struct {
	Feasible    string `bson:"feasible" json:"feasible"`
	Interesting string `bson:"interesting" json:"interesting"`
	Novel       string `bson:"novel" json:"novel"`
	Ethical     string `bson:"ethical" json:"ethical"`
	Relevant    string `bson:"relevant" json:"relevant"`
}

type CoherenceCheck struct {
	PICOAdequacy     string `bson:"pico_adequacy" json:"pico_adequacy"`
	ScopeFeasibility string `bson:"scope_feasibility" json:"scope_feasibility"`
	Terminology      string `bson:"terminology" json:"terminology"`
	Recommendation   string `bson:"recommendation" json:"recommendation"`
}

type FinerNoveltyCheck struct {
	FINER             FINERCriteria  `bson:"finer" json:"finer"`
	InternalCoherence CoherenceCheck `bson:"internal_coherence" json:"internal_coherence"`
	IsPass            bool           `bson:"is_pass" json:"is_pass"`
}

type Modul2Summary struct {
	Markdown string `bson:"markdown" json:"markdown"`
}

type DatabaseMatrixRow struct {
	Database         string `bson:"database" json:"database"`
	CoverageStrength string `bson:"coverage_strength" json:"coverage_strength"`
	Limitation       string `bson:"limitation" json:"limitation"`
	FitDenganTopik   string `bson:"fit_dengan_topik" json:"fit_dengan_topik"`
}

type DatabaseSelection struct {
	CekCoverageBidang string              `bson:"cek_coverage_bidang" json:"cek_coverage_bidang"`
	MatriksDatabase   []DatabaseMatrixRow `bson:"matriks_database" json:"matriks_database"`
	Decision          string              `bson:"decision" json:"decision"`
	JustifikasiFinal  string              `bson:"justifikasi_final" json:"justifikasi_final"`
}

type SLRSession struct {
	ID                  string               `bson:"_id,omitempty"`
	Topic               string               `bson:"topic"`
	SuggestedTopics     []SuggestedTopic     `bson:"suggested_topics,omitempty"`
	SelectedTopic       *SuggestedTopic      `bson:"selected_topic,omitempty"`
	PriorReviewsMatrix  *PriorReviewsMatrix  `bson:"prior_reviews_matrix,omitempty"`
	PICODefinitions     *PICODefinitions     `bson:"pico_definitions,omitempty"`
	ScopeFilters        *ScopeFilters        `bson:"scope_filters,omitempty"`
	ScopeJustifications []ScopeJustification `bson:"scope_justifications,omitempty"`
	ResearchQuestions   []ResearchQuestion   `bson:"research_questions,omitempty"`
	FinerNoveltyCheck   *FinerNoveltyCheck   `bson:"finer_novelty_check,omitempty"`
	Modul2Summary       *Modul2Summary       `bson:"modul2_summary,omitempty"`
	DatabaseSelection   *DatabaseSelection   `bson:"database_selection,omitempty"`
	InclusionCriteria   []string             `bson:"inclusion_criteria"`
	ExclusionCriteria []string          `bson:"exclusion_criteria"`
	Status            string            `bson:"status"`   // "INIT", "WAITING_APPROVAL", "APPROVED", "NEEDS_REVISION", "REJECTED"
	Feedback          string            `bson:"feedback"` // Catatan dari manusia jika butuh revisi
	UpdatedAt         time.Time         `bson:"updated_at"`
}

type Paper struct {
	ID        string `bson:"_id,omitempty"`
	SessionID string `bson:"session_id"`
	Title     string `bson:"title"`
	Abstract  string `bson:"abstract"`
	Status    string `bson:"status"` // "PENDING", "ACCEPT", "REJECT"
	Reason    string `bson:"reason"`
}
