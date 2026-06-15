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

// ProposalPipeline orchestrates the proposal writing pipeline modules.
type ProposalPipeline struct {
	mongoRepo         *repository.MongoRepository
	llmFactory        *llm.LLMFactory
	registry          map[string]modules.ProposalModule
	activeWorkers     map[string]bool
	activeCancelFuncs map[string]context.CancelFunc
	mu                sync.Mutex
}

// NewProposalPipeline creates a new ProposalPipeline with registered modules.
func NewProposalPipeline(mongo *repository.MongoRepository, factory *llm.LLMFactory, neo4jRepo *repository.Neo4jRepository, neo4jConnErr string) *ProposalPipeline {
	deps := &modules.ModuleDeps{
		MongoRepo:    mongo,
		Neo4jRepo:    neo4jRepo,
		Neo4jConnErr: neo4jConnErr,
		LLMFactory:   factory,
	}

	registry := map[string]modules.ProposalModule{
		"P0_": modules.NewP0Ingest(deps),
	}

	return &ProposalPipeline{
		mongoRepo:         mongo,
		llmFactory:        factory,
		registry:          registry,
		activeWorkers:     make(map[string]bool),
		activeCancelFuncs: make(map[string]context.CancelFunc),
	}
}

// Execute runs a single step of the proposal pipeline for the given session.
func (p *ProposalPipeline) Execute(ctx context.Context, sessionID string) error {
	session, err := p.mongoRepo.GetProposalSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("gagal mengambil sesi proposal: %w", err)
	}

	if session.Status == "P0_DONE" {
		logger.Log(sessionID, "[ProposalPipeline] Pipeline proposal telah selesai.")
		return nil
	}

	logger.Logf(sessionID, "\n[ProposalPipeline] Memeriksa status sesi proposal: %s\n", session.Status)

	// Route by status prefix to the appropriate module
	var activeModule modules.ProposalModule
	for prefix, mod := range p.registry {
		if strings.HasPrefix(session.Status, prefix) {
			activeModule = mod
			break
		}
	}

	if activeModule == nil {
		logger.Logf(sessionID, "[ProposalPipeline] Belum ada modul terdaftar yang bisa menangani status: %s\n", session.Status)
		return nil
	}

	return activeModule.Execute(ctx, session)
}

// ExecuteAsync runs the proposal pipeline asynchronously in a goroutine.
func (p *ProposalPipeline) ExecuteAsync(ctx context.Context, sessionID string) {
	p.mu.Lock()
	if p.activeWorkers[sessionID] {
		p.mu.Unlock()
		return // Worker already running
	}
	p.activeWorkers[sessionID] = true
	p.mu.Unlock()

	go func() {
		asyncCtx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)

		p.mu.Lock()
		p.activeCancelFuncs[sessionID] = cancel
		p.mu.Unlock()

		defer func() {
			if r := recover(); r != nil {
				logger.Logf(sessionID, "❌ [PANIC RECOVERED] %v", r)
				session, getErr := p.mongoRepo.GetProposalSession(context.Background(), sessionID)
				if getErr == nil {
					session.SystemError = fmt.Sprintf("PANIC: %v", r)
					if !strings.Contains(session.Status, "_ERROR") {
						session.Status = session.Status + "_ERROR"
					}
					_ = p.mongoRepo.UpdateProposalSession(context.Background(), session)
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
				logger.Logf(sessionID, "[ProposalPipeline] Pipeline terhenti: %v\n", err)

				// Set status to ERROR
				session, getErr := p.mongoRepo.GetProposalSession(asyncCtx, sessionID)
				if getErr == nil {
					session.SystemError = fmt.Sprintf("System Error: %v", err)
					if !strings.Contains(session.Status, "_ERROR") {
						session.Status = session.Status + "_ERROR"
						_ = p.mongoRepo.UpdateProposalSession(asyncCtx, session)
					}
				}
				break
			}

			// Get latest state to check if we should stop
			session, err := p.mongoRepo.GetProposalSession(asyncCtx, sessionID)
			if err != nil {
				break
			}

			// Stop looping if waiting for human interaction, error, or done
			if strings.Contains(session.Status, "WAITING") ||
				strings.Contains(session.Status, "ERROR") ||
				strings.Contains(session.Status, "DONE") {
				break
			}

			time.Sleep(500 * time.Millisecond)
		}
	}()
}
