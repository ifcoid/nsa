package llm

import (
	"context"
	"errors"
	"fmt"
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

	// 2. Tentukan jenis adapter yang harus dibuat
	switch config.ProviderName {
	case "gemini":
		return NewGeminiClient(config.APIKey, config.DefaultModel), nil

	case "claude":
		return NewClaudeClient(config.APIKey, config.DefaultModel), nil

	// Sisanya masuk ke jalur universal (OpenAI Compatible)
	default:
		return NewOpenAICompatibleClient(config.APIKey, config.BaseURL, config.DefaultModel), nil
	}
}
