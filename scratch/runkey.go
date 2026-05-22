package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	if err := godotenv.Load("../.env"); err != nil {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database(dbName)
	llmColl := db.Collection("llm_providers")

	providersToUpdate := map[string]string{
		"gemini": "AIzaSyCOm3cKm_p0qziiCixSsLko5J6Tj-m6CdM",
		"zhipu":  "ffeade6ff7464124a2b4d8b0187ddf65.2eAdK8a7fGi15uoF",
		"groq":   "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",
	}

	fmt.Println("Mengupdate API Keys di MongoDB...")

	for providerID, apiKey := range providersToUpdate {
		filter := bson.M{"_id": providerID}
		update := bson.M{
			"$set": bson.M{
				"api_key":    apiKey,
				"updated_at": time.Now(),
			},
		}

		result, err := llmColl.UpdateOne(ctx, filter, update)
		if err != nil {
			fmt.Printf("❌ Gagal mengupdate provider %s: %v\n", providerID, err)
		} else {
			if result.MatchedCount == 0 {
				var providerName, baseURL, defaultModel string
				if providerID == "gemini" {
					providerName = "gemini"
					baseURL = "https://generativelanguage.googleapis.com/v1beta"
					defaultModel = "gemini-2.5-flash"
				} else if providerID == "zhipu" {
					providerName = "openai-compatible"
					baseURL = "https://open.bigmodel.cn/api/paas/v4"
					defaultModel = "glm-4"
				} else if providerID == "groq" {
					providerName = "openai-compatible"
					baseURL = "https://api.groq.com/openai/v1"
					defaultModel = "llama3-70b-8192"
				}

				_, err = llmColl.InsertOne(ctx, bson.M{
					"_id":           providerID,
					"provider_name": providerName,
					"base_url":      baseURL,
					"api_key":       apiKey,
					"default_model": defaultModel,
					"is_active":     true,
					"updated_at":    time.Now(),
				})
				if err != nil {
					fmt.Printf("❌ Gagal membuat provider baru %s: %v\n", providerID, err)
				} else {
					fmt.Printf("✅ Berhasil membuat & mengatur API Key untuk provider: %s\n", providerID)
				}
			} else {
				fmt.Printf("✅ Berhasil mengupdate API Key untuk provider: %s\n", providerID)
			}
		}
	}
}
