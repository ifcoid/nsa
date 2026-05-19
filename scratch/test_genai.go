package main

import (
	"context"
	"fmt"
	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()
	apiKey := "dummy"
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		panic(err)
	}

	systemPrompt := "You are a bot"
	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: systemPrompt}},
		},
		Tools: []*genai.Tool{
			{GoogleSearch: &genai.GoogleSearch{}},
		},
	}

	res, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", genai.Text("hello"), config)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)
}
