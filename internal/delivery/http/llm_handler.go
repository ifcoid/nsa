package http

import (
	"context"
	"encoding/json"
	"fmt"
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

	if payload.Provider == "" {
		sendJSONError(w, http.StatusBadRequest, "Provider is required")
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

	// API key kosong + belum pernah ada -> provider baru wajib isi key.
	if payload.APIKey == "" && config.APIKey == "" {
		sendJSONError(w, http.StatusBadRequest, "API Key wajib diisi untuk provider baru")
		return
	}

	// Update fields. API key kosong -> PERTAHANKAN yang lama (edit model/base_url tanpa
	// ketik ulang key). Sama seperti GitHub/Embed config.
	if payload.APIKey != "" {
		config.APIKey = payload.APIKey
	}
	if payload.DefaultModel != "" {
		config.DefaultModel = payload.DefaultModel
	}
	if payload.BaseURL != "" {
		config.BaseURL = payload.BaseURL
	} else {
		if payload.Provider == "groq" {
			config.BaseURL = "https://api.groq.com/openai/v1"
		} else if payload.Provider == "zhipu" || payload.Provider == "z-ai" {
			config.BaseURL = "https://open.bigmodel.cn/api/paas/v4"
		} else if payload.Provider == "xiaomi" {
			config.BaseURL = "https://token-plan-sgp.xiaomimimo.com/v1"
		} else if payload.Provider == "nvidia" {
			config.BaseURL = "https://integrate.api.nvidia.com/v1"
		} else if strings.HasPrefix(payload.Provider, "rprompt") {
			config.BaseURL = "https://rprompt.ll.my.id/v1"
		}
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

// GetGitHubConfig mengembalikan config GitHub (token DIREDAKSI).
func (h *LLMHandler) GetGitHubConfig(w http.ResponseWriter, req *http.Request) {
	cfg := h.mongoRepo.GetGitHubConfig(context.Background())
	tokenSet := cfg.Token != ""
	cfg.Token = "" // jangan kirim token ke klien
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{"config": cfg, "token_set": tokenSet})
}

// UpdateGitHubConfig menyimpan config GitHub. Token kosong -> pertahankan yang lama.
func (h *LLMHandler) UpdateGitHubConfig(w http.ResponseWriter, req *http.Request) {
	var cfg model.GitHubConfig
	if err := json.NewDecoder(req.Body).Decode(&cfg); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if cfg.Token == "" {
		cfg.Token = h.mongoRepo.GetGitHubConfig(context.Background()).Token // preserve
	}
	if err := h.mongoRepo.UpdateGitHubConfig(context.Background(), &cfg); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to update GitHub config: "+err.Error())
		return
	}
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{"message": "GitHub config updated"})
}

// GetEmbedConfig mengembalikan konfigurasi endpoint embedding (api_key disembunyikan).
func (h *LLMHandler) GetEmbedConfig(w http.ResponseWriter, req *http.Request) {
	cfg := h.mongoRepo.GetEmbedConfig(context.Background())
	keySet := cfg.APIKey != ""
	cfg.APIKey = ""
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{"config": cfg, "key_set": keySet})
}

// UpdateEmbedConfig menyimpan endpoint embedding (runtime). api_key kosong -> pertahankan lama.
func (h *LLMHandler) UpdateEmbedConfig(w http.ResponseWriter, req *http.Request) {
	var cfg model.EmbedConfig
	if err := json.NewDecoder(req.Body).Decode(&cfg); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if cfg.APIKey == "" {
		cfg.APIKey = h.mongoRepo.GetEmbedConfig(context.Background()).APIKey // preserve
	}
	if err := h.mongoRepo.UpdateEmbedConfig(context.Background(), &cfg); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to update embed config: "+err.Error())
		return
	}
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{"message": "Embed config updated"})
}

// GetScopusConfig mengembalikan konfigurasi Scopus API key (api_key disembunyikan).
func (h *LLMHandler) GetScopusConfig(w http.ResponseWriter, req *http.Request) {
	cfg := h.mongoRepo.GetScopusConfig(context.Background())
	keySet := cfg.APIKey != ""
	cfg.APIKey = ""
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{"config": cfg, "key_set": keySet})
}

// UpdateScopusConfig menyimpan API key Scopus (runtime). api_key kosong -> pertahankan lama.
func (h *LLMHandler) UpdateScopusConfig(w http.ResponseWriter, req *http.Request) {
	var cfg model.ScopusConfig
	if err := json.NewDecoder(req.Body).Decode(&cfg); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if cfg.APIKey == "" {
		cfg.APIKey = h.mongoRepo.GetScopusConfig(context.Background()).APIKey // preserve
	}
	if err := h.mongoRepo.UpdateScopusConfig(context.Background(), &cfg); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to update scopus config: "+err.Error())
		return
	}
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{"message": "Scopus config updated"})
}

