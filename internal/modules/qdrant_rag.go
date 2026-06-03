package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
