package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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

// FetchModels mengambil daftar model langsung dari API Vendor menggunakan API Key yang diberikan
func (h *LLMHandler) FetchModels(w http.ResponseWriter, req *http.Request) {
	provider := req.PathValue("id")
	if provider == "" {
		sendJSONError(w, http.StatusBadRequest, "Provider ID is required")
		return
	}

	var payload struct {
		APIKey  string `json:"api_key"`
		BaseURL string `json:"base_url,omitempty"`
	}

	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if payload.APIKey == "" && provider != "claude" {
		sendJSONError(w, http.StatusBadRequest, "API Key is required to fetch models")
		return
	}

	var models []string

	client := &http.Client{Timeout: 10 * time.Second}

	switch provider {
	case "gemini":
		url := "https://generativelanguage.googleapis.com/v1beta/models?key=" + payload.APIKey
		resp, err := client.Get(url)
		if err != nil || resp.StatusCode != 200 {
			sendJSONError(w, http.StatusInternalServerError, "Failed to fetch from Gemini API")
			return
		}
		defer resp.Body.Close()

		var res struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
			for _, m := range res.Models {
				// Format name is usually "models/gemini-1.5-pro", we extract the part after slash
				parts := strings.Split(m.Name, "/")
				if len(parts) > 1 {
					models = append(models, parts[1])
				} else {
					models = append(models, m.Name)
				}
			}
		}

	case "claude":
		url := "https://api.anthropic.com/v1/models"
		httpReq, _ := http.NewRequest("GET", url, nil)
		httpReq.Header.Set("x-api-key", payload.APIKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
		
		resp, err := client.Do(httpReq)
		if err != nil || resp.StatusCode != 200 {
			sendJSONError(w, http.StatusInternalServerError, "Failed to fetch from Anthropic API")
			return
		}
		defer resp.Body.Close()

		var res struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
			for _, m := range res.Data {
				models = append(models, m.ID)
			}
		}

	default: // groq, zhipu (OpenAI compatible)
		baseURL := payload.BaseURL
		if baseURL == "" {
			sendJSONError(w, http.StatusBadRequest, "Base URL is required for this provider")
			return
		}
		
		// Pastikan tidak berakhiran slash
		baseURL = strings.TrimSuffix(baseURL, "/")
		url := baseURL + "/models"
		
		httpReq, _ := http.NewRequest("GET", url, nil)
		httpReq.Header.Set("Authorization", "Bearer "+payload.APIKey)
		
		resp, err := client.Do(httpReq)
		if err != nil || resp.StatusCode != 200 {
			sendJSONError(w, http.StatusInternalServerError, "Failed to fetch from OpenAI-compatible API")
			return
		}
		defer resp.Body.Close()

		var res struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
			for _, m := range res.Data {
				models = append(models, m.ID)
			}
		}
	}

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"provider": provider,
		"models":   models,
	})
}
