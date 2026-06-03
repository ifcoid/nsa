package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const maxFulltextChars = 24000 // batasi panjang konteks RAG per paper agar prompt aman

// normalizeDOIForRAG menormalkan DOI dengan aturan yang sama seperti SyncQdrant
// (lowercase, strip prefix doi.org, normalisasi ligatur) agar pencocokan konsisten.
func normalizeDOIForRAG(d string) string {
	d = strings.TrimPrefix(d, "https://doi.org/")
	d = strings.TrimPrefix(d, "http://doi.org/")
	d = strings.ToLower(strings.TrimSpace(d))
	rep := map[string]string{
		"ﬀ": "ff", "ﬁ": "fi", "ﬂ": "fl",
		"ﬃ": "ffi", "ﬄ": "ffl", "ﬅ": "ft", "ﬆ": "st",
	}
	for k, v := range rep {
		d = strings.ReplaceAll(d, k, v)
	}
	return d
}

type ragChunk struct {
	idx     float64
	content string
}

// BuildFulltextIndex melakukan scroll seluruh collection Qdrant `scientific_articles`
// dan mengembalikan map[normalizedDOI] -> teks full-text tergabung (urut chunk_index).
// available=false jika environment Qdrant belum diset (mode tanpa RAG).
func BuildFulltextIndex(ctx context.Context) (index map[string]string, available bool, err error) {
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = os.Getenv("QDRANT_ENDPOINT")
	}
	if qdrantURL == "" {
		return nil, false, nil // RAG tidak tersedia
	}
	qdrantKey := os.Getenv("QDRANT_API_KEY")

	client := &http.Client{Timeout: 60 * time.Second}
	raw := make(map[string][]ragChunk)
	counter := make(map[string]float64) // fallback order jika chunk_index tak ada

	var nextOffset string
	for {
		reqBody := `{"limit": 2000, "with_payload": ["doi", "content", "chunk_index"]}`
		if nextOffset != "" {
			reqBody = fmt.Sprintf(`{"limit": 2000, "with_payload": ["doi", "content", "chunk_index"], "offset": "%s"}`, nextOffset)
		}

		req, e := http.NewRequestWithContext(ctx, "POST",
			fmt.Sprintf("%s/collections/scientific_articles/points/scroll", qdrantURL),
			strings.NewReader(reqBody))
		if e != nil {
			return nil, true, e
		}
		req.Header.Set("Content-Type", "application/json")
		if qdrantKey != "" {
			req.Header.Set("api-key", qdrantKey)
		}

		resp, e := client.Do(req)
		if e != nil {
			return nil, true, e
		}
		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, true, fmt.Errorf("qdrant scroll status %d: %s", resp.StatusCode, string(b))
		}

		var qResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&qResp)
		resp.Body.Close()

		result, ok := qResp["result"].(map[string]interface{})
		if !ok {
			break
		}
		points, _ := result["points"].([]interface{})
		for _, pt := range points {
			pMap, ok := pt.(map[string]interface{})
			if !ok {
				continue
			}
			payload, ok := pMap["payload"].(map[string]interface{})
			if !ok {
				continue
			}
			doi, _ := payload["doi"].(string)
			if doi == "" {
				continue // tanpa DOI tak bisa dipetakan balik ke MongoDB
			}
			content, _ := payload["content"].(string)
			if content == "" {
				continue
			}
			nd := normalizeDOIForRAG(doi)
			var idx float64
			if ci, ok := payload["chunk_index"].(float64); ok {
				idx = ci
			} else {
				idx = counter[nd]
				counter[nd]++
			}
			raw[nd] = append(raw[nd], ragChunk{idx: idx, content: content})
		}

		offsetVal, has := result["next_page_offset"]
		if has && offsetVal != nil {
			if s, ok := offsetVal.(string); ok && s != "" {
				nextOffset = s
				continue
			}
		}
		break
	}

	index = make(map[string]string, len(raw))
	for doi, chunks := range raw {
		sort.Slice(chunks, func(i, j int) bool { return chunks[i].idx < chunks[j].idx })
		var sb strings.Builder
		for _, c := range chunks {
			if sb.Len()+len(c.content)+2 > maxFulltextChars {
				sb.WriteString("\n\n[...teks dipotong untuk batas konteks...]")
				break
			}
			sb.WriteString(c.content)
			sb.WriteString("\n\n")
		}
		index[doi] = strings.TrimSpace(sb.String())
	}
	return index, true, nil
}

// ── Top-k semantic RAG ──────────────────────────────────────────────────────
// FulltextRAG menyimpan chunk BESERTA vektor dense (dim 1024) per DOI sehingga
// kita bisa memilih hanya top-k chunk paling relevan terhadap query screening,
// alih-alih menyuap seluruh paper ke LLM. Vektor diranking cosine di Go (chunk
// sudah ada di memori), jadi tak perlu panggilan search per paper ke Qdrant.

type ragChunkVec struct {
	idx     float64
	content string
	vec     []float32
}

type FulltextRAG struct {
	byDOI map[string][]ragChunkVec
}

// Has melaporkan apakah ada chunk untuk DOI (ternormalisasi otomatis).
func (r *FulltextRAG) Has(doi string) bool {
	return r != nil && len(r.byDOI[normalizeDOIForRAG(doi)]) > 0
}

