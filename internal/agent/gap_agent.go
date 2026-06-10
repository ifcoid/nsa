package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"nsa/internal/llm"
	"nsa/internal/model"
)

type GapAgent struct {
	client llm.LLMClient
}

func NewGapAgent(client llm.LLMClient) *GapAgent {
	return &GapAgent{client: client}
}

func (a *GapAgent) GenerateSuggestedTopics(ctx context.Context, initialTopic string) ([]model.SuggestedTopic, error) {
	systemPrompt := `Anda adalah agen riset akademik tingkat lanjut yang ahli dalam Systematic Literature Review (SLR).
Tugas Anda adalah mengambil gagasan topik riset awal dari pengguna, mensimulasikan pencarian literatur terbaru (3 tahun terakhir), dan mengidentifikasi 'Research Gap' yang belum terjawab.

Klasifikasikan setiap gap ke dalam salah satu tipe berikut:
- TIPE A: FRAGMENTASI LITERATUR (studi tersebar tanpa sintesis)
- TIPE B: KONTRADIKSI ANTAR STUDI (temuan primer bertentangan)
- TIPE C: KETIADAAN INTEGRATIVE FRAMEWORK (konsep belum terikat framework)

Kriteria Topik yang disarankan:
- Gap jelas + terverifikasi dari literatur terbaru
- Cocok untuk SLR
- Relevan dengan praktik saat ini

Keluarkan output HANYA dalam bentuk array JSON dengan struktur berikut:
[
  {
    "name": "Judul Topik",
    "gap": "Penjelasan spesifik mengenai gap yang ada",
    "type": "TIPE A / TIPE B / TIPE C",
    "type_reason": "Alasan mengapa masuk tipe ini",
    "evidence": "Contoh literatur/DOI/URL/fenomena terbaru",
    "importance": "Mengapa topik ini krusial diteliti sekarang",
    "references": ["Daftar SELURUH judul literatur asli / URL yang relevan dari hasil pencarian Anda sebagai bukti"]
  }
]
Pastikan mengembalikan tepat 3 saran topik berbeda. Output HANYA array JSON murni.`

	userPrompt := fmt.Sprintf("Topik mentah awal: %s\nBuatkan 3 saran topik SLR terbaik beserta Gap Characterization-nya.\n\nPENTING: Output Anda HARUS BERUPA ARRAY JSON MURNI (dimulai dengan '[' dan diakhiri dengan ']'). DILARANG memberikan teks pengantar, penutup, atau menjelaskan rencana/langkah pencarian Anda. LANGSUNG HASILKAN JSON SAJA.", initialTopic)

	response, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	cleaned := CleanJSONResponse(response)

	var suggestions []model.SuggestedTopic
	err = json.Unmarshal([]byte(cleaned), &suggestions)
	if err != nil {
		// FALLBACK: Coba parsing format teks/markdown manual jika LLM keras kepala tidak mau JSON
		parsed, parseErr := a.parseMarkdownResponse(response)
		if parseErr == nil && len(parsed) > 0 {
			return parsed, nil
		}
		return nil, fmt.Errorf("gagal parsing JSON dari LLM (%w). Raw response: %s", err, response)
	}

	return suggestions, nil
}

// parseMarkdownResponse mengekstrak data dari respons teks manual yang tidak berformat JSON
func (a *GapAgent) parseMarkdownResponse(text string) ([]model.SuggestedTopic, error) {
	var topics []model.SuggestedTopic
	
	// Pisahkan per topik menggunakan delimiter "---" atau pemisah yang umum
	blocks := strings.Split(text, "---")
	if len(blocks) < 2 {
		// Coba pisahkan berdasarkan kemunculan "Judul:" atau "**Judul"
		blocks = strings.Split(strings.ReplaceAll(text, "**Judul", "Judul"), "Judul:")
		// Hapus elemen pertama jika kosong
		if len(blocks) > 0 && strings.TrimSpace(blocks[0]) == "" {
			blocks = blocks[1:]
		}
	}
	
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		
		topic := model.SuggestedTopic{}
		lines := strings.Split(block, "\n")
		
		for _, line := range lines {
			line = strings.TrimSpace(line)
			lineLower := strings.ToLower(line)
			
			// Hapus karakter markdown tebal
			line = strings.ReplaceAll(line, "**", "")
			line = strings.ReplaceAll(line, "*", "")
			
			if strings.HasPrefix(lineLower, "judul:") {
				topic.Name = strings.TrimSpace(line[6:])
			} else if !strings.Contains(lineLower, ":") && topic.Name == "" && len(line) > 10 {
				// Terkadang baris pertama langsung judul
				topic.Name = line
			} else if strings.HasPrefix(lineLower, "gap:") {
				topic.Gap = strings.TrimSpace(line[4:])
			} else if strings.HasPrefix(lineLower, "alasan klasifikasi") {
				idx := strings.Index(line, ":")
				if idx != -1 {
					topic.TypeReason = strings.TrimSpace(line[idx+1:])
					if strings.Contains(lineLower, "tipe a") { topic.Type = "TIPE A" }
					if strings.Contains(lineLower, "tipe b") { topic.Type = "TIPE B" }
					if strings.Contains(lineLower, "tipe c") { topic.Type = "TIPE C" }
				}
			} else if strings.HasPrefix(lineLower, "bukti") || strings.HasPrefix(lineLower, "konteks") {
				idx := strings.Index(line, ":")
				if idx != -1 {
					topic.Evidence = strings.TrimSpace(line[idx+1:])
				}
			} else if strings.HasPrefix(lineLower, "pentingnya") {
				idx := strings.Index(line, ":")
				if idx != -1 {
					topic.Importance = strings.TrimSpace(line[idx+1:])
				}
			}
		}
		
		if topic.Name != "" && topic.Gap != "" {
			if topic.Type == "" {
				topic.Type = "TIPE A" // fallback default
			}
			topic.References = []string{"Disintesis dari literatur"} // placeholder
			topics = append(topics, topic)
		}
	}
	
	if len(topics) == 0 {
		return nil, fmt.Errorf("tidak ada topik yang bisa diekstrak dari markdown")
	}
	return topics, nil
}
