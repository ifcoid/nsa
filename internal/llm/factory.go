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
	var client LLMClient
	switch config.ProviderName {
	case "gemini":
		client = NewGeminiClient(config.APIKey, config.DefaultModel)

	case "claude", "anthropic":
		baseURL := config.BaseURL
		if baseURL == "" {
			baseURL = "https://api.anthropic.com"
		}
		client = NewAnthropicCompatibleClient(config.APIKey, baseURL, config.DefaultModel)

	case "aerolink":
		baseURL := config.BaseURL
		if baseURL == "" {
			baseURL = "https://capi.aerolink.lat"
		}
		client = NewAnthropicCompatibleClient(config.APIKey, baseURL, config.DefaultModel)

	case "cohere":
		client = NewCohereClient(config.APIKey, config.DefaultModel)

	case "xiaomi":
		baseURL := config.BaseURL
		if baseURL == "" {
			baseURL = "https://token-plan-sgp.xiaomimimo.com/v1"
		}
		client = NewOpenAICompatibleClient(config.APIKey, baseURL, config.DefaultModel)

	case "unimodel":
		baseURL := config.BaseURL
		if baseURL == "" {
			baseURL = "https://unimodel.ai/v1"
		}
		client = NewOpenAICompatibleClient(config.APIKey, baseURL, config.DefaultModel)

	default:
		client = NewOpenAICompatibleClient(config.APIKey, config.BaseURL, config.DefaultModel)
	}

	// Wrap with xAI logging (transparently records every LLM call)
	return NewXAILoggingClient(client, f.mongoRepo), nil
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

// BrainClient membuat client untuk peran "brain" (gemini default) dengan retry+fallback.
func (f *LLMFactory) BrainClient(ctx context.Context) (LLMClient, error) {
	r := f.mongoRepo.GetLLMRoles(ctx)
	primary, errP := f.CreateClient(ctx, r.Brain)
	var fallback LLMClient
	if r.BrainFallback != "" {
		if fb, e := f.CreateClient(ctx, r.BrainFallback); e == nil {
			fallback = fb
		}
	}
	if errP != nil {
		if fallback != nil {
			return NewRetryingClient(nil, fallback), nil
		}
		return nil, errP
	}
	return NewRetryingClient(primary, fallback), nil
}

// RoleClient membuat client untuk sebuah peran: coba primary, lalu fallback (sesuai config).
// Mengembalikan client + provider id yang dipakai.
func (f *LLMFactory) RoleClient(ctx context.Context, role string) (LLMClient, string, error) {
	primaryID, fallbackID := f.RoleProviders(ctx, role)
	primary, errP := f.CreateClient(ctx, primaryID)
	var fallback LLMClient
	if fallbackID != "" {
		if fb, e := f.CreateClient(ctx, fallbackID); e == nil {
			fallback = fb
		}
	}
	if errP == nil {
		return NewRetryingClient(primary, fallback), primaryID, nil
	}
	if fallback != nil {
		return NewRetryingClient(nil, fallback), fallbackID, nil
	}
	return nil, "", fmt.Errorf("peran '%s': tidak ada provider yang bisa dimuat (primary=%s, fallback=%s)", role, primaryID, fallbackID)
}
