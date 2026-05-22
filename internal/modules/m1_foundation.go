package modules

import (
	"context"
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
	logger.Log(session.ID, ">> [MODUL 1: FONDASI TEORI] Memulai briefing agen terkait aturan PRISMA 2020...")
	
	// Simulasi eksekusi langkah 1.1 - 1.5
	logger.Log(session.ID, "   [Langkah 1.1] Pengenalan Systematic Literature Review...")
	logger.Log(session.ID, "   [Langkah 1.5] Menerapkan Aturan Global SLR + CoWork...")

	// Jika sukses, transisi langsung ke Modul 2
	logger.Log(session.ID, ">> [MODUL 1] Selesai. Transisi otomatis ke M2_STEP1_TOPIC_GAP.")
	session.Status = "M2_STEP1_TOPIC_GAP"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}