func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return -1
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return -1
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// TopK mengembalikan konteks full-text untuk satu DOI: bila query (qvec) tersedia,
// pilih k chunk dengan cosine tertinggi lalu urutkan ulang per chunk_index agar
// runtut; bila qvec kosong/k<=0, fallback ke seluruh chunk berurutan (dipotong
// hingga maxChars) — setara perilaku BuildFulltextIndex.
func (r *FulltextRAG) TopK(doi string, qvec []float32, k, maxChars int) string {
	if r == nil {
		return ""
	}
	chunks := r.byDOI[normalizeDOIForRAG(doi)]
	if len(chunks) == 0 {
		return ""
	}
	selected := chunks
	if len(qvec) > 0 && k > 0 && k < len(chunks) {
		ranked := make([]ragChunkVec, len(chunks))
		copy(ranked, chunks)
		sort.Slice(ranked, func(i, j int) bool {
			return cosine(qvec, ranked[i].vec) > cosine(qvec, ranked[j].vec)
		})
		selected = ranked[:k]
	}
	// Urutkan (kembali) per chunk_index agar konteks runtut dibaca LLM.
	sort.Slice(selected, func(i, j int) bool { return selected[i].idx < selected[j].idx })

	if maxChars <= 0 {
		maxChars = maxFulltextChars
	}
	var sb strings.Builder
	for _, c := range selected {
		if sb.Len()+len(c.content)+2 > maxChars {
			sb.WriteString("\n\n[...teks dipotong untuk batas konteks...]")
			break
		}
		sb.WriteString(c.content)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

// BuildFulltextRAG seperti BuildFulltextIndex namun menyertakan vektor dense tiap
// chunk (with_vector) untuk dukung pemilihan top-k semantik. available=false bila
// Qdrant belum dikonfigurasi.
func BuildFulltextRAG(ctx context.Context) (rag *FulltextRAG, available bool, err error) {
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = os.Getenv("QDRANT_ENDPOINT")
	}
	if qdrantURL == "" {
		return nil, false, nil
	}
	qdrantKey := os.Getenv("QDRANT_API_KEY")

	client := &http.Client{Timeout: 120 * time.Second}
	byDOI := make(map[string][]ragChunkVec)
	counter := make(map[string]float64)

	var nextOffset string
	for {
		reqBody := `{"limit": 1000, "with_payload": ["doi", "content", "chunk_index"], "with_vector": ["dense"]}`
		if nextOffset != "" {
			reqBody = fmt.Sprintf(`{"limit": 1000, "with_payload": ["doi", "content", "chunk_index"], "with_vector": ["dense"], "offset": "%s"}`, nextOffset)
		}
		req, e := http.NewRequestWithContext(ctx, "POST",
			fmt.Sprintf("%s/collections/scientific_articles/points/scroll", qdrantURL),
			strings.NewReader(reqBody))
		if e != nil {
			return nil, true, e
		}
		req.Header.Set("Content-Type", "application/json")
		if qdrantKey != "" {
			req.Header.Set("api-key", qdrantKey)
		}
		resp, e := client.Do(req)
		if e != nil {
			return nil, true, e
		}
		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, true, fmt.Errorf("qdrant scroll status %d: %s", resp.StatusCode, string(b))
		}
		var qResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&qResp)
		resp.Body.Close()

		result, ok := qResp["result"].(map[string]interface{})
		if !ok {
			break
		}
		points, _ := result["points"].([]interface{})
		for _, pt := range points {
			pMap, ok := pt.(map[string]interface{})
			if !ok {
				continue
			}
			payload, ok := pMap["payload"].(map[string]interface{})
			if !ok {
				continue
			}
			doi, _ := payload["doi"].(string)
			if doi == "" {
				continue
			}
			content, _ := payload["content"].(string)
			if content == "" {
				continue
			}
			nd := normalizeDOIForRAG(doi)
			var idx float64
			if ci, ok := payload["chunk_index"].(float64); ok {
				idx = ci
			} else {
				idx = counter[nd]
				counter[nd]++
			}
			byDOI[nd] = append(byDOI[nd], ragChunkVec{idx: idx, content: content, vec: extractDenseVec(pMap["vector"])})
		}

		offsetVal, has := result["next_page_offset"]
		if has && offsetVal != nil {
			if s, ok := offsetVal.(string); ok && s != "" {
				nextOffset = s
				continue
			}
		}
		break
	}
	return &FulltextRAG{byDOI: byDOI}, true, nil
}

// extractDenseVec mengambil vektor bernama "dense" dari field "vector" titik Qdrant
// (bentuk: {"dense":[...]} untuk named vectors, atau [...] untuk single unnamed).
func extractDenseVec(v interface{}) []float32 {
	switch vv := v.(type) {
	case map[string]interface{}:
		if arr, ok := vv["dense"].([]interface{}); ok {
			return toFloat32(arr)
		}
	case []interface{}:
		return toFloat32(vv)
	}
	return nil
}

func toFloat32(arr []interface{}) []float32 {
	out := make([]float32, 0, len(arr))
	for _, x := range arr {
		if f, ok := x.(float64); ok {
			out = append(out, float32(f))
		}
	}
	return out
}
