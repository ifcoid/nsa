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
	targetDoi := "10.1109/bibm66473.2025.11356013"
	foundCount := 0

	for {
		payload := map[string]interface{}{
			"limit": 1000,
			"with_payload": true,
			"with_vector": false,
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
			fmt.Println("Error HTTP:", err)
			return
		}
		
		var qdrantResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&qdrantResp)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			fmt.Println("Status != 200:", qdrantResp)
			return
		}

		result, ok := qdrantResp["result"].(map[string]interface{})
		if !ok { break }
		
		points, ok := result["points"].([]interface{})
		if !ok || len(points) == 0 { break }
		
		for _, pt := range points {
			pMap := pt.(map[string]interface{})
			if payloadMap, ok := pMap["payload"].(map[string]interface{}); ok {
				if doi, exists := payloadMap["doi"]; exists && doi == targetDoi {
					foundCount++
					fmt.Printf("--- Point %d ---\n", foundCount)
					for k, v := range payloadMap {
						if k == "text" {
							str := fmt.Sprintf("%v", v)
							if len(str) > 300 {
								fmt.Printf("%s: %s...[TRUNCATED]\n", k, str[:300])
							} else {
								fmt.Printf("%s: %s\n", k, str)
							}
						} else {
							fmt.Printf("%s: %v\n", k, v)
						}
					}
					fmt.Println()
				}
			}
		}

		if offsetVal, hasOffset := result["next_page_offset"]; hasOffset && offsetVal != nil {
			nextOffset = offsetVal
		} else {
			break
		}
	}
	
	if foundCount == 0 {
		fmt.Printf("DOI %s NOT FOUND in Qdrant!\n", targetDoi)
	} else {
		fmt.Printf("Finished. Found %d points for the DOI.\n", foundCount)
	}
}
