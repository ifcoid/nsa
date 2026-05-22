package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"nsa/internal/model"
	"nsa/internal/repository"
)

type LLMHandler struct {
	mongoRepo *repository.MongoRepository
}

func NewLLMHandler(mongoRepo *repository.MongoRepository) *LLMHandler {
	return &LLMHandler{
		mongoRepo: mongoRepo,
	}
}

func (h *LLMHandler) UpdateConfig(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		Provider     string `json:"provider"`
		APIKey       string `json:"api_key"`
		DefaultModel string `json:"default_model,omitempty"`
		BaseURL      string `json:"base_url,omitempty"`
	}

	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if payload.Provider == "" || payload.APIKey == "" {
		sendJSONError(w, http.StatusBadRequest, "Provider and APIKey are required")
		return
	}

	ctx := context.Background()
	
	// Fetch existing config to update or create new
	config, err := h.mongoRepo.GetLLMConfig(ctx, payload.Provider)
	if err != nil {
		// Doesn't exist, create default structure
		config = &model.LLMConfig{
			ID:           payload.Provider,
			ProviderName: payload.Provider, // default mapping
			IsActive:     true,
		}
	}

	// Update fields
	config.APIKey = payload.APIKey
	if payload.DefaultModel != "" {
		config.DefaultModel = payload.DefaultModel
	}
	if payload.BaseURL != "" {
		config.BaseURL = payload.BaseURL
	}
	config.UpdatedAt = time.Now()

	err = h.mongoRepo.UpdateLLMConfig(ctx, config)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to update LLM config")
		return
	}

	sendJSONResponse(w, http.StatusOK, map[string]string{
		"message": "LLM config updated successfully",
		"provider": payload.Provider,
	})
}
