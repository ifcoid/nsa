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
	"unicode"
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
		reqBody := `{"limit": 2000, "with_payload": ["doi", "title", "content", "chunk_index"]}`
		if nextOffset != "" {
			reqBody = fmt.Sprintf(`{"limit": 2000, "with_payload": ["doi", "title", "content", "chunk_index"], "offset": "%s"}`, nextOffset)
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
			title, _ := payload["title"].(string)

			nd := normalizeDOIForRAG(doi)
			nt := NormTitle(title)

			if nd == "" && nt == "" {
				continue
			}

			content, _ := payload["content"].(string)
			if content == "" {
				continue
			}

			var idx float64
			if ci, ok := payload["chunk_index"].(float64); ok {
				idx = ci
			}

			if nd != "" {
				i := idx
				if i == 0 {
					i = counter[nd]
					counter[nd]++
				}
				raw[nd] = append(raw[nd], ragChunk{idx: i, content: content})
			}
			
			if nt != "" {
				key := "title:" + nt
				i := idx
				if i == 0 {
					i = counter[key]
					counter[key]++
				}
				raw[key] = append(raw[key], ragChunk{idx: i, content: content})
			}
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
	section string
	vec     []float32
}

type ragGroup struct {
	title  string
	chunks []ragChunkVec
}

type ragTitleRef struct {
	norm string
	key  string
}

type FulltextRAG struct {
	byKey    map[string]*ragGroup // key = DOI ternormalisasi, atau "aid:"+article_id bila DOI kosong
	doiToKey map[string]string    // DOI ternormalisasi -> key
	titles   []ragTitleRef        // untuk fallback kemiripan judul (chunk DOI-kosong / DOI buku)
}

// Has melaporkan apakah ada chunk untuk DOI (ternormalisasi otomatis).
func (r *FulltextRAG) Has(doi string) bool {
	if r == nil {
		return false
	}
	_, ok := r.doiToKey[normalizeDOIForRAG(doi)]
	return ok
}

func (r *FulltextRAG) groupForDOI(doi string) *ragGroup {
	if r == nil {
		return nil
	}
	if k, ok := r.doiToKey[normalizeDOIForRAG(doi)]; ok {
		return r.byKey[k]
	}
	return nil
}

