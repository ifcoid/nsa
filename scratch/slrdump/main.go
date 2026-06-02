package main

// READ-ONLY dump of SLR session data for proposal slides.
import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	_ = godotenv.Load("../../.env")
	uri := os.Getenv("MONGO_URI")
	dbName := os.Getenv("DB_NAME")
	if uri == "" {
		log.Fatal("MONGO_URI empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(ctx)
	db := client.Database(dbName)

	// 1) list all sessions (id + topic + status)
	fmt.Println("==== SESSIONS ====")
	cur, err := db.Collection("slr_sessions").Find(ctx, bson.M{})
	if err != nil {
		log.Fatal(err)
	}
	var sessions []bson.M
	cur.All(ctx, &sessions)
	for _, s := range sessions {
		fmt.Printf("ID=%v | status=%v | topic=%v\n", s["_id"], s["status"], s["topic"])
	}

	// 2) Find the disertasi session (try common ids)
	var sess bson.M
	for _, candidate := range []bson.M{
		{"_id": "disertasi"},
		{"_id": bson.M{"$regex": "disertasi", "$options": "i"}},
		{"topic": bson.M{"$regex": "disertasi", "$options": "i"}},
	} {
		err = db.Collection("slr_sessions").FindOne(ctx, candidate).Decode(&sess)
		if err == nil {
			break
		}
	}
	if sess == nil {
		log.Fatal("session disertasi not found")
	}
	sessID := fmt.Sprintf("%v", sess["_id"])
	fmt.Printf("\n==== FULL SESSION DOC (id=%s) ====\n", sessID)
	b, _ := json.MarshalIndent(sess, "", "  ")
	os.WriteFile("session_full.json", b, 0644)
	fmt.Printf("(written session_full.json, %d bytes)\n", len(b))

	// 3) Screening counts for this session
	fmt.Println("\n==== SCREENING STATS ====")
	for _, coll := range []string{"slr_screening", "slr_papers", "slr_papers_post_dedup"} {
		c := db.Collection(coll)
		total, _ := c.CountDocuments(ctx, bson.M{"session_id": sessID})
		fmt.Printf("[%s] total for session: %d\n", coll, total)
		if total == 0 {
			// maybe no session_id filter
			tAll, _ := c.CountDocuments(ctx, bson.M{})
			fmt.Printf("   (collection total all sessions: %d)\n", tAll)
		}
		// status breakdown
		pipe := mongo.Pipeline{
			{{Key: "$match", Value: bson.M{"session_id": sessID}}},
			{{Key: "$group", Value: bson.M{"_id": "$status", "n": bson.M{"$sum": 1}}}},
		}
		agg, e := c.Aggregate(ctx, pipe)
		if e == nil {
			var rows []bson.M
			agg.All(ctx, &rows)
			for _, r := range rows {
				fmt.Printf("   status=%v -> %v\n", r["_id"], r["n"])
			}
		}
	}

	// 4) full-text acquisition stats (module 6)
	fmt.Println("\n==== MODULE 6 (FULL-TEXT) STATS ====")
	sc := db.Collection("slr_screening")
	for _, f := range []struct {
		label  string
		filter bson.M
	}{
		{"full_text_retrieved=true", bson.M{"session_id": sessID, "full_text_retrieved": true}},
		{"inaccessible=true", bson.M{"session_id": sessID, "inaccessible": true}},
		{"has download_url", bson.M{"session_id": sessID, "download_url": bson.M{"$ne": ""}}},
	} {
		n, _ := sc.CountDocuments(ctx, f.filter)
		fmt.Printf("   %s -> %d\n", f.label, n)
	}

	// 5) year distribution among accepted/include papers
	fmt.Println("\n==== YEAR DISTRIBUTION (status ACCEPT/INCLUDE) ====")
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"session_id": sessID}}},
		{{Key: "$group", Value: bson.M{"_id": "$year", "n": bson.M{"$sum": 1}}}},
		{{Key: "$sort", Value: bson.M{"_id": 1}}},
	}
	agg, _ := sc.Aggregate(ctx, pipe)
	var yrows []bson.M
	if agg != nil {
		agg.All(ctx, &yrows)
	}
	for _, r := range yrows {
		fmt.Printf("   year=%v -> %v\n", r["_id"], r["n"])
	}

	// 6) database/source distribution
	fmt.Println("\n==== SOURCE DATABASE DISTRIBUTION ====")
	pipe2 := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"session_id": sessID}}},
		{{Key: "$group", Value: bson.M{"_id": "$database", "n": bson.M{"$sum": 1}}}},
		{{Key: "$sort", Value: bson.M{"n": -1}}},
	}
	agg2, _ := sc.Aggregate(ctx, pipe2)
	var drows []bson.M
	if agg2 != nil {
		agg2.All(ctx, &drows)
	}
	for _, r := range drows {
		fmt.Printf("   db=%v -> %v\n", r["_id"], r["n"])
	}
}
