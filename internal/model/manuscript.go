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
}
