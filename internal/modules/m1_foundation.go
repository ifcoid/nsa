package modules

import (
	"context"
	"fmt"
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
	fmt.Println(">> [MODUL 1: FONDASI TEORI] Memulai briefing agen terkait aturan PRISMA 2020...")
	
	// Simulasi eksekusi langkah 1.1 - 1.5
	fmt.Println("   [Langkah 1.1] Pengenalan Systematic Literature Review...")
	fmt.Println("   [Langkah 1.5] Menerapkan Aturan Global SLR + CoWork...")

	// Jika sukses, transisi langsung ke Modul 2
	fmt.Println(">> [MODUL 1] Selesai. Transisi otomatis ke M2_PICO.")
	session.Status = "M2_PICO"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}
