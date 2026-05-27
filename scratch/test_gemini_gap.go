//go:build ignore

package main

import (
	"context"
	"fmt"
	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()
	apiKey := "AIzaSyCOm3cKm_p0qziiCixSsLko5J6Tj-m6CdM"
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		panic(err)
	}

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
    "importance": "Mengapa topik ini krusial diteliti sekarang"
  }
]
Pastikan mengembalikan tepat 3 saran topik berbeda. Output HANYA array JSON murni.`

	userPrompt := "Topik mentah awal: Penggunaan Active Learning pada Machine Learning untuk klasifikasi data Brain-Computer Interface (BCI)\nBuatkan 3 saran topik SLR terbaik beserta Gap Characterization-nya."

	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: systemPrompt}},
		},
		Tools: []*genai.Tool{
			{GoogleSearch: &genai.GoogleSearch{}},
		},
	}

	res, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", genai.Text(userPrompt), config)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}

	fmt.Printf("RESPONSE CANDIDATES LEN: %d\n", len(res.Candidates))
	if len(res.Candidates) > 0 {
		candidate := res.Candidates[0]
		fmt.Printf("FINISH REASON: %v\n", candidate.FinishReason)
		if candidate.Content != nil {
			fmt.Printf("PARTS LEN: %d\n", len(candidate.Content.Parts))
			for i, p := range candidate.Content.Parts {
				fmt.Printf("  PART %d TEXT: %q\n", i, p.Text)
			}
		} else {
			fmt.Println("CONTENT IS NIL")
		}
	}
}
