//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func main() {
	qdrantURL := "https://67983937-4c0e-403a-9f44-9934c86743e9.australia-southeast1-0.gcp.cloud.qdrant.io"
	qdrantKey := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhY2Nlc3MiOiJtIiwic3ViamVjdCI6ImFwaS1rZXk6YTAwZTdlYjItNTM3Yy00MjM5LWIwM2ItZDk1OTE0NGU3ZWEyIn0.Jf0_GsAldUY4-SICiEcYfBG36l5vE2DQmKdsJOjoN94"
	client := &http.Client{Timeout: 30 * time.Second}

	var nextOffset interface{}
	pointsCount := 0
	reqCount := 0

	for {
		reqCount++
		payload := map[string]interface{}{
			"limit": 5000,
			"with_payload": []string{"doi"},
		}
		if nextOffset != nil {
			payload["offset"] = nextOffset
		}

		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", qdrantURL+"/collections/scientific_articles/points/scroll", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("api-key", qdrantKey)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		
		var qdrantResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&qdrantResp)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			fmt.Println("Status != 200:", qdrantResp)
			return
		}

		result := qdrantResp["result"].(map[string]interface{})
		points := result["points"].([]interface{})
		pointsCount += len(points)
		
		fmt.Printf("Req %d: got %d points\n", reqCount, len(points))

		if offsetVal, hasOffset := result["next_page_offset"]; hasOffset && offsetVal != nil {
			nextOffset = offsetVal
			fmt.Printf("Next offset: %v (%T)\n", nextOffset, nextOffset)
		} else {
			break
		}
	}
	fmt.Printf("Total points: %d in %d requests\n", pointsCount, reqCount)
}