//go:build ignore
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	_ = godotenv.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cl, err := mongo.Connect(ctx, options.Client().ApplyURI(os.Getenv("MONGO_URI")))
	if err != nil { panic(err) }
	defer cl.Disconnect(ctx)
	db := cl.Database(os.Getenv("DB_NAME"))

	// find session by name
	var sess bson.M
	err = db.Collection("slr_sessions").FindOne(ctx, bson.M{"_id": "disertasi"}).Decode(&sess)
	if err != nil {
		fmt.Println("NOT FOUND by name=disertasi, listing all session names:")
		cur, _ := db.Collection("slr_sessions").Find(ctx, bson.M{})
		var all []bson.M
		cur.All(ctx, &all)
		for _, s := range all {
			fmt.Printf("  _id=%v  name=%q  status=%v\n", s["_id"], s["name"], s["status"])
		}
		return
	}
	id := sess["_id"]
	idHex := fmt.Sprintf("%v", id)
	fmt.Printf("FOUND session: _id=%v  status=%v\n", idHex, sess["status"])
	fmt.Println("Populated top-level fields:")
	for k, v := range sess {
		mark := ""
		switch vv := v.(type) {
		case nil:
		case string:
			if vv != "" { mark = fmt.Sprintf("(str %d ch)", len(vv)) }
		case bson.M:
			mark = "(obj)"
		case bson.A:
			mark = fmt.Sprintf("(arr %d)", len(vv))
		default:
			mark = "(set)"
		}
		if mark != "" { fmt.Printf("   - %s %s\n", k, mark) }
	}

	// backup
	dir := filepath.Join("backup", "disertasi_2026-06-04")
	os.MkdirAll(dir, 0o755)
	dump := func(coll string, filter bson.M) {
		cur, err := db.Collection(coll).Find(ctx, filter)
		if err != nil { fmt.Printf("  %s ERR %v\n", coll, err); return }
		var docs []bson.M
		cur.All(ctx, &docs)
		b, _ := bson.MarshalExtJSON(bson.M{"collection": coll, "count": len(docs), "docs": docs}, false, false)
		os.WriteFile(filepath.Join(dir, coll+".json"), b, 0o644)
		fmt.Printf("  backed up %s: %d docs\n", coll, len(docs))
	}
	fmt.Println("Backup ->", dir)
	dump("slr_sessions", bson.M{"_id": id})
	for _, c := range []string{"slr_papers", "slr_papers_post_dedup", "slr_screening", "slr_extraction"} {
		dump(c, bson.M{"session_id": idHex})
	}
}
