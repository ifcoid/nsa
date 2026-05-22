package main

import (
	"context"
	"fmt"
	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()
	apiKey := "AIzaSyCOm3cKm_p0qziiCixSsLko5J6Tj-m6CdM"
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		panic(err)
	}

	systemPrompt := "You are a research bot"
	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: systemPrompt}},
		},
		Tools: []*genai.Tool{
			{GoogleSearch: &genai.GoogleSearch{}},
		},
	}

	res, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", genai.Text("suggest 1 research topic on brain computer interface"), config)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}

	fmt.Printf("RESPONSE CANDIDATES LEN: %d\n", len(res.Candidates))
	if len(res.Candidates) > 0 {
		candidate := res.Candidates[0]
		fmt.Printf("FINISH REASON: %v\n", candidate.FinishReason)
		if candidate.Content != nil {
			fmt.Printf("PARTS LEN: %d\n", len(candidate.Content.Parts))
			for i, p := range candidate.Content.Parts {
				fmt.Printf("  PART %d TEXT: %q\n", i, p.Text)
			}
		} else {
			fmt.Println("CONTENT IS NIL")
		}
	}
}
