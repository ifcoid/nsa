package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nsa/internal/llm"
	"nsa/internal/model"
)

type PriorReviewAgent struct {
	client llm.LLMClient
}

func NewPriorReviewAgent(client llm.LLMClient) *PriorReviewAgent {
	return &PriorReviewAgent{client: client}
}

func (a *PriorReviewAgent) GenerateMatrix(ctx context.Context, topicContext string) (*model.PriorReviewsMatrix, error) {
	systemPrompt := `Anda adalah asisten peneliti akademik profesional dengan akses ke Internet.
Diberikan sebuah konteks Topik Penelitian beserta Gap-nya, tugas Anda adalah MENGGUNAKAN KEMAMPUAN WEB SEARCH ANDA secara real-time untuk mengidentifikasi 3-5 systematic review/literature review TERDEKAT dalam 5-10 tahun terakhir. JANGAN MENGARANG (HALLUCINATE), gunakan data asli dari web.

Aturan pencarian / ekstraksi:
1. Lakukan pencarian web untuk literatur review terdahulu. Jika sangat sedikit (1-2), sampaikan apa adanya. Jika TIDAK ADA, sampaikan "No prior systematic review identified" di author_year dan cari review naratif terdekat.
2. Untuk setiap review, isi kolom "selisih" secara eksplisit dengan HANYA salah satu/kombinasi tag ini: BEDA POPULASI / BEDA METODE / BEDA PERIODE / BEDA FOKUS / BEDA FRAMEWORK.
3. Untuk setiap review, buat "synthesis_novelty" spesifik (150-200 kata) yang mengaitkan kelemahan paper tersebut dengan riset pengguna, menjelaskan mengapa riset pengguna MENUTUP gap dari paper tersebut.

Output HARUS dalam format JSON dengan struktur persis seperti ini:
{
  "reviews": [
    {
      "author_year": "Nama dkk. (Tahun)",
      "scope": "Populasi, Area, Periode",
      "methodology": "SLR/Bibliometric, Database, jumlah (n)",
      "key_findings": "Temuan utama",
      "limitations": "Kelemahan studi tersebut",
      "selisih": "BEDA POPULASI / BEDA FOKUS",
      "synthesis_novelty": "Sintesis spesifik 150-200 kata terkait paper ini..."
    }
  ]
}
Output HANYA JSON MURNI tanpa markdown tambahan atau teks di luar JSON.`

	response, err := a.client.Generate(ctx, systemPrompt, topicContext)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	cleaned := CleanJSONResponse(response)

	var matrix model.PriorReviewsMatrix
	if err := json.Unmarshal([]byte(cleaned), &matrix); err != nil {
		snippet := strings.TrimSpace(response)
		if len(snippet) > 300 {
			snippet = snippet[:300] + "…"
		}
		// Saat web search tak tersedia, model sering MEMBALAS PERCAKAPAN (menolak meng-
		// hasilkan JSON agar tak hallucinate) alih-alih JSON murni. Beri pesan actionable
		// yang menyebut model + akar masalah + remedi — bukan error parser mentah + dump
		// raksasa yang membuat user bingung.
		if !strings.HasPrefix(strings.TrimSpace(cleaned), "{") {
			return nil, fmt.Errorf("Prior Reviews: model %s tidak mengembalikan JSON, kemungkinan menolak karena WEB SEARCH tidak tersedia (step ini menyuruh pencarian web real-time). Solusi: (a) pakai model Brain berkemampuan web search untuk step ini (mis. Gemini dengan GEMINI_GROUNDING=true), atau (b) isi matriks prior-review secara manual via revisi (HITL). Cuplikan respons: %q",
				a.client.ModelName(), snippet)
		}
		return nil, fmt.Errorf("Prior Reviews: gagal parsing JSON dari model %s: %w. Cuplikan: %q", a.client.ModelName(), err, snippet)
	}

	return &matrix, nil
}
