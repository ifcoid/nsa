//go:build ignore

// Probe end-to-end Modul 7 (Extraction + QA) — sesi BARU terisolasi (__m7_e2e_probe__).
// Menyalin paper FINAL INCLUDED + tervektorisasi (punya konten RAG di Qdrant) ke sesi probe,
// lalu menjalankan L1->L4 dengan auto-approve di tiap gate HITL. TIDAK menghapus sesi lain.
//
// Jalankan: go run scratch/m7_e2e_probe.go
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
	"nsa/internal/modules"
	"nsa/internal/orchestrator"
	"nsa/internal/repository"
)

const probeID = "__m7_e2e_probe__"
const copyN = 2

func sval(p bson.M, keys ...string) string {
	for _, k := range keys {
		if v, ok := p[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
func normDOI(d string) string {
	d = strings.TrimPrefix(d, "https://doi.org/")
	d = strings.TrimPrefix(d, "http://doi.org/")
	return strings.ToLower(strings.TrimSpace(d))
}
func writeResult(s string) { _ = os.WriteFile("scratch/m7_e2e_result.txt", []byte(s), 0644) }

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
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	repo, err := repository.NewMongoRepository(uri, dbName)
	if err != nil {
		writeResult("❌ M7 E2E gagal konek Mongo: " + err.Error())
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("✅ MongoDB (db=%s)\n", dbName)
	rawClient, _ := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	defer rawClient.Disconnect(ctx)
	sessColl := rawClient.Database(dbName).Collection("slr_sessions")
	scrColl := repo.GetScreeningCollection()
	extColl := repo.GetExtractionCollection()

	// Provider wajib: gemini (brain), zhipu, groq.
	for _, pid := range []string{"gemini", "zhipu", "groq"} {
		cfg, e := repo.GetLLMConfig(ctx, pid)
		if e != nil || cfg.APIKey == "" || strings.HasPrefix(cfg.APIKey, "GANTI_DENGAN") {
			msg := fmt.Sprintf("⚠️ Provider '%s' belum dikonfigurasi. Probe M7 dibatalkan.", pid)
			writeResult(msg)
			fmt.Println(msg)
			os.Exit(2)
		}
	}
	fmt.Println("✅ Provider gemini, zhipu, groq siap.")

	// Cari paper tervektorisasi yang punya konten RAG.
	ftIndex, ragAvail, _ := modules.BuildFulltextIndex(ctx)
	if !ragAvail || len(ftIndex) == 0 {
		msg := "⚠️ Qdrant tidak tersedia / kosong. Probe M7 dibatalkan."
		writeResult(msg)
		fmt.Println(msg)
		os.Exit(2)
	}
	fmt.Printf("✅ Indeks RAG Qdrant: %d DOI berkonten.\n", len(ftIndex))

	cur, _ := scrColl.Find(ctx, bson.M{"full_text_retrieved": true}, options.Find().SetLimit(40))
	var cand []bson.M
	_ = cur.All(ctx, &cand)
	var src []bson.M
	for _, p := range cand {
		if c, ok := ftIndex[normDOI(sval(p, "DOI", "doi"))]; ok && strings.TrimSpace(c) != "" {
			src = append(src, p)
		}
		if len(src) >= copyN {
			break
		}
	}
	if len(src) == 0 {
		msg := "⚠️ Tidak ada paper retrieved yang punya konten RAG. Probe M7 dibatalkan."
		writeResult(msg)
		fmt.Println(msg)
		os.Exit(2)
	}
	srcSession := sval(src[0], "session_id")
	fmt.Printf("✅ %d paper berkonten RAG dipilih (dari sesi %s).\n", len(src), srcSession)

	// Bersihkan sisa probe SENDIRI.
	_, _ = sessColl.DeleteOne(ctx, bson.M{"_id": probeID})
	_, _ = scrColl.DeleteMany(ctx, bson.M{"session_id": probeID})
	_, _ = extColl.DeleteMany(ctx, bson.M{"session_id": probeID})

	// Salin sebagai FINAL INCLUDED.
	var docs []interface{}
	for _, p := range src {
		doi := sval(p, "DOI", "doi")
		docs = append(docs, bson.M{
			"session_id": probeID, "Title": sval(p, "Title", "title"),
			"Abstract": sval(p, "Abstract", "abstract"), "Keywords": sval(p, "Keywords", "keywords"),
			"Authors": sval(p, "Authors", "authors"), "Year": sval(p, "Year", "year"),
			"Journal": sval(p, "Journal", "journal"), "Document_Type": sval(p, "Document_Type", "document_type"),
			"DOI": doi, "doi": doi, "full_text_retrieved": true,
			"Screener_1_Decision": "INCLUDE", "Final_Decision": "INCLUDE", "Final_Decision_Full": "INCLUDE",
		})
	}
	if _, err := scrColl.InsertMany(ctx, docs); err != nil {
		writeResult("❌ InsertMany: " + err.Error())
		os.Exit(1)
	}
	fmt.Printf("✅ %d paper disalin ke sesi probe.\n", len(docs))

	// Sesi probe: status M7_EXTRACTION + PICO/RQ dari sumber.
	probe := &model.SLRSession{ID: probeID, Topic: "E2E probe Modul 7", Status: "M7_EXTRACTION"}
	if srcSession != "" {
		if ss, e := repo.GetSession(ctx, srcSession); e == nil && ss != nil {
			probe.PICODefinitions = ss.PICODefinitions
			probe.ResearchQuestions = ss.ResearchQuestions
		}
	}
	if err := repo.UpdateSession(ctx, probe); err != nil {
		writeResult("❌ buat sesi: " + err.Error())
		os.Exit(1)
	}
	fmt.Println("✅ Sesi probe dibuat. Menjalankan M7 L1->L4 (auto-approve)...")

	factory := llm.NewLLMFactory(repo)
	pipeline := orchestrator.NewSLRPipeline(repo, factory)

	finalStatus := ""
	for i := 0; i < 80; i++ {
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
			fmt.Printf("   [probe] auto-approve -> %s\n", s.Status)
			continue
		}
		if s.Status == "M8_SYNTHESIS" || strings.Contains(s.Status, "ERROR") || s.Status == "COMPLETED" {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	// Laporan.
	final, _ := repo.GetSession(ctx, probeID)
	var sb strings.Builder
	fmt.Fprintf(&sb, "M7 E2E selesai. Status akhir: %s\n", finalStatus)
	if final.FrameworkSelection != nil {
		fmt.Fprintf(&sb, "Framework: %s (%d kolom)\n", final.FrameworkSelection.Framework, len(final.FrameworkSelection.Columns))
	}
	if final.ExtractionLog != nil {
		fmt.Fprintf(&sb, "Extraction: %d paper | verifikasi %d | disagreement %.1f%%\n",
			final.ExtractionLog.TotalExtracted, final.ExtractionLog.VerifiedSample, final.ExtractionLog.DisagreementRate)
	}
	if final.QAThreshold != nil {
		fmt.Fprintf(&sb, "QA: tool %s | threshold %.0f%% | kappa %.3f\n", final.QAThreshold.Tool, final.QAThreshold.Threshold, final.QAThreshold.Kappa)
	}
	if final.SensitivityAnalysis != nil {
		fmt.Fprintf(&sb, "Sensitivity verdict: %s\n", final.SensitivityAnalysis.Verdict)
	}
	if final.SynthesisPrep != nil {
		fmt.Fprintf(&sb, "Heterogeneity: %s | Meta-analysis: %s\n", final.SynthesisPrep.HeterogeneityVerdict, final.SynthesisPrep.MetaFeasibility)
	}
	nExt, _ := extColl.CountDocuments(ctx, bson.M{"session_id": probeID})
	fmt.Fprintf(&sb, "Dokumen slr_extraction: %d\n", nExt)
	if final.Modul7Summary != nil {
		mtxt := final.Modul7Summary.Markdown
		if len(mtxt) > 800 {
			mtxt = mtxt[:800] + "…"
		}
		fmt.Fprintf(&sb, "\n--- modul7_summary (potongan) ---\n%s\n", mtxt)
	}
	fmt.Fprintf(&sb, "\n(Sesi probe '%s' DIPERTAHANKAN untuk inspeksi.)", probeID)

	out := sb.String()
	fmt.Println("\n================ HASIL ================")
	fmt.Println(out)
	writeResult(out)
}
