package modules

import (
	"context"

	"nsa/internal/agent"
	"nsa/internal/logger"
	"nsa/internal/model"
)

type M1Foundation struct {
	deps *ModuleDeps
}

func NewM1Foundation(deps *ModuleDeps) *M1Foundation {
	return &M1Foundation{deps: deps}
}

func (m *M1Foundation) Name() string { return "M1_FOUNDATION" }

func (m *M1Foundation) Execute(ctx context.Context, session *model.SLRSession) error {
	logger.Logf(session.ID, ">> [MODUL 1: FONDASI TEORI] Memproses State: %s\n", session.Status)

	switch session.Status {

	// Entry dari orchestrator (INIT -> M1_FOUNDATION): susun briefing.
	case "M1_FOUNDATION":
		return m.generateBriefing(ctx, session)

	case "M1_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi dikunci. Buka field 'foundation' di sesi Anda lalu:")
		logger.Log(session.ID, "   - Jika SUDAH paham/sesuai: ubah 'status' menjadi 'M1_APPROVED' (pipeline lanjut ke Modul 2).")
		logger.Log(session.ID, "   - Jika perlu disesuaikan: ubah 'status' menjadi 'M1_NEEDS_REVISION' dan isi 'feedback'.")
		return nil

	case "M1_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 1] Menyusun ulang briefing teori berdasarkan feedback: '%s'\n", session.Feedback)
		return m.generateBriefing(ctx, session)

	case "M1_APPROVED":
		logger.Log(session.ID, ">> [MODUL 1] Briefing fondasi disetujui. Transisi ke M2_STEP1_TOPIC_GAP.")
		session.Status = "M2_STEP1_TOPIC_GAP"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	default:
		return nil
	}
}

// generateBriefing menyusun output Modul 1: bagian teori (LLM, disesuaikan topik) +
// bagian etika/kapabilitas AI dan aturan global (kanonik statik), lalu menjeda untuk approval.
func (m *M1Foundation) generateBriefing(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 1] Menyusun briefing fondasi teori SLR (disesuaikan dengan topik)...")

	llmBrain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return err
	}

	foundationAgent := agent.NewFoundationAgent(llmBrain)
	theory, err := foundationAgent.GenerateTheoryBriefing(ctx, session.Topic, session.Feedback)
	if err != nil {
		return err
	}

	session.Foundation = &model.FoundationBriefing{
		TopicContext:        session.Topic,
		TheoryMarkdown:      theory,
		AIPracticeMarkdown:  m1AIPractice,
		GlobalRulesMarkdown: m1GlobalRules,
	}
	session.Feedback = ""
	session.Status = "M1_WAITING_APPROVAL"

	logger.Log(session.ID, "   [System] DIJEDA. Briefing tersimpan di field 'foundation'. Menunggu persetujuan peneliti.")
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// ===========================================================================
// Konten kanonik statik (tidak di-generate LLM agar aturan baku tidak terdistorsi).
// ===========================================================================

const m1AIPractice = `## Generative AI dalam Penelitian: Etika & Kapabilitas

**Etika penggunaan AI**
- *Transparansi:* deklarasikan model, peran, dan batasan AI di bagian Methods.
- *Bias:* LLM dapat mereproduksi bias data latih (mis. dominasi literatur berbahasa Inggris). Waspadai saat seleksi & sintesis.
- *Limitasi & halusinasi:* LLM bisa menghasilkan referensi/temuan yang terdengar masuk akal tetapi salah. Selalu verifikasi ke sumber asli.
- *Akuntabilitas:* peneliti bertanggung jawab penuh atas seluruh output akhir.

**Kapabilitas LLM untuk SLR**
- Cocok: brainstorming gap, drafting PICO/keywords, klasifikasi judul/abstrak (dengan dual-review), sintesis naratif, drafting manuskrip.
- Perlu kehati-hatian: estimasi jumlah studi, perhitungan statistik/kappa (gunakan rumus eksplisit, bukan tebakan LLM), klaim faktual (wajib verifikasi).
- Tidak boleh: dijadikan satu-satunya sumber kebenaran tanpa verifikasi manusia.`

const m1GlobalRules = `## Aturan Global SLR + CoWork (berlaku untuk seluruh Modul 2-9)

**1. Human-in-the-Loop (HITL).** Setiap modul menghasilkan output lalu DIJEDA pada status ` + "`*_WAITING_APPROVAL`" + `. Peneliti wajib me-review hasil sebelum lanjut:
- Setuju -> ubah status ke ` + "`*_APPROVED`" + ` (pipeline lanjut ke langkah berikutnya).
- Perlu revisi -> ubah status ke ` + "`*_NEEDS_REVISION`" + ` dan isi ` + "`feedback`" + `; sistem akan men-generate ulang.

**2. Keputusan akhir milik peneliti.** AI (cowork) hanya membantu menyusun, menyaring, dan memberi perspektif. Penilaian inklusi/eksklusi, interpretasi, dan klaim ilmiah tetap tanggung jawab manusia.

**3. Transparansi.** Penggunaan AI wajib dideklarasikan di Methods. Cohen's kappa dihitung antar reviewer manusia, bukan AI.

**4. Reproducibility.** Catat tanggal pencarian (cut-off), string pencarian final, filter, dan jumlah hits per database. Sumber yang terbit setelah cut-off tidak boleh menjadi studi primer tanpa prosedur re-run.

**5. Anti-fabrikasi.** Jangan pernah mengarang referensi, DOI, atau temuan. Setiap klaim faktual (tahun publikasi, target SDG, definisi resmi) wajib diverifikasi via web search / sumber asli.

**6. Konsistensi terminologi.** Istilah kanonikal yang ditetapkan di Modul 2 (PICO) dipakai konsisten hingga Modul 9.

### Peta Modul
- M1  Fondasi Teori + Aturan Global -> briefing
- M2  Topik & PICO -> pico_definitions
- M3  Search Strategy -> search_log
- M4  Data Mining & Export -> screening
- M5  Title/Abstract Screening -> screening (filled)
- M6  Full-text Acquisition -> pdfs + tracking
- M7  Data Extraction + QA -> extraction
- M8 / M8b  Analysis + Synthesis / Bibliometric -> synthesis_results
- M9  Manuscript Writing -> manuscript_final`
