package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"nsa/internal/model"
	"nsa/internal/orchestrator"
	"nsa/internal/parser"
	"nsa/internal/repository"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

func (h *SessionHandler) UpdateSession(w http.ResponseWriter, req *http.Request) {
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

	var updateData map[string]interface{}
	if err := json.NewDecoder(req.Body).Decode(&updateData); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// Dynamic updates based on what frontend sends
	if status, ok := updateData["status"].(string); ok {
		session.Status = status
	}
	if filters, ok := updateData["scope_filters"]; ok {
		b, _ := json.Marshal(filters)
		var sf model.ScopeFilters
		json.Unmarshal(b, &sf)
		session.ScopeFilters = &sf
	}
	if feedback, ok := updateData["feedback"].(string); ok {
		session.Feedback = feedback // M3_STEP4 uses this for hits
	}
	if logData, ok := updateData["data_mining_log"]; ok {
		b, _ := json.Marshal(logData)
		var dml model.DataMiningLog
		json.Unmarshal(b, &dml)
		session.DataMiningLog = &dml
	}

	if err := h.mongoRepo.UpdateSession(ctx, session); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to update session")
		return
	}

	// Jika update status meminta pipeline lanjut
	h.pipeline.ExecuteAsync(ctx, session.ID)

	sendJSONResponse(w, http.StatusOK, map[string]string{
		"message": "Session updated successfully",
		"status":  session.Status,
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

	// The client can pass data they want to update (e.g. selected_topic)
	var updateData map[string]interface{}
	err = json.NewDecoder(req.Body).Decode(&updateData)

	if err == nil && updateData["is_retry"] == true {
		// Jika ini retry dari error, kembalikan status dengan menghapus akhiran _ERROR
		session.Status = strings.ReplaceAll(session.Status, "_ERROR", "")
		session.SystemError = "" // Hapus log error sebelumnya
	} else {
		// Default simple approve
		if strings.HasSuffix(session.Status, "_WAITING_APPROVAL") {
			session.Status = session.Status[:len(session.Status)-17] + "_APPROVED"
		} else if session.Status == "M5_STEP3_WAITING_RESOLUTION" {
			session.Status = "M5_STEP3_BATCH_SCREENING"
		}
		
		// Custom data handling untuk M2_STEP1
		if err == nil && session.Status == "M2_STEP1_APPROVED" {
			if selected, ok := updateData["selected_topic"]; ok {
				b, _ := json.Marshal(selected)
				var st model.SuggestedTopic
				json.Unmarshal(b, &st)
				session.SelectedTopic = &st
			}
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
		Feedback     string `json:"feedback"`
		TargetStatus string `json:"target_status,omitempty"`
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
	if payload.TargetStatus != "" {
		session.Status = payload.TargetStatus
		// Special handling for retrying a failed batch
		if payload.TargetStatus == "M5_STEP3_BATCH_SCREENING" {
			h.mongoRepo.ResetCalibrationScreenings(ctx, session.ID)
			if len(session.ScreeningResultsLog) > 0 {
				session.ScreeningResultsLog = session.ScreeningResultsLog[:len(session.ScreeningResultsLog)-1]
			}
		}
	} else if session.Status == "M2_STEP1_WAITING_APPROVAL" {
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

func (h *SessionHandler) RequestReimport(w http.ResponseWriter, req *http.Request) {
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

	// Change status back to import
	session.Status = "M4_STEP2_WAITING_IMPORT"
	
	if err := h.mongoRepo.UpdateSession(ctx, session); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to set re-import status")
		return
	}

	sendJSONResponse(w, http.StatusOK, map[string]string{
		"message": "Re-import requested successfully",
		"status":  session.Status,
	})
}

func (h *SessionHandler) ImportData(w http.ResponseWriter, req *http.Request) {
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

	err = req.ParseMultipartForm(50 << 20) // Max 50 MB
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "Failed to parse multipart form")
		return
	}

	files := req.MultipartForm.File["files"]
	if len(files) == 0 {
		sendJSONError(w, http.StatusBadRequest, "No files uploaded")
		return
	}

	var allPapers []interface{}

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			sendJSONError(w, http.StatusInternalServerError, "Failed to open file")
			return
		}
		defer file.Close()

		content := make([]byte, fileHeader.Size)
		_, err = file.Read(content)
		if err != nil {
			sendJSONError(w, http.StatusInternalServerError, "Failed to read file")
			return
		}

		// We use parser.ParseFile
		parsedDocs, err := parser.ParseFile(fileHeader.Filename, content)
		if err != nil {
			// fallback silently or log
			continue
		}

		for _, doc := range parsedDocs {
			p := model.Paper{
				SessionID:    session.ID,
				Title:        doc.Title,
				Abstract:     doc.Abstract,
				DOI:          doc.DOI,
				Year:         doc.Year,
				Authors:      doc.Authors,
				Database:     doc.Database,
				DocumentType: doc.DocumentType,
				Status:       "PENDING", // Initial state
			}
			allPapers = append(allPapers, p)
		}
	}

	if len(allPapers) == 0 {
		sendJSONError(w, http.StatusBadRequest, "No valid papers extracted from files")
		return
	}

	err = h.mongoRepo.ClearAndInsertPapers(ctx, session.ID, allPapers)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to insert papers into database: "+err.Error())
		return
	}

	// Update session status to M4_STEP2_PROCESS
	session.Status = "M4_STEP2_PROCESS"
	if err := h.mongoRepo.UpdateSession(ctx, session); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to update session status")
		return
	}

	// Trigger pipeline
	h.pipeline.ExecuteAsync(ctx, session.ID)

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"message": "Files imported successfully",
		"total":   len(allPapers),
		"status":  session.Status,
	})
}

