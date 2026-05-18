package model

import "time"

type LLMConfig struct {
	ID           string    `bson:"_id"` // Contoh: "deepseek", "gemini"
	ProviderName string    `bson:"provider_name"`
	BaseURL      string    `bson:"base_url"`
	APIKey       string    `bson:"api_key"`
	DefaultModel string    `bson:"default_model"`
	IsActive     bool      `bson:"is_active"`
	UpdatedAt    time.Time `bson:"updated_at"`
}
