//go:build ignore

// Probe end-to-end Modul 9 (Manuscript) — sesi BARU terisolasi (__m9_e2e_probe__).
// Menyemai artefak M2-M8 sintetis + 3 studi included ber-DOI NYATA (agar References Crossref
// tervalidasi), menjalankan GROUPA->GROUPB->COMPILE dengan auto-approve. Brain=rprompt(sonnet).
//
// Jalankan: go run scratch/m9_e2e_probe.go
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"nsa/internal/llm"
	"nsa/internal/model"
	"nsa/internal/orchestrator"
	"nsa/internal/repository"
)

const probeID = "__m9_e2e_probe__"

func writeResult(s string) { _ = os.WriteFile("scratch/m9_e2e_result.txt", []byte(s), 0644) }
func sval(p bson.M, keys ...string) string {
	for _, k := range keys {
		if v, ok := p[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func main() {
	_ = godotenv.Load()
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "slr_agentic_db"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	repo, err := repository.NewMongoRepository(uri, dbName)
	if err != nil {
		writeResult("❌ Mongo: " + err.Error())
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("✅ MongoDB (db=%s)\n", dbName)
	rawClient, _ := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	defer rawClient.Disconnect(ctx)
	sessColl := rawClient.Database(dbName).Collection("slr_sessions")
	scrColl := repo.GetScreeningCollection()

	okBrain := false
	for _, pid := range []string{"rprompt", "gemini"} {
		if c, e := repo.GetLLMConfig(ctx, pid); e == nil && c.APIKey != "" && !strings.HasPrefix(c.APIKey, "GANTI_DENGAN") {
			okBrain = true
			break
		}
	}
	if !okBrain {
		writeResult("⚠️ Brain belum dikonfigurasi. Probe M9 dibatalkan.")
		os.Exit(2)
	}
	fmt.Println("✅ Brain siap.")

	// Ambil 3 paper retrieved (DOI nyata) untuk referensi.
	cur, _ := scrColl.Find(ctx, bson.M{"full_text_retrieved": true}, options.Find().SetLimit(8))
	var cand []bson.M
	_ = cur.All(ctx, &cand)
	var src []bson.M
	for _, p := range cand {
		if sval(p, "DOI", "doi") != "" {
			src = append(src, p)
		}
		if len(src) >= 3 {
			break
		}
	}
	fmt.Printf("✅ %d studi ber-DOI untuk references.\n", len(src))

	_, _ = sessColl.DeleteOne(ctx, bson.M{"_id": probeID})
	_, _ = scrColl.DeleteMany(ctx, bson.M{"session_id": probeID})
	var docs []interface{}
	for _, p := range src {
		doi := sval(p, "DOI", "doi")
		docs = append(docs, bson.M{
			"session_id": probeID, "Title": sval(p, "Title", "title"), "Authors": sval(p, "Authors", "authors"),
			"Year": sval(p, "Year", "year"), "Journal": sval(p, "Journal", "journal"), "DOI": doi, "doi": doi,
			"full_text_retrieved": true, "Screener_1_Decision": "INCLUDE", "Final_Decision": "INCLUDE", "Final_Decision_Full": "INCLUDE",
		})
	}
	if len(docs) > 0 {
		_, _ = scrColl.InsertMany(ctx, docs)
	}

	// Sesi + artefak M2-M8 sintetis (ringkas tapi cukup).
	probe := &model.SLRSession{
		ID: probeID, Status: "M9_MANUSCRIPT",
		Topic: "Self-Regulated Learning in Online Higher Education",
		SelectedTopic: &model.SuggestedTopic{Name: "SRL in online learning", Gap: "Fragmentasi temuan SRL-performa daring", Type: "TIPE A"},
		PICODefinitions: &model.PICODefinitions{
			P: model.PICOElement{Value: "Mahasiswa pendidikan tinggi daring"},
			I: model.PICOElement{Value: "Strategi self-regulated learning"},
			C: model.PICOElement{Value: "no comparison / pembelajaran konvensional"},
			O: model.PICOElement{Value: "Performa akademik & engagement"},
			CanonicalTerm: model.CanonicalTerm{Term: "self-regulated learning", Definition: "proses regulasi kognisi-motivasi-perilaku oleh pembelajar"},
		},
		ResearchQuestions: []model.ResearchQuestion{
			{Type: "PRIMARY", Question: "Bagaimana SRL memengaruhi performa akademik pada pendidikan tinggi daring?"},
			{Type: "SECONDARY", Question: "Apa moderator kontekstual hubungan tersebut?"},
			{Type: "SECONDARY", Question: "Teori apa yang dominan dipakai?"},
		},
		PriorReviewsMatrix: &model.PriorReviewsMatrix{Reviews: []model.PriorReview{
			{AuthorYear: "Broadbent & Poon (2015)", Scope: "SRL online, 2004-2014", Methodology: "Narrative review, n=12", KeyFindings: "Beberapa strategi SRL terkait prestasi", Limitations: "Pra-2015, tanpa PRISMA", Selisih: "BEDA PERIODE + BEDA METODE", SynthesisNovelty: "Belum ada sintesis PRISMA pasca-2015"},
			{AuthorYear: "Wong et al. (2019)", Scope: "SRL intervensi", Methodology: "SLR, n=35", KeyFindings: "Prompts SRL efektif", Limitations: "Fokus intervensi, bukan framework", Selisih: "BEDA FOKUS"},
		}},
		ScopeJustifications: []model.ScopeJustification{{Name: "Rentang 2015-2024", Theoretical: "Era pasca-MOOC", Methodological: "Pasca-PRISMA 2015", Practical: "Relevansi pembelajaran daring pasca-COVID"}},
		SearchLog: &model.SearchLog{SearchStringFinal: `TITLE-ABS-KEY(("self-regulated learning") AND ("online learning") AND ("academic performance"))`, Databases: []string{"Scopus"}, DateExecuted: map[string]string{"Scopus": "2024-11-01"}, TotalHits: map[string]string{"Scopus": "312"}, UpdatePolicy: "Re-run bila >6 bulan"},
		KalibrasiLog: []model.KalibrasiIteration{{Iterasi: 1, Kappa: 0.78, Passed: true}},
		ExclusionTable: &model.ExclusionTable{
			FlowNumbers: "- Total records identified: 312\n- Duplicates removed: 41\n- Records screened: 271\n- Records excluded: 238\n- Included for full-text: 33",
			KappaReport: "Kalibrasi κ_TA=0.78; batch final κ=0.81; klasifikasi Substantial",
			ExclusionReasons: "| Reason | Count | % |\n|---|---|---|\n| I-NOMATCH | 120 | 50% |\n| O-NOMATCH | 70 | 29% |",
		},
		FrameworkSelection: &model.FrameworkSelection{Framework: "TCCM", Justification: "RQ tentang bagaimana konsep SRL beroperasi lintas konteks daring"},
		ExtractionLog:      &model.ExtractionLog{TotalExtracted: 18, VerifiedSample: 4, DisagreementRate: 5.5, ExtractionKappa: 0.80},
		QAThreshold:        &model.QAThresholdJustification{Tool: "MMAT", Threshold: 70, Kappa: 0.74, Categorization: "HIGH>=80 MODERATE 70-79 LOW<70"},
		SensitivityAnalysis: &model.SensitivityAnalysis{Verdict: "ROBUST", Markdown: "Findings stabil lintas threshold."},
		SynthesisPathDecision: &model.SynthesisPathDecision{Verdict: "JALUR A", Rationale: "Heterogenitas tinggi + outcome heterogen → narrative."},
		SynthesisResults:    &model.SynthesisResults{Path: "JALUR A", Markdown: "Sintesis tematik: SRL konsisten terkait performa (indicative); motivasi memediasi; feedback & scaffolding memperkuat."},
		GradeEvidence:       &model.GradeEvidence{TableMarkdown: "| Outcome | GRADE |\n|---|---|\n| SRL→performa | MODERATE |", RobustnessVerdict: "CONDITIONALLY ROBUST"},
		DescriptiveAnalysis: &model.DescriptiveAnalysis{Markdown: "18 studi; design cross-sectional dominan; geografis: Indonesia 40%, USA 25%, China 20%; kualitas HIGH 6/MOD 8/LOW 4.", HeterogeneityVerdict: "HIGH"},
		InterpretationPackage: &model.InterpretationPackage{Markdown: "Key findings: SRL↑performa; motivasi mediator; gap: peran AI/adaptive learning emerging; bias geografis Indonesia. Future: RQ AI-SRL, longitudinal, non-SEA."},
	}
	if err := repo.UpdateSession(ctx, probe); err != nil {
		writeResult("❌ buat sesi: " + err.Error())
		os.Exit(1)
	}
	fmt.Println("✅ Sesi probe + artefak disemai. Menjalankan M9 (brain rprompt-sonnet, agak lama)...")

	pipeline := orchestrator.NewSLRPipeline(repo, llm.NewLLMFactory(repo))
	finalStatus := ""
	for i := 0; i < 30; i++ {
		if err := pipeline.Execute(ctx, probeID); err != nil {
			finalStatus = "ERROR: " + err.Error()
			fmt.Println("❌", err)
			break
		}
		s, _ := repo.GetSession(ctx, probeID)
		finalStatus = s.Status
		fmt.Printf("→ iter %d: %s\n", i+1, s.Status)
		if strings.HasSuffix(s.Status, "_WAITING_APPROVAL") {
			s.Status = strings.TrimSuffix(s.Status, "_WAITING_APPROVAL") + "_APPROVED"
			_ = repo.UpdateSession(ctx, s)
			continue
		}
		if s.Status == "COMPLETED" || strings.Contains(s.Status, "ERROR") {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	final, _ := repo.GetSession(ctx, probeID)
	var sb strings.Builder
	fmt.Fprintf(&sb, "M9 E2E selesai. Status akhir: %s\n", finalStatus)
	if final != nil && final.Manuscript != nil {
		ms := final.Manuscript
		cl := func(s string) int { return len(s) }
		fmt.Fprintf(&sb, "Sections (char): Methods=%d Results=%d Discussion=%d Future=%d Intro=%d Concl=%d Abstract=%d Title=%d\n",
			cl(ms.Methods), cl(ms.Results), cl(ms.Discussion), cl(ms.FutureResearch), cl(ms.Introduction), cl(ms.Conclusions), cl(ms.Abstract), cl(ms.Title))
		fmt.Fprintf(&sb, "References=%d char | Bibtex=%d char | Final=%d char | Latex=%d char\n", cl(ms.References), cl(ms.Bibtex), cl(ms.Final), cl(ms.Latex))
		fmt.Fprintf(&sb, "Coherence audit=%d char | PRISMA checklist=%d char\n", cl(ms.CoherenceAudit), cl(ms.PrismaChecklist))
		ab := ms.Abstract
		if len(ab) > 500 {
			ab = ab[:500] + "…"
		}
		fmt.Fprintf(&sb, "\n--- Abstract (potongan) ---\n%s\n", ab)
	}
	fmt.Fprintf(&sb, "\n(Sesi probe '%s' DIPERTAHANKAN.)", probeID)
	out := sb.String()
	fmt.Println("\n================ HASIL ================")
	fmt.Println(out)
	writeResult(out)
}
