package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
	uri := os.Getenv("MONGO_URI")
	dbName := os.Getenv("DB_NAME")
	fmt.Printf("Connecting to %s\n", dbName)

	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(context.TODO())

	coll := client.Database(dbName).Collection("slr_screening")
	var result bson.M
	// Fetch one record that has a DOI
	err = coll.FindOne(context.TODO(), bson.M{"doi": bson.M{"$exists": true, "$ne": ""}}).Decode(&result)
	if err != nil {
		fmt.Printf("Searching with lowercase 'doi' failed: %v\n", err)
		err = coll.FindOne(context.TODO(), bson.M{"DOI": bson.M{"$exists": true, "$ne": ""}}).Decode(&result)
		if err != nil {
			log.Fatalf("Searching with uppercase 'DOI' failed: %v", err)
		}
	}

	fmt.Printf("DOI field (upper): %v\n", result["DOI"])
	fmt.Printf("doi field (lower): %v\n", result["doi"])
	fmt.Printf("Title field: %v\n", result["title"])

	// Print all keys
	keys := []string{}
	for k := range result {
		keys = append(keys, k)
	}
	fmt.Printf("Keys: %v\n", keys)
}
