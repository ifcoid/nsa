//go:build ignore

// Script untuk me-reset paper full-text yang kena "Supervisor gagal merespons"
// agar di-screen ulang oleh batch runner.
// go run scratch/reset_supervisor_errors.go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
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

	// Cari paper dengan Conflict_Resolution_Full = "[AI_SUGGESTION: ERROR] Supervisor gagal merespons."
	filter := bson.M{
		"Conflict_Resolution_Full": bson.M{
			"$regex": "Supervisor gagal",
		},
		"Final_Decision_Full": bson.M{"$in": bson.A{"", nil}}, // yang belum difinalkan user
	}

	update := bson.M{
		"$set": bson.M{
			"Screener_1_Decision_Full":    "",
			"Screener_2_Decision_Full":    "",
			"Screener_1_Reason_Code_Full": "",
			"Screener_2_Reason_Code_Full": "",
			"Screener_1_Notes_Full":       "",
			"Screener_2_Notes_Full":       "",
			"Agreement_Full":              "",
			"Conflict_Resolution_Full":    "",
			"Batch_Evaluated_Full":        false,
		},
	}

	result, err := repo.GetScreeningCollection().UpdateMany(ctx, filter, update)
	if err != nil {
		fmt.Printf("❌ Update gagal: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Berhasil me-reset %d paper yang supervisor-nya gagal.\n", result.ModifiedCount)
	fmt.Println("Silakan Resume (Setuju & Lanjut) di UI untuk mencoba screen ulang.")
}
