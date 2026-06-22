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
	systemPrompt := `Anda adalah asisten peneliti akademik. Anda TIDAK memiliki akses web/pencarian live.
Diberikan konteks Topik Penelitian beserta Gap-nya, usulkan 3-5 systematic review/literature review terdahulu yang PALING MUNGKIN relevan, berdasarkan PENGETAHUAN Anda (bukan pencarian web).

PENTING (anti-halusinasi + HITL):
- Anda BUKAN sedang mencari web. Jangan berpura-pura mencari, jangan minta mengaktifkan web search, dan JANGAN membalas dengan percakapan/penolakan — selalu kembalikan JSON.
- Karena tanpa akses web, semua usulan Anda adalah KANDIDAT yang HARUS diverifikasi peneliti. Set "verification": "UNVERIFIED" untuk setiap entri.
- JANGAN mengarang detail presisi palsu (DOI palsu, jumlah n yang dikarang, tahun pasti yang tidak Anda yakini). Bila ragu pada author/tahun, tulis perkiraan dengan penanda "(perlu verifikasi)" di "author_year" daripada memalsukan.
- Bila Anda benar-benar tidak mengetahui review yang relevan, kembalikan minimal satu entri dengan author_year "No prior systematic review identified (perlu verifikasi)" dan jelaskan arah pencarian yang disarankan di "key_findings".

Aturan isi:
1. "selisih" HANYA salah satu/kombinasi tag: BEDA POPULASI / BEDA METODE / BEDA PERIODE / BEDA FOKUS / BEDA FRAMEWORK.
2. "synthesis_novelty" spesifik (150-200 kata): kaitkan kelemahan review tersebut dengan riset pengguna dan mengapa riset pengguna MENUTUP gap-nya.
3. "search_guidance" WAJIB diisi: resep pencarian SIAP-PAKAI agar peneliti dapat MENEMUKAN & MEMVERIFIKASI sendiri prior-review nyata. Sertakan: (a) satu query Boolean Scopus berformat TITLE-ABS-KEY(...) yang diturunkan dari topik & gap, (b) saran filter (rentang tahun, document type = Review/Article), (c) database alternatif (Google Scholar scholar.google.com, Web of Science) + varian kata kunci. Tulis sebagai teks ringkas yang bisa langsung disalin ke scopus.com. Contoh format: "Scopus (scopus.com): TITLE-ABS-KEY(\"systematic review\" AND \"deep learning\" AND \"EEG\" AND emotion*) AND PUBYEAR > 2018, filter Document Type = Review. Alternatif: Google Scholar / Web of Science dengan kata kunci ...".

Output HARUS JSON MURNI dengan struktur persis ini (tanpa markdown/teks di luar JSON):
{
  "search_guidance": "Scopus (scopus.com): TITLE-ABS-KEY(...) AND PUBYEAR > 20xx, Document Type = Review. Alternatif: Google Scholar / WoS dengan kata kunci ...",
  "reviews": [
    {
      "author_year": "Nama dkk. (Tahun)",
      "scope": "Populasi, Area, Periode",
      "methodology": "SLR/Bibliometric, Database, jumlah (n)",
      "key_findings": "Temuan utama",
      "limitations": "Kelemahan studi tersebut",
      "selisih": "BEDA POPULASI / BEDA FOKUS",
      "synthesis_novelty": "Sintesis spesifik 150-200 kata terkait paper ini...",
      "verification": "UNVERIFIED"
    }
  ]
}`

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

	// xAI/HITL: usulan tanpa web search WAJIB ditandai perlu-verifikasi. Default bila model
	// lupa mengisi, agar peneliti tahu setiap entri masih harus diverifikasi sebelum approve.
	for i := range matrix.Reviews {
		if strings.TrimSpace(matrix.Reviews[i].Verification) == "" {
			matrix.Reviews[i].Verification = "UNVERIFIED"
		}
	}

	// Fallback panduan pencarian bila model lupa mengisi: minimal arahkan peneliti ke
	// Scopus/Scholar untuk menemukan & memverifikasi sendiri.
	if strings.TrimSpace(matrix.SearchGuidance) == "" {
		matrix.SearchGuidance = "Cari & verifikasi prior-review di Scopus (scopus.com) atau Google Scholar (scholar.google.com)/Web of Science: gunakan TITLE-ABS-KEY(\"systematic review\" OR \"literature review\") DIGABUNG kata kunci inti topik & gap Anda, filter Document Type = Review dan rentang tahun 5-10 tahun terakhir."
	}

	return &matrix, nil
}
