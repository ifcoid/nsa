package agent

import (
	"context"
	"fmt"
	"strings"

	"nsa/internal/llm"
)

type FoundationAgent struct {
	client llm.LLMClient
}

func NewFoundationAgent(client llm.LLMClient) *FoundationAgent {
	return &FoundationAgent{client: client}
}

// GenerateTheoryBriefing menyusun bagian "Fondasi Teori" Modul 1 dalam Markdown,
// disesuaikan dengan topik riset. feedback (opsional) dipakai saat regenerasi/revisi.
func (a *FoundationAgent) GenerateTheoryBriefing(ctx context.Context, topic, feedback string) (string, error) {
	systemPrompt := `Anda adalah pengajar metodologi Systematic Literature Review (SLR) yang ringkas dan akurat.
Tugas: susun briefing FONDASI TEORI dalam Bahasa Indonesia (format Markdown) yang disesuaikan dengan topik riset peneliti.

Cakup TIGA bagian (gunakan heading '##'):

## 1. Pengenalan Systematic Literature Review
- Definisi SLR, tujuannya, dan bedanya dengan literature review naratif.
- Sebutkan singkat jenis-jenis review (mis. SLR, scoping review, bibliometric/SLNA) dan kapan dipakai.

## 2. Metodologi SLR untuk Topik Ini
- Alur metodologi SLR secara ringkas (formulasi pertanyaan, protokol, pencarian, screening, ekstraksi, sintesis).
- Perkenalkan kerangka PICO (Population, Intervention/Exposure, Comparison, Outcome) dan ILUSTRASIKAN bagaimana PICO dapat diterapkan pada topik peneliti.

## 3. Mengapa Topik Ini Cocok untuk SLR
- Argumentasikan kesesuaian topik dengan pendekatan SLR (potensi gap, ketersediaan literatur, relevansi praktik).

Aturan:
- Ringkas, padat, akurat. JANGAN mengarang statistik atau DOI spesifik.
- JANGAN menambahkan aturan workflow/HITL atau etika AI (itu bagian terpisah di luar lingkup Anda).
- Keluarkan HANYA Markdown, tanpa pembungkus code fence (tanpa tiga backtick).`

	userPrompt := fmt.Sprintf("Topik riset peneliti: %s\n\nSusun briefing fondasi teori SLR yang disesuaikan dengan topik di atas.", topic)
	if strings.TrimSpace(feedback) != "" {
		userPrompt += fmt.Sprintf("\n\n[INSTRUKSI REVISI DARI PENELITI]:\n%s\nPerbaiki/sesuaikan briefing sesuai instruksi revisi ini.", feedback)
	}

	response, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("LLM error: %w", err)
	}

	return stripMarkdownFence(response), nil
}

// stripMarkdownFence membuang pembungkus code fence (```markdown ... ```) bila LLM menambahkannya.
func stripMarkdownFence(s string) string {
	out := strings.TrimSpace(s)
	if strings.HasPrefix(out, "```") {
		if nl := strings.IndexByte(out, '\n'); nl != -1 {
			out = out[nl+1:]
		}
		out = strings.TrimSuffix(strings.TrimSpace(out), "```")
	}
	return strings.TrimSpace(out)
}
