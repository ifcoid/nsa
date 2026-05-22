package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"nsa/internal/model"
	"nsa/internal/orchestrator"
	"nsa/internal/repository"
)

type SessionHandler struct {
	mongoRepo *repository.MongoRepository
	pipeline  *orchestrator.SLRPipeline
}

func NewSessionHandler(mongo *repository.MongoRepository, pipeline *orchestrator.SLRPipeline) *SessionHandler {
	return &SessionHandler{
		mongoRepo: mongo,
		pipeline:  pipeline,
	}
}

func (h *SessionHandler) CreateSession(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		ID    string `json:"id"`
		Topic string `json:"topic"`
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

	// Check if session exists
	existingSession, err := h.mongoRepo.GetSession(ctx, payload.ID)
	if err == nil {
		// Jika sesi sudah ada, biarkan state-nya seperti semula (Resume)
		// Tapi kita update topiknya jika ternyata user memasukkan topik baru
		existingSession.Topic = payload.Topic
		if err := h.mongoRepo.UpdateSession(ctx, existingSession); err != nil {
			sendJSONError(w, http.StatusInternalServerError, "Failed to update session")
			return
		}
	} else {
		// Create new session
		session := &model.SLRSession{
			ID:     payload.ID,
			Topic:  payload.Topic,
			Status: "INIT",
		}
		if err := h.mongoRepo.UpdateSession(ctx, session); err != nil {
			sendJSONError(w, http.StatusInternalServerError, "Failed to create session")
			return
		}
	}

	// Trigger pipeline asynchronously
	h.pipeline.ExecuteAsync(ctx, payload.ID)

	sendJSONResponse(w, http.StatusCreated, map[string]string{
		"message": "Session created/reset successfully and pipeline started",
		"id":      payload.ID,
	})
}

func (h *SessionHandler) GetSession(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	session, err := h.mongoRepo.GetSession(context.Background(), id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	sendJSONResponse(w, http.StatusOK, session)
}

func (h *SessionHandler) ResumeSession(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	_, err := h.mongoRepo.GetSession(context.Background(), id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	// Trigger pipeline asynchronously (will be ignored if already running)
	h.pipeline.ExecuteAsync(context.Background(), id)

	sendJSONResponse(w, http.StatusOK, map[string]string{
		"message": "Session resume triggered",
		"id":      id,
	})
}

func (h *SessionHandler) ApproveStep(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	ctx := context.Background()
	session, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	// Here we just change the status based on current status
	// The client can pass data they want to update (e.g. selected_topic)
	var updateData map[string]interface{}
	if err := json.NewDecoder(req.Body).Decode(&updateData); err == nil {
		if retry, ok := updateData["is_retry"].(bool); ok && retry {
			// Jika ini retry dari error, kembalikan status dengan menghapus akhiran _ERROR
			session.Status = strings.ReplaceAll(session.Status, "_ERROR", "")
			session.Feedback = "" // Hapus log error sebelumnya
		} else if session.Status == "M2_STEP1_WAITING_APPROVAL" {
			if selected, ok := updateData["selected_topic"]; ok {
				b, _ := json.Marshal(selected)
				var st model.SuggestedTopic
				json.Unmarshal(b, &st)
				session.SelectedTopic = &st
			}
			session.Status = "M2_STEP1_APPROVED"
		} else if session.Status == "M2_STEP2_WAITING_APPROVAL" {
			session.Status = "M2_STEP2_APPROVED"
		} else if session.Status == "M2_STEP3_WAITING_APPROVAL" {
			session.Status = "M2_STEP3_APPROVED"
		} else if session.Status == "M2_STEP4_WAITING_APPROVAL" {
			session.Status = "M2_STEP4_APPROVED"
		} else if session.Status == "M2_STEP5_WAITING_APPROVAL" {
			session.Status = "M2_STEP5_APPROVED"
		} else if session.Status == "M2_STEP6_WAITING_APPROVAL" {
			session.Status = "M2_STEP6_APPROVED"
		}
	} else {
		// Default simple approve without body
		if strings.HasSuffix(session.Status, "_WAITING_APPROVAL") {
			session.Status = session.Status[:len(session.Status)-17] + "_APPROVED"
		}
	}

	if err := h.mongoRepo.UpdateSession(ctx, session); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to approve session")
		return
	}

	// Trigger pipeline for next step
	h.pipeline.ExecuteAsync(ctx, session.ID)

	sendJSONResponse(w, http.StatusOK, map[string]string{
		"message": "Step approved successfully, pipeline progressing",
		"status":  session.Status,
	})
}

func (h *SessionHandler) ReviseStep(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	var payload struct {
		Feedback string `json:"feedback"`
	}

	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	ctx := context.Background()
	session, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	session.Feedback = payload.Feedback

	// Determine NEEDS_REVISION status
	if session.Status == "M2_STEP1_WAITING_APPROVAL" {
		session.Status = "M2_STEP1_NEEDS_REVISION"
	} else {
		session.Status = fmt.Sprintf("%s_NEEDS_REVISION", session.Status[:len(session.Status)-17])
	}

	if err := h.mongoRepo.UpdateSession(ctx, session); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to set revision status")
		return
	}

	// Trigger pipeline again
	h.pipeline.ExecuteAsync(ctx, session.ID)

	sendJSONResponse(w, http.StatusOK, map[string]string{
		"message": "Revision requested successfully, pipeline processing",
		"status":  session.Status,
	})
}
