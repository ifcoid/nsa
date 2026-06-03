//go:build ignore

// Probe end-to-end Modul 8 (Analysis + Synthesis) — sesi BARU terisolasi (__m8_e2e_probe__).
// Menyemai slr_extraction sintetis (design/geo/tahun/kualitas/findings) agar descriptive +
// figur + sintesis berisi nyata, lalu menjalankan L1->L4 dengan auto-approve. Brain=gemini->rprompt.
// TIDAK menghapus sesi lain.
//
// Jalankan: go run scratch/m8_e2e_probe.go
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

const probeID = "__m8_e2e_probe__"

func writeResult(s string) { _ = os.WriteFile("scratch/m8_e2e_result.txt", []byte(s), 0644) }

func extDoc(title, year, design, geo, qaCat string, score float64, finding string) bson.M {
	return bson.M{
		"session_id":        probeID,
		"Title":             title,
		"Year":              year,
		"qa_final_category": qaCat,
		"qa_total_score":    score,
		"qa_rated":          true,
		"extracted":         true,
		"coverage":          "COMPLETE",
		"key_findings":      finding,
		"fields": bson.A{
			bson.M{"key": "Theory", "value": "Self-regulated learning", "evidence": "Intro", "status": "REPORTED"},
			bson.M{"key": "Methodology_Design", "value": design, "evidence": "Methods", "status": "REPORTED"},
			bson.M{"key": "Context_Geographic", "value": geo, "evidence": "Methods", "status": "REPORTED"},
		},
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	repo, err := repository.NewMongoRepository(uri, dbName)
	if err != nil {
		writeResult("❌ M8 E2E gagal konek Mongo: " + err.Error())
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("✅ MongoDB (db=%s)\n", dbName)
	rawClient, _ := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	defer rawClient.Disconnect(ctx)
	sessColl := rawClient.Database(dbName).Collection("slr_sessions")
	extColl := repo.GetExtractionCollection()

	// Brain (gemini atau fallback rprompt) harus tersedia.
	okBrain := false
	for _, pid := range []string{"gemini", "rprompt"} {
		if cfg, e := repo.GetLLMConfig(ctx, pid); e == nil && cfg.APIKey != "" && !strings.HasPrefix(cfg.APIKey, "GANTI_DENGAN") {
			okBrain = true
			break
		}
	}
	if !okBrain {
		msg := "⚠️ Provider brain (gemini/rprompt) belum dikonfigurasi. Probe M8 dibatalkan."
		writeResult(msg)
		fmt.Println(msg)
		os.Exit(2)
	}
	fmt.Println("✅ Brain provider siap.")

	// Bersihkan sisa probe sendiri + semai extraction sintetis.
	_, _ = sessColl.DeleteOne(ctx, bson.M{"_id": probeID})
	_, _ = extColl.DeleteMany(ctx, bson.M{"session_id": probeID})
	docs := []interface{}{
		extDoc("Study A: SRL & online learning", "2021", "Cross-sectional", "Indonesia", "HIGH", 85, "SRL berkorelasi positif dengan performa."),
		extDoc("Study B: RCT scaffolding", "2022", "RCT", "USA", "HIGH", 88, "Scaffolding meningkatkan outcome (efek sedang)."),
		extDoc("Study C: survey motivation", "2023", "Cross-sectional", "Indonesia", "MODERATE", 72, "Motivasi memediasi SRL-performa."),
		extDoc("Study D: quasi-experiment", "2023", "Quasi-experimental", "China", "MODERATE", 70, "Intervensi digital efektif pada subkelompok."),
		extDoc("Study E: small pilot", "2024", "Cross-sectional", "USA", "LOW", 55, "Temuan awal, sampel kecil tanpa power analysis."),
	}
	if _, err := extColl.InsertMany(ctx, docs); err != nil {
		writeResult("❌ InsertMany extraction: " + err.Error())
		os.Exit(1)
	}
	fmt.Printf("✅ %d slr_extraction sintetis disemai.\n", len(docs))

	// Sesi probe + artefak M7 minimal.
	probe := &model.SLRSession{
		ID:     probeID,
		Topic:  "E2E probe Modul 8 (Self-Regulated Learning in online education)",
		Status: "M8_SYNTHESIS",
		ResearchQuestions: []model.ResearchQuestion{
			{Type: "PRIMARY", Question: "Bagaimana self-regulated learning memengaruhi performa pada pembelajaran daring?"},
			{Type: "SECONDARY", Question: "Apa moderator kontekstual yang memengaruhi hubungan tersebut?"},
		},
		FrameworkSelection: &model.FrameworkSelection{Framework: "TCCM", Justification: "Probe"},
		QAThreshold:        &model.QAThresholdJustification{Tool: "MMAT", Threshold: 70, Kappa: 0.7},
		SynthesisPrep: &model.SynthesisPrep{
			HeterogeneityVerdict: "MODERATE",
			MetaFeasibility:      "JALUR A",
			CriteriaCheck:        "Hanya 5 studi, outcome heterogen.",
		},
	}
	if err := repo.UpdateSession(ctx, probe); err != nil {
		writeResult("❌ buat sesi: " + err.Error())
		os.Exit(1)
	}
	fmt.Println("✅ Sesi probe dibuat. Menjalankan M8 L1->L4 (auto-approve)...")

	pipeline := orchestrator.NewSLRPipeline(repo, llm.NewLLMFactory(repo))
	finalStatus := ""
	for i := 0; i < 40; i++ {
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
		if s.Status == "M9_MANUSCRIPT" || strings.Contains(s.Status, "ERROR") || s.Status == "COMPLETED" {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	final, _ := repo.GetSession(ctx, probeID)
	var sb strings.Builder
	fmt.Fprintf(&sb, "M8 E2E selesai. Status akhir: %s\n", finalStatus)
	if final.DescriptiveAnalysis != nil {
		fmt.Fprintf(&sb, "Figur SVG: %d | Heterogeneity: %s\n", len(final.DescriptiveAnalysis.Figures), final.DescriptiveAnalysis.HeterogeneityVerdict)
	}
	if final.SynthesisPathDecision != nil {
		fmt.Fprintf(&sb, "Synthesis path: %s\n", final.SynthesisPathDecision.Verdict)
	}
	if final.SynthesisResults != nil {
		fmt.Fprintf(&sb, "Synthesis results: %d char%s\n", len(final.SynthesisResults.Markdown),
			map[bool]string{true: " (+forest script)", false: ""}[final.SynthesisResults.ForestPlotScript != ""])
	}
	if final.GradeEvidence != nil {
		fmt.Fprintf(&sb, "GRADE robustness: %s\n", final.GradeEvidence.RobustnessVerdict)
	}
	if final.Modul8Summary != nil {
		mt := final.Modul8Summary.Markdown
		if len(mt) > 700 {
			mt = mt[:700] + "…"
		}
		fmt.Fprintf(&sb, "\n--- modul8_summary (potongan) ---\n%s\n", mt)
	}
	fmt.Fprintf(&sb, "\n(Sesi probe '%s' DIPERTAHANKAN untuk inspeksi.)", probeID)

	out := sb.String()
	fmt.Println("\n================ HASIL ================")
	fmt.Println(out)
	writeResult(out)
}
