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

// AnthropicCompatibleClient adalah adapter untuk API berbasis Anthropic (termasuk Claude dan Aerolink).
type AnthropicCompatibleClient struct {
	APIKey  string
	BaseURL string
	Model   string
}

func NewAnthropicCompatibleClient(apiKey, baseURL, model string) *AnthropicCompatibleClient {
	return &AnthropicCompatibleClient{APIKey: apiKey, BaseURL: baseURL, Model: model}
}

func (c *AnthropicCompatibleClient) ModelName() string {
	return "anthropic/" + c.Model
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream"`
}

type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *AnthropicCompatibleClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	endpoint := c.BaseURL
	if !strings.HasSuffix(endpoint, "/messages") && !strings.HasSuffix(endpoint, "/messages/") {
		endpoint = strings.TrimRight(endpoint, "/") + "/v1/messages"
	}

	payload := anthropicRequest{
		Model:     c.Model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Stream:    true,
		Messages: []anthropicMessage{
			{Role: "user", Content: userPrompt},
		},
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("gagal me-marshal request: %w", err)
	}

	client := &http.Client{}
	maxRetries := 3
	baseDelay := 2 * time.Second
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		reqCtx, cancel := context.WithTimeout(ctx, requestTimeout())
		req, err := http.NewRequestWithContext(reqCtx, "POST", endpoint, bytes.NewBuffer(jsonPayload))
		if err != nil {
			cancel()
			return "", fmt.Errorf("gagal membuat http request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("x-api-key", c.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		// Aerolink or other proxies might expect Authorization header instead
		req.Header.Set("Authorization", "Bearer "+c.APIKey)

		resp, doErr := client.Do(req)
		if doErr != nil {
			cancel()
			lastErr = doErr
		} else if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			cancel()
			lastErr = fmt.Errorf("error dari provider (HTTP %d): %s", resp.StatusCode, string(body))
		} else {
			content, perr := readAnthropicSSE(resp.Body)
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
			delay := baseDelay * time.Duration(1<<i)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}
	return "", fmt.Errorf("gagal setelah %d retries: %w", maxRetries, lastErr)
}

func readAnthropicSSE(body io.Reader) (string, error) {
	var sb strings.Builder
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		if event.Type == "error" && event.Error != nil {
			return sb.String(), fmt.Errorf("error dari provider di tengah stream: %s", event.Error.Message)
		}
		if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
			sb.WriteString(event.Delta.Text)
		}
	}
	if err := sc.Err(); err != nil {
		return sb.String(), err
	}
	return sb.String(), nil
}