// ListConfigs mengembalikan ringkasan provider yang TELAH dikonfigurasi: provider id +
// NAMA MODEL (default_model) + apakah API key sudah diisi + base_url. TANPA membocorkan
// API key. Dipakai UI Model Routing agar user tahu MODEL apa (bukan cuma provider) yang
// dipakai tiap role, dan provider mana yang belum dikonfigurasi.
func (h *LLMHandler) ListConfigs(w http.ResponseWriter, req *http.Request) {
	configs, err := h.mongoRepo.GetAllLLMConfigs(context.Background())
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to fetch LLM configs")
		return
	}
	type ConfigSummary struct {
		Provider     string `json:"provider"`
		DefaultModel string `json:"default_model"`
		HasKey       bool   `json:"has_key"`
		BaseURL      string `json:"base_url,omitempty"`
	}
	out := make([]ConfigSummary, 0, len(configs))
	for _, c := range configs {
		out = append(out, ConfigSummary{
			Provider:     c.ID,
			DefaultModel: c.DefaultModel,
			HasKey:       c.APIKey != "",
			BaseURL:      c.BaseURL,
		})
	}
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{"configs": out})
}

// GetRoles mengembalikan pemetaan peran->provider (Model Routing).
func (h *LLMHandler) GetRoles(w http.ResponseWriter, req *http.Request) {
	roles := h.mongoRepo.GetLLMRoles(context.Background())
	sendJSONResponse(w, http.StatusOK, roles)
}

