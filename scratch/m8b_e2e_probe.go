//go:build ignore

// Probe end-to-end Modul 8b (Bibliometric/SLNA) — sesi BARU terisolasi (__m8b_e2e_probe__).
// Menyemai slr_screening (Keywords) untuk thesaurus, menjalankan L1->L4 dengan auto-approve,
// dan mensimulasikan paste hasil VOSviewer di L2->L3. Brain=gemini->rprompt. Tidak menghapus sesi lain.
//
// Jalankan: go run scratch/m8b_e2e_probe.go
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

const probeID = "__m8b_e2e_probe__"

const vosPaste = `Total nodes: 84
Total edges: 312
Total clusters: 4
Top-3 clusters: Cluster 1 (n=28, "self-regulated learning"), Cluster 2 (n=21, "online learning engagement"), Cluster 3 (n=18, "motivation & metacognition")
Bridge nodes: "feedback", "scaffolding" (menghubungkan cluster 1 & 2)
Temporal (overlay): cluster 2 keywords mayoritas 2021-2024 (emerging); cluster 1 established 2015-2020; cluster 4 (n=6) peripheral.`

func writeResult(s string) { _ = os.WriteFile("scratch/m8b_e2e_result.txt", []byte(s), 0644) }

func scrDoc(title, kw string) bson.M {
	return bson.M{"session_id": probeID, "Title": title, "Keywords": kw}
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
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	repo, err := repository.NewMongoRepository(uri, dbName)
	if err != nil {
		writeResult("❌ M8b E2E gagal konek Mongo: " + err.Error())
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("✅ MongoDB (db=%s)\n", dbName)
	rawClient, _ := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	defer rawClient.Disconnect(ctx)
	sessColl := rawClient.Database(dbName).Collection("slr_sessions")
	scrColl := repo.GetScreeningCollection()

	okBrain := false
	for _, pid := range []string{"gemini", "rprompt"} {
		if cfg, e := repo.GetLLMConfig(ctx, pid); e == nil && cfg.APIKey != "" && !strings.HasPrefix(cfg.APIKey, "GANTI_DENGAN") {
			okBrain = true
			break
		}
	}
	if !okBrain {
		writeResult("⚠️ Brain (gemini/rprompt) belum dikonfigurasi. Probe M8b dibatalkan.")
		fmt.Println("brain not ready")
		os.Exit(2)
	}
	fmt.Println("✅ Brain provider siap.")

	_, _ = sessColl.DeleteOne(ctx, bson.M{"_id": probeID})
	_, _ = scrColl.DeleteMany(ctx, bson.M{"session_id": probeID})
	docs := []interface{}{
		scrDoc("A", "self-regulated learning; online learning; motivation; metacognition"),
		scrDoc("B", "SRL; engagement; scaffolding; feedback; online education"),
		scrDoc("C", "self regulated learning; academic performance; e-learning; AI"),
		scrDoc("D", "artificial intelligence; adaptive learning; motivation; MOOC"),
		scrDoc("E", "metacognition; feedback; self-efficacy; distance learning"),
		scrDoc("F", "machine learning; learning analytics; engagement; SRL"),
	}
	if _, err := scrColl.InsertMany(ctx, docs); err != nil {
		writeResult("❌ InsertMany screening: " + err.Error())
		os.Exit(1)
	}
	fmt.Printf("✅ %d slr_screening (Keywords) disemai.\n", len(docs))

	probe := &model.SLRSession{
		ID:     probeID,
		Topic:  "E2E probe Modul 8b (SLNA — SRL in online learning)",
		Status: "M8B_INIT",
		ResearchQuestions: []model.ResearchQuestion{
			{Type: "PRIMARY", Question: "Bagaimana SRL memengaruhi performa pada pembelajaran daring?"},
		},
		FrameworkSelection: &model.FrameworkSelection{Framework: "TCCM"},
		InterpretationPackage: &model.InterpretationPackage{
			Markdown: "Temuan SLR: (1) SRL berkorelasi positif dengan performa daring; (2) motivasi memediasi; (3) feedback & scaffolding memperkuat efek. Gap: peran AI/adaptive learning masih emerging.",
		},
	}
	if err := repo.UpdateSession(ctx, probe); err != nil {
		writeResult("❌ buat sesi: " + err.Error())
		os.Exit(1)
	}
	fmt.Println("✅ Sesi probe dibuat. Menjalankan M8b L1->L4...")

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

		if s.Status == "M8B_STEP2_WAITING_VOSVIEWER" {
			// Simulasikan user paste hasil VOSviewer.
			s.BibliometricInput = vosPaste
			s.Status = "M8B_STEP3_INTERPRET"
			_ = repo.UpdateSession(ctx, s)
			fmt.Println("   [probe] paste hasil VOSviewer (simulasi) -> M8B_STEP3_INTERPRET")
			continue
		}
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
	if final == nil {
		writeResult("Status akhir: " + finalStatus + " (gagal membaca sesi untuk laporan)")
		fmt.Println("Status akhir:", finalStatus)
		return
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "M8b E2E selesai. Status akhir: %s\n", finalStatus)
	if final.BibliometricData != nil {
		fmt.Fprintf(&sb, "Thesaurus: %d entri | records: %d | approach: %s\n",
			strings.Count(final.BibliometricData.ThesaurusKeywords, "\n"), final.BibliometricData.RecordsAnalyzed, final.BibliometricData.Approach)
	}
	if final.VOSViewerParams != nil {
		fmt.Fprintf(&sb, "VOS params: type=%s unit=%s\n", final.VOSViewerParams.TypeOfAnalysis, final.VOSViewerParams.UnitOfAnalysis)
	}
	if final.ClusterInterpretation != nil {
		fmt.Fprintf(&sb, "Cluster interpretation: %d char tabel\n", len(final.ClusterInterpretation.TableMarkdown))
	}
	if final.SLNAIntegration != nil {
		fmt.Fprintf(&sb, "SLNA integration: %d char | convergent gaps: %d char\n", len(final.SLNAIntegration.Markdown), len(final.SLNAIntegration.ConvergentGaps))
	}
	if final.ModulBibliometricSummary != nil {
		mt := final.ModulBibliometricSummary.Markdown
		if len(mt) > 600 {
			mt = mt[:600] + "…"
		}
		fmt.Fprintf(&sb, "\n--- modul_bibliometric_summary (potongan) ---\n%s\n", mt)
	}
	fmt.Fprintf(&sb, "\n(Sesi probe '%s' DIPERTAHANKAN.)", probeID)

	out := sb.String()
	fmt.Println("\n================ HASIL ================")
	fmt.Println(out)
	writeResult(out)
}
