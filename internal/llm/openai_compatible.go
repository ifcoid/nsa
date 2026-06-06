package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// requestTimeout adalah batas per-attempt untuk satu panggilan LLM. Default GENEROUS
// (45 menit) karena model besar (opus) menyusun section manuskrip panjang bisa lama
// (draft → verifikasi → guardrail → verifikasi lagi; bisa 30-60 menit). Streaming SSE
// menjaga koneksi tetap hidup sepanjang itu (hindari Cloudflare 524). Bisa diubah via
// env LLM_REQUEST_TIMEOUT_MINUTES tanpa redeploy kode.
// Catatan: screening tetap 60s karena dibungkus ctx pendek oleh reviewWithRetry
// (min(60s, timeout ini) = 60s), jadi nilai besar ini hanya berlaku untuk generasi.
func requestTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("LLM_REQUEST_TIMEOUT_MINUTES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}
	return 60 * time.Minute
}

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
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
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
		reqCtx, cancel := context.WithTimeout(ctx, requestTimeout())
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
			
			if strings.HasPrefix(strings.TrimSpace(content), "[error]") {
				return "", fmt.Errorf("provider merespons dengan error: %s", content)
			}

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
		if chunk.Error != nil && chunk.Error.Message != "" {
			return sb.String(), fmt.Errorf("error dari provider di tengah stream: %s", chunk.Error.Message)
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
