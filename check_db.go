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
	
	pipeline := mongo.Pipeline{
		{{"$match", bson.D{{"session_id", "disertasi"}}}},
		{{"$group", bson.D{{"_id", "$Final_Decision"}, {"count", bson.D{{"$sum", 1}}}}}},
	}

	cursor, err := coll.Aggregate(context.Background(), pipeline)
	if err != nil {
		log.Fatal(err)
	}

	var results []bson.M
	if err = cursor.All(context.Background(), &results); err != nil {
		log.Fatal(err)
	}

	for _, result := range results {
		fmt.Printf("Decision: %v, Count: %v\n", result["_id"], result["count"])
	}
}
