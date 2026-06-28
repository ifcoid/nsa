package http

import (
	"context"
	"encoding/json"
	"net/http"

	"nsa/internal/repository"
)

type BansosHandler struct {
	mongoRepo *repository.MongoRepository
}

func NewBansosHandler(mongoRepo *repository.MongoRepository) *BansosHandler {
	return &BansosHandler{
		mongoRepo: mongoRepo,
	}
}

// ReceiveBansosKey menerima {id, api_key} dan menyimpannya ke koleksi bansos jika api_key belum ada.
func (h *BansosHandler) ReceiveBansosKey(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		ID     string `json:"id"`
		APIKey string `json:"api_key"`
	}

	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if payload.ID == "" || payload.APIKey == "" {
		sendJSONError(w, http.StatusBadRequest, "Both id and api_key are required")
		return
	}

	ctx := context.Background()
	if err := h.mongoRepo.InsertBansosKey(ctx, payload.ID, payload.APIKey); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to insert bansos key")
		return
	}

	sendJSONResponse(w, http.StatusOK, map[string]bool{"ok": true})
}
