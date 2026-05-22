package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/bson"
)

func main() {
	if err := godotenv.Load("../.env"); err != nil {
		// Try root env if running from different directory
		_ = godotenv.Load(".env")
	}

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
		log.Fatal("SESSION_ID not found in environment")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database(dbName)
	sessionsColl := db.Collection("slr_sessions")

	var session bson.M
	err = sessionsColl.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&session)
	if err != nil {
		log.Fatalf("Failed to find session %s: %v", sessionID, err)
	}

	fmt.Printf("SESSION_ID: %v\n", session["_id"])
	fmt.Printf("TOPIC: %v\n", session["topic"])
	fmt.Printf("STATUS: %v\n", session["status"])
	fmt.Printf("UPDATED_AT: %v\n", session["updated_at"])

	// Check OS Environment variables
	fmt.Println("\nVariabel Lingkungan OS:")
	envKeys := []string{"GEMINI_API_KEY", "DEEPSEEK_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GITHUB_TOKEN"}
	for _, k := range envKeys {
		val := os.Getenv(k)
		if val != "" {
			fmt.Printf("- %s: TERSEDIA di OS (panjang: %d)\n", k, len(val))
		} else {
			fmt.Printf("- %s: TIDAK TERSEDIA di OS\n", k)
		}
	}

	// Check LLM Provider Configuration
	fmt.Println("\nKonfigurasi LLM Providers:")
	llmColl := db.Collection("llm_providers")
	cursor, err := llmColl.Find(ctx, bson.M{})
	if err == nil {
		defer cursor.Close(ctx)
		for cursor.Next(ctx) {
			var provider bson.M
			if err := cursor.Decode(&provider); err == nil {
				apiKeyStr := fmt.Sprintf("%v", provider["api_key"])
				statusStr := "Belum Diatur (Placeholder)"
				if apiKeyStr != "" && !((apiKeyStr == "GANTI_DENGAN_API_KEY_GEMINI_ANDA") || (apiKeyStr == "GANTI_DENGAN_API_KEY_DEEPSEEK_ANDA") || (apiKeyStr == "GANTI_DENGAN_GITHUB_TOKEN_ANDA") || (apiKeyStr == "GANTI_DENGAN_API_KEY_ANTHROPIC_ANDA") || (apiKeyStr == "GANTI_DENGAN_API_KEY_NVIDIA_ANDA") || (apiKeyStr == "GANTI_DENGAN_API_KEY_MISTRAL_ANDA") || (apiKeyStr == "GANTI_DENGAN_API_KEY_DASHSCOPE_ANDA") || (apiKeyStr == "GANTI_DENGAN_API_KEY_OPENROUTER_ANDA") || (apiKeyStr == "GANTI_DENGAN_API_KEY_GROQ_ANDA") || (apiKeyStr == "GANTI_DENGAN_API_KEY_ZHIPU_ANDA")) {
					statusStr = "Sudah Diatur (Aktif)"
				}
				fmt.Printf("- %v (%v): %s\n", provider["_id"], provider["default_model"], statusStr)
			}
		}
	} else {
		fmt.Printf("Gagal membaca llm_providers: %v\n", err)
	}

	// Print non-empty fields to understand progress
	fmt.Println("\nDetail Kemajuan Dokumen:")
	fieldsToCheck := []string{
		"selected_topic", "prior_reviews_matrix", "pico_definitions",
		"scope_filters", "scope_justifications", "research_questions",
		"finer_novelty_check", "modul2_summary", "database_selection",
		"keywords", "search_string", "search_log", "modul3_summary",
		"data_mining_log", "screening_setup", "modul4_summary",
		"screener_briefing", "kalibrasi_log", "exclusion_table", "modul5_summary",
	}

	for _, field := range fieldsToCheck {
		if val, exists := session[field]; exists && val != nil {
			fmt.Printf("- %s: ADA (Tipe: %T)\n", field, val)
		} else {
			fmt.Printf("- %s: KOSONG\n", field)
		}
	}
}
