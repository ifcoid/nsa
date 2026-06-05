package modules

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// EmbedText meng-embed teks query memakai endpoint embedding OpenAI-compatible.
// WAJIB model BGE-M3 (atau yang menghasilkan ruang vektor sama dengan chunk di
// Qdrant, dense dim 1024) — kalau beda model, hasil pencarian top-k jadi ngawur.
//
// Dikonfigurasi via environment (config-driven, tidak hardcode):
//   EMBED_ENDPOINT  base URL OpenAI-compatible, mis. https://api.siliconflow.cn/v1
//                   atau URL self-host (VPS/Colab+cloudflared). Kosong => nonaktif.
//   EMBED_API_KEY   bearer token (boleh kosong untuk self-host tanpa auth).
//   EMBED_MODEL     default "BAAI/bge-m3".
//
// available=false bila EMBED_ENDPOINT kosong; caller harus fallback ke konteks
// penuh/terpotong (bukan error).
func EmbedText(ctx context.Context, text string) (vec []float32, available bool, err error) {
	return EmbedWith(ctx, text, os.Getenv("EMBED_ENDPOINT"), os.Getenv("EMBED_API_KEY"), os.Getenv("EMBED_MODEL"))
}

// EmbedWith sama seperti EmbedText namun memakai konfigurasi endpoint eksplisit
// (dari embed_config DB yang bisa diubah runtime via web), bukan env.
func EmbedWith(ctx context.Context, text, endpoint, apiKey, model string) (vec []float32, available bool, err error) {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/ ")
	if base == "" {
		return nil, false, nil
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "BAAI/bge-m3"
	}
	key := strings.TrimSpace(apiKey)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model": model,
		"input": text,
	})

	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(cctx, "POST", base+"/embeddings", bytes.NewReader(reqBody))
	if e != nil {
		return nil, true, e
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, e := (&http.Client{Timeout: 35 * time.Second}).Do(req)
	if e != nil {
		return nil, true, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, true, fmt.Errorf("embed status %d: %s", resp.StatusCode, string(b))
	}

	var out struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if e := json.NewDecoder(resp.Body).Decode(&out); e != nil {
		return nil, true, e
	}
	if len(out.Data) == 0 || len(out.Data[0].Embedding) == 0 {
		return nil, true, fmt.Errorf("embed: respons kosong")
	}
	vec = make([]float32, len(out.Data[0].Embedding))
	for i, f := range out.Data[0].Embedding {
		vec[i] = float32(f)
	}
	return vec, true, nil
}
