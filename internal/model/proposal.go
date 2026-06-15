package model

import "time"

// ProposalSession represents a proposal writing session.
type ProposalSession struct {
	ID            string    `bson:"_id,omitempty" json:"id"`
	UserID        string    `bson:"user_id" json:"user_id"`
	Topic         string    `bson:"topic" json:"topic"`
	Status        string    `bson:"status" json:"status"`
	Feedback      string    `bson:"feedback" json:"feedback"`
	SystemError   string    `bson:"system_error" json:"system_error"`
	EmbedEndpoint string    `bson:"embed_endpoint" json:"embed_endpoint"`
	UpdatedAt     time.Time `bson:"updated_at" json:"updated_at"`
	CreatedAt     time.Time `bson:"created_at" json:"created_at"`
}

// ProposalRef represents a parsed reference from a BibTeX file for the proposal pipeline.
type ProposalRef struct {
	CiteKey    string `bson:"cite_key" json:"cite_key"`
	Title      string `bson:"title" json:"title"`
	Authors    string `bson:"authors" json:"authors"`
	Year       string `bson:"year" json:"year"`
	Journal    string `bson:"journal" json:"journal"`
	Abstract   string `bson:"abstract" json:"abstract"`
	DOI        string `bson:"doi" json:"doi"`
	Keywords   string `bson:"keywords" json:"keywords"`
	IsEmbedded bool   `bson:"is_embedded" json:"is_embedded"`
	ChunkCount int    `bson:"chunk_count" json:"chunk_count"`
	SessionID  string `bson:"session_id" json:"session_id"`
}

// ProposalGradingResult represents the grading of a proposal along a single dimension.
type ProposalGradingResult struct {
	Dimension   string   `bson:"dimension" json:"dimension"`
	Score       float64  `bson:"score" json:"score"`
	Level       string   `bson:"level" json:"level"`
	Weaknesses  []string `bson:"weaknesses" json:"weaknesses"`
	Suggestions []string `bson:"suggestions" json:"suggestions"`
}

// CitationValidationResult represents the outcome of validating citations in proposal text.
type CitationValidationResult struct {
	ValidatedText      string   `bson:"validated_text" json:"validated_text"`
	InvalidCitations   []string `bson:"invalid_citations" json:"invalid_citations"`
	MisattributedClaims []string `bson:"misattributed_claims" json:"misattributed_claims"`
}
