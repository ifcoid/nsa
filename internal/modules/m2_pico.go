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
	case "M2_PICO":
		fmt.Println("   [Langkah 2.3 & 2.4] Menjalankan Agent PICO dan Perumusan Kriteria...")
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil {
			return err
		}

		picoAgent := agent.NewPicoAgent(llmBrain)
		critAgent := agent.NewCriteriaAgent(llmBrain)

		picoResult, err := picoAgent.Analyze(ctx, session.Topic)
		if err != nil { return err }

		criteria, err := critAgent.GenerateCriteria(ctx, picoResult)
		if err != nil { return err }

		session.PICO = picoResult
		session.InclusionCriteria = criteria.Inclusion
		session.ExclusionCriteria = criteria.Exclusion
		session.Status = "M2_PICO_WAITING_APPROVAL"

		fmt.Println("   [System] DIJEDA. Menunggu review manusia (M2_PICO_WAITING_APPROVAL).")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_PICO_WAITING_APPROVAL":
		fmt.Println("   [System] Sesi masih dikunci. Menunggu keputusan manusia (M2_PICO_APPROVED / M2_PICO_NEEDS_REVISION).")
		return nil

	case "M2_PICO_NEEDS_REVISION":
		fmt.Printf("   [Langkah 2.4-Rev] Memperbaiki kriteria berdasarkan feedback: '%s'\n", session.Feedback)
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		critAgent := agent.NewCriteriaAgent(llmBrain)
		revised, err := critAgent.RefineCriteria(ctx, session.InclusionCriteria, session.ExclusionCriteria, session.Feedback)
		if err != nil { return err }

		session.InclusionCriteria = revised.Inclusion
		session.ExclusionCriteria = revised.Exclusion
		session.Feedback = ""
		session.Status = "M2_PICO_WAITING_APPROVAL"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_PICO_APPROVED":
		fmt.Println("   [Langkah 2.6] Cek FINER & Novelty selesai. Modul 2 Disetujui! Lanjut ke Modul 3...")
		session.Status = "M3_SEARCH"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	return nil
}
