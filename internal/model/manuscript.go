package model

// Manuscript = output Modul 9 (semua section + artefak akhir).
type Manuscript struct {
	// Sections (Markdown)
	Methods        string `bson:"methods" json:"methods"`
	Results        string `bson:"results" json:"results"`
	Discussion     string `bson:"discussion" json:"discussion"`
	FutureResearch string `bson:"future_research" json:"future_research"`
	Introduction   string `bson:"introduction" json:"introduction"`
	Conclusions    string `bson:"conclusions" json:"conclusions"`
	Abstract       string `bson:"abstract" json:"abstract"`
	Title          string `bson:"title" json:"title"`
	References      string `bson:"references" json:"references"`
	// Artefak akhir (L10)
	Bibtex          string `bson:"bibtex" json:"bibtex"`
	CoherenceAudit  string `bson:"coherence_audit" json:"coherence_audit"`
	PrismaChecklist string `bson:"prisma_checklist" json:"prisma_checklist"`
	PrismaFlow      string `bson:"prisma_flow" json:"prisma_flow"` // complete validated PRISMA 2020 flow (identification->inclusion)
	Final           string `bson:"final" json:"final"`   // manuscript_final.md
	Latex           string `bson:"latex" json:"latex"`   // manuscript_final.tex
	Summary         string `bson:"summary" json:"summary"` // modul9_summary
	// xAI provenance M9 (atribusi model + bukti verifikasi klaim neuro-symbolic).
	ModelUsed          string              `bson:"model_used,omitempty" json:"model_used,omitempty"`                     // nama model Brain penulis section
	ClaimVerifications []ClaimVerification `bson:"claim_verifications,omitempty" json:"claim_verifications,omitempty"` // hasil triangulasi 3-sumber per klaim
}

// ClaimVerification = bukti xAI: hasil triangulasi neuro-symbolic satu klaim manuskrip
// terhadap Qdrant (semantik) + Neo4j (knowledge graph) + MongoDB (ekstraksi). Klaim
// dianggap terverifikasi bila Sources>=2. Disimpan agar dapat diaudit/diekspor (Q1).
type ClaimVerification struct {
	Section        string `bson:"section,omitempty" json:"section,omitempty"`
	Claim          string `bson:"claim" json:"claim"`
	CitationKey    string `bson:"citation_key,omitempty" json:"citation_key,omitempty"`
	QdrantVerified bool   `bson:"qdrant_verified" json:"qdrant_verified"`
	Neo4jVerified  bool   `bson:"neo4j_verified" json:"neo4j_verified"`
	MongoVerified  bool   `bson:"mongo_verified" json:"mongo_verified"`
	Sources        int    `bson:"sources" json:"sources"`
}
