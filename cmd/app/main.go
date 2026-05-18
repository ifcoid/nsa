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

	_, err = repo.GetLLMConfig(ctx, "zhipu")
	if err != nil {
		fmt.Println("[Seeding] Menyiapkan data default provider 'zhipu' (GLM) di MongoDB...")
		zhipuConfig := &model.LLMConfig{
			ID:           "zhipu",
			ProviderName: "openai-compatible",
			BaseURL:      "https://open.bigmodel.cn/api/paas/v4",
			APIKey:       "GANTI_DENGAN_API_KEY_ZHIPU_ANDA",
			DefaultModel: "glm-4",
			IsActive:     true,
			UpdatedAt:    time.Now(),
		}
		_ = updateLLMConfigDirectly(ctx, repo, zhipuConfig)
	}

	_, err = repo.GetLLMConfig(ctx, "groq")
	if err != nil {
		fmt.Println("[Seeding] Menyiapkan data default provider 'groq' di MongoDB...")
		groqConfig := &model.LLMConfig{
			ID:           "groq",
			ProviderName: "openai-compatible",
			BaseURL:      "https://api.groq.com/openai/v1",
			APIKey:       "GANTI_DENGAN_API_KEY_GROQ_ANDA",
			DefaultModel: "llama3-70b-8192",
			IsActive:     true,
			UpdatedAt:    time.Now(),
		}
		_ = updateLLMConfigDirectly(ctx, repo, groqConfig)
	}

	_, err = repo.GetLLMConfig(ctx, "openrouter")
	if err != nil {
		fmt.Println("[Seeding] Menyiapkan data default provider 'openrouter' di MongoDB...")
		openrouterConfig := &model.LLMConfig{
			ID:           "openrouter",
			ProviderName: "openai-compatible",
			BaseURL:      "https://openrouter.ai/api/v1",
			APIKey:       "GANTI_DENGAN_API_KEY_OPENROUTER_ANDA",
			DefaultModel: "anthropic/claude-3-5-sonnet",
			IsActive:     true,
			UpdatedAt:    time.Now(),
		}
		_ = updateLLMConfigDirectly(ctx, repo, openrouterConfig)
	}

	_, err = repo.GetLLMConfig(ctx, "claude")
	if err != nil {
		fmt.Println("[Seeding] Menyiapkan data default provider 'claude' (Anthropic) di MongoDB...")
		claudeConfig := &model.LLMConfig{
			ID:           "claude",
			ProviderName: "claude",
			BaseURL:      "https://api.anthropic.com/v1",
			APIKey:       "GANTI_DENGAN_API_KEY_ANTHROPIC_ANDA",
			DefaultModel: "claude-3-5-sonnet",
			IsActive:     true,
			UpdatedAt:    time.Now(),
		}
		_ = updateLLMConfigDirectly(ctx, repo, claudeConfig)
	}

	_, err = repo.GetLLMConfig(ctx, "mistral")
	if err != nil {
		fmt.Println("[Seeding] Menyiapkan data default provider 'mistral' di MongoDB...")
		mistralConfig := &model.LLMConfig{
			ID:           "mistral",
			ProviderName: "openai-compatible",
			BaseURL:      "https://api.mistral.ai/v1",
			APIKey:       "GANTI_DENGAN_API_KEY_MISTRAL_ANDA",
			DefaultModel: "open-mistral-7b",
			IsActive:     true,
			UpdatedAt:    time.Now(),
		}
		_ = updateLLMConfigDirectly(ctx, repo, mistralConfig)
	}

	_, err = repo.GetLLMConfig(ctx, "qwen")
	if err != nil {
		fmt.Println("[Seeding] Menyiapkan data default provider 'qwen' (Alibaba) di MongoDB...")
		qwenConfig := &model.LLMConfig{
			ID:           "qwen",
			ProviderName: "openai-compatible",
			BaseURL:      "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
			APIKey:       "GANTI_DENGAN_API_KEY_DASHSCOPE_ANDA",
			DefaultModel: "qwen-plus",
			IsActive:     true,
			UpdatedAt:    time.Now(),
		}
		_ = updateLLMConfigDirectly(ctx, repo, qwenConfig)
	}

	_, err = repo.GetLLMConfig(ctx, "github")
	if err != nil {
		fmt.Println("[Seeding] Menyiapkan data default provider 'github' (Models API) di MongoDB...")
		githubConfig := &model.LLMConfig{
			ID:           "github",
			ProviderName: "openai-compatible",
			BaseURL:      "https://models.inference.ai.azure.com",
			APIKey:       "GANTI_DENGAN_GITHUB_TOKEN_ANDA",
			DefaultModel: "gpt-4o-mini", // atau bisa diganti meta-llama-3-8b-instruct
			IsActive:     true,
			UpdatedAt:    time.Now(),
		}
		_ = updateLLMConfigDirectly(ctx, repo, githubConfig)
	}

	_, err = repo.GetLLMConfig(ctx, "nvidia")
	if err != nil {
		fmt.Println("[Seeding] Menyiapkan data default provider 'nvidia' (NIM) di MongoDB...")
		nvidiaConfig := &model.LLMConfig{
			ID:           "nvidia",
			ProviderName: "openai-compatible",
			BaseURL:      "https://integrate.api.nvidia.com/v1",
			APIKey:       "GANTI_DENGAN_API_KEY_NVIDIA_ANDA",
			DefaultModel: "meta/llama3-70b-instruct", // Model populer di Nvidia NIM
			IsActive:     true,
			UpdatedAt:    time.Now(),
		}
		_ = updateLLMConfigDirectly(ctx, repo, nvidiaConfig)
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
