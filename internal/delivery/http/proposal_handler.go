package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"nsa/internal/model"
	"nsa/internal/orchestrator"
	"nsa/internal/parser"
	"nsa/internal/repository"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ProposalHandler handles HTTP requests for the proposal pipeline.
type ProposalHandler struct {
	mongoRepo *repository.MongoRepository
	pipeline  *orchestrator.ProposalPipeline
}

// NewProposalHandler creates a new ProposalHandler instance.
func NewProposalHandler(mongoRepo *repository.MongoRepository, pipeline *orchestrator.ProposalPipeline) *ProposalHandler {
	return &ProposalHandler{
		mongoRepo: mongoRepo,
		pipeline:  pipeline,
	}
}

// CreateProposalSession handles POST /api/proposal/sessions
func (h *ProposalHandler) CreateProposalSession(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		ID     string `json:"id"`
		Topic  string `json:"topic"`
		UserID string `json:"user_id"`
	}

	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if payload.ID == "" || payload.Topic == "" {
		sendJSONError(w, http.StatusBadRequest, "ID and Topic are required")
		return
	}

	ctx := context.Background()

	session := &model.ProposalSession{
		ID:        payload.ID,
		Topic:     payload.Topic,
		UserID:    payload.UserID,
		Status:    "P0_INIT",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := h.mongoRepo.CreateProposalSession(ctx, session); err != nil {
		// If session already exists, update it
		existing, getErr := h.mongoRepo.GetProposalSession(ctx, payload.ID)
		if getErr == nil {
			existing.Topic = payload.Topic
			existing.UserID = payload.UserID
			if err := h.mongoRepo.UpdateProposalSession(ctx, existing); err != nil {
				sendJSONError(w, http.StatusInternalServerError, "Failed to update proposal session")
				return
			}
		} else {
			sendJSONError(w, http.StatusInternalServerError, "Failed to create proposal session")
			return
		}
	}

	// Trigger pipeline asynchronously
	h.pipeline.ExecuteAsync(ctx, payload.ID)

	sendJSONResponse(w, http.StatusCreated, map[string]string{
		"message": "Proposal session created successfully and pipeline started",
		"id":      payload.ID,
	})
}

// GetProposalSession handles GET /api/proposal/sessions/{id}
func (h *ProposalHandler) GetProposalSession(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	session, err := h.mongoRepo.GetProposalSession(context.Background(), id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Proposal session not found")
		return
	}

	sendJSONResponse(w, http.StatusOK, session)
}

// UpdateProposalSession handles PUT /api/proposal/sessions/{id}
func (h *ProposalHandler) UpdateProposalSession(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	ctx := context.Background()
	session, err := h.mongoRepo.GetProposalSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Proposal session not found")
		return
	}

	var updateData map[string]interface{}
	if err := json.NewDecoder(req.Body).Decode(&updateData); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if status, ok := updateData["status"].(string); ok {
		session.Status = status
	}
	if feedback, ok := updateData["feedback"].(string); ok {
		session.Feedback = feedback
	}

	if err := h.mongoRepo.UpdateProposalSession(ctx, session); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to update proposal session")
		return
	}

	// Trigger pipeline if status changed
	h.pipeline.ExecuteAsync(ctx, session.ID)

	sendJSONResponse(w, http.StatusOK, map[string]string{
		"message": "Proposal session updated successfully",
		"status":  session.Status,
	})
}

