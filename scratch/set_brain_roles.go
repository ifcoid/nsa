//go:build ignore

// Set Model Routing: Brain = rprompt (claude-sonnet, kuota lega), Brain Fallback = gemini.
// Mempertahankan peran reviewer/supervisor yang sudah ada.
//
// Jalankan: go run scratch/set_brain_roles.go
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
	db := os.Getenv("DB_NAME")
	repo, err := repository.NewMongoRepository(uri, db)
	if err != nil {
		fmt.Println("mongo:", err)
		os.Exit(1)
	}
	ctx := context.Background()
	roles := repo.GetLLMRoles(ctx) // sudah merge default + tersimpan
	roles.Brain = "rprompt"
	roles.BrainFallback = "gemini"
	if err := repo.UpdateLLMRoles(ctx, roles); err != nil {
		fmt.Println("update:", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Roles tersimpan: R1=%s/%s R2=%s/%s Sup=%s/%s Brain=%s/%s\n",
		roles.Reviewer1, roles.Reviewer1Fallback, roles.Reviewer2, roles.Reviewer2Fallback,
		roles.Supervisor, roles.SupervisorFallback, roles.Brain, roles.BrainFallback)
}
