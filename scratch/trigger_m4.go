//go:build ignore

package main

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		panic(err)
	}
	defer client.Disconnect(ctx)

	coll := client.Database("slr_db").Collection("slr_sessions")

	// Update all M4_STEP2_WAITING_APPROVAL to M4_STEP2_PROCESS
	res, err := coll.UpdateMany(ctx,
		bson.M{"status": "M4_STEP2_WAITING_APPROVAL"},
		bson.M{"$set": bson.M{"status": "M4_STEP2_PROCESS"}},
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Updated %d sessions to re-process M4.\n", res.ModifiedCount)
}
