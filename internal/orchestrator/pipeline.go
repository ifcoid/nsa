package orchestrator

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"nsa/internal/llm"
	"nsa/internal/logger"
	"nsa/internal/modules"
	"nsa/internal/notify"
	"nsa/internal/repository"
)

// pipelineTimeout adalah plafon waktu satu run worker background (ExecuteAsync). Default
// GENEROUS 24 jam karena ekstraksi/QA ratusan paper (LLM per-paper + jeda rate-limit) bisa
// memakan banyak jam. Bisa diubah tanpa redeploy via env PIPELINE_TIMEOUT_HOURS.
func pipelineTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("PIPELINE_TIMEOUT_HOURS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Hour
		}
	}
	return 24 * time.Hour
}

type SLRPipeline struct {
	mongoRepo     *repository.MongoRepository
	llmFactory    *llm.LLMFactory
	registry          map[string]modules.Module
	activeWorkers     map[string]bool
	activeCancelFuncs map[string]context.CancelFunc
	mu                sync.Mutex
}

func NewSLRPipeline(mongo *repository.MongoRepository, factory *llm.LLMFactory, neo4jRepo *repository.Neo4jRepository, neo4jConnErr string) *SLRPipeline {
	deps := &modules.ModuleDeps{
		MongoRepo:    mongo,
		Neo4jRepo:    neo4jRepo,
		Neo4jConnErr: neo4jConnErr,
		LLMFactory:   factory,
	}

	// Mendaftarkan semua modul dari 1 hingga 9
	registry := map[string]modules.Module{
		"M1_":  modules.NewM1Foundation(deps),
		"M2_":  modules.NewM2Pico(deps),
		"M3_":  modules.NewM3Search(deps),
		"M4_":  modules.NewM4Mining(deps),
		"M5_":  modules.NewM5Screening(deps),
		"M6_":  modules.NewM6Acquisition(deps),
		"M7_":  modules.NewM7Extraction(deps),
		"M8_":  modules.NewM8Synthesis(deps),
		"M8B_": modules.NewM8bBibliometric(deps),
		"M9_":  modules.NewM9Manuscript(deps),
	}

	return &SLRPipeline{
		mongoRepo:         mongo,
		llmFactory:        factory,
		registry:          registry,
		activeWorkers:     make(map[string]bool),
		activeCancelFuncs: make(map[string]context.CancelFunc),
	}
}

func (p *SLRPipeline) GetLLMFactory() *llm.LLMFactory {
	return p.llmFactory
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
		// Beri timeout panjang (configurable) karena panggilan LLM bisa memakan banyak jam
		asyncCtx, cancel := context.WithTimeout(context.Background(), pipelineTimeout())
		
		p.mu.Lock()
		p.activeCancelFuncs[sessionID] = cancel
		p.mu.Unlock()

		defer func() {
			if r := recover(); r != nil {
				logger.Logf(sessionID, "❌ [PANIC RECOVERED] %v", r)
				session, getErr := p.mongoRepo.GetSession(context.Background(), sessionID)
				if getErr == nil {
					session.SystemError = fmt.Sprintf("PANIC: %v", r)
					if !strings.Contains(session.Status, "_ERROR") {
						session.Status = session.Status + "_ERROR"
					}
					_ = p.mongoRepo.UpdateSession(context.Background(), session)
				}
			}
			cancel()
			p.mu.Lock()
			delete(p.activeWorkers, sessionID)
			delete(p.activeCancelFuncs, sessionID)
			p.mu.Unlock()
		}()

		for {
			err := p.Execute(asyncCtx, sessionID)
			if err != nil {
				totalP, screenedP, _ := p.mongoRepo.GetScreeningProgress(asyncCtx, sessionID)
				logger.Logf(sessionID, "❌ [ExecuteAsync] Pipeline terhenti untuk session %s: %v\n", sessionID, err)
				if totalP > 0 {
					logger.Logf(sessionID, "   ℹ️ [Info] Meskipun terhenti, progres Anda tersimpan aman: %d dari %d papers telah berhasil discreening.\n", screenedP, totalP)
				}
				
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
					notify.Telegram(notify.GateMessage(sessionID, session.Status))
				}
				break
			}

			// Ambil state terbaru untuk mengecek apakah harus berhenti
			session, err := p.mongoRepo.GetSession(asyncCtx, sessionID)
			if err != nil {
				break
			}

			// Berhenti looping jika butuh interaksi manusia, sedang error, atau sudah selesai
			if strings.Contains(session.Status, "WAITING") ||
			   strings.Contains(session.Status, "NEEDS_REVISION") ||
			   strings.Contains(session.Status, "ERROR") ||
			   strings.Contains(session.Status, "LOW_KAPPA") ||
			   (strings.Contains(session.Status, "DONE") && session.Status != "M5_DONE") ||
			   session.Status == "COMPLETED" {
				notify.Telegram(notify.GateMessage(sessionID, session.Status))
				break
			}
			
			// Jeda singkat antar eksekusi untuk keamanan
			time.Sleep(500 * time.Millisecond)
		}
	}()
}

// ResumeInProgress melanjutkan otomatis sesi yang berstatus "sedang jalan" saat
// startup (mis. setelah deploy/restart mesin fly). Menutup celah perlu klik Resume
// manual: worker yang terputus dilanjutkan dari progres terakhir (tersimpan di DB).
func (p *SLRPipeline) ResumeInProgress(ctx context.Context) {
	ids, err := p.mongoRepo.ListResumableSessions(ctx)
	if err != nil {
		logger.Logf("system", "[Startup] Gagal memindai sesi untuk auto-resume: %v\n", err)
		return
	}
	if len(ids) == 0 {
		logger.Log("system", "[Startup] Tak ada sesi 'sedang jalan' — tak perlu auto-resume.")
		return
	}
	for _, id := range ids {
		logger.Logf(id, "[Startup] Auto-resume sesi (status sedang jalan) setelah restart.\n")
		p.ExecuteAsync(context.Background(), id)
	}
}

// StopWorker membatalkan eksekusi pipeline yang sedang berjalan untuk sesi tertentu
func (p *SLRPipeline) StopWorker(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if cancel, exists := p.activeCancelFuncs[sessionID]; exists {
		logger.Logf(sessionID, "[Orchestrator] Menghentikan paksa worker untuk sesi %s...\n", sessionID)
		cancel()
		delete(p.activeCancelFuncs, sessionID)
		// activeWorkers map will be cleaned up by the defer func in ExecuteAsync
	}
}
