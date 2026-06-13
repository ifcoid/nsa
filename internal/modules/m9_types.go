package modules

// ExtractionPaperData holds per-paper extraction data from the slr_extraction collection.
type ExtractionPaperData struct {
	DOI         string
	Title       string
	Authors     string
	Year        string
	Journal     string
	KeyFindings string
	Fields      []ExtractedField
	QARedFlags  string
}

// ExtractedField represents a single extracted field from M7 data extraction.
type ExtractedField struct {
	Key      string
	Value    string
	Evidence string
	Status   string
}

// PaperCitation represents a cited paper with a generated citation key.
type PaperCitation struct {
	Key     string
	Authors string
	Title   string
	Year    string
	Journal string
	DOI     string
}

// SemanticResult holds one result from Qdrant semantic search.
type SemanticResult struct {
	DOI     string
	Title   string
	Score   float64
	Snippet string
}

// VerificationResult tracks multi-source verification of a claim.
type VerificationResult struct {
	Claim          string
	CitationKey    string
	QdrantVerified bool
	Neo4jVerified  bool
	MongoVerified  bool
	Sources        int
}