// UploadBib handles POST /api/proposal/sessions/{id}/upload-bib
func (h *ProposalHandler) UploadBib(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	ctx := context.Background()
	session, err := h.mongoRepo.GetProposalSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Proposal session not found")
		return
	}

	err = req.ParseMultipartForm(50 << 20) // Max 50 MB
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "Failed to parse multipart form")
		return
	}

	file, _, err := req.FormFile("file")
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "Failed to read uploaded file")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to read file content")
		return
	}

	refs, parseErrs := parser.ParseBibTeX(content)
	if len(refs) == 0 {
		errMsg := "No valid BibTeX entries found"
		if len(parseErrs) > 0 {
			errMsg = parseErrs[0].Error()
		}
		sendJSONError(w, http.StatusBadRequest, errMsg)
		return
	}

	// Upsert refs to MongoDB
	if err := h.mongoRepo.UpsertProposalRefs(ctx, id, refs); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to save references: "+err.Error())
		return
	}

	// Update session status
	session.Status = "P0_BIB_PARSED"
	if err := h.mongoRepo.UpdateProposalSession(ctx, session); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to update session status")
		return
	}

	// Trigger pipeline
	h.pipeline.ExecuteAsync(ctx, session.ID)

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"message":      "BibTeX file parsed successfully",
		"total_refs":   len(refs),
		"parse_errors": len(parseErrs),
		"status":       session.Status,
	})
}

// UploadPDF handles POST /api/proposal/sessions/{id}/upload-pdf
func (h *ProposalHandler) UploadPDF(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	err := req.ParseMultipartForm(50 << 20)
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "Failed to parse multipart form")
		return
	}

	citeKey := req.FormValue("cite_key")
	if citeKey == "" {
		sendJSONError(w, http.StatusBadRequest, "cite_key is required")
		return
	}

	ctx := context.Background()

	// Mark the ref as is_embedded=true in MongoDB
	collection := h.mongoRepo.GetDB().Collection("proposal_refs")
	filter := bson.M{"cite_key": citeKey, "session_id": id}
	update := bson.M{"$set": bson.M{"is_embedded": true}}
	opts := options.Update().SetUpsert(false)

	result, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to update reference: "+err.Error())
		return
	}

	if result.MatchedCount == 0 {
		sendJSONError(w, http.StatusNotFound, "Reference not found for cite_key: "+citeKey)
		return
	}

	sendJSONResponse(w, http.StatusOK, map[string]string{
		"message":  "PDF reference marked as embedded",
		"cite_key": citeKey,
	})
}

// SetEmbedEndpoint handles PUT /api/proposal/sessions/{id}/embed-endpoint
func (h *ProposalHandler) SetEmbedEndpoint(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	var payload struct {
		Endpoint string `json:"endpoint"`
	}

	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if payload.Endpoint == "" {
		sendJSONError(w, http.StatusBadRequest, "endpoint is required")
		return
	}

	ctx := context.Background()
	session, err := h.mongoRepo.GetProposalSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Proposal session not found")
		return
	}

	session.EmbedEndpoint = payload.Endpoint
	session.Status = "P0_EMBED_SERVER_READY"

	if err := h.mongoRepo.UpdateProposalSession(ctx, session); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to update session")
		return
	}

	// Trigger pipeline
	h.pipeline.ExecuteAsync(ctx, session.ID)

	sendJSONResponse(w, http.StatusOK, map[string]string{
		"message":  "Embed endpoint set successfully",
		"endpoint": payload.Endpoint,
		"status":   session.Status,
	})
}

// ResumeProposal handles POST /api/proposal/sessions/{id}/resume
func (h *ProposalHandler) ResumeProposal(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	ctx := context.Background()
	_, err := h.mongoRepo.GetProposalSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Proposal session not found")
		return
	}

	// Trigger pipeline asynchronously
	h.pipeline.ExecuteAsync(ctx, id)

	sendJSONResponse(w, http.StatusOK, map[string]string{
		"message": "Proposal pipeline resume triggered",
		"id":      id,
	})
}

// GetProposalRefs handles GET /api/proposal/sessions/{id}/refs
func (h *ProposalHandler) GetProposalRefs(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	refs, err := h.mongoRepo.GetProposalRefs(context.Background(), id)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to get proposal refs: "+err.Error())
		return
	}

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"refs": refs,
	})
}

// GetMissingPDFRefs handles GET /api/proposal/sessions/{id}/refs/missing-pdfs
func (h *ProposalHandler) GetMissingPDFRefs(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	refs, err := h.mongoRepo.GetMissingPDFRefs(context.Background(), id)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to get missing PDF refs: "+err.Error())
		return
	}

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"refs": refs,
	})
}
