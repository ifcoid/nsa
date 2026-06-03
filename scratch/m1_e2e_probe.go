//go:build ignore

// Probe end-to-end Modul 1 terhadap MongoDB & gemini asli (dari .env + llm_providers).
// Membuat sesi uji bertanda, menjalankan pipeline sampai jeda, mencetak hasil 'foundation',
// lalu MENGHAPUS sesi uji tersebut agar tidak mengotori data.
//
// Jalankan: go run scratch/m1_e2e_probe.go
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

const probeSessionID = "__m1_e2e_probe__"

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

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	repo, err := repository.NewMongoRepository(uri, dbName)
	if err != nil {
		fmt.Printf("❌ Gagal konek MongoDB: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Terhubung ke MongoDB (db=%s)\n", dbName)

	// Klien langsung untuk cleanup sesi uji.
	rawClient, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		fmt.Printf("❌ Gagal membuat klien mongo untuk cleanup: %v\n", err)
		os.Exit(1)
	}
	defer rawClient.Disconnect(ctx)
	sessionsColl := rawClient.Database(dbName).Collection("slr_sessions")

	// 1. Pastikan key gemini asli (bukan placeholder seeding).
	cfg, err := repo.GetLLMConfig(ctx, "gemini")
	if err != nil {
		fmt.Printf("❌ Provider 'gemini' tidak ada di llm_providers: %v\n", err)
		os.Exit(1)
	}
	if cfg.APIKey == "" || strings.HasPrefix(cfg.APIKey, "GANTI_DENGAN") {
		fmt.Println("⚠️  Key 'gemini' di llm_providers masih placeholder/kosong.")
		fmt.Println("    Set key asli dulu agar Modul 1 bisa men-generate. Probe dihentikan (tanpa memanggil API, tanpa membuat sesi).")
		os.Exit(2)
	}
	fmt.Printf("✅ Provider gemini siap (model=%s, key=%s…)\n", cfg.DefaultModel, cfg.APIKey[:4])

	// 2. Bersihkan sisa probe lama lalu buat sesi uji INIT.
	_, _ = sessionsColl.DeleteOne(ctx, bson.M{"_id": probeSessionID})
	testSession := &model.SLRSession{
		ID:     probeSessionID,
		Topic:  "Penerapan Active Learning pada klasifikasi sinyal EEG Brain-Computer Interface (BCI)",
		Status: "INIT",
	}
	if err := repo.UpdateSession(ctx, testSession); err != nil {
		fmt.Printf("❌ Gagal membuat sesi uji: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Sesi uji dibuat (id=%s, status=INIT)\n\n", probeSessionID)

	// Cleanup di akhir apa pun yang terjadi.
	defer func() {
		_, derr := sessionsColl.DeleteOne(context.Background(), bson.M{"_id": probeSessionID})
		if derr != nil {
			fmt.Printf("\n⚠️  Gagal menghapus sesi uji (%s): %v — hapus manual di Compass.\n", probeSessionID, derr)
		} else {
			fmt.Printf("\n🧹 Sesi uji %s dihapus.\n", probeSessionID)
		}
	}()

	// 3. Jalankan pipeline langkah demi langkah sampai terminal (mirip ExecuteAsync, sinkron).
	factory := llm.NewLLMFactory(repo)
	pipeline := orchestrator.NewSLRPipeline(repo, factory)

	for i := 0; i < 8; i++ {
		if err := pipeline.Execute(ctx, probeSessionID); err != nil {
			fmt.Printf("❌ pipeline.Execute error: %v\n", err)
			break
		}
		s, err := repo.GetSession(ctx, probeSessionID)
		if err != nil {
			fmt.Printf("❌ Gagal baca sesi: %v\n", err)
			break
		}
		fmt.Printf("→ iterasi %d: status = %s\n", i+1, s.Status)
		if strings.Contains(s.Status, "WAITING") || strings.Contains(s.Status, "NEEDS_REVISION") ||
			strings.Contains(s.Status, "ERROR") || s.Status == "COMPLETED" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// 4. Tampilkan hasil foundation.
	final, _ := repo.GetSession(ctx, probeSessionID)
	fmt.Printf("\n================ HASIL ================\nStatus akhir: %s\n", final.Status)
	if final.Foundation == nil {
		fmt.Println("⚠️  session.foundation masih kosong.")
		return
	}
	f := final.Foundation
	fmt.Printf("Topik konteks: %s\n", f.TopicContext)
	preview := func(label, s string) {
		s = strings.TrimSpace(s)
		n := len(s)
		if n > 600 {
			s = s[:600] + "…"
		}
		fmt.Printf("\n----- %s (%d char) -----\n%s\n", label, n, s)
	}
	preview("theory_markdown (LLM)", f.TheoryMarkdown)
	preview("ai_practice_markdown (statik)", f.AIPracticeMarkdown)
	preview("global_rules_markdown (statik)", f.GlobalRulesMarkdown)
}
