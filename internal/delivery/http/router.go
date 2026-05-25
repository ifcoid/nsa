package http

import (
	"encoding/json"
	"net/http"

	"nsa/internal/orchestrator"
	"nsa/internal/repository"
)

type Router struct {
	mux          *http.ServeMux
	sessionHndlr *SessionHandler
	llmHndlr     *LLMHandler
}

func NewRouter(mongoRepo *repository.MongoRepository, pipeline *orchestrator.SLRPipeline) *Router {
	mux := http.NewServeMux()

	sessionHandler := NewSessionHandler(mongoRepo, pipeline)
	llmHandler := NewLLMHandler(mongoRepo)

	r := &Router{
		mux:          mux,
		sessionHndlr: sessionHandler,
		llmHndlr:     llmHandler,
	}

	r.registerRoutes()
	return r
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// CORS middleware can be added here
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

	if req.Method == "OPTIONS" {
		return
	}

	r.mux.ServeHTTP(w, req)
}

func (r *Router) registerRoutes() {
	// API Endpoints using Go 1.22+ routing syntax
	
	// Session endpoints
	r.mux.HandleFunc("POST /api/sessions", r.sessionHndlr.CreateSession)
	r.mux.HandleFunc("POST /api/sessions/{id}/resume", r.sessionHndlr.ResumeSession)
	r.mux.HandleFunc("GET /api/sessions/{id}", r.sessionHndlr.GetSession)
	r.mux.HandleFunc("PUT /api/sessions/{id}", r.sessionHndlr.UpdateSession)
	r.mux.HandleFunc("PUT /api/sessions/{id}/approve", r.sessionHndlr.ApproveStep)
	r.mux.HandleFunc("PUT /api/sessions/{id}/revise", r.sessionHndlr.ReviseStep)
	r.mux.HandleFunc("PUT /api/sessions/{id}/reimport", r.sessionHndlr.RequestReimport)
	r.mux.HandleFunc("POST /api/sessions/{id}/import-data", r.sessionHndlr.ImportData)
	
	// WebSocket endpoint untuk logs
	r.mux.HandleFunc("GET /api/ws/logs/{id}", LogStreamHandler)
	
	// LLM config endpoints
	r.mux.HandleFunc("PUT /api/llm/config", r.llmHndlr.UpdateConfig)
	r.mux.HandleFunc("POST /api/llm/providers/{id}/models", r.llmHndlr.FetchModels)
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
