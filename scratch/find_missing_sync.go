//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	godotenv.Load(".env")

	// 1. Connect to Mongo
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb+srv://nsadb:P0rtalif!@apkflydev.b9uok.mongodb.net/?retryWrites=true&w=majority&appName=apkflydev"
	}
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(context.TODO())

	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "slr_agentic_db"
	}
	coll := client.Database(dbName).Collection("screening_papers")

	// Get DOIs from Mongo where status = "INCLUDE"
	cursor, err := coll.Find(context.TODO(), bson.M{"status": "INCLUDE"})
	if err != nil {
		log.Fatal(err)
	}
	var papers []bson.M
	if err = cursor.All(context.TODO(), &papers); err != nil {
		log.Fatal(err)
	}

	mongoDOIs := make(map[string]bool)
	mongoTitles := make(map[string]string)
	for _, p := range papers {
		var doi string
		if val, ok := p["doi"].(string); ok && val != "" {
			doi = val
		} else if val, ok := p["DOI"].(string); ok && val != "" {
			doi = val
		}
		if doi != "" {
			doi = strings.TrimPrefix(doi, "https://doi.org/")
			doi = strings.TrimPrefix(doi, "http://doi.org/")
			doi = strings.ToLower(strings.TrimSpace(doi))
			mongoDOIs[doi] = true
			if title, ok := p["title"].(string); ok {
				mongoTitles[doi] = title
			}
		}
	}

	// 2. Fetch DOIs from Qdrant
	qdrantURL := os.Getenv("QDRANT_ENDPOINT")
	qdrantKey := os.Getenv("QDRANT_API_KEY")
	
	reqQdrant, _ := http.NewRequest("POST", qdrantURL+"/collections/scientific_articles/points/scroll", strings.NewReader(`{"limit": 1000, "with_payload": ["doi", "title"], "with_vector": false}`))
	reqQdrant.Header.Set("Content-Type", "application/json")
	if qdrantKey != "" {
		reqQdrant.Header.Set("api-key", qdrantKey)
	}
	
	resp, err := http.DefaultClient.Do(reqQdrant)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	
	var qdrantResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&qdrantResp)
	
	qdrantDOIs := make(map[string]string) // doi -> title
	if result, ok := qdrantResp["result"].(map[string]interface{}); ok {
		if points, ok := result["points"].([]interface{}); ok {
			for _, pt := range points {
				if pMap, ok := pt.(map[string]interface{}); ok {
					if payload, ok := pMap["payload"].(map[string]interface{}); ok {
						if d, ok := payload["doi"].(string); ok && d != "" {
							d = strings.TrimPrefix(d, "https://doi.org/")
							d = strings.TrimPrefix(d, "http://doi.org/")
							d = strings.ToLower(strings.TrimSpace(d))
							title, _ := payload["title"].(string)
							qdrantDOIs[d] = title
						}
					}
				}
			}
		}
	}

	// 3. Compare
	fmt.Printf("Total Mongo INCLUDE DOIs: %d\n", len(mongoDOIs))
	fmt.Printf("Total Qdrant unique DOIs: %d\n", len(qdrantDOIs))
	
	fmt.Println("\nDOIs in Qdrant but NOT in Mongo INCLUDE:")
	count := 0
	for qDoi, qTitle := range qdrantDOIs {
		if !mongoDOIs[qDoi] {
			fmt.Printf("- %s (Title: %s)\n", qDoi, qTitle)
			count++
		}
	}
	if count == 0 {
		fmt.Println("None! All Qdrant DOIs match Mongo DOIs.")
	}
}