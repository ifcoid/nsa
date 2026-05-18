package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"nsa/internal/llm"
	"nsa/internal/model"
	"nsa/internal/orchestrator"
	"nsa/internal/repository"
)

const (
	MongoURI     = "mongodb://localhost:27017"
	DatabaseName = "slr_agentic_db"
	SessionID    = "bci_active_learning_2026"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("====================================================")
	fmt.Println("        STARTING MULTI-AGENT SLR SYSTEM             ")
	fmt.Println("====================================================")

	// 1. Inisialisasi Repositori MongoDB
	mongoRepo, err := repository.NewMongoRepository(MongoURI, DatabaseName)
	if err != nil {
		log.Fatalf("❌ Gagal terhubung ke MongoDB: %v", err)
	}
	fmt.Println("✅ Berhasil terhubung ke MongoDB.")

	// 2. Pragmatis: Seed Data Konfigurasi LLM & Sesi Awal (Jika Belum Ada)
	// Ini memastikan aplikasi tidak error saat pertama kali dijalankan di database kosong
	seedInitialData(ctx, mongoRepo)

	// 3. Inisialisasi LLM Factory (Penyedia Otak AI Dinamis)
	llmFactory := llm.NewLLMFactory(mongoRepo)

	// 4. Inisialisasi Main Orchestrator (State Machine Pipeline)
	// Kita perbarui pipeline agar menerima factory dinamis
	pipeline := orchestrator.NewSLRPipeline(mongoRepo, llmFactory)

	// 5. Jalankan Siklus State Machine
	fmt.Printf("\n[Orchestrator] Mengeksekusi Pipeline untuk Sesi: %s...\n", SessionID)
	err = pipeline.Execute(ctx, SessionID)
	if err != nil {
		log.Fatalf("❌ Pipeline mengalami error saat eksekusi: %v", err)
	}

	fmt.Println("\n====================================================")
	fmt.Println("          PROSES ORCHESTRATOR SELESAI               ")
	fmt.Println("====================================================")
}

// seedInitialData bertugas mengisi data default ke MongoDB agar sistem portabel langsung jalan
func seedInitialData(ctx context.Context, repo *repository.MongoRepository) {
	// A. Seed Konfigurasi LLM (Contoh: DeepSeek & Gemini)
	// Kita gunakan trik GetLLMConfig, jika error (artinya data belum ada), kita buat baru
	_, err := repo.GetLLMConfig(ctx, "deepseek")
	if err != nil {
		fmt.Println("[Seeding] Menyiapkan data default provider 'deepseek' di MongoDB...")
		deepseekConfig := &model.LLMConfig{
			ID:           "deepseek",
			ProviderName: "deepseek",
			BaseURL:      "https://api.deepseek.com/v1",
			APIKey:       "GANTI_DENGAN_API_KEY_DEEPSEEK_ANDA", // Silakan ganti langsung di MongoDB nanti
			DefaultModel: "deepseek-chat",
			IsActive:     true,
			UpdatedAt:    time.Now(),
		}
		// Menyimpan langsung memanfaatkan method update (upsert) yang sudah kita buat
		_ = updateLLMConfigDirectly(ctx, repo, deepseekConfig)
	}

	_, err = repo.GetLLMConfig(ctx, "gemini")
	if err != nil {
		fmt.Println("[Seeding] Menyiapkan data default provider 'gemini' di MongoDB...")
		geminiConfig := &model.LLMConfig{
			ID:           "gemini",
			ProviderName: "gemini",
			BaseURL:      "https://generativelanguage.googleapis.com/v1beta",
			APIKey:       "GANTI_DENGAN_API_KEY_GEMINI_ANDA",
			DefaultModel: "gemini-2.5-flash",
			IsActive:     true,
			UpdatedAt:    time.Now(),
		}
		_ = updateLLMConfigDirectly(ctx, repo, geminiConfig)
	}

	// B. Seed Sesi Riset SLR Baru (Status: INIT)
	_, err = repo.GetSession(ctx, SessionID)
	if err != nil {
		fmt.Println("[Seeding] Membuat sesi riset baru dengan Status: 'INIT'...")
		newSession := &model.SLRSession{
			ID:     SessionID,
			Topic:  "Penggunaan Active Learning pada Machine Learning untuk klasifikasi data Brain-Computer Interface (BCI)",
			Status: "INIT",
		}
		err = repo.UpdateSession(ctx, newSession)
		if err != nil {
			log.Printf("Warn: Gagal seeding session awal: %v", err)
		}
	}
}

// updateLLMConfigDirectly bertugas menjembatani seeding data dari main ke repository
func updateLLMConfigDirectly(ctx context.Context, repo *repository.MongoRepository, config *model.LLMConfig) error {
	err := repo.UpdateLLMConfig(ctx, config)
	if err != nil {
		log.Printf("⚠️  Warn: Gagal melakukan seeding untuk provider %s: %v", config.ID, err)
		return err
	}

	fmt.Printf("🔹 [Seeding] Berhasil mengonfigurasi provider '%s' di MongoDB.\n", config.ID)
	return nil
}
