package http

import (
	"encoding/json"
	"net/http"

	"nsa/internal/delivery/http/middleware"
	"nsa/internal/orchestrator"
	"nsa/internal/repository"
)

type Router struct {
	mux          *http.ServeMux
	sessionHndlr *SessionHandler
	llmHndlr     *LLMHandler
	authHndlr    *AuthHandler
}

func NewRouter(mongoRepo *repository.MongoRepository, pipeline *orchestrator.SLRPipeline) *Router {
	mux := http.NewServeMux()

	sessionHandler := NewSessionHandler(mongoRepo, pipeline)
	llmHandler := NewLLMHandler(mongoRepo)

	authHandler := NewAuthHandler(mongoRepo)

	r := &Router{
		mux:          mux,
		sessionHndlr: sessionHandler,
		llmHndlr:     llmHandler,
		authHndlr:    authHandler,
	}

	r.registerRoutes()
	return r
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Daftar origin yang diizinkan (CORS Whitelist)
	origin := req.Header.Get("Origin")
	allowedOrigins := map[string]bool{
		"https://www.if.co.id": true,
		"https://if.co.id":     true,
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
	
	// Protected endpoints
	protected := http.NewServeMux()
	
	// Session endpoints
	protected.HandleFunc("POST /api/sessions", r.sessionHndlr.CreateSession)
	protected.HandleFunc("POST /api/sessions/{id}/resume", r.sessionHndlr.ResumeSession)
	protected.HandleFunc("GET /api/sessions/{id}", r.sessionHndlr.GetSession)
	protected.HandleFunc("PUT /api/sessions/{id}", r.sessionHndlr.UpdateSession)
	protected.HandleFunc("PUT /api/sessions/{id}/approve", r.sessionHndlr.ApproveStep)
	protected.HandleFunc("PUT /api/sessions/{id}/revise", r.sessionHndlr.ReviseStep)
	protected.HandleFunc("PUT /api/sessions/{id}/reimport", r.sessionHndlr.RequestReimport)
	protected.HandleFunc("POST /api/sessions/{id}/import-data", r.sessionHndlr.ImportData)
	protected.HandleFunc("GET /api/sessions/{id}/disagreements", r.sessionHndlr.GetDisagreements)
	protected.HandleFunc("POST /api/sessions/{id}/resolve-conflicts", r.sessionHndlr.ResolveConflicts)
	
	// LLM config endpoints
	protected.HandleFunc("PUT /api/llm/config", r.llmHndlr.UpdateConfig)
	protected.HandleFunc("POST /api/llm/providers/{id}/models", r.llmHndlr.FetchModels)
	
	// Apply Auth Middleware to all protected routes
	r.mux.Handle("/api/sessions", middleware.AuthMiddleware(protected))
	r.mux.Handle("/api/sessions/", middleware.AuthMiddleware(protected))
	r.mux.Handle("/api/llm/", middleware.AuthMiddleware(protected))
	
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
