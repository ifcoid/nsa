package llm

import (
	"context"
	"log"
	"strings"
	"time"

	"nsa/internal/model"
	"nsa/internal/repository"
)

// displayModelName membuang prefix ADAPTER (mis. "openai/", "claude/", "gemini/")
// dari ModelName() agar yang tersimpan adalah NAMA MODEL asli. Tanpa ini, model groq
// yang id-nya sudah mengandung slash (mis. "openai/gpt-oss-120b") tampil dobel:
// adapter "openai/" + model "openai/gpt-oss-120b" = "openai/openai/gpt-oss-120b".
// "openai/" di sini hanya tipe adapter API, bukan vendor (lih. CLAUDE.md atribusi xAI).
func displayModelName(raw string) string {
	if k := strings.Index(raw, "/"); k >= 0 {
		return raw[k+1:]
	}
	return raw
}

// xaiLoggingClient wraps an LLMClient and logs every Generate call to MongoDB
// as an xAI audit entry (non-blocking).
type xaiLoggingClient struct {
	inner LLMClient
	repo  *repository.MongoRepository
}

// NewXAILoggingClient creates a logging wrapper that transparently records every LLM
// interaction to the session's xai_log field in MongoDB.
func NewXAILoggingClient(inner LLMClient, repo *repository.MongoRepository) LLMClient {
	if repo == nil {
		return inner
	}
	return &xaiLoggingClient{inner: inner, repo: repo}
}

func (c *xaiLoggingClient) ModelName() string {
	return c.inner.ModelName()
}

func (c *xaiLoggingClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	xaiCtx, ok := XAIContextFrom(ctx)
	if !ok {
		// No xAI context attached, just delegate without logging
		return c.inner.Generate(ctx, systemPrompt, userPrompt)
	}

	start := time.Now()
	result, err := c.inner.Generate(ctx, systemPrompt, userPrompt)
	duration := time.Since(start).Milliseconds()

	// Truncate user prompt to max 500 chars for storage efficiency
	preview := userPrompt
	if len(preview) > 500 {
		preview = preview[:500] + "..."
	}

	entry := model.XAIEntry{
		Step:              xaiCtx.Step,
		AgentFunc:         xaiCtx.AgentFunc,
		ModelName:         displayModelName(c.inner.ModelName()),
		SystemPrompt:      systemPrompt,
		UserPromptPreview: preview,
		Timestamp:         time.Now(),
		DurationMs:        duration,
	}

	// Non-blocking append: use a goroutine with a background context so that
	// a cancelled request context does not prevent the log from being written.
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if appendErr := c.repo.AppendXAIEntry(bgCtx, xaiCtx.SessionID, entry); appendErr != nil {
			log.Printf("[xAI] failed to append entry for session %s: %v", xaiCtx.SessionID, appendErr)
		}
	}()

	return result, err
}
