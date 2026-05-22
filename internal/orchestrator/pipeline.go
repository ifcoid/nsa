package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nsa/internal/llm"
	"nsa/internal/modules"
	"nsa/internal/repository"
)

type SLRPipeline struct {
	mongoRepo  *repository.MongoRepository
	llmFactory *llm.LLMFactory
	registry   map[string]modules.Module
}

func NewSLRPipeline(mongo *repository.MongoRepository, factory *llm.LLMFactory) *SLRPipeline {
	deps := &modules.ModuleDeps{
		MongoRepo:  mongo,
		LLMFactory: factory,
	}

	// Mendaftarkan semua modul dari 1 hingga 9
	registry := map[string]modules.Module{
		"M1_":  modules.NewM1Foundation(deps),
		"M2_":  modules.NewM2Pico(deps),
		"M3_":  modules.NewM3Search(deps),
		"M4_":  modules.NewM4Mining(deps),
		"M5_":  modules.NewM5Screening(deps),
		"M6_":  modules.NewM6Fulltext(deps),
		"M7_":  modules.NewM7Extraction(deps),
		"M8_":  modules.NewM8Synthesis(deps),
		"M8B_": modules.NewM8bBibliometric(deps),
		"M9_":  modules.NewM9Manuscript(deps),
	}

	return &SLRPipeline{
		mongoRepo:  mongo,
		llmFactory: factory,
		registry:   registry,
	}
}

func (p *SLRPipeline) Execute(ctx context.Context, sessionID string) error {
	// 1. Ambil state sesi riset dari MongoDB
	session, err := p.mongoRepo.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("gagal mengambil sesi: %w", err)
	}

	// 2. Transisi Status Awal (Jika masih INIT)
	if session.Status == "INIT" {
		session.Status = "M1_FOUNDATION"
		err = p.mongoRepo.UpdateSession(ctx, session)
		if err != nil {
			return fmt.Errorf("gagal mengupdate status awal: %w", err)
		}
	}

	fmt.Printf("\n[Orchestrator] Memeriksa status sesi: %s\n", session.Status)

	if session.Status == "COMPLETED" {
		fmt.Println("[Orchestrator] HORE! Seluruh pipeline SLR telah selesai (Manuskrip PRISMA siap).")
		return nil
	}

	// 3. Routing Berdasarkan Prefix Status (M1, M2, dst)
	var activeModule modules.Module
	for prefix, mod := range p.registry {
		if strings.HasPrefix(session.Status, prefix) {
			activeModule = mod
			break
		}
	}

	if activeModule == nil {
		fmt.Printf("[Orchestrator] Belum ada modul terdaftar yang bisa menangani status: %s\n", session.Status)
		return nil
	}

	// 4. Eksekusi Modul Terpilih
	return activeModule.Execute(ctx, session)
}

// ExecuteAsync menjalankan pipeline di goroutine terpisah
func (p *SLRPipeline) ExecuteAsync(ctx context.Context, sessionID string) {
	go func() {
		// Gunakan background context baru agar tidak ter-cancel saat request HTTP selesai
		// Namun kita buat timeout yang cukup panjang untuk amannya
		asyncCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second) // atau lebih lama
		defer cancel()

		err := p.Execute(asyncCtx, sessionID)
		if err != nil {
			fmt.Printf("❌ [ExecuteAsync] Pipeline error untuk session %s: %v\n", sessionID, err)
		}
	}()
}

