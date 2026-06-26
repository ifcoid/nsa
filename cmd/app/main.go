package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"

	httpapi "nsa/internal/delivery/http"
	"nsa/internal/llm"
	"nsa/internal/model"
	"nsa/internal/orchestrator"
	"nsa/internal/repository"

	"aidanwoods.dev/go-paseto"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	fmt.Println("====================================================")
	fmt.Println("        STARTING MULTI-AGENT SLR SYSTEM             ")
	fmt.Println("====================================================")

	// Load file .env jika ada
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  Info: File .env tidak ditemukan, menggunakan environment OS bawaan.")
	}

	ensurePasetoKeys()

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "slr_agentic_db"
	}

	sessionID := os.Getenv("SESSION_ID")
	if sessionID == "" {
		log.Println("⚠️  Info: SESSION_ID tidak ditemukan di file .env. Sistem tidak akan membuat sesi default.")
	}

	// 1. Inisialisasi Repositori MongoDB
	mongoRepo, err := repository.NewMongoRepository(mongoURI, dbName)
	if err != nil {
		log.Fatalf("❌ Gagal terhubung ke MongoDB: %v", err)
	}
	fmt.Println("✅ Berhasil terhubung ke MongoDB.")

	// Pastikan index filter panas (session_id dll) ada — idempoten, non-fatal. Mencegah
	// full-collection-scan yang bikin query melambat saat koleksi membesar.
	idxCtx, idxCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := mongoRepo.EnsureIndexes(idxCtx); err != nil {
		log.Printf("⚠️ Sebagian index Mongo gagal dibuat (non-fatal): %v", err)
	} else {
		fmt.Println("✅ Index Mongo (session_id dll) siap.")
	}
	idxCancel()

	// 2. Pragmatis: Seed Data Konfigurasi LLM & Sesi Awal (Jika Belum Ada)
	// Ini memastikan aplikasi tidak error saat pertama kali dijalankan di database kosong
	isFirstRun := seedInitialData(ctx, mongoRepo, sessionID)
	if isFirstRun {
		fmt.Println("\n⚠️ [TINDAKAN DIBUTUHKAN] Sistem baru saja melakukan pengisian data (seeding) awal ke MongoDB.")
		fmt.Println("⚠️ Silakan buka database Anda (koleksi 'llm_providers') dan ubah nilai API Key yang berawalan 'GANTI_DENGAN_...' menjadi kunci asli Anda.")
		fmt.Println("⚠️ Program dihentikan sementara. Silakan jalankan ulang aplikasi ini setelah siap!")
		return
	}

	// 3. Inisialisasi LLM Factory (Penyedia Otak AI Dinamis)
	llmFactory := llm.NewLLMFactory(mongoRepo)

	// 4. Inisialisasi Neo4j (Opsional, untuk Knowledge Graph / GraphRAG)
	neo4jURI := os.Getenv("NEO4JURI") // sesuai file .env user
	neo4jUser := os.Getenv("NEO4JUSER")
	neo4jPass := os.Getenv("NEO4JPASSWORD")

	var neo4jRepo *repository.Neo4jRepository
	var neo4jConnErr string

	// Log diagnostik: apakah env vars terbaca?
	if neo4jURI == "" {
		log.Println("⚠️  [Neo4j] NEO4JURI env kosong - Knowledge Graph (GraphRAG) tidak aktif.")
		neo4jConnErr = "NEO4JURI env var kosong (tidak terbaca dari environment)"
	} else {
		maskedURI := neo4jURI
		if len(maskedURI) > 10 {
			maskedURI = maskedURI[:10] + "..."
		}
		log.Printf("[Neo4j] NEO4JURI terbaca: %s (user=%q, pass_len=%d)", maskedURI, neo4jUser, len(neo4jPass))

		if neo4jUser == "" {
			log.Println("⚠️  [Neo4j] NEO4JUSER env kosong")
		}
		if neo4jPass == "" {
			log.Println("⚠️  [Neo4j] NEO4JPASSWORD env kosong")
		}

		var err error
		neo4jRepo, err = repository.NewNeo4jRepository(neo4jURI, neo4jUser, neo4jPass)
		if err != nil {
			neo4jConnErr = fmt.Sprintf("Neo4j connection failed (uri=%s, user=%q): %v", maskedURI, neo4jUser, err)
			log.Printf("❌ [Neo4j] %s", neo4jConnErr)
		} else {
			fmt.Println("✅ Berhasil terhubung ke Neo4j (GraphRAG Ready).")
			defer neo4jRepo.Close(ctx)
		}
	}

	// 5. Inisialisasi Main Orchestrator (State Machine Pipeline)
	// Kita perbarui pipeline agar menerima factory dinamis dan neo4j
	pipeline := orchestrator.NewSLRPipeline(mongoRepo, llmFactory, neo4jRepo, neo4jConnErr)

	// 5b. Inisialisasi Proposal Pipeline
	proposalPipeline := orchestrator.NewProposalPipeline(mongoRepo, llmFactory, neo4jRepo, neo4jConnErr)

	// 6. Inisialisasi HTTP Router
	router := httpapi.NewRouter(mongoRepo, pipeline, proposalPipeline)

	// Auto-resume: lanjutkan sesi yang berstatus "sedang jalan" (mis. worker terputus
	// karena deploy/restart mesin fly) tanpa perlu klik Resume manual di web.
	go pipeline.ResumeInProgress(context.Background())

	// 6. Jalankan Web Server
	port := os.Getenv("PORT")
	if port == "" {
		port = "50607"
	}

	fmt.Printf("🚀 Server berjalan di http://localhost:%s\n", port)

	// Server blocking
	err = http.ListenAndServe(":"+port, router)
	if err != nil {
		log.Fatalf("❌ Gagal menjalankan server: %v", err)
	}

	fmt.Println("\n====================================================")
	fmt.Println("          SERVER BERHENTI                           ")
	fmt.Println("====================================================")
}

