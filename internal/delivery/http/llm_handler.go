package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"nsa/internal/llm"
	"nsa/internal/model"
	"nsa/internal/repository"
)

// preflightRoles = daftar role yang BENAR-BENAR dipakai pipeline SLR (M1-M9). Pre-flight
// menguji tiap role (primary + fallback) dengan panggilan generate NYATA — beda dari
// CheckHealth yang hanya GET /models (cek konektivitas/kunci, TIDAK menangkap nama model
// salah/terkunci yang baru muncul 404 saat generate). Urutan = urutan tampil di UI.
var preflightRoles = []string{"reviewer1", "reviewer2", "supervisor", "brain", "auditor"}

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
		"message":  "LLM config updated successfully",
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

// TestModel menguji apakah MODEL yang dikonfigurasi untuk sebuah provider benar-benar BISA
// DIPAKAI — bukan sekadar API key valid. Beda dari CheckHealth (/v1/models = cek key saja):
// ini melakukan satu pemanggilan completion NYATA ke model default provider, sehingga
// menangkap kasus "model terkunci / tak tersedia untuk akun" (mis. nvidia HTTP 404
// "Function ... Not found for account") yang TAK terdeteksi health check biasa.
func (h *LLMHandler) TestModel(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		Provider string `json:"provider"`
		Role     string `json:"role,omitempty"`    // alternatif: uji provider yang dipakai sebuah role
		APIKey   string `json:"api_key,omitempty"` // override (uji config BELUM disimpan)
		BaseURL  string `json:"base_url,omitempty"`
		Model    string `json:"model,omitempty"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	factory := llm.NewLLMFactory(h.mongoRepo)
	provider := strings.TrimSpace(payload.Provider)
	if provider == "" && payload.Role != "" {
		provider, _ = factory.RoleProviders(context.Background(), payload.Role)
	}
	if provider == "" {
		sendJSONError(w, http.StatusBadRequest, "provider atau role wajib diisi")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	saved, _ := h.mongoRepo.GetLLMConfig(context.Background(), provider)
	var client llm.LLMClient
	var modelName string

	if payload.APIKey != "" || payload.Model != "" || payload.BaseURL != "" {
		// Uji config BELUM DISIMPAN: bangun config sementara dari form + fallback ke tersimpan.
		cfg := &model.LLMConfig{ID: provider, ProviderName: provider, IsActive: true}
		if saved != nil {
			*cfg = *saved
			cfg.IsActive = true
		}
		if payload.APIKey != "" {
			cfg.APIKey = payload.APIKey
		}
		if payload.Model != "" {
			cfg.DefaultModel = payload.Model
		}
		if payload.BaseURL != "" {
			cfg.BaseURL = payload.BaseURL
		}
		if cfg.BaseURL == "" { // isi default utk provider tanpa field base URL (groq/zhipu)
			switch provider {
			case "groq":
				cfg.BaseURL = "https://api.groq.com/openai/v1"
			case "zhipu", "z-ai":
				cfg.BaseURL = "https://open.bigmodel.cn/api/paas/v4"
			}
		}
		if cfg.APIKey == "" {
			sendJSONResponse(w, http.StatusOK, map[string]interface{}{
				"ok": false, "provider": provider, "model": cfg.DefaultModel,
				"message": "API Key kosong — isi API Key (atau simpan dulu) untuk menguji.",
			})
			return
		}
		modelName = cfg.DefaultModel
		client = factory.ClientFromConfig(cfg)
	} else {
		// Uji config TERSIMPAN.
		if saved != nil {
			modelName = saved.DefaultModel
		}
		c, err := factory.CreateClient(ctx, provider)
		if err != nil {
			sendJSONResponse(w, http.StatusOK, map[string]interface{}{
				"ok": false, "provider": provider, "model": modelName,
				"message": "Gagal memuat client (cek API key/base URL): " + err.Error(),
			})
			return
		}
		client = c
	}
	out, gerr := client.Generate(ctx, "Jawab satu kata: ok", "ok")
	if gerr != nil {
		sendJSONResponse(w, http.StatusOK, map[string]interface{}{
			"ok": false, "provider": provider, "model": modelName, "message": gerr.Error(),
		})
		return
	}
	sample := strings.TrimSpace(out)
	if len(sample) > 120 {
		sample = sample[:120]
	}
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"ok": true, "provider": provider, "model": modelName, "sample": sample,
	})
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

// PreflightRoles menguji SEMUA role pipeline (primary + fallback) dengan generate NYATA,
// di awal — agar provider rusak (404 model salah/terkunci, 401 key, 429 kuota) ketahuan
// SEBELUM run panjang (ekstraksi/QA ratusan paper) terlanjur jalan. Tiap provider UNIK
// diuji sekali saja (banyak role berbagi provider → hemat waktu & kuota), lalu hasil
// dipetakan balik ke tiap role. Menguji config TERSIMPAN (yang dipakai pipeline).
func (h *LLMHandler) PreflightRoles(w http.ResponseWriter, req *http.Request) {
	factory := llm.NewLLMFactory(h.mongoRepo)
	bg := context.Background()

	// 1) Resolusi role → (primary, fallback) + kumpulkan himpunan provider unik.
	roleProv := make(map[string][2]string, len(preflightRoles))
	uniq := make(map[string]bool)
	for _, role := range preflightRoles {
		p, f := factory.RoleProviders(bg, role)
		roleProv[role] = [2]string{p, f}
		if p != "" {
			uniq[p] = true
		}
		if f != "" {
			uniq[f] = true
		}
	}

	// 2) Smoke-test tiap provider unik secara paralel (satu generate kecil per provider).
	type provResult struct {
		ok    bool
		model string
		msg   string
	}
	provResults := make(map[string]provResult, len(uniq))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for prov := range uniq {
		wg.Add(1)
		go func(prov string) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(bg, 45*time.Second)
			defer cancel()
			pr := provResult{}
			if cfg, _ := h.mongoRepo.GetLLMConfig(bg, prov); cfg != nil {
				pr.model = cfg.DefaultModel
			}
			c, err := factory.CreateClient(ctx, prov)
			if err != nil {
				pr.ok, pr.msg = false, "Gagal memuat client (cek API key/base URL): "+err.Error()
			} else if _, gerr := c.Generate(ctx, "Anda asisten uji koneksi.", "Jawab satu kata: ok"); gerr != nil {
				pr.ok, pr.msg = false, gerr.Error()
			} else {
				pr.ok, pr.msg = true, "OK"
			}
			mu.Lock()
			provResults[prov] = pr
			mu.Unlock()
		}(prov)
	}
	wg.Wait()

	// 3) Susun hasil per-role: primary + fallback + apakah role bisa dipakai (salah satu OK).
	type RolePreflight struct {
		Role            string `json:"role"`
		Primary         string `json:"primary"`
		PrimaryModel    string `json:"primary_model"`
		PrimaryOK       bool   `json:"primary_ok"`
		PrimaryMessage  string `json:"primary_message"`
		Fallback        string `json:"fallback"`
		FallbackModel   string `json:"fallback_model"`
		FallbackOK      bool   `json:"fallback_ok"`
		FallbackMessage string `json:"fallback_message"`
		Usable          bool   `json:"usable"`
	}
	out := make([]RolePreflight, 0, len(preflightRoles))
	allUsable := true
	for _, role := range preflightRoles {
		pp, fp := roleProv[role][0], roleProv[role][1]
		pr := provResults[pp]
		fr := provResults[fp]
		usable := pr.ok || (fp != "" && fr.ok)
		if !usable {
			allUsable = false
		}
		rp := RolePreflight{
			Role:    role,
			Primary: pp, PrimaryModel: pr.model, PrimaryOK: pr.ok, PrimaryMessage: pr.msg,
			Usable: usable,
		}
		if fp != "" {
			rp.Fallback, rp.FallbackModel, rp.FallbackOK, rp.FallbackMessage = fp, fr.model, fr.ok, fr.msg
		}
		out = append(out, rp)
	}

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"all_usable": allUsable,
		"roles":      out,
	})
}
