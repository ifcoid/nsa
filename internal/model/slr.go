package model

import (
	"encoding/json"
	"time"
)

// FoundationBriefing = output Modul 1 (Fondasi Teori + Aturan Global).
// Hybrid: bagian teori di-generate LLM (disesuaikan topik), sisanya kanonik statik.
type FoundationBriefing struct {
	TopicContext        string `bson:"topic_context" json:"topic_context"`                 // topik mentah yang dipakai menyusun briefing
	TheoryMarkdown      string `bson:"theory_markdown" json:"theory_markdown"`             // teori SLR (LLM, disesuaikan topik)
	AIPracticeMarkdown  string `bson:"ai_practice_markdown" json:"ai_practice_markdown"`   // etika & kapabilitas GenAI (statik)
	GlobalRulesMarkdown string `bson:"global_rules_markdown" json:"global_rules_markdown"` // aturan global SLR + CoWork (statik)
}

type SuggestedTopic struct {
	Name       string `bson:"name" json:"name"`
	Gap        string `bson:"gap" json:"gap"`
	Type       string `bson:"type" json:"type"` // A, B, atau C
	TypeReason string `bson:"type_reason" json:"type_reason"`
	Evidence   string   `bson:"evidence" json:"evidence"` // DOI / URL
	Importance string   `bson:"importance" json:"importance"`
	References []string `bson:"references" json:"references"`
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
	PreValidation  string          `bson:"pre_validation,omitempty" json:"pre_validation,omitempty"`
}

type SearchLog struct {
	SearchStringFinal string            `bson:"search_string_final" json:"search_string_final"`
	FiltersApplied    []FilterSpec      `bson:"filters_applied" json:"filters_applied"`
	Databases         []string          `bson:"databases" json:"databases"`
	DateExecuted      map[string]string `bson:"date_executed" json:"date_executed"`
	TotalHits         map[string]string `bson:"total_hits" json:"total_hits"`
	UpdatePolicy      string            `bson:"update_policy" json:"update_policy"`
}

// UnmarshalJSON adaptif untuk SearchLog. Menangani kasus dimana LLM halusinasi
// mengembalikan search_string_final atau total_hits sebagai JSON object (map) alih-alih tipe aslinya.
func (s *SearchLog) UnmarshalJSON(data []byte) error {
	// Parse ke map generik terlebih dahulu untuk menangani anomali tipe data
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// 1. SearchStringFinal
	if val, ok := raw["search_string_final"]; ok {
		switch v := val.(type) {
		case string:
			s.SearchStringFinal = v
		case map[string]interface{}:
			s.SearchStringFinal = "GABUNGAN (LLM Object Halusinasi):\n" + joinMapString(v)
		default:
			b, _ := json.Marshal(v)
			s.SearchStringFinal = string(b)
		}
	}

	// 2. FiltersApplied
	if val, ok := raw["filters_applied"]; ok {
		b, _ := json.Marshal(val)
		json.Unmarshal(b, &s.FiltersApplied)
	}

	// 3. Databases
	if val, ok := raw["databases"]; ok {
		b, _ := json.Marshal(val)
		json.Unmarshal(b, &s.Databases)
	}

	// 4. DateExecuted
	if val, ok := raw["date_executed"]; ok {
		if mapVal, isMap := val.(map[string]interface{}); isMap {
			s.DateExecuted = make(map[string]string)
			for k, v := range mapVal {
				if strVal, isStr := v.(string); isStr {
					s.DateExecuted[k] = strVal
				} else {
					b, _ := json.Marshal(v)
					s.DateExecuted[k] = string(b)
				}
			}
		}
	}

	// 5. TotalHits (Ini sering halusinasi menjadi objek bersarang { Scopus: { post_filter: 54 } })
	if val, ok := raw["total_hits"]; ok {
		if mapVal, isMap := val.(map[string]interface{}); isMap {
			s.TotalHits = make(map[string]string)
			for k, v := range mapVal {
				if strVal, isStr := v.(string); isStr {
					s.TotalHits[k] = strVal
				} else {
					// Jika LLM mereturn object (seperti { "post_filter": 54, "notes": "..." })
					b, _ := json.Marshal(v)
					s.TotalHits[k] = string(b)
				}
			}
		}
	}

	// 6. UpdatePolicy
	if val, ok := raw["update_policy"]; ok {
		if strVal, isStr := val.(string); isStr {
			s.UpdatePolicy = strVal
		} else {
			b, _ := json.Marshal(val)
			s.UpdatePolicy = string(b)
		}
	}

	return nil
}

