package model

// ===== Modul 8b: Bibliometric / SLNA (opsional) =====

// BibliometricData = output L1 (data prep + thesaurus).
type BibliometricData struct {
	RecordsAnalyzed   int    `bson:"records_analyzed" json:"records_analyzed"`
	ThesaurusKeywords string `bson:"thesaurus_keywords" json:"thesaurus_keywords"` // format VOSviewer
	ThesaurusAuthors  string `bson:"thesaurus_authors" json:"thesaurus_authors"`
	Approach          string `bson:"approach" json:"approach"`
	LogMarkdown       string `bson:"log_markdown" json:"log_markdown"`
}

// VOSViewerParams = output L2 (9-parameter justification, siap-Methods).
type VOSViewerParams struct {
	TypeOfAnalysis string `bson:"type_of_analysis" json:"type_of_analysis"`
	UnitOfAnalysis string `bson:"unit_of_analysis" json:"unit_of_analysis"`
	TableMarkdown  string `bson:"table_markdown" json:"table_markdown"`
}

// ClusterInterpretation = output L3 (tier 1-4 + bridge + structural holes).
type ClusterInterpretation struct {
	Markdown      string `bson:"markdown" json:"markdown"`
	TableMarkdown string `bson:"table_markdown" json:"table_markdown"`
}

// SLNAIntegration = output L4 (validasi tema lintas-method + convergent gaps).
type SLNAIntegration struct {
	Markdown       string `bson:"markdown" json:"markdown"`
	ConvergentGaps string `bson:"convergent_gaps" json:"convergent_gaps"`
}

type ModulBibliometricSummary struct {
	Markdown string `bson:"markdown" json:"markdown"`
}
