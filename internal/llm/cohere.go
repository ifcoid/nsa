package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CohereClient merupakan implementasi LLMClient khusus untuk Cohere API v2
type CohereClient struct {
	APIKey       string
	BaseURL      string
	DefaultModel string
	client       *http.Client
}

// NewCohereClient membuat instance baru dari CohereClient
func NewCohereClient(apiKey, defaultModel string) *CohereClient {
	return &CohereClient{
		APIKey:       apiKey,
		BaseURL:      "https://api.cohere.com/v2/chat", // Selalu paksa menggunakan v2
		DefaultModel: defaultModel,
		client:       &http.Client{Timeout: 3 * time.Minute},
	}
}

func (c *CohereClient) ModelName() string {
	return "cohere/" + c.DefaultModel
}

// Generate melakukan pemanggilan API ke endpoint Cohere v2
func (c *CohereClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Format payload Cohere v2
	payload := map[string]interface{}{
		"model": c.DefaultModel,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.2, // Temperatur rendah untuk tugas analitis SLR
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cohere request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("cohere request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error dari cohere (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	// Parsing response v2:
	// { "message": { "content": [ { "type": "text", "text": "..." } ] } }
	var res struct {
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}

	if err := json.Unmarshal(respBody, &res); err != nil {
		return "", fmt.Errorf("failed to decode cohere response: %w. Raw: %s", err, string(respBody))
	}

	// Gabungkan semua teks (meskipun biasanya hanya ada 1 elemen)
	var finalResponse string
	for _, c := range res.Message.Content {
		if c.Type == "text" {
			finalResponse += c.Text
		}
	}

	if finalResponse == "" {
		return "", fmt.Errorf("tidak ada teks balasan dari cohere. Raw: %s", string(respBody))
	}

	return finalResponse, nil
}