// Helper lokal
func joinMapString(m map[string]interface{}) string {
	var res string
	for k, v := range m {
		res += "- [" + k + "]: "
		if strVal, ok := v.(string); ok {
			res += strVal + "\n"
		} else {
			b, _ := json.Marshal(v)
			res += string(b) + "\n"
		}
	}
	return res
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

type MissingDataDetail struct {
	Title    string `bson:"title" json:"title"`
	Database string `bson:"database" json:"database"`
}

type BasicQualityAudit struct {
	TotalRecords           int                 `bson:"total_records" json:"total_records"`
	MissingAbstract        int                 `bson:"missing_abstract" json:"missing_abstract"`
	MissingAbstractSources map[string]int      `bson:"missing_abstract_sources,omitempty" json:"missing_abstract_sources,omitempty"`
	MissingAbstractDetails []MissingDataDetail `bson:"missing_abstract_details,omitempty" json:"missing_abstract_details,omitempty"`
	MissingDOI             int                 `bson:"missing_doi" json:"missing_doi"`
	MissingDOIDetails      []MissingDataDetail `bson:"missing_doi_details,omitempty" json:"missing_doi_details,omitempty"`
	YearDistribution       map[string]int      `bson:"year_distribution" json:"year_distribution"`
	DocTypes               map[string]int      `bson:"doc_types" json:"doc_types"`
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
	Iterasi       int     `bson:"iterasi" json:"iterasi"`
	Tanggal       string  `bson:"tanggal" json:"tanggal"`
	TotalSample   int     `bson:"total_sample" json:"total_sample"`
	AgreeCount    int     `bson:"agree_count" json:"agree_count"`
	DisagreeCount int     `bson:"disagree_count" json:"disagree_count"`
	BothInclude   int     `bson:"both_include" json:"both_include"`
	BothExclude   int     `bson:"both_exclude" json:"both_exclude"`
	R1IncR2Exc    int     `bson:"r1_inc_r2_exc" json:"r1_inc_r2_exc"`
	R1ExcR2Inc    int     `bson:"r1_exc_r2_inc" json:"r1_exc_r2_inc"`
	PO            float64 `bson:"po" json:"po"`
	PE            float64 `bson:"pe" json:"pe"`
	Kappa         float64 `bson:"kappa" json:"kappa"`
	AgreementPct  float64 `bson:"agreement_pct" json:"agreement_pct"`
	Passed        bool    `bson:"passed" json:"passed"`
	Revisi        string  `bson:"revisi" json:"revisi"`
}

type ScreeningPerspective struct {
	PaperID    string `bson:"paper_id" json:"paper_id"`
	Title      string `bson:"title" json:"title"`
	Strict     string `bson:"strict" json:"strict"`
	Liberal    string `bson:"liberal" json:"liberal"`
	Recommend  string `bson:"recommend" json:"recommend"`
	ReasonCode string `bson:"reason_code" json:"reason_code"`
	Evidence   string `bson:"evidence" json:"evidence"`
	Confidence string `bson:"confidence" json:"confidence"`
}

type ScreeningResultsLog struct {
	BatchNumber       int     `bson:"batch_number" json:"batch_number"`
	ProcessedRecords  int     `bson:"processed_records" json:"processed_records"`
	CurrentKappa      float64 `bson:"current_kappa" json:"current_kappa"`
	DisagreementCases int     `bson:"disagreement_cases" json:"disagreement_cases"`
	DriftDetected     bool    `bson:"drift_detected" json:"drift_detected"`
	Tanggal           string  `bson:"tanggal" json:"tanggal"`
}

type Modul5Summary struct {
	Markdown string `bson:"markdown" json:"markdown"`
}

type ExclusionTable struct {
	FlowNumbers      string `bson:"flow_numbers" json:"flow_numbers"`
	ExclusionReasons string `bson:"exclusion_reasons" json:"exclusion_reasons"`
	KappaReport      string `bson:"kappa_report" json:"kappa_report"`
	PICOAudit        string `bson:"pico_audit" json:"pico_audit"`
	FullTextPrep     string `bson:"full_text_prep" json:"full_text_prep"`
}

type DataMiningLog struct {
	InitialSample InitialSearchSample `bson:"initial_sample" json:"initial_sample"`
	SanityCheck   *SanityCheckVerdict `bson:"sanity_check,omitempty" json:"sanity_check,omitempty"`
	QualityAudit  *BasicQualityAudit  `bson:"quality_audit,omitempty" json:"quality_audit,omitempty"`
	Dedup         *DedupBreakdown     `bson:"dedup,omitempty" json:"dedup,omitempty"`
	PICOPreview   *PICOPreviewCheck   `bson:"pico_preview,omitempty" json:"pico_preview,omitempty"`
}

type SLRSession struct {
	ID                  string               `bson:"_id,omitempty" json:"id"`
	Topic               string               `bson:"topic" json:"topic"`
	Foundation          *FoundationBriefing  `bson:"foundation,omitempty" json:"foundation,omitempty"`
	SuggestedTopics     []SuggestedTopic     `bson:"suggested_topics,omitempty" json:"suggested_topics,omitempty"`
	SelectedTopic       *SuggestedTopic      `bson:"selected_topic,omitempty" json:"selected_topic,omitempty"`
	PriorReviewsMatrix  *PriorReviewsMatrix  `bson:"prior_reviews_matrix,omitempty" json:"prior_reviews_matrix,omitempty"`
	PICODefinitions     *PICODefinitions     `bson:"pico_definitions,omitempty" json:"pico_definitions,omitempty"`
	ScopeFilters        *ScopeFilters        `bson:"scope_filters,omitempty" json:"scope_filters,omitempty"`
	ScopeJustifications []ScopeJustification `bson:"scope_justifications,omitempty" json:"scope_justifications,omitempty"`
	ResearchQuestions   []ResearchQuestion   `bson:"research_questions,omitempty" json:"research_questions,omitempty"`
	FinerNoveltyCheck   *FinerNoveltyCheck   `bson:"finer_novelty_check,omitempty" json:"finer_novelty_check,omitempty"`
	Modul2Summary       *Modul2Summary       `bson:"modul2_summary,omitempty" json:"modul2_summary,omitempty"`
	DatabaseSelection   *DatabaseSelection   `bson:"database_selection,omitempty" json:"database_selection,omitempty"`
	Keywords            *KeywordsDevelopment `bson:"keywords,omitempty" json:"keywords,omitempty"`
	SearchString        *SearchStringData    `bson:"search_string,omitempty" json:"search_string,omitempty"`
	SearchLog           *SearchLog           `bson:"search_log,omitempty" json:"search_log,omitempty"`
	Modul3Summary       *Modul3Summary       `bson:"modul3_summary,omitempty" json:"modul3_summary,omitempty"`
	DataMiningLog       *DataMiningLog       `bson:"data_mining_log,omitempty" json:"data_mining_log,omitempty"`
	ScreeningSetup        *ScreeningSetup        `bson:"screening_setup,omitempty" json:"screening_setup,omitempty"`
	Modul4Summary         *Modul4Summary         `bson:"modul4_summary,omitempty" json:"modul4_summary,omitempty"`
	ScreenerBriefing      *ScreenerBriefing      `bson:"screener_briefing,omitempty" json:"screener_briefing,omitempty"`
	KalibrasiLog          []KalibrasiIteration   `bson:"kalibrasi_log,omitempty" json:"kalibrasi_log,omitempty"`
	Reviewer1Perspectives []ScreeningPerspective `bson:"reviewer1_perspectives,omitempty" json:"reviewer1_perspectives,omitempty"`
	Reviewer2Perspectives []ScreeningPerspective `bson:"reviewer2_perspectives,omitempty" json:"reviewer2_perspectives,omitempty"`
	ScreeningResultsLog   []ScreeningResultsLog  `bson:"screening_results_log,omitempty" json:"screening_results_log,omitempty"`
	ExclusionTable        *ExclusionTable        `bson:"exclusion_table,omitempty" json:"exclusion_table,omitempty"`
	Modul5Summary         *Modul5Summary         `bson:"modul5_summary,omitempty" json:"modul5_summary,omitempty"`
	AcquisitionLog        *AcquisitionLog        `bson:"acquisition_log,omitempty" json:"acquisition_log,omitempty"`
	FulltextScreeningLog  []ScreeningResultsLog  `bson:"fulltext_screening_log,omitempty" json:"fulltext_screening_log,omitempty"`
	FulltextKappa         float64                `bson:"fulltext_kappa,omitempty" json:"fulltext_kappa,omitempty"`
	InaccessibleImpact    *InaccessibleImpact    `bson:"inaccessible_impact,omitempty" json:"inaccessible_impact,omitempty"`
	ExtractionReadiness   *ExtractionReadiness   `bson:"extraction_readiness,omitempty" json:"extraction_readiness,omitempty"`
	Modul6Summary         *Modul6Summary         `bson:"modul6_summary,omitempty" json:"modul6_summary,omitempty"`
	FrameworkSelection    *FrameworkSelection    `bson:"framework_selection,omitempty" json:"framework_selection,omitempty"`
	ExtractionLog         *ExtractionLog         `bson:"extraction_log,omitempty" json:"extraction_log,omitempty"`
	QAThreshold           *QAThresholdJustification `bson:"qa_threshold_justification,omitempty" json:"qa_threshold_justification,omitempty"`
	QACalibration         *QACalibration         `bson:"qa_calibration,omitempty" json:"qa_calibration,omitempty"`
	SensitivityAnalysis   *SensitivityAnalysis   `bson:"sensitivity_analysis,omitempty" json:"sensitivity_analysis,omitempty"`
	SynthesisPrep         *SynthesisPrep         `bson:"synthesis_prep,omitempty" json:"synthesis_prep,omitempty"`
	Modul7Summary         *Modul7Summary         `bson:"modul7_summary,omitempty" json:"modul7_summary,omitempty"`
	DescriptiveAnalysis   *DescriptiveAnalysis   `bson:"descriptive_analysis,omitempty" json:"descriptive_analysis,omitempty"`
	SynthesisPathDecision *SynthesisPathDecision `bson:"synthesis_path_decision,omitempty" json:"synthesis_path_decision,omitempty"`
	SynthesisResults      *SynthesisResults      `bson:"synthesis_results,omitempty" json:"synthesis_results,omitempty"`
	GradeEvidence         *GradeEvidence         `bson:"grade_evidence_table,omitempty" json:"grade_evidence_table,omitempty"`
	InterpretationPackage *InterpretationPackage `bson:"interpretation_package,omitempty" json:"interpretation_package,omitempty"`
	Modul8Summary         *Modul8Summary         `bson:"modul8_summary,omitempty" json:"modul8_summary,omitempty"`
	BibliometricData      *BibliometricData      `bson:"bibliometric_data,omitempty" json:"bibliometric_data,omitempty"`
	VOSViewerParams       *VOSViewerParams       `bson:"vosviewer_parameters,omitempty" json:"vosviewer_parameters,omitempty"`
	BibliometricInput     string                 `bson:"bibliometric_input,omitempty" json:"bibliometric_input,omitempty"`
	ClusterInterpretation *ClusterInterpretation `bson:"cluster_interpretation,omitempty" json:"cluster_interpretation,omitempty"`
	SLNAIntegration       *SLNAIntegration       `bson:"slna_integration,omitempty" json:"slna_integration,omitempty"`
	ModulBibliometricSummary *ModulBibliometricSummary `bson:"modul_bibliometric_summary,omitempty" json:"modul_bibliometric_summary,omitempty"`
	Manuscript            *Manuscript            `bson:"manuscript,omitempty" json:"manuscript,omitempty"`
	ManuscriptLang        string                 `bson:"manuscript_lang,omitempty" json:"manuscript_lang,omitempty"` // "id" (default, draft) atau "en" (submission)
	InclusionCriteria     []string               `bson:"inclusion_criteria" json:"inclusion_criteria"`
	ExclusionCriteria []string          `bson:"exclusion_criteria" json:"exclusion_criteria"`
	Status            string            `bson:"status" json:"status"`   // "INIT", "WAITING_APPROVAL", "APPROVED", "NEEDS_REVISION", "REJECTED"
	Feedback          string            `bson:"feedback" json:"feedback"` // Catatan dari manusia jika butuh revisi
	SystemError       string            `bson:"system_error" json:"system_error"` // Pesan error dari mesin/pipeline
	EmbedError        string            `bson:"embed_error,omitempty" json:"embed_error,omitempty"` // alasan pause di M6_STEP2_WAITING_EMBED (endpoint embedding mati)
	UpdatedAt         time.Time         `bson:"updated_at" json:"updated_at"`
}

type Paper struct {
	ID           string `bson:"_id,omitempty" json:"id"`
	SessionID    string `bson:"session_id" json:"session_id"`
	Title        string `bson:"title" json:"title"`
	Abstract     string `bson:"abstract" json:"abstract"`
	DOI          string `bson:"doi" json:"doi"`
	Year         string `bson:"year" json:"year"`
	Authors      string `bson:"authors" json:"authors"`
	Database     string `bson:"database" json:"database"` // e.g. "Scopus", "IEEE", "PubMed"
	Journal      string `bson:"journal" json:"journal"`
	DocumentType string `bson:"document_type" json:"document_type"`
	Status       string `bson:"status" json:"status"` // "PENDING", "ACCEPT", "REJECT"
	Reason       string `bson:"reason" json:"reason"`

	// Modul 6: Full-text Acquisition Tracking
	FullTextLocation         string `bson:"full_text_location" json:"full_text_location"`                   // "unpaywall", "arxiv", "hitl download"
	DownloadURL              string `bson:"download_url" json:"download_url"`                               // URL to download PDF
	FullTextRetrieved        bool   `bson:"full_text_retrieved" json:"full_text_retrieved"`                 // Verified in Qdrant
	AcquisitionDate          string `bson:"acquisition_date" json:"acquisition_date"`                       // Date of retrieval
	Inaccessible             bool   `bson:"inaccessible" json:"inaccessible"`                               // User marked as inaccessible
	DocumentationInaccessible string `bson:"documentation_inaccessible" json:"documentation_inaccessible"` // Reason for failure
}

// ===== Modul 6 Langkah 2-3: Full-text Screening =====

// FulltextImpact = OUTPUT 2 (inaccessible_impact) Modul 6 L3.
type InaccessibleImpact struct {
	Count    int     `bson:"count" json:"count"`
	Pct      float64 `bson:"pct" json:"pct"`
	Markdown string  `bson:"markdown" json:"markdown"`
}

// ExtractionReadiness = OUTPUT 3 Modul 6 L3 (checklist sebelum Modul 7).
type ExtractionReadiness struct {
	AllReady bool   `bson:"all_ready" json:"all_ready"`
	Markdown string `bson:"markdown" json:"markdown"`
}

// Modul6Summary = OUTPUT 4 Modul 6 L3 (hasil akhir).
type Modul6Summary struct {
	Markdown string `bson:"markdown" json:"markdown"`
}

// ===== Modul 7: Data Extraction + QA =====

type FrameworkColumn struct {
	Key      string `bson:"key" json:"key"`
	Category string `bson:"category" json:"category"` // Meta/T/C/Ch/M/Output/QA
	Desc     string `bson:"desc" json:"desc"`
}

type FrameworkSelection struct {
	Framework     string            `bson:"framework" json:"framework"` // TCCM / ADO / PICO / CUSTOM
	Justification string            `bson:"justification" json:"justification"`
	Columns       []FrameworkColumn `bson:"columns" json:"columns"`
	SystemPrompt  string            `bson:"system_prompt,omitempty" json:"system_prompt,omitempty"`
	UserPrompt    string            `bson:"user_prompt,omitempty" json:"user_prompt,omitempty"`
	ModelUsed     string            `bson:"model_used,omitempty" json:"model_used,omitempty"`
}

// ExtractionLog = log L2 (progress + verifikasi spot-check).
type ExtractionLog struct {
	TotalExtracted      int     `bson:"total_extracted" json:"total_extracted"`
	VerifiedSample      int     `bson:"verified_sample" json:"verified_sample"`
	DisagreementRate    float64 `bson:"disagreement_rate" json:"disagreement_rate"`
	AmbiguousCount      int     `bson:"ambiguous_count" json:"ambiguous_count"`
	NRNote              string  `bson:"nr_note" json:"nr_note"`
	ExtractionKappa     float64 `bson:"extraction_kappa,omitempty" json:"extraction_kappa,omitempty"`
	SystemPrompt        string  `bson:"system_prompt,omitempty" json:"system_prompt,omitempty"`
	ModelExtraction     string  `bson:"model_extraction,omitempty" json:"model_extraction,omitempty"`
	ModelRefineProtocol string  `bson:"model_refine_protocol,omitempty" json:"model_refine_protocol,omitempty"`
}

// QAAnchorExample = contoh sintetis anchor untuk kalibrasi QA.
type QAAnchorExample struct {
	Category    string  `bson:"category" json:"category"`       // HIGH / MODERATE / LOW
	Description string  `bson:"description" json:"description"` // synthetic paper description
	Score       float64 `bson:"score" json:"score"`             // expected score
	Reasoning   string  `bson:"reasoning" json:"reasoning"`     // why this category
}

// QACalibrationPilot = hasil rating pilot untuk satu paper.
type QACalibrationPilot struct {
	PaperID       string  `bson:"paper_id" json:"paper_id"`
	Title         string  `bson:"title" json:"title"`
	R1Score       float64 `bson:"r1_score" json:"r1_score"`
	R1Category    string  `bson:"r1_category" json:"r1_category"`
	R2Score       float64 `bson:"r2_score" json:"r2_score"`
	R2Category    string  `bson:"r2_category" json:"r2_category"`
	FinalCategory string  `bson:"final_category" json:"final_category"`
	Disagreement  bool    `bson:"disagreement" json:"disagreement"`
}

// QACalibration = state kalibrasi QA (anchor + pilot batch + kappa check).
type QACalibration struct {
	Anchors           []QAAnchorExample    `bson:"anchors" json:"anchors"`
	PilotResults      []QACalibrationPilot `bson:"pilot_results" json:"pilot_results"`
	PilotKappa        float64              `bson:"pilot_kappa" json:"pilot_kappa"`
	CalibrationPassed bool                 `bson:"calibration_passed" json:"calibration_passed"`
	Attempts          int                  `bson:"attempts" json:"attempts"`
	MaxAttempts       int                  `bson:"max_attempts" json:"max_attempts"` // default 3
	RefinementNote    string               `bson:"refinement_note,omitempty" json:"refinement_note,omitempty"`
	// Transparency fields: model names used during calibration.
	R1Model      string `bson:"r1_model,omitempty" json:"r1_model,omitempty"`           // model name used for Rater 1
	R2Model      string `bson:"r2_model,omitempty" json:"r2_model,omitempty"`           // model name used for Rater 2
	BrainModel   string `bson:"brain_model,omitempty" json:"brain_model,omitempty"`     // model name used for Brain (anchors + refinement)
	SystemPrompt string `bson:"system_prompt,omitempty" json:"system_prompt,omitempty"` // the system prompt used for raters in this calibration round
	ActionItems  string `bson:"action_items,omitempty" json:"action_items,omitempty"`   // what happens on retry (derived from refinement_note)
}

// QAKappaDetails = transparansi detail kesepakatan R1 dan R2.
type QAKappaDetails struct {
	TotalRated   int `bson:"total_rated" json:"total_rated"`
	BothPass     int `bson:"both_pass" json:"both_pass"`
	BothFail     int `bson:"both_fail" json:"both_fail"`
	R1PassR2Fail int `bson:"r1_pass_r2_fail" json:"r1_pass_r2_fail"`
	R1FailR2Pass int `bson:"r1_fail_r2_pass" json:"r1_fail_r2_pass"`
}

// QAThresholdJustification = output L3 (tool + threshold 3-lapis + kappa).
type QAThresholdJustification struct {
	Tool             string          `bson:"tool" json:"tool"`
	ToolJustification string         `bson:"tool_justification" json:"tool_justification"`
	QARubric         string          `bson:"qa_rubric,omitempty" json:"qa_rubric,omitempty"` // operational scoring rubric per domain
	Threshold        float64         `bson:"threshold" json:"threshold"`
	LayerLiterature  string          `bson:"layer_literature" json:"layer_literature"`
	LayerDeveloper   string          `bson:"layer_developer" json:"layer_developer"`
	LayerFeasibility string          `bson:"layer_feasibility" json:"layer_feasibility"`
	Categorization   string          `bson:"categorization" json:"categorization"`
	Kappa            float64         `bson:"kappa" json:"kappa"`
	KappaDetails     *QAKappaDetails `bson:"kappa_details,omitempty" json:"kappa_details,omitempty"`
	// Point 1: Actual feasibility calculated from rated data (post-rating).
	ActualFeasibility     float64  `bson:"actual_feasibility,omitempty" json:"actual_feasibility,omitempty"`
	ActualFeasibilityNote string   `bson:"actual_feasibility_note,omitempty" json:"actual_feasibility_note,omitempty"`
	// Point 4: Literature grounding from known tool cutoffs.
	LiteratureReferences   []string `bson:"literature_references,omitempty" json:"literature_references,omitempty"`
	ThresholdDeviationNote string   `bson:"threshold_deviation_note,omitempty" json:"threshold_deviation_note,omitempty"`
}

type SensitivityScenario struct {
	Name      string `bson:"name" json:"name"`
	Threshold string `bson:"threshold" json:"threshold"`
	NIncluded int    `bson:"n_included" json:"n_included"`
	Findings  string `bson:"findings" json:"findings"`
}

// SensitivityAnalysis = output L3 fase 4 (sensitivity_analysis).
type SensitivityAnalysis struct {
	Scenarios []SensitivityScenario `bson:"scenarios" json:"scenarios"`
	Verdict   string                `bson:"verdict" json:"verdict"` // ROBUST / CONDITIONALLY ROBUST / SENSITIVE
	Reasoning string                `bson:"reasoning,omitempty" json:"reasoning,omitempty"`
	Markdown  string                `bson:"markdown" json:"markdown"`
}

// SynthesisPrep = output L4 (synthesis_prep, input Modul 8).
type SynthesisPrep struct {
	DescriptiveOverview  string `bson:"descriptive_overview" json:"descriptive_overview"`
	HeterogeneityVerdict string `bson:"heterogeneity_verdict" json:"heterogeneity_verdict"` // LOW/MODERATE/HIGH/VERY HIGH
	MetaFeasibility      string `bson:"meta_feasibility" json:"meta_feasibility"`           // JALUR A / JALUR B / HYBRID
	CriteriaCheck        string `bson:"criteria_check" json:"criteria_check"`
	Groupings            string `bson:"groupings" json:"groupings"`
	Markdown             string `bson:"markdown" json:"markdown"`
}

type Modul7Summary struct {
	Markdown string `bson:"markdown" json:"markdown"`
}

type AcquisitionLog struct {
	TotalInclude       int     `bson:"total_include" json:"total_include"`
	HighRetrieved      int     `bson:"high_retrieved" json:"high_retrieved"` // from unpaywall/arxiv OA
	MediumRetrieved    int     `bson:"medium_retrieved" json:"medium_retrieved"` // from hitl download
	LowRetrieved       int     `bson:"low_retrieved" json:"low_retrieved"` // if we want to differentiate
	InaccessibleCount  int     `bson:"inaccessible_count" json:"inaccessible_count"`
	InaccessiblePct    float64 `bson:"inaccessible_pct" json:"inaccessible_pct"`
	VectorizedCount    int     `bson:"vectorized_count" json:"vectorized_count"` // found in qdrant
}
