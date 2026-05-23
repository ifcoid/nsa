package main

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: "TEST"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%T\n", client.Models)
}
