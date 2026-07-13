package http

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"nsa/internal/delivery/http/middleware"
	nsamcp "nsa/internal/delivery/mcp"
	"nsa/internal/orchestrator"
	"nsa/internal/repository"
	"nsa/internal/version"
)

type Router struct {
	mux           *http.ServeMux
	sessionHndlr  *SessionHandler
	llmHndlr      *LLMHandler
	authHndlr     *AuthHandler
	bansosHndlr   *BansosHandler
	proposalHndlr *ProposalHandler
	sseServer     *mcpserver.SSEServer
}

func NewRouter(mongoRepo *repository.MongoRepository, pipeline *orchestrator.SLRPipeline, proposalPipeline *orchestrator.ProposalPipeline) *Router {
	mux := http.NewServeMux()

	sessionHandler := NewSessionHandler(mongoRepo, pipeline)
	llmHandler := NewLLMHandler(mongoRepo)

	authHandler := NewAuthHandler(mongoRepo)
	bansosHandler := NewBansosHandler(mongoRepo)

	proposalHandler := NewProposalHandler(mongoRepo, proposalPipeline)

	mcpSrv := nsamcp.NewMCPServer(mongoRepo)
	sseServer := mcpserver.NewSSEServer(mcpSrv.MCPServer,
		mcpserver.WithSSEEndpoint("/api/mcp/sse"),
		mcpserver.WithMessageEndpoint("/api/mcp/messages"),
	)

	r := &Router{
		mux:           mux,
		sessionHndlr:  sessionHandler,
		llmHndlr:      llmHandler,
		authHndlr:     authHandler,
		bansosHndlr:   bansosHandler,
		proposalHndlr: proposalHandler,
		sseServer:     sseServer,
	}

	r.registerRoutes()

	// BugBot MCP (optional, only if BUGLAPOR_BOT_TOKEN is set)
	if bugToken := os.Getenv("BUGLAPOR_BOT_TOKEN"); bugToken != "" {
		bugbotSrv := nsamcp.NewBugBotMCPServer(bugToken, mongoRepo)
		bugbotSSE := mcpserver.NewSSEServer(bugbotSrv.MCPServer,
			mcpserver.WithSSEEndpoint("/api/mcp/bugbot/sse"),
			mcpserver.WithMessageEndpoint("/api/mcp/bugbot/messages"),
		)
		bearerMW := middleware.BearerTokenMiddleware(bugToken)
		mux.Handle("GET /api/mcp/bugbot/sse", bearerMW(bugbotSSE.SSEHandler()))
		mux.Handle("POST /api/mcp/bugbot/messages", bearerMW(bugbotSSE.MessageHandler()))
		log.Println("[router] BugBot MCP enabled at /api/mcp/bugbot/")
	}

	return r
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Daftar origin yang diizinkan (CORS Whitelist)
	origin := req.Header.Get("Origin")
	allowedOrigins := map[string]bool{
		"https://www.if.co.id":  true,
		"https://if.co.id":      true,
		"http://localhost:5173": true, // Untuk dev
		"http://localhost:3000": true, // Untuk dev
	}

	if allowedOrigins[origin] {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else if origin == "" {
		// Bolehkan request langsung tanpa origin (misal dari curl/backend lain)
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

	if req.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	r.mux.ServeHTTP(w, req)
}

func (r *Router) registerRoutes() {
	// API Endpoints using Go 1.22+ routing syntax

	// Auth endpoints (Public)
	r.mux.HandleFunc("POST /api/auth/login", r.authHndlr.Login)
	r.mux.HandleFunc("POST /api/auth/register", r.authHndlr.Register)

	// Public: versi build backend (Reproducible Error — verifikasi deploy/version TANPA auth &
	// TANPA rahasia; cuma commit nsa). Path "/api/version" tak masuk prefix yang di-AuthMiddleware.
	r.mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, _ *http.Request) {
		sendJSONResponse(w, http.StatusOK, map[string]interface{}{"ok": true, "backend_version": version.Commit})
	})

	// Bansos endpoint (Public) — menerima API key dari layanan bansos
	r.mux.HandleFunc("POST /api/bansos", r.bansosHndlr.ReceiveBansosKey)

	// Protected endpoints
	protected := http.NewServeMux()

	// Session endpoints
	protected.HandleFunc("GET /api/sessions", r.sessionHndlr.ListSessions)
	protected.HandleFunc("POST /api/sessions", r.sessionHndlr.CreateSession)
	protected.HandleFunc("POST /api/sessions/{id}/resume", r.sessionHndlr.ResumeSession)
	protected.HandleFunc("GET /api/sessions/{id}", r.sessionHndlr.GetSession)
	protected.HandleFunc("PUT /api/sessions/{id}", r.sessionHndlr.UpdateSession)
	protected.HandleFunc("PUT /api/sessions/{id}/approve", r.sessionHndlr.ApproveStep)
	protected.HandleFunc("PUT /api/sessions/{id}/revise", r.sessionHndlr.ReviseStep)
	protected.HandleFunc("PUT /api/sessions/{id}/reimport", r.sessionHndlr.RequestReimport)
	protected.HandleFunc("POST /api/sessions/{id}/import-data", r.sessionHndlr.ImportData)
	protected.HandleFunc("POST /api/sessions/{id}/m4/recount-identification", r.sessionHndlr.RecountIdentification)
	protected.HandleFunc("GET /api/sessions/{id}/disagreements", r.sessionHndlr.GetDisagreements)
	protected.HandleFunc("POST /api/sessions/{id}/resolve-conflicts", r.sessionHndlr.ResolveConflicts)
	protected.HandleFunc("POST /api/sessions/{id}/pico-audit/resolve", r.sessionHndlr.ResolvePICOAudit)
	protected.HandleFunc("POST /api/sessions/{id}/pico-audit/rerun", r.sessionHndlr.RerunPICOAudit)
	protected.HandleFunc("POST /api/sessions/{id}/pico-audit/scope", r.sessionHndlr.SaveAuditScopeRules)
	protected.HandleFunc("PUT /api/sessions/{id}/framework/columns", r.sessionHndlr.SaveFrameworkColumns)
	protected.HandleFunc("PUT /api/sessions/{id}/prior-reviews", r.sessionHndlr.SavePriorReviews)
	protected.HandleFunc("GET /api/sessions/{id}/report", r.sessionHndlr.GenerateReport)
	protected.HandleFunc("GET /api/sessions/{id}/report-tex", r.sessionHndlr.GenerateReportLatex)
	protected.HandleFunc("GET /api/sessions/{id}/handoff-guide", r.sessionHndlr.HandoffGuide)
	protected.HandleFunc("GET /api/sessions/{id}/schema-guide", r.sessionHndlr.SchemaGuide)
	protected.HandleFunc("POST /api/sessions/{id}/figures", r.sessionHndlr.UploadFigures)
	protected.HandleFunc("GET /api/sessions/{id}/figures", r.sessionHndlr.ListFigures)
	protected.HandleFunc("GET /api/sessions/{id}/figures/{name}", r.sessionHndlr.GetFigure)
	protected.HandleFunc("GET /api/sessions/{id}/screening-review", r.sessionHndlr.ScreeningReview)
	protected.HandleFunc("POST /api/sessions/{id}/screening-correction", r.sessionHndlr.CorrectScreening)
	protected.HandleFunc("GET /api/sessions/{id}/extractions", r.sessionHndlr.GetExtractions)
	protected.HandleFunc("GET /api/sessions/{id}/extractions/ambiguous", r.sessionHndlr.GetAmbiguousExtractions)
	protected.HandleFunc("PUT /api/sessions/{id}/extractions/{ext_id}/resolve", r.sessionHndlr.ResolveExtractionManual)
	protected.HandleFunc("POST /api/sessions/{id}/extractions/{ext_id}/auto-resolve", r.sessionHndlr.ResolveExtractionAuto)

	// Modul 6 (Full-Text Acquisition)
	protected.HandleFunc("POST /api/sessions/{id}/m6/sync-qdrant", r.sessionHndlr.SyncQdrant)
	protected.HandleFunc("GET /api/sessions/{id}/m6/sync-qdrant/result", r.sessionHndlr.GetSyncQdrantResult)
	protected.HandleFunc("DELETE /api/sessions/{id}/m6/qdrant/paper", r.sessionHndlr.DeleteQdrantPaper)
	protected.HandleFunc("POST /api/sessions/{id}/m6/mark-inaccessible", r.sessionHndlr.MarkInaccessible)
	protected.HandleFunc("GET /api/sessions/{id}/m6/export-links", r.sessionHndlr.ExportM6Links)
	protected.HandleFunc("GET /api/sessions/{id}/m6/papers", r.sessionHndlr.GetM6Papers)
	protected.HandleFunc("GET /api/sessions/{id}/m6/excluded-fulltext", r.sessionHndlr.GetExcludedFullText)
	protected.HandleFunc("POST /api/sessions/{id}/m6/recode-exclusions", r.sessionHndlr.RecodeExclusions)
	protected.HandleFunc("POST /api/sessions/{id}/m6/suggest-recodes", r.sessionHndlr.SuggestRecodes)
	protected.HandleFunc("GET /api/sessions/{id}/m6/suggest-recodes/result", r.sessionHndlr.GetRecodeResult)

	// Modul 7 Reset
	protected.HandleFunc("POST /api/sessions/{id}/reset-m7", r.sessionHndlr.ResetModul7)

	// Modul 7 QA Recalculation
	protected.HandleFunc("POST /api/sessions/{id}/m7/recalculate-qa", r.sessionHndlr.RecalculateQA)

	// Modul 7 QA jalankan-ulang seluruh proses (tool → kalibrasi → rating), ekstraksi dipertahankan
	protected.HandleFunc("POST /api/sessions/{id}/m7/rerun-qa", r.sessionHndlr.RerunQA)
	protected.HandleFunc("POST /api/sessions/{id}/m10/fix-qa-errors", r.sessionHndlr.FixQAErrors)

	// Modul 7 QA Re-rate single paper
	protected.HandleFunc("POST /api/sessions/{id}/m7/rerate-paper", r.sessionHndlr.ReratePaper)

	// Modul 7 QA Prompt (xAI transparency)
	protected.HandleFunc("GET /api/sessions/{id}/m7/qa-prompt", r.sessionHndlr.GetQAPrompt)

	// Modul 7 Metadata Enrichment (CrossRef)
	protected.HandleFunc("POST /api/sessions/{id}/m7/enrich-metadata", r.sessionHndlr.EnrichMetadata)

	// xAI Audit Log
	protected.HandleFunc("GET /api/sessions/{id}/xai-log", r.sessionHndlr.GetXAILog)
	// Reproducible Error (xAI): jejak panggilan LLM gagal terakhir (utk tampil + replay)
	protected.HandleFunc("GET /api/sessions/{id}/llm-debug", r.sessionHndlr.GetLLMDebug)
	// Diagnostic state DB tersanitasi (disisipkan ke laporan bug → tak perlu akses Mongo user)
	protected.HandleFunc("GET /api/sessions/{id}/diagnostic", r.sessionHndlr.GetSessionDiagnostic)

	// Modul 8b (Bibliometric/SLNA)
	protected.HandleFunc("POST /api/sessions/{id}/m8b/vosviewer", r.sessionHndlr.SubmitVOSviewer)
	protected.HandleFunc("GET /api/sessions/{id}/m8b/export-ris", r.sessionHndlr.ExportRIS)
	protected.HandleFunc("GET /api/sessions/{id}/m8b/export-bibtex", r.sessionHndlr.ExportBibTeX) // alias for backward compat
	protected.HandleFunc("POST /api/sessions/{id}/m8b/enrich-scopus-keywords", r.sessionHndlr.EnrichScopusKeywords)
	protected.HandleFunc("POST /api/sessions/{id}/m8b/upload-scopus-csv", r.sessionHndlr.UploadScopusCSV)
	protected.HandleFunc("POST /api/sessions/{id}/m8b/upload-ieee-csv", r.sessionHndlr.UploadIEEECSV)
	protected.HandleFunc("POST /api/sessions/{id}/m8b/upload-pubmed-txt", r.sessionHndlr.UploadPubMedTXT)

	// Modul 9 Manuscript downloads
	protected.HandleFunc("GET /api/sessions/{id}/manuscript/download-tex", r.sessionHndlr.DownloadTex)
	protected.HandleFunc("GET /api/sessions/{id}/manuscript/download-bib", r.sessionHndlr.DownloadBib)
	protected.HandleFunc("GET /api/sessions/{id}/manuscript/download-md", r.sessionHndlr.DownloadMd)
	protected.HandleFunc("GET /api/sessions/{id}/manuscript/meta", r.sessionHndlr.ManuscriptMeta)

	// LLM config endpoints
	protected.HandleFunc("GET /api/llm/health", r.llmHndlr.CheckHealth)
	protected.HandleFunc("GET /api/llm/preflight", r.llmHndlr.PreflightRoles)
	protected.HandleFunc("POST /api/llm/replay", r.llmHndlr.ReplayLLM)
	protected.HandleFunc("GET /api/llm/replay/{id}", r.llmHndlr.GetReplayResult)
	protected.HandleFunc("GET /api/llm/config", r.llmHndlr.ListConfigs)
	protected.HandleFunc("PUT /api/llm/config", r.llmHndlr.UpdateConfig)
	protected.HandleFunc("POST /api/llm/providers/{id}/models", r.llmHndlr.FetchModels)
	protected.HandleFunc("POST /api/llm/test", r.llmHndlr.TestModel)

	// MCP (Server-Sent Events) Endpoints (Publicly accessible for the agent)
	r.mux.Handle("GET /api/mcp/sse", r.sseServer.SSEHandler())
	r.mux.Handle("POST /api/mcp/messages", r.sseServer.MessageHandler())
	protected.HandleFunc("GET /api/llm/roles", r.llmHndlr.GetRoles)
	protected.HandleFunc("PUT /api/llm/roles", r.llmHndlr.UpdateRoles)
	protected.HandleFunc("GET /api/github/config", r.llmHndlr.GetGitHubConfig)
	protected.HandleFunc("PUT /api/github/config", r.llmHndlr.UpdateGitHubConfig)
	protected.HandleFunc("GET /api/embed/config", r.llmHndlr.GetEmbedConfig)
	protected.HandleFunc("PUT /api/embed/config", r.llmHndlr.UpdateEmbedConfig)
	protected.HandleFunc("GET /api/scopus/config", r.llmHndlr.GetScopusConfig)
	protected.HandleFunc("PUT /api/scopus/config", r.llmHndlr.UpdateScopusConfig)
	protected.HandleFunc("GET /api/zenodo/config", r.sessionHndlr.GetZenodoConfig)
	protected.HandleFunc("PUT /api/zenodo/config", r.sessionHndlr.UpdateZenodoConfig)
	protected.HandleFunc("POST /api/sessions/{id}/zenodo/deposit", r.sessionHndlr.ZenodoDeposit)

	// Proposal endpoints
	protected.HandleFunc("POST /api/proposal/sessions", r.proposalHndlr.CreateProposalSession)
	protected.HandleFunc("GET /api/proposal/sessions/{id}", r.proposalHndlr.GetProposalSession)
	protected.HandleFunc("PUT /api/proposal/sessions/{id}", r.proposalHndlr.UpdateProposalSession)
	protected.HandleFunc("POST /api/proposal/sessions/{id}/upload-bib", r.proposalHndlr.UploadBib)
	protected.HandleFunc("POST /api/proposal/sessions/{id}/upload-pdf", r.proposalHndlr.UploadPDF)
	protected.HandleFunc("PUT /api/proposal/sessions/{id}/embed-endpoint", r.proposalHndlr.SetEmbedEndpoint)
	protected.HandleFunc("POST /api/proposal/sessions/{id}/resume", r.proposalHndlr.ResumeProposal)
	protected.HandleFunc("GET /api/proposal/sessions/{id}/refs", r.proposalHndlr.GetProposalRefs)
	protected.HandleFunc("GET /api/proposal/sessions/{id}/refs/missing-pdfs", r.proposalHndlr.GetMissingPDFRefs)

	// Apply Auth Middleware to all protected routes
	r.mux.Handle("/api/sessions", middleware.AuthMiddleware(protected))
	r.mux.Handle("/api/sessions/", middleware.AuthMiddleware(protected))
	r.mux.Handle("/api/llm/", middleware.AuthMiddleware(protected))
	r.mux.Handle("/api/github/", middleware.AuthMiddleware(protected))
	r.mux.Handle("/api/embed/", middleware.AuthMiddleware(protected))
	r.mux.Handle("/api/scopus/", middleware.AuthMiddleware(protected))
	// BUGFIX: prefix /api/zenodo/ WAJIB di-mount ke protected — tanpa ini GET/PUT
	// /api/zenodo/config 404 (handler terdaftar tapi tak terjangkau) → user tak bisa simpan
	// token Zenodo (lapor Sindy). Deposit lolos karena ada di bawah /api/sessions/.
	r.mux.Handle("/api/zenodo/", middleware.AuthMiddleware(protected))
	r.mux.Handle("/api/proposal/sessions", middleware.AuthMiddleware(protected))
	r.mux.Handle("/api/proposal/", middleware.AuthMiddleware(protected))

	// WebSocket endpoint untuk logs (Tidak diproteksi ketat karena via URL /ws/, jika butuh auth bisa pasang token di query)
	r.mux.HandleFunc("GET /api/ws/logs/{id}", LogStreamHandler)
}

// Utility function to send JSON response
func sendJSONResponse(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if payload != nil {
		json.NewEncoder(w).Encode(payload)
	}
}

// Utility function to send JSON error response
func sendJSONError(w http.ResponseWriter, statusCode int, message string) {
	sendJSONResponse(w, statusCode, map[string]string{"error": message})
}
