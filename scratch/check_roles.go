//go:build ignore

// Cek roles dan providers yang tersimpan di MongoDB.
// go run scratch/check_roles.go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"nsa/internal/repository"
)

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

	repo, err := repository.NewMongoRepository(uri, dbName)
	if err != nil {
		fmt.Printf("❌ Mongo: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Cek roles
	roles := repo.GetLLMRoles(ctx)
	fmt.Println("=== LLM ROLES (tersimpan di DB) ===")
	fmt.Printf("  Reviewer1:          %s\n", roles.Reviewer1)
	fmt.Printf("  Reviewer1Fallback:  %s\n", roles.Reviewer1Fallback)
	fmt.Printf("  Reviewer2:          %s\n", roles.Reviewer2)
	fmt.Printf("  Reviewer2Fallback:  %s\n", roles.Reviewer2Fallback)
	fmt.Printf("  Supervisor:         %s\n", roles.Supervisor)
	fmt.Printf("  SupervisorFallback: %s\n", roles.SupervisorFallback)
	fmt.Printf("  Brain:              %s\n", roles.Brain)
	fmt.Printf("  BrainFallback:      %s\n", roles.BrainFallback)

	// Cek provider configs
	fmt.Println("\n=== LLM PROVIDERS ===")
	for _, id := range []string{"rprompt", "rprompt1", "rprompt2", "groq", "xiaomi", "cohere"} {
		cfg, err := repo.GetLLMConfig(ctx, id)
		if err != nil {
			fmt.Printf("  %-12s ❌ %v\n", id, err)
		} else {
			key := "(kosong)"
			if len(cfg.APIKey) > 4 {
				key = cfg.APIKey[:4] + "…"
			}
			fmt.Printf("  %-12s ✅ active=%v model=%s base=%s key=%s\n", id, cfg.IsActive, cfg.DefaultModel, cfg.BaseURL, key)
		}
	}
}
