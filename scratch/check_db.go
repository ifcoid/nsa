//go:build ignore

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
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil { log.Fatal(err) }
	coll := client.Database("slr_db").Collection("screening_papers")
	var result bson.M
	err = coll.FindOne(context.TODO(), bson.M{}).Decode(&result)
	if err != nil { log.Fatal(err) }
	for k, v := range result {
		fmt.Printf("%s: %v\n", k, v)
	}
}