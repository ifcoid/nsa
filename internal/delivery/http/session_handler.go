package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
		} else if session.Status == "M6_STEP1_WAITING_SYNC" {
			// Setelah sinkronisasi Qdrant, lanjut ke full-text screening (Modul 6 L2)
			session.Status = "M6_STEP2_FULLTEXT_SCREENING"
		} else if session.Status == "M6_STEP2_WAITING_RESOLUTION" {
			// Lanjut batch full-text berikutnya / evaluasi
			session.Status = "M6_STEP2_FULLTEXT_SCREENING"
		} else if session.Status == "M6_STEP2_WAITING_EMBED" {
			// User sudah memasukkan endpoint embedding baru (via /api/embed-config) -> lanjut.
			session.Status = "M6_STEP2_FULLTEXT_SCREENING"
			session.EmbedError = ""
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
				Journal:      doc.Journal,
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

	var papers []map[string]interface{}
	var err error
	if r.URL.Query().Get("stage") == "fulltext" {
		papers, err = h.mongoRepo.GetDisagreedFullTextPapers(r.Context(), id)
	} else {
		papers, err = h.mongoRepo.GetDisagreedPapers(r.Context(), id)
	}
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
		Stage       string `json:"stage,omitempty"` // "" (abstrak/M5) atau "fulltext" (M6)
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

	isFulltext := payload.Stage == "fulltext"
	for _, res := range payload.Resolutions {
		if res.PaperID != "" && res.FinalDecision != "" {
			if isFulltext {
				err = h.mongoRepo.UpdateScreeningPaperResolutionFull(ctx, id, res.PaperID, res.FinalDecision, res.ConflictResolution)
			} else {
				err = h.mongoRepo.UpdateScreeningPaperResolution(ctx, id, res.PaperID, res.FinalDecision, res.ConflictResolution)
			}
			if err != nil {
				sendJSONError(w, http.StatusInternalServerError, "Gagal mengupdate resolusi: "+err.Error())
				return
			}
		}
	}

	if isFulltext {
		session.Status = "M6_STEP2_FULLTEXT_SCREENING"
	} else {
		session.Status = "M5_STEP3_BATCH_SCREENING"
	}
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

// SubmitVOSviewer menerima hasil VOSviewer yang di-paste user (Modul 8b L2 -> L3).
func (h *SessionHandler) SubmitVOSviewer(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}
	var payload struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if strings.TrimSpace(payload.Data) == "" {
		sendJSONError(w, http.StatusBadRequest, "Data VOSviewer kosong")
		return
	}
	ctx := context.Background()
	session, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}
	session.BibliometricInput = payload.Data
	session.Status = "M8B_STEP3_INTERPRET"
	if err := h.mongoRepo.UpdateSession(ctx, session); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal menyimpan input VOSviewer: "+err.Error())
		return
	}
	h.pipeline.ExecuteAsync(ctx, session.ID)
	sendJSONResponse(w, http.StatusOK, map[string]string{"message": "Input VOSviewer diterima, interpretasi cluster dimulai"})
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
	if qdrantURL == "" {
		qdrantURL = os.Getenv("QDRANT_ENDPOINT")
	}
	qdrantKey := os.Getenv("QDRANT_API_KEY")
	if qdrantURL == "" {
		// Mock testing mode jika environment Qdrant belum diset
		qdrantURL = "mock-mode"
	}

	coll := h.mongoRepo.GetScreeningCollection()
	filter := bson.M{
		"session_id": id,
		"$or": []bson.M{
			{"Final_Decision": "INCLUDE"},
			{"Final_Decision": "", "Screener_1_Decision": "INCLUDE"},
		},
	}
	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal mengambil data paper")
		return
	}
	var papers []bson.M
	_ = cursor.All(ctx, &papers)

	syncedCount := 0
	type QdrantPaper struct {
		DOI   string
		Title string
	}
	var qdrantPapers []QdrantPaper
	qdrantDOIs := make(map[string]bool)
	
	if qdrantURL != "mock-mode" {
		client := &http.Client{Timeout: 30 * time.Second}

		var nextOffset string
		for {
			reqBody := `{"limit": 5000, "with_payload": ["doi", "title"]}`
			if nextOffset != "" {
				reqBody = fmt.Sprintf(`{"limit": 5000, "with_payload": ["doi", "title"], "offset": "%s"}`, nextOffset)
			}
			
			reqQdrant, err := http.NewRequest("POST", fmt.Sprintf("%s/collections/scientific_articles/points/scroll", qdrantURL), strings.NewReader(reqBody))
			if err != nil {
				sendJSONError(w, http.StatusInternalServerError, "Gagal membuat request ke Qdrant: " + err.Error())
				return
			}
			reqQdrant.Header.Set("Content-Type", "application/json")
			if qdrantKey != "" {
				reqQdrant.Header.Set("api-key", qdrantKey)
			}
			
			resp, err := client.Do(reqQdrant)
			if err != nil {
				sendJSONError(w, http.StatusInternalServerError, "Gagal terhubung ke Qdrant: " + err.Error())
				return
			}

			if resp.StatusCode != 200 {
				bodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				errMsg := fmt.Sprintf("Qdrant mengembalikan status %d: %s", resp.StatusCode, string(bodyBytes))
				sendJSONError(w, http.StatusInternalServerError, errMsg)
				return
			}

			var qdrantResp map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&qdrantResp)
			resp.Body.Close()
				
			if result, ok := qdrantResp["result"].(map[string]interface{}); ok {
				if points, ok := result["points"].([]interface{}); ok {
					for _, pt := range points {
						if pMap, ok := pt.(map[string]interface{}); ok {
							if payload, ok := pMap["payload"].(map[string]interface{}); ok {
								var qTitle string
								if t, ok := payload["title"].(string); ok {
									qTitle = t
								}
								
								var qDOI string
								if d, ok := payload["doi"].(string); ok && d != "" {
									d = strings.TrimPrefix(d, "https://doi.org/")
									d = strings.TrimPrefix(d, "http://doi.org/")
									d = strings.ToLower(strings.TrimSpace(d))
									d = strings.ReplaceAll(d, "\ufb00", "ff")
									d = strings.ReplaceAll(d, "\ufb01", "fi")
									d = strings.ReplaceAll(d, "\ufb02", "fl")
									d = strings.ReplaceAll(d, "\ufb03", "ffi")
									d = strings.ReplaceAll(d, "\ufb04", "ffl")
									d = strings.ReplaceAll(d, "\ufb05", "ft")
									d = strings.ReplaceAll(d, "\ufb06", "st")
									qdrantDOIs[d] = true
									qDOI = d
								}
								
								// Always add to qdrantPapers so title similarity can work even if DOI is empty
								qdrantPapers = append(qdrantPapers, QdrantPaper{DOI: qDOI, Title: qTitle})
							}
						}
					}
				}
				
				if offsetVal, hasOffset := result["next_page_offset"]; hasOffset && offsetVal != nil {
					nextOffset = offsetVal.(string)
				} else {
					break // Selesai semua page
				}
			} else {
				break
			}
		}

		// Update MongoDB
		for _, p := range papers {
			var doi string
			var title string
			
			if val, ok := p["doi"].(string); ok && val != "" {
				doi = val
			} else if val, ok := p["DOI"].(string); ok && val != "" {
				doi = val
			}
			
			if val, ok := p["title"].(string); ok && val != "" {
				title = val
			} else if val, ok := p["Title"].(string); ok && val != "" {
				title = val
			}

			matched := false
			newDOI := ""

			if doi != "" {
				doi = strings.TrimPrefix(doi, "https://doi.org/")
				doi = strings.TrimPrefix(doi, "http://doi.org/")
				doi = strings.ToLower(strings.TrimSpace(doi))
				doi = strings.ReplaceAll(doi, "\ufb00", "ff")
				doi = strings.ReplaceAll(doi, "\ufb01", "fi")
				doi = strings.ReplaceAll(doi, "\ufb02", "fl")
				doi = strings.ReplaceAll(doi, "\ufb03", "ffi")
				doi = strings.ReplaceAll(doi, "\ufb04", "ffl")
				doi = strings.ReplaceAll(doi, "\ufb05", "ft")
				doi = strings.ReplaceAll(doi, "\ufb06", "st")
				
				if qdrantDOIs[doi] {
					matched = true
				}
			}
			
			// Fallback: Match by title similarity if DOI didn't match
			if !matched && title != "" {
				for _, qp := range qdrantPapers {
					if qp.Title != "" {
						sim := similarityRatio(title, qp.Title)
						if sim > 0.8 {
							matched = true
							newDOI = qp.DOI
							break
						}
					}
				}
			}

			if matched {
				updateFields := bson.M{
					"full_text_retrieved": true, 
					"acquisition_date": time.Now().Format(time.RFC3339),
				}
				if newDOI != "" {
					updateFields["doi"] = newDOI
				}
				// Also update uppercase Full_Text_Retrieved just in case UI reads it
				updateFields["Full_Text_Retrieved"] = true

				update := bson.M{"$set": updateFields}
				coll.UpdateByID(ctx, p["_id"], update)
				syncedCount++
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
	// Lakukan kalkulasi ulang AcquisitionLog secara sinkron agar UI langsung ter-update
	h.recalculateAcquisitionLogSync(ctx, session)
	
	// Collect debug info
	qDOIs := []string{}
	for k := range qdrantDOIs {
		if len(qDOIs) < 5 {
			qDOIs = append(qDOIs, k)
		}
	}
	mDOIs := []string{}
	for _, p := range papers {
		var doi string
		if val, ok := p["doi"].(string); ok && val != "" {
			doi = val
		} else if val, ok := p["DOI"].(string); ok && val != "" {
			doi = val
		}
		if doi != "" {
			doi = strings.TrimPrefix(doi, "https://doi.org/")
			doi = strings.TrimPrefix(doi, "http://doi.org/")
			if len(mDOIs) < 5 {
				mDOIs = append(mDOIs, doi)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":      "success",
		"synced_count": syncedCount,
		"debug_qdrant_unique": len(qdrantDOIs),
		"debug_mongo_papers": len(papers),
		"debug_qdrant_sample": qDOIs,
		"debug_mongo_sample": mDOIs,
		"version": "v4",
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

	// Trigger kalkulasi sinkron
	session, errSession := h.mongoRepo.GetSession(ctx, id)
	if errSession == nil {
		h.recalculateAcquisitionLogSync(ctx, session)
	}
	
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
	filter := bson.M{
		"session_id": id,
		"$or": []bson.M{
			{"Final_Decision": "INCLUDE"},
			{"Final_Decision": "", "Screener_1_Decision": "INCLUDE"},
		},
	}
	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		http.Error(w, "Gagal mengambil data", http.StatusInternalServerError)
		return
	}
	var papers []bson.M
	_ = cursor.All(ctx, &papers)

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment;filename=m6_acquisition_links_%s.csv", id))

	fmt.Fprintf(w, "Title,Authors,DOI,Publisher,Journal,Article_Type,Location,Download_URL,Retrieved,Inaccessible\n")
	for _, p := range papers {
		title, _ := p["title"].(string)
		if title == "" { title, _ = p["Title"].(string) }
		
		authors, _ := p["authors"].(string)
		if authors == "" { authors, _ = p["Authors"].(string) }

		doi, _ := p["doi"].(string)
		if doi == "" { doi, _ = p["DOI"].(string) }
		
		journal, _ := p["journal"].(string)
		if journal == "" { journal, _ = p["Journal"].(string) }
		
		articleType, _ := p["document_type"].(string)
		if articleType == "" { articleType, _ = p["Article_Type"].(string) }
		
		loc, _ := p["full_text_location"].(string)
		url, _ := p["download_url"].(string)
		retrieved, _ := p["full_text_retrieved"].(bool)
		inacc, _ := p["inaccessible"].(bool)
		
		publisher := getPublisherFromDOI(doi)
		
		title = strings.ReplaceAll(title, "\"", "\"\"")
		authors = strings.ReplaceAll(authors, "\"", "\"\"")
		journal = strings.ReplaceAll(journal, "\"", "\"\"")
		articleType = strings.ReplaceAll(articleType, "\"", "\"\"")
		
		if doi != "" && !strings.HasPrefix(doi, "http") {
			doi = "https://doi.org/" + doi
		}
		fmt.Fprintf(w, "\"%s\",\"%s\",\"%s\",\"%s\",\"%s\",\"%s\",\"%s\",\"%s\",%t,%t\n", title, authors, doi, publisher, journal, articleType, loc, url, retrieved, inacc)
	}
}

// getPublisherFromDOI attempts to infer the publisher from the DOI prefix
func getPublisherFromDOI(doi string) string {
	if doi == "" {
		return "Unknown"
	}
	// Extract prefix if it's a full URL
	prefix := doi
	if strings.Contains(doi, "10.") {
		parts := strings.SplitN(doi, "10.", 2)
		if len(parts) == 2 {
			prefix = "10." + parts[1]
		}
	}
	
	if strings.HasPrefix(prefix, "10.1109") {
		return "IEEE"
	} else if strings.HasPrefix(prefix, "10.1016") {
		return "Elsevier"
	} else if strings.HasPrefix(prefix, "10.1007") {
		return "Springer"
	} else if strings.HasPrefix(prefix, "10.1145") {
		return "ACM"
	} else if strings.HasPrefix(prefix, "10.1049") {
		return "IET"
	} else if strings.HasPrefix(prefix, "10.1038") {
		return "Nature"
	} else if strings.HasPrefix(prefix, "10.3389") {
		return "Frontiers"
	} else if strings.HasPrefix(prefix, "10.3390") {
		return "MDPI"
	} else if strings.HasPrefix(prefix, "10.1101") {
		return "bioRxiv/medRxiv"
	} else if strings.HasPrefix(prefix, "10.48550") || strings.Contains(strings.ToLower(doi), "arxiv") {
		return "arXiv"
	}
	
	return "Other"
}

// GetM6Papers mengembalikan data paper Modul 6 dalam format JSON untuk Web Viewer
func (h *SessionHandler) GetM6Papers(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	ctx := context.Background()
	coll := h.mongoRepo.GetScreeningCollection()
	filter := bson.M{
		"session_id": id,
		"$or": []bson.M{
			{"Final_Decision": "INCLUDE"},
			{"Final_Decision": "", "Screener_1_Decision": "INCLUDE"},
		},
	}
	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal mengambil data")
		return
	}
	var papers []bson.M
	_ = cursor.All(ctx, &papers)

	var result []map[string]interface{}
	for _, p := range papers {
		title, _ := p["Title"].(string)
		doi, _ := p["DOI"].(string)
		journal, _ := p["Journal"].(string)
		articleType, _ := p["Article_Type"].(string)
		loc, _ := p["full_text_location"].(string)
		url, _ := p["download_url"].(string)
		retrieved, _ := p["full_text_retrieved"].(bool)
		inacc, _ := p["inaccessible"].(bool)
		
		if doi != "" && !strings.HasPrefix(doi, "http") {
			doi = "https://doi.org/" + doi
		}
		
		publisher := getPublisherFromDOI(doi)
		if loc == "arxiv" {
			publisher = "arXiv"
		}
		
		// Map `_id` to string safely
		paperID := ""
		if oid, ok := p["_id"].(primitive.ObjectID); ok {
			paperID = oid.Hex()
		}

		result = append(result, map[string]interface{}{
			"id": paperID,
			"title": title,
			"doi": doi,
			"journal": journal,
			"article_type": articleType,
			"location": loc,
			"download_url": url,
			"retrieved": retrieved,
			"inaccessible": inacc,
			"publisher": publisher,
		})
	}

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"papers": result,
		"total": len(result),
	})
}

