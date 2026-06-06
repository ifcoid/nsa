//go:build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(context.TODO())

	dbName := os.Getenv("MONGO_DB_NAME")
	if dbName == "" {
		dbName = "slr_agent"
	}

	col2 := client.Database(dbName).Collection("slr_screening")
	
	// Create a regex for the title to be safe
	filter := bson.M{"title": primitive.Regex{Pattern: "Cross-Attentioned Dynamic", Options: "i"}}
	
	var result bson.M
	err = col2.FindOne(context.TODO(), filter).Decode(&result)
	if err != nil {
		fmt.Println("\nError in slr_screening col:", err)
	} else {
		fmt.Println("\n--- From 'slr_screening' collection ---")
		for k, v := range result {
			if k == "abstract" || strings.Contains(k, "Notes") { continue }
			fmt.Printf("%s: %v\n", k, v)
		}
	}
}