// seedInitialData bertugas mengisi data default ke MongoDB agar sistem portabel langsung jalan
// Mengembalikan nilai true jika ada data yang baru saja dimasukkan (first run)
func seedInitialData(ctx context.Context, repo *repository.MongoRepository, sessionID string) bool {
	isSeeded := false

	// A. Seed Konfigurasi LLM (Contoh: DeepSeek & Gemini)
	// Kita gunakan trik GetLLMConfig, jika error (artinya data belum ada), kita buat baru
	_, err := repo.GetLLMConfig(ctx, "deepseek")
	if err != nil {
		isSeeded = true
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

	_, err = repo.GetLLMConfig(ctx, "nvidia")
	if err != nil {
		fmt.Println("[Seeding] Menyiapkan data default provider 'nvidia' (NIM) di MongoDB...")
		nvidiaConfig := &model.LLMConfig{
			ID:           "nvidia",
			ProviderName: "openai-compatible",
			BaseURL:      "https://integrate.api.nvidia.com/v1",
			APIKey:       "GANTI_DENGAN_API_KEY_NVIDIA_ANDA",
			DefaultModel: "meta/llama-3.3-70b-instruct",
			IsActive:     true,
			UpdatedAt:    time.Now(),
		}
		_ = updateLLMConfigDirectly(ctx, repo, nvidiaConfig)
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

	// B. Seed Sesi Riset SLR Baru (Status: INIT) - Hanya jika sessionID tidak kosong
	if sessionID != "" {
		_, err = repo.GetSession(ctx, sessionID)
		if err != nil {
			isSeeded = true
			fmt.Printf("[Seeding] Membuat sesi riset baru (%s) dengan Status: 'INIT'...\n", sessionID)
			newSession := &model.SLRSession{
				ID:     sessionID,
				Topic:  "Penggunaan Active Learning pada Machine Learning untuk klasifikasi data Brain-Computer Interface (BCI)",
				Status: "INIT",
			}
			err = repo.UpdateSession(ctx, newSession)
			if err != nil {
				log.Printf("Warn: Gagal seeding session awal: %v", err)
			}
		}
	}

	return isSeeded
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

// ensurePasetoKeys mengecek apakah PASETO keys sudah ada di environment.
// Jika belum (misal saat run pertama di komputer/hosting baru), ia akan membuat pasangan kunci baru
// dan menyimpannya ke file .env (jika bisa ditulisi) serta ke memory environment OS saat ini.
func ensurePasetoKeys() {
	privKeyHex := os.Getenv("PASETO_PRIVATE_KEY")
	pubKeyHex := os.Getenv("PASETO_PUBLIC_KEY")

	if privKeyHex == "" || pubKeyHex == "" {
		fmt.Println("⚠️  Info: PASETO keys tidak ditemukan. Men-generate pasangan kunci V4 Asymmetric baru...")

		secretKey := paseto.NewV4AsymmetricSecretKey()
		publicKey := secretKey.Public()

		privHex := secretKey.ExportHex()
		pubHex := publicKey.ExportHex()

		os.Setenv("PASETO_PRIVATE_KEY", privHex)
		os.Setenv("PASETO_PUBLIC_KEY", pubHex)

		f, err := os.OpenFile(".env", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err == nil {
			defer f.Close()
			f.WriteString(fmt.Sprintf("\n# Auto-generated PASETO Keys\nPASETO_PRIVATE_KEY=%s\nPASETO_PUBLIC_KEY=%s\n", privHex, pubHex))
			fmt.Println("✅ PASETO keys berhasil dibuat dan disimpan ke file .env.")
		} else {
			fmt.Printf("⚠️  Warn: Gagal menyimpan PASETO keys ke .env: %v. Key hanya berlaku selama aplikasi berjalan.\n", err)
		}
	}
}