func (h *SessionHandler) recalculateAcquisitionLogSync(ctx context.Context, session *model.SLRSession) {
	coll := h.mongoRepo.GetScreeningCollection()
	filter := bson.M{
		"session_id": session.ID,
		"$or": []bson.M{
			{"Final_Decision": "INCLUDE"},
			{"Final_Decision": "", "Screener_1_Decision": "INCLUDE"},
		},
	}
	cursor, _ := coll.Find(ctx, filter)
	var finalPapers []bson.M
	_ = cursor.All(ctx, &finalPapers)

	var log model.AcquisitionLog
	log.TotalInclude = len(finalPapers)

	for _, p := range finalPapers {
		loc, _ := p["full_text_location"].(string)
		if loc == "unpaywall" || loc == "arxiv" {
			log.HighRetrieved++
		} else if loc == "hitl download" {
			log.MediumRetrieved++
		}

		retrieved, _ := p["full_text_retrieved"].(bool)
		if retrieved {
			log.VectorizedCount++
		}

		inaccessible, _ := p["inaccessible"].(bool)
		if inaccessible {
			log.InaccessibleCount++
		}
	}
	if log.TotalInclude > 0 {
		log.InaccessiblePct = float64(log.InaccessibleCount) / float64(log.TotalInclude) * 100
	}
	session.AcquisitionLog = &log
	_ = h.mongoRepo.UpdateSession(ctx, session)
}

func levenshtein(s1, s2 string) int {
	lenS1 := len(s1)
	lenS2 := len(s2)
	matrix := make([][]int, lenS1+1)
	for i := range matrix {
		matrix[i] = make([]int, lenS2+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}
	for i := 1; i <= lenS1; i++ {
		for j := 1; j <= lenS2; j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			min1 := matrix[i-1][j] + 1
			min2 := matrix[i][j-1] + 1
			min3 := matrix[i-1][j-1] + cost
			
			min := min1
			if min2 < min { min = min2 }
			if min3 < min { min = min3 }
			matrix[i][j] = min
		}
	}
	return matrix[lenS1][lenS2]
}

func similarityRatio(s1, s2 string) float64 {
	// Normalize strings for comparison
	clean1 := strings.ToLower(strings.TrimSpace(s1))
	clean2 := strings.ToLower(strings.TrimSpace(s2))
	
	dist := levenshtein(clean1, clean2)
	maxLen := len(clean1)
	if len(clean2) > maxLen { maxLen = len(clean2) }
	if maxLen == 0 { return 1.0 }
	return 1.0 - float64(dist)/float64(maxLen)
}
