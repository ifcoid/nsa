//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load(".env")
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = os.Getenv("QDRANT_ENDPOINT")
	}
	qdrantKey := os.Getenv("QDRANT_API_KEY")

	client := &http.Client{}
	
	uniqueDOIs := make(map[string]bool)
	var nextOffset string
	totalPointsFetched := 0

	for {
		reqBody := fmt.Sprintf(`{"limit": 1000, "with_payload": ["doi"]}`)
		if nextOffset != "" {
			reqBody = fmt.Sprintf(`{"limit": 1000, "with_payload": ["doi"], "offset": "%s"}`, nextOffset)
		}
		
		req, _ := http.NewRequest("POST", fmt.Sprintf("%s/collections/scientific_articles/points/scroll", qdrantURL), strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		if qdrantKey != "" {
			req.Header.Set("api-key", qdrantKey)
		}

		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var qdrantResp map[string]interface{}
		json.Unmarshal(body, &qdrantResp)
		
		result, ok := qdrantResp["result"].(map[string]interface{})
		if !ok {
			fmt.Println("Invalid response format")
			break
		}
		
		points, ok := result["points"].([]interface{})
		if !ok {
			break
		}
		
		totalPointsFetched += len(points)
		
		for _, pt := range points {
			pMap := pt.(map[string]interface{})
			payload, hasPayload := pMap["payload"].(map[string]interface{})
			if hasPayload {
				if d, isStr := payload["doi"].(string); isStr && d != "" {
					uniqueDOIs[d] = true
				}
			}
		}
		
		offsetVal, hasOffset := result["next_page_offset"]
		if hasOffset && offsetVal != nil {
			nextOffset = offsetVal.(string)
		} else {
			break
		}
	}
	
	fmt.Printf("Total points fetched: %d\n", totalPointsFetched)
	fmt.Printf("Total unique DOIs in Qdrant: %d\n", len(uniqueDOIs))
	fmt.Println("DOIs:")
	for doi := range uniqueDOIs {
		fmt.Println(" -", doi)
	}
}