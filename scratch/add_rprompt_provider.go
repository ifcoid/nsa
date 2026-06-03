//go:build ignore

// Upsert provider 'rprompt' (OpenAI-compatible) ke koleksi llm_providers.
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

	cfg := &model.LLMConfig{
		ID:           "rprompt",
		ProviderName: "openai-compatible",
		BaseURL:      "https://rprompt.ll.my.id/v1",
		APIKey:       token,
		DefaultModel: "opus",
		IsActive:     true,
		UpdatedAt:    time.Now(),
	}
	if err := repo.UpdateLLMConfig(context.Background(), cfg); err != nil {
		fmt.Printf("❌ Upsert gagal: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Provider 'rprompt' di-upsert. base=%s model=%s key=%s…(panjang %d)\n",
		cfg.BaseURL, cfg.DefaultModel, token[:4], len(token))
}
