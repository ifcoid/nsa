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

// OpenAICompatibleClient adalah adapter universal untuk API berbasis standar OpenAI
type OpenAICompatibleClient struct {
	APIKey  string
	BaseURL string
	Model   string
}

// NewOpenAICompatibleClient adalah constructor untuk membuat client baru
func NewOpenAICompatibleClient(apiKey, baseURL, model string) *OpenAICompatibleClient {
	return &OpenAICompatibleClient{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}
}

// Struct internal untuk menyusun Payload Request sesuai standar OpenAI
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model       string          `json:"model"`
	Temperature float32         `json:"temperature"`
	Messages    []openAIMessage `json:"messages"`
}

// Struct internal untuk membaca Payload Response dari OpenAI
type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Generate adalah implementasi dari interface LLMClient
func (c *OpenAICompatibleClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// 1. Susun endpoint lengkap (biasanya diakhiri dengan /chat/completions)
	url := fmt.Sprintf("%s/chat/completions", c.BaseURL)

	payload := openAIRequest{
		Model:       c.Model,
		Temperature: 0.1, // Set sangat rendah (dingin) untuk menjamin objektivitas & konsistensi format JSON
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("gagal me-marshal request: %w", err)
	}

	// 3. Eksekusi HTTP Request dengan Exponential Backoff (Khusus 429 & 50x)
	// Timeout longgar: backend bridge berbasis Claude Code headless (mis. rprompt) bisa 30-90s+.
	client := &http.Client{Timeout: 180 * time.Second}
	var resp *http.Response
	var body []byte
	var lastErr error

	maxRetries := 5
	baseDelay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
		if err != nil {
			return "", fmt.Errorf("gagal membuat http request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if c.APIKey != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))
		}

		resp, lastErr = client.Do(req)
		if lastErr == nil {
			body, _ = io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				lastErr = nil // Clear error on success
				break
			}

			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				// Retry untuk 429 atau Server Error
				var openAIResp openAIResponse
				_ = json.Unmarshal(body, &openAIResp)
				errMsg := string(body)
				if openAIResp.Error != nil {
					errMsg = openAIResp.Error.Message
				}
				lastErr = fmt.Errorf("error dari provider (HTTP %d): %s", resp.StatusCode, errMsg)
			} else {
				// Jangan retry untuk 400 Bad Request, 401 Unauthorized, dll
				var openAIResp openAIResponse
				_ = json.Unmarshal(body, &openAIResp)
				errMsg := string(body)
				if openAIResp.Error != nil {
					errMsg = openAIResp.Error.Message
				}
				return "", fmt.Errorf("fatal error dari provider (HTTP %d): %s", resp.StatusCode, errMsg)
			}
		}

		if i < maxRetries-1 {
			delay := baseDelay * time.Duration(1<<i) // 2s, 4s, 8s, 16s
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}

	if lastErr != nil {
		return "", fmt.Errorf("gagal setelah %d retries: %w", maxRetries, lastErr)
	}

	// 7. Parse JSON Response
	var openAIResp openAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return "", fmt.Errorf("gagal unmarshal response JSON: %w. Body: %s", err, string(body))
	}

	// 9. Ambil hasil teks jawaban LLM
	if len(openAIResp.Choices) > 0 {
		return openAIResp.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("response sukses tapi tidak ada pilihan jawaban (choices kosong)")
}
