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
	godotenv.Load(".env")
	uri := os.Getenv("MONGO_URI")
	dbName := os.Getenv("DB_NAME")
	
	clientMongo, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(uri))
	if err != nil { log.Fatal(err) }
	defer clientMongo.Disconnect(context.TODO())

	coll := clientMongo.Database(dbName).Collection("slr_screening")
	
	sessions, _ := coll.Distinct(context.TODO(), "session_id", bson.M{})
	fmt.Printf("Found sessions: %v\n", sessions)
	
	for _, sess := range sessions {
		sessID := sess.(string)
		cursor, _ := coll.Find(context.TODO(), bson.M{"session_id": sessID})
		var papers []bson.M
		cursor.All(context.TODO(), &papers)
		fmt.Printf("Session %s: %d total papers\n", sessID, len(papers))
		
		filterInclude := bson.M{
			"session_id": sessID,
			"$or": []bson.M{
				{"Final_Decision": "INCLUDE"},
				{"Final_Decision": "", "Screener_1_Decision": "INCLUDE"},
			},
		}
		cursorInc, _ := coll.Find(context.TODO(), filterInclude)
		var papersInc []bson.M
		cursorInc.All(context.TODO(), &papersInc)
		fmt.Printf("Session %s: %d INCLUDE papers\n", sessID, len(papersInc))
	}
}
