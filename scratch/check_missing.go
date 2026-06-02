package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"net/http"
	"encoding/json"
	"io"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	godotenv.Load(".env")
	
	// GET QDRANT DOIS
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = os.Getenv("QDRANT_ENDPOINT")
	}
	qdrantKey := os.Getenv("QDRANT_API_KEY")

	clientHTTP := &http.Client{}
	qdrantDOIs := make(map[string]bool)
	var nextOffset string

	for {
		reqBody := `{"limit": 5000, "with_payload": ["doi"]}`
		if nextOffset != "" {
			reqBody = fmt.Sprintf(`{"limit": 5000, "with_payload": ["doi"], "offset": "%s"}`, nextOffset)
		}
		
		req, _ := http.NewRequest("POST", fmt.Sprintf("%s/collections/scientific_articles/points/scroll", qdrantURL), strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		if qdrantKey != "" {
			req.Header.Set("api-key", qdrantKey)
		}

		resp, err := clientHTTP.Do(req)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var qdrantResp map[string]interface{}
		json.Unmarshal(body, &qdrantResp)
		
		result, ok := qdrantResp["result"].(map[string]interface{})
		if !ok { break }
		
		points, ok := result["points"].([]interface{})
		if !ok { break }
		
		for _, pt := range points {
			pMap := pt.(map[string]interface{})
			payload, hasPayload := pMap["payload"].(map[string]interface{})
			if hasPayload {
				if d, isStr := payload["doi"].(string); isStr && d != "" {
					d = strings.TrimPrefix(d, "https://doi.org/")
					d = strings.TrimPrefix(d, "http://doi.org/")
					qdrantDOIs[d] = true
				}
			}
		}
		
		offsetVal, hasOffset := result["next_page_offset"]
		if hasOffset && offsetVal != nil {
			nextOffset = offsetVal.(string)
		} else { break }
	}

	uri := os.Getenv("MONGO_URI")
	dbName := os.Getenv("DB_NAME")
	
	clientMongo, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(uri))
	if err != nil { log.Fatal(err) }
	defer clientMongo.Disconnect(context.TODO())

	coll := clientMongo.Database(dbName).Collection("slr_screening")
	
	cursor, _ := coll.Find(context.TODO(), bson.M{})
	var papers []bson.M
	cursor.All(context.TODO(), &papers)
	
	mongoDOIs := make(map[string]bool)
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
			mongoDOIs[doi] = true
		}
	}
	
	fmt.Println("Missing DOIs from Mongo (present in Qdrant but NOT in Mongo):")
	missing := 0
	for qDOI := range qdrantDOIs {
		if !mongoDOIs[qDOI] {
			fmt.Println("- " + qDOI)
			missing++
		}
	}
	fmt.Printf("Total Missing: %d\n", missing)
}
