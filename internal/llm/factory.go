package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"nsa/internal/model"
	"nsa/internal/repository"
)

type LLMFactory struct {
	mongoRepo *repository.MongoRepository
}

func NewLLMFactory(repo *repository.MongoRepository) *LLMFactory {
	return &LLMFactory{mongoRepo: repo}
}

// CreateClient mengambil konfigurasi langsung dari DB berdasarkan ID provider
func (f *LLMFactory) CreateClient(ctx context.Context, providerID string) (LLMClient, error) {
	// 1. Ambil data konfig dari MongoDB
	config, err := f.mongoRepo.GetLLMConfig(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("gagal mengambil konfigurasi %s: %w", providerID, err)
	}

	if !config.IsActive {
		return nil, errors.New("provider ini sedang dinonaktifkan di database")
	}

	if strings.HasPrefix(config.APIKey, "GANTI_DENGAN_") {
		return nil, fmt.Errorf("⚠️ API Key untuk provider '%s' belum diatur! Silakan ubah data pada MongoDB dari '%s' menjadi API Key asli Anda", providerID, config.APIKey)
	}

	// 2. Tentukan jenis adapter yang harus dibuat
	switch config.ProviderName {
	case "gemini":
		return NewGeminiClient(config.APIKey, config.DefaultModel), nil

	case "claude":
		return NewClaudeClient(config.APIKey, config.DefaultModel), nil

	case "cohere":
		return NewCohereClient(config.APIKey, config.DefaultModel), nil

	case "xiaomi":
		baseURL := config.BaseURL
		if baseURL == "" {
			baseURL = "https://token-plan-sgp.xiaomimimo.com/v1"
		}
		return NewOpenAICompatibleClient(config.APIKey, baseURL, config.DefaultModel), nil

	default:
		return NewOpenAICompatibleClient(config.APIKey, config.BaseURL, config.DefaultModel), nil
	}
}

// Roles mengembalikan pemetaan peran->provider (config-driven, default bila kosong).
func (f *LLMFactory) Roles(ctx context.Context) *model.LLMRoles {
	return f.mongoRepo.GetLLMRoles(ctx)
}

// RoleProviders mengembalikan (primary, fallback) provider untuk sebuah peran.
func (f *LLMFactory) RoleProviders(ctx context.Context, role string) (primary, fallback string) {
	r := f.mongoRepo.GetLLMRoles(ctx)
	switch role {
	case "reviewer1":
		return r.Reviewer1, r.Reviewer1Fallback
	case "reviewer2":
		return r.Reviewer2, r.Reviewer2Fallback
	case "supervisor":
		return r.Supervisor, r.SupervisorFallback
	case "brain":
		return r.Brain, r.BrainFallback
	}
	return "", ""
}

// BrainClient membuat client untuk peran "brain" (gemini default): coba primary lalu fallback.
func (f *LLMFactory) BrainClient(ctx context.Context) (LLMClient, error) {
	r := f.mongoRepo.GetLLMRoles(ctx)
	c, err := f.CreateClient(ctx, r.Brain)
	if err == nil {
		return c, nil
	}
	if r.BrainFallback != "" {
		if c2, e2 := f.CreateClient(ctx, r.BrainFallback); e2 == nil {
			return c2, nil
		}
	}
	return nil, err
}

// RoleClient membuat client untuk sebuah peran: coba primary, lalu fallback (sesuai config).
// Mengembalikan client + provider id yang dipakai.
func (f *LLMFactory) RoleClient(ctx context.Context, role string) (LLMClient, string, error) {
	primary, fallback := f.RoleProviders(ctx, role)
	if primary != "" {
		if c, err := f.CreateClient(ctx, primary); err == nil {
			return c, primary, nil
		}
	}
	if fallback != "" {
		if c, err := f.CreateClient(ctx, fallback); err == nil {
			return c, fallback, nil
		}
	}
	return nil, "", fmt.Errorf("peran '%s': tidak ada provider yang bisa dimuat (primary=%s, fallback=%s)", role, primary, fallback)
}