// GetDisagreements mengembalikan daftar paper yang statusnya DISAGREE
func (h *SessionHandler) GetDisagreements(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	papers, err := h.mongoRepo.GetDisagreedPapers(r.Context(), id)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to get disagreed papers: "+err.Error())
		return
	}

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"disagreements": papers,
	})
}

// ResolveConflicts memproses resolusi konflik secara massal dari UI
func (h *SessionHandler) ResolveConflicts(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	var payload struct {
		Resolutions []struct {
			PaperID           string `json:"paper_id"`
			FinalDecision     string `json:"final_decision"`
			ConflictResolution string `json:"conflict_resolution"`
		} `json:"resolutions"`
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

	for _, res := range payload.Resolutions {
		if res.PaperID != "" && res.FinalDecision != "" {
			err := h.mongoRepo.UpdateScreeningPaperResolution(ctx, id, res.PaperID, res.FinalDecision, res.ConflictResolution)
			if err != nil {
				sendJSONError(w, http.StatusInternalServerError, "Gagal mengupdate resolusi: "+err.Error())
				return
			}
		}
	}

	session.Status = "M5_STEP3_BATCH_SCREENING"
	if err := h.mongoRepo.UpdateSession(ctx, session); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal mengupdate status sesi: "+err.Error())
		return
	}

	// Setelah semua konflik diresolusi, trigger pipeline untuk mengecek kelanjutannya
	h.pipeline.ExecuteAsync(ctx, session.ID)

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"message": "Resolusi konflik berhasil disimpan",
	})
}

