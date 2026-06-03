//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	uri := "mongodb+srv://apkflydev:aqv082W6Xy4ercBt@apkflydev.cysnbuw.mongodb.net/?appName=apkflydev"
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(context.TODO())

	coll := client.Database("slr_agentic_db").Collection("slr_screening")

	cursor, err := coll.Find(context.TODO(), bson.M{})
	if err != nil {
		log.Fatal(err)
	}

	var papers []bson.M
	if err := cursor.All(context.TODO(), &papers); err != nil {
		log.Fatal(err)
	}

	updatedCount := 0
	fmt.Printf("Total papers found in DB: %d\n", len(papers))
	for _, p := range papers {
		id := p["_id"].(primitive.ObjectID)
		doi, _ := p["DOI"].(string)
		
		if doi == "" {
			doi, _ = p["doi"].(string)
		}

		journal, _ := p["Journal"].(string)
		if journal == "" {
			journal, _ = p["journal"].(string)
		}

		articleType, _ := p["Article_Type"].(string)
		if articleType == "" {
			articleType, _ = p["document_type"].(string)
		}

		// Clean DOI
		doi = strings.TrimPrefix(doi, "https://doi.org/")
		doi = strings.TrimPrefix(doi, "http://doi.org/")
		doi = strings.TrimSpace(doi)

		if doi != "" && (journal == "" || articleType == "") {
			fmt.Printf("Fetching Crossref for DOI: %s\n", doi)
			j, a, pub := fetchCrossref(doi)
			if j != "" || a != "" {
				update := bson.M{"$set": bson.M{}}
				if j != "" {
					update["$set"].(bson.M)["Journal"] = j
				}
				if a != "" {
					update["$set"].(bson.M)["Article_Type"] = a
				}
				if pub != "" {
					update["$set"].(bson.M)["Publisher"] = pub
				}
				
				_, err := coll.UpdateByID(context.TODO(), id, update)
				if err == nil {
					updatedCount++
				}
			}
			time.Sleep(200 * time.Millisecond) // Polite delay
		}
	}

	fmt.Printf("Successfully updated %d papers with Crossref data!\n", updatedCount)
}

func fetchCrossref(doi string) (string, string, string) {
	url := "https://api.crossref.org/works/" + doi
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "SLR-NSA-Tool/1.0 (mailto:rolly@example.com)")
	
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return "", "", ""
	}
	defer resp.Body.Close()

	var data struct {
		Message struct {
			ContainerTitle []string `json:"container-title"`
			Type           string   `json:"type"`
			Publisher      string   `json:"publisher"`
		} `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", ""
	}

	journal := ""
	if len(data.Message.ContainerTitle) > 0 {
		journal = data.Message.ContainerTitle[0]
	}
	
	aType := data.Message.Type
	aType = strings.ReplaceAll(aType, "-", " ")
	aType = strings.Title(aType)
	
	return journal, aType, data.Message.Publisher
}