package orchestrator

import (
	"context"
	"fmt"
	"nsa/internal/agent"
	"nsa/internal/llm"
	"nsa/internal/repository"
)

type SLRPipeline struct {
	mongoRepo  *repository.MongoRepository
	llmFactory *llm.LLMFactory
}

func NewSLRPipeline(mongo *repository.MongoRepository, factory *llm.LLMFactory) *SLRPipeline {
	return &SLRPipeline{
		mongoRepo:  mongo,
		llmFactory: factory,
	}
}

func (p *SLRPipeline) Execute(ctx context.Context, sessionID string) error {
	// 1. Ambil state sesi riset dari MongoDB
	session, err := p.mongoRepo.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("gagal mengambil sesi: %w", err)
	}

	switch session.Status {
	case "INIT":
		fmt.Println("[Step 1] Menjalankan Agent PICO dan Perumusan Kriteria...")

		// 2. DETIK INI: Ambil otak LLM secara dinamis dari DB (misal pakai claude untuk penalaran tinggi)
		// Anda bisa mengubah string "claude" atau "gemini" ini menjadi kolom di model.SLRSession jika ingin diatur per sesi
		llmBrain, err := p.llmFactory.CreateClient(ctx, "gemini")
		if err != nil {
			return fmt.Errorf("gagal menyiapkan otak LLM: %w", err)
		}

		// 3. Spawning Agen secara instan dengan otak yang sudah siap
		picoAgent := agent.NewPicoAgent(llmBrain)
		critAgent := agent.NewCriteriaAgent(llmBrain)

		// 4. Jalankan tugas masing-masing agen
		picoResult, err := picoAgent.Analyze(ctx, session.Topic)
		if err != nil {
			return err
		}

		criteria, err := critAgent.GenerateCriteria(ctx, picoResult)
		if err != nil {
			return err
		}

		// 5. Simpan hasil kerja agen ke MongoDB dan ubah status ke WAITING_APPROVAL (Jeda HitL)
		session.PICO = picoResult
		session.InclusionCriteria = criteria.Inclusion
		session.ExclusionCriteria = criteria.Exclusion
		session.Status = "WAITING_APPROVAL"

		err = p.mongoRepo.UpdateSession(ctx, session)
		if err != nil {
			return fmt.Errorf("gagal menyimpan hasil analisis awal: %w", err)
		}

		fmt.Println("[System] Proses DIJEDA. Menunggu review manusia di MongoDB...")
		return nil

	case "WAITING_APPROVAL":
		fmt.Println("[System] Sesi dikunci. Menunggu keputusan manusia (APPROVED / NEEDS_REVISION).")
		return nil

	case "NEEDS_REVISION":
		fmt.Printf("[Step 1-Rev] Memperbaiki kriteria berdasarkan feedback: '%s'\n", session.Feedback)

		// Ambil otak LLM untuk revisi (bisa pakai model yang sama atau berbeda)
		llmBrain, err := p.llmFactory.CreateClient(ctx, "gemini")
		if err != nil {
			return err
		}

		critAgent := agent.NewCriteriaAgent(llmBrain)

		// Perbaiki kriteria lama menggunakan feedback manusia
		revised, err := critAgent.RefineCriteria(ctx, session.InclusionCriteria, session.ExclusionCriteria, session.Feedback)
		if err != nil {
			return err
		}

		// Balikkan status ke WAITING_APPROVAL agar direview ulang oleh manusia
		session.InclusionCriteria = revised.Inclusion
		session.ExclusionCriteria = revised.Exclusion
		session.Feedback = ""
		session.Status = "WAITING_APPROVAL"

		return p.mongoRepo.UpdateSession(ctx, session)

	case "APPROVED":
		fmt.Println("[Step 2] Kriteria disetujui! Memulai pemanenan & paralel screening...")
		// Di sini nanti tempat modul screener_agent.go berjalan menggunakan Goroutines
		return nil
	}

	return nil
}
