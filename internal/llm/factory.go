package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

	case "xiaomi_openai":
		baseURL := config.BaseURL
		if baseURL == "" {
			baseURL = "https://token-plan-sgp.xiaomimimo.com/v1"
		}
		return NewOpenAICompatibleClient(config.APIKey, baseURL, config.DefaultModel), nil

	case "xiaomi_anthropic":
		baseURL := config.BaseURL
		if baseURL == "" {
			baseURL = "https://token-plan-sgp.xiaomimimo.com/anthropic"
		}
		// Gunakan OpenAICompatibleClient karena banyak proxy menerjemahkan format OpenAI -> Anthropic di sisi server.
		return NewOpenAICompatibleClient(config.APIKey, baseURL, config.DefaultModel), nil

	default:
		return NewOpenAICompatibleClient(config.APIKey, config.BaseURL, config.DefaultModel), nil
	}
}
