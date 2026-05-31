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

	coll := client.Database("slr_agentic_db").Collection("slr_screening")
	
	countAll, _ := coll.CountDocuments(context.Background(), bson.M{"session_id": "disertasi"})
	countEvaluated, _ := coll.CountDocuments(context.Background(), bson.M{"session_id": "disertasi", "Batch_Evaluated": true})
	countScreened, _ := coll.CountDocuments(context.Background(), bson.M{"session_id": "disertasi", "Screener_1_Decision": bson.M{"$ne": ""}})
	countIncludeExplicit, _ := coll.CountDocuments(context.Background(), bson.M{"session_id": "disertasi", "Final_Decision": "INCLUDE"})
	countExcludeExplicit, _ := coll.CountDocuments(context.Background(), bson.M{"session_id": "disertasi", "Final_Decision": "EXCLUDE"})
	countIncludeImplicit, _ := coll.CountDocuments(context.Background(), bson.M{"session_id": "disertasi", "Final_Decision": "", "Screener_1_Decision": "INCLUDE", "Screener_2_Decision": "INCLUDE"})
	
	fmt.Printf("Total Papers: %d\n", countAll)
	fmt.Printf("Screened by R1: %d\n", countScreened)
	fmt.Printf("Evaluated (Finalized): %d\n", countEvaluated)
	fmt.Printf("INCLUDE (Explicit): %d\n", countIncludeExplicit)
	fmt.Printf("EXCLUDE (Explicit): %d\n", countExcludeExplicit)
	fmt.Printf("INCLUDE (Implicit R1+R2): %d\n", countIncludeImplicit)
}