// UpdateRoles menyimpan pemetaan peran->provider.
func (h *LLMHandler) UpdateRoles(w http.ResponseWriter, req *http.Request) {
	var roles model.LLMRoles
	if err := json.NewDecoder(req.Body).Decode(&roles); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.mongoRepo.UpdateLLMRoles(context.Background(), &roles); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to update LLM roles: "+err.Error())
		return
	}
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{"message": "LLM roles updated", "roles": roles})
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

	// API key kosong -> pakai key tersimpan (agar bisa muat ulang model untuk provider yang
	// sudah dikonfigurasi tanpa ketik ulang key). Base URL juga diisi dari config bila kosong.
	if payload.APIKey == "" {
		if cfg, err := h.mongoRepo.GetLLMConfig(context.Background(), provider); err == nil && cfg != nil {
			payload.APIKey = cfg.APIKey
			if payload.BaseURL == "" {
				payload.BaseURL = cfg.BaseURL
			}
		}
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

	case "cohere":
		url := "https://api.cohere.com/v1/models"
		httpReq, _ := http.NewRequest("GET", url, nil)
		httpReq.Header.Set("Authorization", "Bearer "+payload.APIKey)
		httpReq.Header.Set("Accept", "application/json")
		
		resp, err := client.Do(httpReq)
		if err != nil || resp.StatusCode != 200 {
			sendJSONError(w, http.StatusInternalServerError, "Failed to fetch from Cohere API")
			return
		}
		defer resp.Body.Close()

		var res struct {
			Models []struct {
				Name      string   `json:"name"`
				Endpoints []string `json:"endpoints"`
			} `json:"models"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
			for _, m := range res.Models {
				// Hanya tampilkan model yang mendukung chat endpoint
				isChat := false
				for _, ep := range m.Endpoints {
					if strings.Contains(ep, "chat") {
						isChat = true
						break
					}
				}
				if isChat {
					models = append(models, m.Name)
				}
			}
		}

	default: // groq, zhipu, rprompt, dll (OpenAI compatible)
		baseURL := payload.BaseURL
		if baseURL == "" {
			if provider == "groq" {
				baseURL = "https://api.groq.com/openai/v1"
			} else if provider == "zhipu" || provider == "z-ai" {
				baseURL = "https://open.bigmodel.cn/api/paas/v4"
			} else if provider == "openrouter" {
				baseURL = "https://openrouter.ai/api/v1"
			} else if provider == "xiaomi" {
				baseURL = "https://token-plan-sgp.xiaomimimo.com/v1"
			} else if provider == "nvidia" {
				baseURL = "https://integrate.api.nvidia.com/v1"
			} else if provider == "unimodel" {
				baseURL = "https://unimodel.ai/v1"
			} else if provider == "aerolink" {
				baseURL = "https://capi.aerolink.lat/v1"
			} else if strings.HasPrefix(provider, "rprompt") {
				baseURL = "https://rprompt.ll.my.id/v1"
			} else {
				sendJSONError(w, http.StatusBadRequest, "Base URL is required for this provider")
				return
			}
		}
		
		// Pastikan tidak berakhiran slash
		baseURL = strings.TrimSuffix(baseURL, "/")
		if provider == "aerolink" && !strings.HasSuffix(baseURL, "/v1") {
			baseURL += "/v1"
		}
		url := baseURL + "/models"
		
		httpReq, _ := http.NewRequest("GET", url, nil)
		httpReq.Header.Set("Authorization", "Bearer "+payload.APIKey)
		
		resp, err := client.Do(httpReq)
		if err != nil {
			sendJSONError(w, http.StatusInternalServerError, "Request failed: "+err.Error())
			return
		}
		if resp.StatusCode != 200 {
			// Read the body for error message but limit the size
			buf := make([]byte, 1024)
			n, _ := resp.Body.Read(buf)
			resp.Body.Close()
			errMsg := fmt.Sprintf("API returned status %d: %s", resp.StatusCode, string(buf[:n]))
			sendJSONError(w, http.StatusInternalServerError, errMsg)
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

		// Inject known hidden models for Zhipu
		if provider == "zhipu" || provider == "z-ai" {
			missingModels := []string{"glm-4.7-flash", "glm-4.5-flash"}
			for _, mm := range missingModels {
				found := false
				for _, exist := range models {
					if exist == mm {
						found = true
						break
					}
				}
				if !found {
					// Prepend so it appears at the very top of the dropdown
					models = append([]string{mm}, models...)
				}
			}
		}
	}

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"provider": provider,
		"models":   models,
	})
}

// CheckHealth mengecek status kesehatan dan kuota semua API LLM
func (h *LLMHandler) CheckHealth(w http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	configs, err := h.mongoRepo.GetAllLLMConfigs(ctx)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to fetch LLM configs")
		return
	}

	type HealthResult struct {
		Provider string `json:"provider"`
		Status   string `json:"status"` // "ALIVE", "UNAUTHORIZED", "QUOTA_EXCEEDED", "ERROR"
		Message  string `json:"message"`
	}

	results := make([]HealthResult, len(configs))
	errCh := make(chan struct {
		index int
		res   HealthResult
	}, len(configs))

	client := &http.Client{Timeout: 10 * time.Second}

	for i, cfg := range configs {
		go func(idx int, c model.LLMConfig) {
			res := HealthResult{Provider: c.ID, Status: "ERROR", Message: "Timeout or unknown error"}
			
			if c.APIKey == "" {
				res.Status = "UNAUTHORIZED"
				res.Message = "API Key kosong"
				errCh <- struct {
					index int
					res   HealthResult
				}{idx, res}
				return
			}

			// Tentukan URL pengecekan (biasanya /v1/models)
			var url string
			reqMethod := "GET"
			
			if strings.HasPrefix(c.ProviderName, "gemini") || c.ProviderName == "gemini" {
				url = fmt.Sprintf("%s/models?key=%s", c.BaseURL, c.APIKey)
			} else if c.ProviderName == "claude" {
				url = "https://api.anthropic.com/v1/models"
			} else if c.ProviderName == "cohere" {
				url = "https://api.cohere.com/v1/models"
			} else {
				// Default OpenAI-compatible
				baseURL := c.BaseURL
				if baseURL == "" {
					baseURL = "https://api.openai.com/v1"
				}
				baseURL = strings.TrimSuffix(baseURL, "/")
				if c.ProviderName == "aerolink" && !strings.HasSuffix(baseURL, "/v1") {
					baseURL += "/v1"
				}
				url = baseURL + "/models"
			}

			httpReq, _ := http.NewRequestWithContext(ctx, reqMethod, url, nil)
			
			// Set headers
			if c.ProviderName == "claude" {
				httpReq.Header.Set("x-api-key", c.APIKey)
				httpReq.Header.Set("anthropic-version", "2023-06-01")
			} else if c.ProviderName == "cohere" {
				httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
			} else if c.ProviderName != "gemini" {
				httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
			}
			httpReq.Header.Set("Accept", "application/json")

			resp, err := client.Do(httpReq)
			if err != nil {
				res.Message = err.Error()
			} else {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					res.Status = "ALIVE"
					res.Message = "API Sehat"
				} else if resp.StatusCode == 401 || resp.StatusCode == 403 {
					res.Status = "UNAUTHORIZED"
					res.Message = "API Key tidak valid (401/403)"
				} else if resp.StatusCode == 429 {
					res.Status = "QUOTA_EXCEEDED"
					res.Message = "Kuota habis / Rate limit (429)"
				} else {
					res.Status = "ERROR"
					res.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
				}
			}

			errCh <- struct {
				index int
				res   HealthResult
			}{idx, res}

		}(i, cfg)
	}

	for i := 0; i < len(configs); i++ {
		r := <-errCh
		results[r.index] = r.res
	}

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"health": results,
	})
}

