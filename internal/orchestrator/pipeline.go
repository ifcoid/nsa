package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"nsa/internal/llm"
	"nsa/internal/logger"
	"nsa/internal/modules"
	"nsa/internal/repository"
)

type SLRPipeline struct {
	mongoRepo     *repository.MongoRepository
	llmFactory    *llm.LLMFactory
	registry      map[string]modules.Module
	activeWorkers map[string]bool
	mu            sync.Mutex
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
		mongoRepo:     mongo,
		llmFactory:    factory,
		registry:      registry,
		activeWorkers: make(map[string]bool),
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

	logger.Logf(sessionID, "\n[Orchestrator] Memeriksa status sesi: %s\n", session.Status)

	if session.Status == "COMPLETED" {
		logger.Log(sessionID, "[Orchestrator] HORE! Seluruh pipeline SLR telah selesai (Manuskrip PRISMA siap).")
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
		logger.Logf(sessionID, "[Orchestrator] Belum ada modul terdaftar yang bisa menangani status: %s\n", session.Status)
		return nil
	}

	// 4. Eksekusi Modul Terpilih
	return activeModule.Execute(ctx, session)
}

// ExecuteAsync menjalankan pipeline di goroutine terpisah
func (p *SLRPipeline) ExecuteAsync(ctx context.Context, sessionID string) {
	p.mu.Lock()
	if p.activeWorkers[sessionID] {
		p.mu.Unlock()
		return // Worker sudah berjalan
	}
	p.activeWorkers[sessionID] = true
	p.mu.Unlock()

	go func() {
		// Gunakan background context baru agar tidak ter-cancel saat request HTTP selesai
		// Beri timeout panjang karena panggilan LLM bisa memakan waktu
		asyncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		
		defer func() {
			cancel()
			p.mu.Lock()
			delete(p.activeWorkers, sessionID)
			p.mu.Unlock()
		}()

		for {
			err := p.Execute(asyncCtx, sessionID)
			if err != nil {
				logger.Logf(sessionID, "❌ [ExecuteAsync] Pipeline error untuk session %s: %v\n", sessionID, err)
				
				// Set status ke ERROR agar UI frontend berhenti loading dan memunculkan tombol revisi/retry
				session, getErr := p.mongoRepo.GetSession(asyncCtx, sessionID)
				if getErr == nil {
					// Tandai error di system_error (biarkan feedback tetap utuh)
					session.SystemError = fmt.Sprintf("System Error: %v", err)
					
					// Jika status belum memiliki _ERROR
					if !strings.Contains(session.Status, "_ERROR") {
						session.Status = session.Status + "_ERROR"
						_ = p.mongoRepo.UpdateSession(asyncCtx, session)
					}
				}
				break
			}

			// Ambil state terbaru untuk mengecek apakah harus berhenti
			session, err := p.mongoRepo.GetSession(asyncCtx, sessionID)
			if err != nil {
				break
			}

			// Berhenti looping jika butuh interaksi manusia atau sudah selesai
			if strings.Contains(session.Status, "WAITING_APPROVAL") || 
			   strings.Contains(session.Status, "NEEDS_REVISION") || 
			   session.Status == "COMPLETED" {
				break
			}
			
			// Jeda singkat antar eksekusi untuk keamanan
			time.Sleep(500 * time.Millisecond)
		}
	}()
}

