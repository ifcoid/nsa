package publisher

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nsa/internal/model"
)

// PublishSVG meng-upload (create/update) file SVG ke repo GitHub via Contents API.
// Mengembalikan URL publik (GitHub Pages bila pages_url diisi; selain itu html_url repo).
func PublishSVG(ctx context.Context, cfg *model.GitHubConfig, relPath, svg string) (string, error) {
	if !cfg.IsReady() {
		return "", fmt.Errorf("konfigurasi GitHub belum siap (enabled/token/owner/repo)")
	}
	path := strings.Trim(cfg.BasePath, "/")
	if path != "" {
		path += "/"
	}
	path += relPath
	api := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", cfg.Owner, cfg.Repo, path)
	client := &http.Client{Timeout: 30 * time.Second}

	// 1. Ambil sha file existing (untuk update). Abaikan error (berarti file baru).
	sha := ""
	if req, e := http.NewRequestWithContext(ctx, "GET", api+"?ref="+cfg.Branch, nil); e == nil {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
		if resp, err := client.Do(req); err == nil {
			if resp.StatusCode == 200 {
				var m map[string]interface{}
				json.NewDecoder(resp.Body).Decode(&m)
				if s, ok := m["sha"].(string); ok {
					sha = s
				}
			}
			resp.Body.Close()
		}
	}

	// 2. PUT contents.
	payload := map[string]interface{}{
		"message": "chore(figures): publish " + relPath,
		"content": base64.StdEncoding.EncodeToString([]byte(svg)),
		"branch":  cfg.Branch,
	}
	if sha != "" {
		payload["sha"] = sha
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "PUT", api, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub PUT status %d: %s", resp.StatusCode, string(b))
	}

	if cfg.PagesURL != "" {
		return strings.TrimRight(cfg.PagesURL, "/") + "/" + path, nil
	}
	var out struct {
		Content struct {
			HTMLURL string `json:"html_url"`
		} `json:"content"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if out.Content.HTMLURL != "" {
		return out.Content.HTMLURL, nil
	}
	return path, nil
}