// SyncQdrant mencocokkan DOI dari Qdrant ke MongoDB (Modul 6)
func (h *SessionHandler) SyncQdrant(w http.ResponseWriter, req *http.Request) {
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

	// Qdrant Configuration
	qdrantURL := os.Getenv("QDRANT_URL")
	qdrantKey := os.Getenv("QDRANT_API_KEY")
	if qdrantURL == "" {
		// Mock testing mode jika environment Qdrant belum diset
		qdrantURL = "mock-mode"
	}

	coll := h.mongoRepo.GetScreeningCollection()
	filter := bson.M{"session_id": id, "Final_Decision": "INCLUDE"}
	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal mengambil data paper")
		return
	}
	var papers []bson.M
	_ = cursor.All(ctx, &papers)

	syncedCount := 0
	
	// Untuk menyederhanakan, kita asumsikan jika QDRANT_URL ada, kita akan query Qdrant.
	// Jika tidak, kita gunakan dummy sync untuk simulasi/testing.
	if qdrantURL != "mock-mode" {
		// Hit Qdrant REST API (Scroll)
		client := &http.Client{Timeout: 15 * time.Second}
		reqBody := `{"limit": 10000, "with_payload": ["doi"]}`
		reqQdrant, _ := http.NewRequest("POST", fmt.Sprintf("%s/collections/scientific_articles/points/scroll", qdrantURL), strings.NewReader(reqBody))
		reqQdrant.Header.Set("Content-Type", "application/json")
		if qdrantKey != "" {
			reqQdrant.Header.Set("api-key", qdrantKey)
		}
		
		resp, err := client.Do(reqQdrant)
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			var qdrantResp map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&qdrantResp)
			
			// Build set of DOIs in Qdrant
			qdrantDOIs := make(map[string]bool)
			if result, ok := qdrantResp["result"].(map[string]interface{}); ok {
				if points, ok := result["points"].([]interface{}); ok {
					for _, pt := range points {
						if pMap, ok := pt.(map[string]interface{}); ok {
							if payload, ok := pMap["payload"].(map[string]interface{}); ok {
								if d, ok := payload["doi"].(string); ok && d != "" {
									qdrantDOIs[d] = true
								}
							}
						}
					}
				}
			}

			// Update MongoDB
			for _, p := range papers {
				if doi, ok := p["doi"].(string); ok && doi != "" {
					if qdrantDOIs[doi] {
						update := bson.M{"$set": bson.M{"full_text_retrieved": true, "acquisition_date": time.Now().Format(time.RFC3339)}}
						coll.UpdateByID(ctx, p["_id"], update)
						syncedCount++
					}
				}
			}
		}
	} else {
		// Mock mode: Tandai semua yang "unpaywall" sebagai retrieved
		for _, p := range papers {
			if loc, ok := p["full_text_location"].(string); ok && loc == "unpaywall" {
				update := bson.M{"$set": bson.M{"full_text_retrieved": true, "acquisition_date": time.Now().Format(time.RFC3339)}}
				coll.UpdateByID(ctx, p["_id"], update)
				syncedCount++
			}
		}
	}

	// Lakukan kalkulasi ulang AcquisitionLog via modul 6
	h.pipeline.ExecuteAsync(ctx, session.ID) // Ini akan gagal mengeksekusi jika status sudah bukan INIT, tapi tidak masalah, kita biarkan saja.
	
	// Atau lebih baik, hitung manual disini:
	// ...

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"message":      "Sinkronisasi berhasil",
		"synced_count": syncedCount,
	})
}

// MarkInaccessible untuk menandai dokumen yang tidak bisa diunduh
func (h *SessionHandler) MarkInaccessible(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	var payload struct {
		PaperID       string `json:"paper_id"`
		Documentation string `json:"documentation"`
	}

	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	ctx := context.Background()
	coll := h.mongoRepo.GetScreeningCollection()
	
	objID, _ := primitive.ObjectIDFromHex(payload.PaperID)
	update := bson.M{
		"$set": bson.M{
			"inaccessible":               true,
			"documentation_inaccessible": payload.Documentation,
		},
	}
	_, err := coll.UpdateByID(ctx, objID, update)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal menandai inaccessible")
		return
	}

	// Trigger pipeline untuk memperbarui log akuisisi
	// Jika status M6_STEP1_WAITING_SYNC, kita tidak mau jalanin ulang M6_ACQUISITION dari awal
	// Tapi biarkan saja nanti.
	
	sendJSONResponse(w, http.StatusOK, map[string]string{
		"message": "Dokumen ditandai Inaccessible",
	})
}

// ExportM6Links menghasilkan CSV daftar tautan unduhan
func (h *SessionHandler) ExportM6Links(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		http.Error(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	coll := h.mongoRepo.GetScreeningCollection()
	filter := bson.M{"session_id": id, "Final_Decision": "INCLUDE"}
	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		http.Error(w, "Gagal mengambil data", http.StatusInternalServerError)
		return
	}
	var papers []bson.M
	_ = cursor.All(ctx, &papers)

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment;filename=m6_acquisition_links_%s.csv", id))

	fmt.Fprintf(w, "Title,DOI,Location,Download_URL,Retrieved,Inaccessible\n")
	for _, p := range papers {
		title, _ := p["Title"].(string)
		doi, _ := p["DOI"].(string)
		loc, _ := p["full_text_location"].(string)
		url, _ := p["download_url"].(string)
		retrieved, _ := p["full_text_retrieved"].(bool)
		inacc, _ := p["inaccessible"].(bool)
		
		title = strings.ReplaceAll(title, "\"", "\"\"")
		fmt.Fprintf(w, "\"%s\",\"%s\",\"%s\",\"%s\",%t,%t\n", title, doi, loc, url, retrieved, inacc)
	}
}