// bestTitleMatch mencari grup chunk dengan judul paling mirip (Jaccard token >= 0.8).
// Dipakai sebagai fallback saat DOI kosong/tak cocok (mis. 4.5% chunk PEDE tanpa DOI).
func (r *FulltextRAG) bestTitleMatch(title string) *ragGroup {
	if r == nil {
		return nil
	}
	nt := NormTitle(title)
	if nt == "" {
		return nil
	}
	best, bestScore := "", 0.0
	for _, t := range r.titles {
		if s := titleSim(nt, t.norm); s > bestScore {
			bestScore, best = s, t.key
		}
	}
	if bestScore >= 0.8 && best != "" {
		return r.byKey[best]
	}
	return nil
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

func isMethodResult(section string) bool {
	s := strings.ToLower(section)
	for _, kw := range []string{"method", "result", "experiment", "evaluat", "dataset",
		"performance", "accuracy", "finding", "analysis", "implementation", "setup", "ablation"} {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

func isAbstractIntro(section string) bool {
	s := strings.ToLower(section)
	return strings.Contains(s, "abstract") || strings.Contains(s, "introduction") || strings.Contains(s, "background")
}

// selectContext memilih konteks SCREENING yang menjamin cakupan keputusan: 1 chunk
// pembuka (Abstract/Intro) + hingga 4 chunk Methods/Results (paling relevan secara
// semantik) + isian top-k semantik, lalu diurutkan per chunk_index. Tujuannya supaya
// metrik Outcome (di Methods/Results) tidak terbuang oleh top-k generik.
func selectContext(chunks []ragChunkVec, qvec []float32, k, maxChars int) string {
	if len(chunks) == 0 {
		return ""
	}
	if maxChars <= 0 {
		maxChars = maxFulltextChars
	}

	order := make([]int, len(chunks))
	for i := range order {
		order[i] = i
	}
	if len(qvec) > 0 {
		sort.SliceStable(order, func(a, b int) bool {
			return cosine(qvec, chunks[order[a]].vec) > cosine(qvec, chunks[order[b]].vec)
		})
	} else {
		sort.SliceStable(order, func(a, b int) bool { return chunks[order[a]].idx < chunks[order[b]].idx })
	}

	picked := make(map[int]bool)

	// 1) Jamin pembuka (Abstract/Intro), kalau ada; jika tidak, chunk idx terkecil.
	openIdx := -1
	for i := range chunks {
		if isAbstractIntro(chunks[i].section) {
			if openIdx < 0 || chunks[i].idx < chunks[openIdx].idx {
				openIdx = i
			}
		}
	}
	if openIdx < 0 {
		openIdx = 0
		for i := range chunks {
			if chunks[i].idx < chunks[openIdx].idx {
				openIdx = i
			}
		}
	}
	picked[openIdx] = true

	// 2) Jamin Methods/Results: hingga 4 chunk section M/R, prioritas semantik.
	mr := 0
	for _, i := range order {
		if mr >= 4 {
			break
		}
		if !picked[i] && isMethodResult(chunks[i].section) {
			picked[i] = true
			mr++
		}
	}

	// 3) Wajib selalu masuk; lalu isi sisa budget dengan top semantik.
	chosen := make(map[int]bool)
	used := 0
	for i := range chunks {
		if picked[i] {
			chosen[i] = true
			used += len(chunks[i].content) + 2
		}
	}
	count := len(chosen)
	for _, i := range order {
		if chosen[i] {
			continue
		}
		if k > 0 && count >= k {
			break
		}
		c := len(chunks[i].content) + 2
		if used+c > maxChars {
			continue
		}
		chosen[i] = true
		used += c
		count++
	}

	// Urutkan hasil per chunk_index agar runtut dibaca LLM.
	var final []int
	for i := range chunks {
		if chosen[i] {
			final = append(final, i)
		}
	}
	sort.Slice(final, func(a, b int) bool { return chunks[final[a]].idx < chunks[final[b]].idx })

	var sb strings.Builder
	for _, i := range final {
		sb.WriteString(chunks[i].content)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

// TopK: konteks full-text untuk satu DOI (section-aware; lihat selectContext).
func (r *FulltextRAG) TopK(doi string, qvec []float32, k, maxChars int) string {
	g := r.groupForDOI(doi)
	if g == nil {
		return ""
	}
	return selectContext(g.chunks, qvec, k, maxChars)
}

// TopKByTitle: fallback bila DOI kosong/tak cocok — cari grup via kemiripan judul.
func (r *FulltextRAG) TopKByTitle(title string, qvec []float32, k, maxChars int) string {
	g := r.bestTitleMatch(title)
	if g == nil {
		return ""
	}
	return selectContext(g.chunks, qvec, k, maxChars)
}

// NormTitle menormalkan judul untuk pencocokan.
func NormTitle(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func titleSim(a, b string) float64 {
	fa, fb := strings.Fields(a), strings.Fields(b)
	if len(fa) == 0 || len(fb) == 0 {
		return 0
	}
	setA := make(map[string]bool, len(fa))
	for _, t := range fa {
		setA[t] = true
	}
	setB := make(map[string]bool, len(fb))
	inter := 0
	for _, t := range fb {
		if !setB[t] {
			setB[t] = true
			if setA[t] {
				inter++
			}
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// BuildFulltextRAG men-scroll Qdrant (with_vector dense) + payload doi/title/
// section_header/article_id, mengelompokkan chunk per artikel (DOI bila ada, kalau
// kosong pakai article_id), dan menyiapkan indeks fallback kemiripan judul.
// available=false bila Qdrant belum dikonfigurasi.
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
	byKey := make(map[string]*ragGroup)
	doiToKey := make(map[string]string)
	counter := make(map[string]float64)

	const fields = `"doi", "content", "chunk_index", "title", "section_header", "article_id"`
	var nextOffset string
	for {
		reqBody := fmt.Sprintf(`{"limit": 1000, "with_payload": [%s], "with_vector": ["dense"]}`, fields)
		if nextOffset != "" {
			reqBody = fmt.Sprintf(`{"limit": 1000, "with_payload": [%s], "with_vector": ["dense"], "offset": "%s"}`, fields, nextOffset)
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
			content, _ := payload["content"].(string)
			if content == "" {
				continue
			}
			doi, _ := payload["doi"].(string)
			articleID, _ := payload["article_id"].(string)
			title, _ := payload["title"].(string)
			section, _ := payload["section_header"].(string)

			nd := normalizeDOIForRAG(doi)
			var key string
			switch {
			case nd != "":
				key = nd
			case strings.TrimSpace(articleID) != "":
				key = "aid:" + strings.TrimSpace(articleID)
			default:
				continue // tak ada DOI maupun article_id -> tak bisa dipetakan
			}

			var idx float64
			if ci, ok := payload["chunk_index"].(float64); ok {
				idx = ci
			} else {
				idx = counter[key]
				counter[key]++
			}

			g := byKey[key]
			if g == nil {
				g = &ragGroup{title: title}
				byKey[key] = g
				if nd != "" {
					doiToKey[nd] = key
				}
			}
			if g.title == "" && title != "" {
				g.title = title
			}
			g.chunks = append(g.chunks, ragChunkVec{idx: idx, content: content, section: section, vec: extractDenseVec(pMap["vector"])})
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

	var titles []ragTitleRef
	for key, g := range byKey {
		if nt := NormTitle(g.title); nt != "" {
			titles = append(titles, ragTitleRef{norm: nt, key: key})
		}
	}
	return &FulltextRAG{byKey: byKey, doiToKey: doiToKey, titles: titles}, true, nil
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
