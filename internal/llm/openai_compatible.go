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

	// 3. Siapkan HTTP Request dengan Context (supaya bisa di-timeout/cancel)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("gagal membuat http request: %w", err)
	}

	// 4. Injeksi Headers wajib
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))
	}

	// 5. Eksekusi HTTP Request
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gagal mengeksekusi request ke provider: %w", err)
	}
	defer resp.Body.Close()

	// 6. Baca body response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("gagal membaca body response: %w", err)
	}

	// 7. Parse JSON Response
	var openAIResp openAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return "", fmt.Errorf("gagal unmarshal response JSON: %w. Body: %s", err, string(body))
	}

	// 8. Validasi jika ada error dari provider
	if resp.StatusCode != http.StatusOK {
		if openAIResp.Error != nil {
			return "", fmt.Errorf("error dari provider (HTTP %d): %s", resp.StatusCode, openAIResp.Error.Message)
		}
		return "", fmt.Errorf("error dari provider (HTTP %d): %s", resp.StatusCode, string(body))
	}

	// 9. Ambil hasil teks jawaban LLM
	if len(openAIResp.Choices) > 0 {
		return openAIResp.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("response sukses tapi tidak ada pilihan jawaban (choices kosong)")
}
