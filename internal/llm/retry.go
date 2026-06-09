package llm

import (
	"context"
	"math/rand"
	"strings"
	"time"
)

// retryingClient membungkus client primary (+fallback opsional). Generate akan retry
// dengan backoff pada error transient (429/503/quota/overload/timeout), lalu fallback
// ke provider cadangan bila primary tetap gagal.
type retryingClient struct {
	primary  LLMClient
	fallback LLMClient // boleh nil
}

// NewRetryingClient membuat client dengan retry+fallback. Jika primary nil, fallback dipakai.
func NewRetryingClient(primary, fallback LLMClient) LLMClient {
	return &retryingClient{primary: primary, fallback: fallback}
}

func (c *retryingClient) ModelName() string {
	if c.primary != nil {
		return c.primary.ModelName()
	}
	if c.fallback != nil {
		return c.fallback.ModelName()
	}
	return "unknown"
}

func (c *retryingClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if c.primary == nil && c.fallback != nil {
		return generateWithBackoff(ctx, c.fallback, systemPrompt, userPrompt, []time.Duration{5 * time.Second, 20 * time.Second})
	}
	out, err := generateWithBackoff(ctx, c.primary, systemPrompt, userPrompt,
		[]time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second})
	if err == nil {
		return out, nil
	}
	if c.fallback != nil {
		if out2, err2 := generateWithBackoff(ctx, c.fallback, systemPrompt, userPrompt,
			[]time.Duration{5 * time.Second, 20 * time.Second}); err2 == nil {
			return out2, nil
		}
	}
	return "", err
}

// generateWithBackoff mencoba ulang pada error transient dengan exponential backoff + jitter.
func generateWithBackoff(ctx context.Context, client LLMClient, sys, usr string, delays []time.Duration) (string, error) {
	var out string
	var err error
	for attempt := 0; attempt <= len(delays); attempt++ {
		out, err = client.Generate(ctx, sys, usr)
		if err == nil {
			return out, nil
		}
		if attempt == len(delays) || !isTransient(err) {
			return "", err
		}
		base := delays[attempt]
		jitter := time.Duration((rand.Float64()*0.4 - 0.2) * float64(base))
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(base + jitter):
		}
	}
	return out, err
}

// isTransient menebak apakah error layak di-retry (rate limit / unavailable / timeout).
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	keys := []string{
		"429", "500", "502", "503", "504",
		"rate limit", "ratelimit", "rate-limit", "quota",
		"overload", "unavailable", "high demand", "temporarily", "try again",
		"timeout", "deadline", "connection reset", "eof",
		"速率", "限制", // pesan rate-limit zhipu (mandarin)
	}
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}
