package modules

import (
	"context"
	"fmt"
	"nsa/internal/agent"
	"nsa/internal/model"
)

type M2Pico struct {
	deps *ModuleDeps
}

func NewM2Pico(deps *ModuleDeps) *M2Pico {
	return &M2Pico{deps: deps}
}

func (m *M2Pico) Name() string { return "M2_PICO" }

func (m *M2Pico) Execute(ctx context.Context, session *model.SLRSession) error {
	fmt.Printf(">> [MODUL 2: PICO] Memproses State: %s\n", session.Status)

	switch session.Status {

	// =========================================================================
	// LANGKAH 1: TENTUKAN TOPIK + KLASIFIKASI TIPE GAP
	// =========================================================================
	case "M2_STEP1_TOPIC_GAP":
		fmt.Println("   [Langkah 2.1] Menganalisis Topik Mentah & Mengklasifikasi tipe GAP...")
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		gapAgent := agent.NewGapAgent(llmBrain)
		suggestions, err := gapAgent.GenerateSuggestedTopics(ctx, session.Topic)
		if err != nil { return err }

		session.SuggestedTopics = suggestions
		session.Status = "M2_STEP1_WAITING_APPROVAL"
		
		fmt.Println("   [System] DIJEDA. Menunggu Anda memilih 1 dari 3 topik yang disarankan.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_STEP1_WAITING_APPROVAL":
		fmt.Println("   [System] Sesi masih dikunci. Silakan buka MongoDB:")
		fmt.Println("   1. Lihat array 'suggested_topics' pada collection slr_sessions.")
		fmt.Println("   2. Salin/Pindah judul topik yang Anda suka ke field 'topic' utama.")
		fmt.Println("   3. Ubah 'status' menjadi 'M2_STEP1_APPROVED'.")
		return nil

	case "M2_STEP1_APPROVED":
		fmt.Println("   [Langkah 2.1] Topik telah dipilih. Melanjutkan ke Analisis Prior Reviews...")
		session.Status = "M2_STEP2_PRIOR_REVIEWS"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 2: REVIEW OF PRIOR REVIEWS (MATRIKS)
	// =========================================================================
	case "M2_STEP2_PRIOR_REVIEWS":
		fmt.Println("   [Langkah 2.2] Menganalisis literatur review terdahulu (Matriks)...")
		// TODO: Logika pencarian/analisis paper review sebelumnya
		
		session.Status = "M2_STEP3_PICO"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 3: PICO FRAMEWORK + OPERATIONAL DEFINITIONS + TERMINOLOGI KANONIKAL
	// =========================================================================
	case "M2_STEP3_PICO":
		fmt.Println("   [Langkah 2.3] Mengekstrak PICO Framework dari Topik...")
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		picoAgent := agent.NewPicoAgent(llmBrain)
		picoResult, err := picoAgent.Analyze(ctx, session.Topic)
		if err != nil { return err }

		session.PICO = picoResult
		session.Status = "M2_STEP4_SCOPE"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 4: JUSTIFIKASI BATASAN SCOPE (3-LAPIS)
	// =========================================================================
	case "M2_STEP4_SCOPE":
		fmt.Println("   [Langkah 2.4] Merumuskan Kriteria Inklusi & Eksklusi (Batasan Scope)...")
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		critAgent := agent.NewCriteriaAgent(llmBrain)
		criteria, err := critAgent.GenerateCriteria(ctx, session.PICO)
		if err != nil { return err }

		session.InclusionCriteria = criteria.Inclusion
		session.ExclusionCriteria = criteria.Exclusion
		
		// Sesuai prinsip HitL, minta validasi manusia di sini setelah mengekstrak scope
		session.Status = "M2_WAITING_APPROVAL"
		fmt.Println("   [System] DIJEDA. Menunggu review manusia (M2_WAITING_APPROVAL).")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// --- SIKLUS HUMAN IN THE LOOP (VALIDASI LANGKAH 3 & 4) ---
	case "M2_WAITING_APPROVAL":
		fmt.Println("   [System] Sesi masih dikunci. Menunggu keputusan manusia (M2_APPROVED / M2_NEEDS_REVISION).")
		return nil

	case "M2_NEEDS_REVISION":
		fmt.Printf("   [Revisi] Memperbaiki kriteria berdasarkan feedback: '%s'\n", session.Feedback)
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		critAgent := agent.NewCriteriaAgent(llmBrain)
		revised, err := critAgent.RefineCriteria(ctx, session.InclusionCriteria, session.ExclusionCriteria, session.Feedback)
		if err != nil { return err }

		session.InclusionCriteria = revised.Inclusion
		session.ExclusionCriteria = revised.Exclusion
		session.Feedback = ""
		session.Status = "M2_WAITING_APPROVAL"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_APPROVED":
		fmt.Println("   [System] PICO dan Kriteria Disetujui! Lanjut ke perumusan Research Questions...")
		session.Status = "M2_STEP5_RESEARCH_QUESTIONS"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 5: FORMULASIKAN RESEARCH QUESTIONS
	// =========================================================================
	case "M2_STEP5_RESEARCH_QUESTIONS":
		fmt.Println("   [Langkah 2.5] Memformulasikan Research Questions utama dan sekunder...")
		// TODO: Agen pembuat pertanyaan penelitian (RQs)
		
		session.Status = "M2_STEP6_FINER_CHECK"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 6: CEK FINER + NOVELTY + INTERNAL COHERENCE + HASIL AKHIR
	// =========================================================================
	case "M2_STEP6_FINER_CHECK":
		fmt.Println("   [Langkah 2.6] Melakukan validasi akhir FINER & Novelty Check...")
		// TODO: Agen validator
		
		fmt.Println("   [System] MODUL 2 SELESAI. Mentransfer data ke Modul 3 (Search Strategy).")
		session.Status = "M3_STEP1_DATABASE_SELECTION" // Transisi ke modul 3
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	default:
		// Jika status diawali "M2_" namun belum terdaftar spesifik
		fmt.Printf("   [Modul 2] Sub-status %s tidak dikenali atau belum diimplementasikan.\n", session.Status)
	}

	return nil
}
