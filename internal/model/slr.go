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

type PICOKeyword struct {
	Component        string   `bson:"component" json:"component"`
	CanonicalTerm    string   `bson:"canonical_term" json:"canonical_term"`
	MainSynonyms     []string `bson:"main_synonyms" json:"main_synonyms"`
	AlternativeTerms []string `bson:"alternative_terms" json:"alternative_terms"`
	AvoidList        []string `bson:"avoid_list" json:"avoid_list"`
	Reasoning        string   `bson:"reasoning,omitempty" json:"reasoning,omitempty"`
}

type KeywordsDevelopment struct {
	Population   PICOKeyword `bson:"population" json:"population"`
	Intervention PICOKeyword `bson:"intervention" json:"intervention"`
	Comparison   PICOKeyword `bson:"comparison" json:"comparison"`
	Outcome      PICOKeyword `bson:"outcome" json:"outcome"`
}

type AdaptedString struct {
	Database string `bson:"database" json:"database"`
	Query    string `bson:"query" json:"query"`
}

type FilterSpec struct {
	Filter        string `bson:"filter" json:"filter"`
	Value         string `bson:"value" json:"value"`
	Justification string `bson:"justification" json:"justification"`
}

type SearchStringData struct {
	ScopusQuery    string          `bson:"scopus_query" json:"scopus_query"`
	AdaptedStrings []AdaptedString `bson:"adapted_strings,omitempty" json:"adapted_strings,omitempty"`
	Filters        []FilterSpec    `bson:"filters" json:"filters"`
}

type SearchLog struct {
	SearchStringFinal string            `bson:"search_string_final" json:"search_string_final"`
	FiltersApplied    []FilterSpec      `bson:"filters_applied" json:"filters_applied"`
	Databases         []string          `bson:"databases" json:"databases"`
	DateExecuted      map[string]string `bson:"date_executed" json:"date_executed"`
	TotalHits         map[string]string `bson:"total_hits" json:"total_hits"`
	UpdatePolicy      string            `bson:"update_policy" json:"update_policy"`
}

type Modul3Summary struct {
	Markdown string `bson:"markdown" json:"markdown"`
}

type InitialSearchSample struct {
	Database            string   `bson:"database" json:"database"`
	TotalHitsPreFilter  string   `bson:"total_hits_pre_filter" json:"total_hits_pre_filter"`
	TotalHitsPostFilter string   `bson:"total_hits_post_filter" json:"total_hits_post_filter"`
	DateExecuted        string   `bson:"date_executed" json:"date_executed"`
	SampleTitles        []string `bson:"sample_titles" json:"sample_titles"`
	KeyPapersFound      []string `bson:"key_papers_found" json:"key_papers_found"`
	KeyPapersMissing    []string `bson:"key_papers_missing" json:"key_papers_missing"`
}

type SanityCheckVerdict struct {
	KeyPapersMissing []string `bson:"key_papers_missing" json:"key_papers_missing"`
	VolumeAnalysis   string   `bson:"volume_analysis" json:"volume_analysis"`
	Decision         string   `bson:"decision" json:"decision"` // PROCEED or REVISE_M3_STEP3
	Recommendation   string   `bson:"recommendation" json:"recommendation"`
}

type BasicQualityAudit struct {
	TotalRecords     int            `bson:"total_records" json:"total_records"`
	MissingAbstract  int            `bson:"missing_abstract" json:"missing_abstract"`
	MissingDOI       int            `bson:"missing_doi" json:"missing_doi"`
	YearDistribution map[string]int `bson:"year_distribution" json:"year_distribution"`
	DocTypes         map[string]int `bson:"doc_types" json:"doc_types"`
}

type DedupBreakdown struct {
	PrimaryMatch    int `bson:"primary_match" json:"primary_match"`
	SecondaryMatch  int `bson:"secondary_match" json:"secondary_match"`
	TotalDuplicates int `bson:"total_duplicates" json:"total_duplicates"`
	TotalUnique     int `bson:"total_unique" json:"total_unique"`
}

type PICOPreviewItem struct {
	Title          string `bson:"title" json:"title"`
	Classification string `bson:"classification" json:"classification"` // MATCH WHAT COUNTS, MATCH WHAT DOESN'T, AMBIGU, OFF-TOPIC
	Reasoning      string `bson:"reasoning" json:"reasoning"`
}

type PICOPreviewCheck struct {
	SamplesAnalyzed []PICOPreviewItem `bson:"samples_analyzed" json:"samples_analyzed"`
	MatchCountsPct  float64           `bson:"match_counts_pct" json:"match_counts_pct"`
	Verdict         string            `bson:"verdict" json:"verdict"`
	Recommendation  string            `bson:"recommendation" json:"recommendation"`
}

type ScreeningSetup struct {
	SearchDate    string   `bson:"search_date" json:"search_date"`
	PCanonical    string   `bson:"p_canonical" json:"p_canonical"`
	PWhatCounts   string   `bson:"p_what_counts" json:"p_what_counts"`
	PWhatDoesnt   string   `bson:"p_what_doesnt" json:"p_what_doesnt"`
	ICOWhatCounts string   `bson:"ico_what_counts" json:"ico_what_counts"`
	ReasonCodes   []string `bson:"reason_codes" json:"reason_codes"`
}

type Modul4Summary struct {
	Markdown string `bson:"markdown" json:"markdown"`
}

type ScreenerBriefing struct {
	ValidationGap  string `bson:"validation_gap" json:"validation_gap"`
	BriefingDoc    string `bson:"briefing_doc" json:"briefing_doc"`
	Decision       string `bson:"decision" json:"decision"` // PROCEED or REVISE_M2
	Recommendation string `bson:"recommendation" json:"recommendation"`
}

type KalibrasiIteration struct {
	Iterasi      int     `bson:"iterasi" json:"iterasi"`
	Tanggal      string  `bson:"tanggal" json:"tanggal"`
	Kappa        float64 `bson:"kappa" json:"kappa"`
	Revisi       string  `bson:"revisi" json:"revisi"`
	Passed       bool    `bson:"passed" json:"passed"`
	AgreementPct float64 `bson:"agreement_pct" json:"agreement_pct"`
}

type DataMiningLog struct {
	InitialSample InitialSearchSample `bson:"initial_sample" json:"initial_sample"`
	SanityCheck   *SanityCheckVerdict `bson:"sanity_check,omitempty" json:"sanity_check,omitempty"`
	QualityAudit  *BasicQualityAudit  `bson:"quality_audit,omitempty" json:"quality_audit,omitempty"`
	Dedup         *DedupBreakdown     `bson:"dedup,omitempty" json:"dedup,omitempty"`
	PICOPreview   *PICOPreviewCheck   `bson:"pico_preview,omitempty" json:"pico_preview,omitempty"`
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
	Keywords            *KeywordsDevelopment `bson:"keywords,omitempty"`
	SearchString        *SearchStringData    `bson:"search_string,omitempty"`
	SearchLog           *SearchLog           `bson:"search_log,omitempty"`
	Modul3Summary       *Modul3Summary       `bson:"modul3_summary,omitempty"`
	DataMiningLog       *DataMiningLog       `bson:"data_mining_log,omitempty"`
	ScreeningSetup      *ScreeningSetup      `bson:"screening_setup,omitempty"`
	Modul4Summary       *Modul4Summary       `bson:"modul4_summary,omitempty"`
	ScreenerBriefing    *ScreenerBriefing    `bson:"screener_briefing,omitempty"`
	KalibrasiLog        []KalibrasiIteration `bson:"kalibrasi_log,omitempty"`
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
