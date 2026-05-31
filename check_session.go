package main

import (
	"context"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	uri := "mongodb+srv://apkflydev:aqv082W6Xy4ercBt@apkflydev.cysnbuw.mongodb.net/?appName=apkflydev"
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatal(err)
	}

	coll := client.Database("slr_agentic_db").Collection("slr_sessions")
	
	var session bson.M
	err = coll.FindOne(context.Background(), bson.M{"_id": "disertasi"}).Decode(&session)
	if err != nil {
		log.Fatal(err)
	}

	// Print summary of data_mining_log if exists
	if dml, ok := session["data_mining_log"].(bson.M); ok {
		fmt.Printf("Data Mining Log: %+v\n", dml)
	}
	
	// Print summary of screening_results_log
	if srl, ok := session["screening_results_log"].(bson.A); ok {
		fmt.Printf("Screening Batches run: %d\n", len(srl))
		totalDisagreements := 0
		for _, batch := range srl {
			if b, ok := batch.(bson.M); ok {
				if da, ok := b["disagreement_cases"].(int32); ok {
					totalDisagreements += int(da)
				}
			}
		}
		fmt.Printf("Total disagreements: %d\n", totalDisagreements)
	}
}
