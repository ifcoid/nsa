//go:build ignore

// Upsert provider 'rprompt1' (Claude Sonnet) dan 'rprompt2' (Gemini Pro) ke koleksi llm_providers.
// Keduanya mengarah ke server rprompt yang sama tapi dengan model default berbeda.
// Token dibaca dari ../rprompt/.env (API_TOKEN) agar rahasia tidak masuk repo.
//
// Jalankan: go run scratch/add_rprompt_provider.go
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"nsa/internal/model"
	"nsa/internal/repository"
)

func readEnvKey(path, key string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, key+"=") {
			v := strings.TrimPrefix(line, key+"=")
			v = strings.TrimSpace(v)
			v = strings.Trim(v, "\"")
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

	token := readEnvKey("../rprompt/.env", "API_TOKEN")
	if token == "" {
		fmt.Println("❌ Gagal membaca API_TOKEN dari ../rprompt/.env")
		os.Exit(1)
	}

	repo, err := repository.NewMongoRepository(uri, dbName)
	if err != nil {
		fmt.Printf("❌ Mongo: %v\n", err)
		os.Exit(1)
	}

	baseURL := "https://rprompt.ll.my.id/v1"

	// rprompt1: Claude Sonnet (untuk R1)
	cfg1 := &model.LLMConfig{
		ID:           "rprompt1",
		ProviderName: "openai-compatible",
		BaseURL:      baseURL,
		APIKey:       token,
		DefaultModel: "sonnet",
		IsActive:     true,
		UpdatedAt:    time.Now(),
	}
	if err := repo.UpdateLLMConfig(context.Background(), cfg1); err != nil {
		fmt.Printf("❌ Upsert rprompt1 gagal: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Provider 'rprompt1' di-upsert. base=%s model=%s key=%s…(panjang %d)\n",
		cfg1.BaseURL, cfg1.DefaultModel, token[:4], len(token))

	// rprompt2: Gemini Pro (untuk R2)
	cfg2 := &model.LLMConfig{
		ID:           "rprompt2",
		ProviderName: "openai-compatible",
		BaseURL:      baseURL,
		APIKey:       token,
		DefaultModel: "gemini-2.5-pro",
		IsActive:     true,
		UpdatedAt:    time.Now(),
	}
	if err := repo.UpdateLLMConfig(context.Background(), cfg2); err != nil {
		fmt.Printf("❌ Upsert rprompt2 gagal: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Provider 'rprompt2' di-upsert. base=%s model=%s key=%s…(panjang %d)\n",
		cfg2.BaseURL, cfg2.DefaultModel, token[:4], len(token))
}
