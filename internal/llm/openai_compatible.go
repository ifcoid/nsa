package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAICompatibleClient adalah adapter universal untuk API berbasis standar OpenAI.
// Memakai STREAMING (SSE) agar koneksi tetap hidup pada generasi panjang — penting untuk
// backend di balik proxy dengan edge-timeout (mis. Cloudflare 524 pada rprompt/Claude headless).
type OpenAICompatibleClient struct {
	APIKey  string
	BaseURL string
	Model   string
}

func NewOpenAICompatibleClient(apiKey, baseURL, model string) *OpenAICompatibleClient {
	return &OpenAICompatibleClient{APIKey: apiKey, BaseURL: baseURL, Model: model}
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model       string          `json:"model"`
	Temperature float32         `json:"temperature"`
	Stream      bool            `json:"stream"`
	Messages    []openAIMessage `json:"messages"`
}

// streamChunk: satu event SSE OpenAI-style.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func (c *OpenAICompatibleClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	url := fmt.Sprintf("%s/chat/completions", c.BaseURL)
	payload := openAIRequest{
		Model:       c.Model,
		Temperature: 0.1,
		Stream:      true,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("gagal me-marshal request: %w", err)
	}

	// Tanpa Timeout di Client (streaming bisa lama); pakai ctx per-attempt sebagai batas.
	client := &http.Client{}
	maxRetries := 3
	baseDelay := 2 * time.Second
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		reqCtx, cancel := context.WithTimeout(ctx, 9*time.Minute)
		req, err := http.NewRequestWithContext(reqCtx, "POST", url, bytes.NewBuffer(jsonPayload))
		if err != nil {
			cancel()
			return "", fmt.Errorf("gagal membuat http request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		if c.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.APIKey)
		}

		resp, doErr := client.Do(req)
		if doErr != nil {
			cancel()
			lastErr = doErr
		} else if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			cancel()
			errMsg := string(body)
			var er struct {
				Error *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(body, &er) == nil && er.Error != nil {
				errMsg = er.Error.Message
			}
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				lastErr = fmt.Errorf("error dari provider (HTTP %d): %s", resp.StatusCode, errMsg)
			} else {
				return "", fmt.Errorf("fatal error dari provider (HTTP %d): %s", resp.StatusCode, errMsg)
			}
		} else {
			content, perr := readSSE(resp.Body)
			resp.Body.Close()
			cancel()
			if perr == nil && strings.TrimSpace(content) != "" {
				return content, nil
			}
			if perr != nil {
				lastErr = fmt.Errorf("gagal membaca stream: %w", perr)
			} else {
				lastErr = fmt.Errorf("stream kosong dari provider")
			}
		}

		if i < maxRetries-1 {
			delay := baseDelay * time.Duration(1<<i) // 2s, 4s
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}
	return "", fmt.Errorf("gagal setelah %d retries: %w", maxRetries, lastErr)
}

// readSSE membaca event-stream OpenAI dan menggabungkan delta.content.
func readSSE(body io.Reader) (string, error) {
	var sb strings.Builder
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // toleransi baris besar
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // lewati keepalive/komentar non-JSON
		}
		for _, ch := range chunk.Choices {
			sb.WriteString(ch.Delta.Content)
		}
	}
	if err := sc.Err(); err != nil {
		return sb.String(), err
	}
	return sb.String(), nil
}
