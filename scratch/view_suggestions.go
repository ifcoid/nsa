package main

import (
	"context"
	"encoding/json"
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

	fmt.Println("--- SUGGESTED TOPICS ---")
	suggested, exists := session["suggested_topics"]
	if !exists || suggested == nil {
		fmt.Println("Tidak ada suggested_topics.")
		return
	}

	jsonBytes, err := json.MarshalIndent(suggested, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal JSON: %v", err)
	}

	fmt.Println(string(jsonBytes))
}
