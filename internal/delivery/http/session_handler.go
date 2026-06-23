package http

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"nsa/internal/agent"
	"nsa/internal/logger"
	"nsa/internal/model"
	"nsa/internal/modules"
	"nsa/internal/orchestrator"
	"nsa/internal/parser"
	"nsa/internal/repository"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SessionHandler struct {
	mongoRepo  *repository.MongoRepository
	pipeline   *orchestrator.SLRPipeline
	recodeJobs map[string]*recodeJob
	recodeMu   sync.Mutex
}

// recodeJob = status job saran re-code (AI) yang berjalan di background, agar progres
// per-paper tampil di Live Log dan hasil bisa di-poll frontend tanpa memblok HTTP.
type recodeJob struct {
	Done        bool                     `json:"done"`
	Model       string                   `json:"model"`
	Total       int                      `json:"total"`
	Progress    int                      `json:"progress"`
	Suggestions []map[string]interface{} `json:"suggestions"`
	Error       string                   `json:"error"`
}

func NewSessionHandler(mongo *repository.MongoRepository, pipeline *orchestrator.SLRPipeline) *SessionHandler {
	return &SessionHandler{
		mongoRepo:  mongo,
		pipeline:   pipeline,
		recodeJobs: make(map[string]*recodeJob),
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
		} else if strings.HasSuffix(session.Status, "_WAITING_EMBED") {
			// M9: server embedding/pencarian sudah dinyalakan lagi -> resume ke group tertunda.
			session.Status = strings.TrimSuffix(session.Status, "_WAITING_EMBED")
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

// moduleNum extracts the leading module number from a status like "M9_GROUPB..." -> 9.
// Returns -1 when the status has no recognizable "M<n>_" prefix.
func moduleNum(status string) int {
	if !strings.HasPrefix(status, "M") {
		return -1
	}
	i := 1
	for i < len(status) && status[i] >= '0' && status[i] <= '9' {
		i++
	}
	if i == 1 {
		return -1
	}
	n := 0
	for _, c := range status[1:i] {
		n = n*10 + int(c-'0')
	}
	return n
}

// isBackwardToM5 reports whether we are jumping from a module after M5 (M6-M9) back
// into M5. M5B/M8B-style suffixes resolve by their leading number, so "M8B_" -> 8.
func isBackwardToM5(current, target string) bool {
	return moduleNum(target) == 5 && moduleNum(current) > 5
}

// invalidateDownstreamForRescreen marks the M6-M9 artifacts as stale when the user goes
// back to re-screen. It clears the regenerable final manuscript (cheap; it is rebuilt by
// M9 on the forward re-run) and raises RescreenPending so the UI and downstream modules
// know the prior results no longer reflect the included-study set. Per-paper extraction
// is preserved (expensive) and re-filtered by current decisions when M6-M9 re-run.
func invalidateDownstreamForRescreen(session *model.SLRSession) {
	session.RescreenPending = true
	session.Manuscript = nil
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

	// Hentikan paksa background worker yang mungkin sedang berjalan (mitigasi race condition)
	h.pipeline.StopWorker(id)

	session.Feedback = payload.Feedback

	// Determine NEEDS_REVISION status
	backToM5 := false
	if payload.TargetStatus != "" {
		// Backward jump to Module 5 from a later module (M6-M9) invalidates everything
		// downstream of screening: the included-study set may change, so any acquisition,
		// extraction, synthesis, and manuscript built on the old set is stale.
		if isBackwardToM5(session.Status, payload.TargetStatus) {
			invalidateDownstreamForRescreen(session)
			backToM5 = true
		}
		session.Status = payload.TargetStatus
		// Special handling for retrying a failed batch
		if payload.TargetStatus == "M5_STEP3_BATCH_SCREENING" {
			h.mongoRepo.ResetCalibrationScreenings(ctx, session.ID)
			if len(session.ScreeningResultsLog) > 0 {
				session.ScreeningResultsLog = session.ScreeningResultsLog[:len(session.ScreeningResultsLog)-1]
			}
		} else if payload.TargetStatus == "M7_STEP3_QA" {
			h.mongoRepo.ResetQAErrors(ctx, session.ID)
		}
	} else if session.Status == "M2_STEP1_WAITING_APPROVAL" {
		session.Status = "M2_STEP1_NEEDS_REVISION"
	} else {
		// Safely extract the module and step prefix (e.g., M7_STEP3) and append NEEDS_REVISION
		parts := strings.Split(session.Status, "_")
		if len(parts) >= 2 && strings.HasPrefix(parts[1], "STEP") {
			session.Status = fmt.Sprintf("%s_%s_NEEDS_REVISION", parts[0], parts[1])
		} else if len(parts) >= 1 && strings.HasPrefix(parts[0], "M") {
			session.Status = fmt.Sprintf("%s_NEEDS_REVISION", parts[0])
		} else {
			// Fallback if status format is unexpected
			session.Status = session.Status + "_NEEDS_REVISION"
		}
	}

	// Going back to M5 must clear the stale manuscript; UpdateSession cannot ($set drops
	// the omitempty nil pointer), so $unset it atomically via SaveSessionUnsetting.
	var saveErr error
	if backToM5 {
		saveErr = h.mongoRepo.SaveSessionUnsetting(ctx, session, "manuscript")
	} else {
		saveErr = h.mongoRepo.UpdateSession(ctx, session)
	}
	if saveErr != nil {
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

	// Breakdown tracking
	type fileBreakdown struct {
		Filename string `json:"filename"`
		Count    int    `json:"count"`
		Database string `json:"database"`
		MissingAbstract int `json:"missing_abstract"`
		MissingDOI      int `json:"missing_doi"`
	}
	var perFileBreakdown []fileBreakdown
	perDatabase := make(map[string]int)
	totalMissingAbstract := 0
	totalMissingDOI := 0

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			sendJSONError(w, http.StatusInternalServerError, "Failed to open file")
			return
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			sendJSONError(w, http.StatusInternalServerError, "Failed to read file")
			return
		}

		// Strip UTF-8 BOM if present
		content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))

		// We use parser.ParseFile
		parsedDocs, err := parser.ParseFile(fileHeader.Filename, content)
		if err != nil {
			// JANGAN diam-diam: file yang gagal parse harus terlihat user (xAI/anti silent-loss).
			logger.Logf(id, "[Import] ⚠️ File '%s' GAGAL di-parse: %v — 0 record dari file ini.", fileHeader.Filename, err)
			continue
		}
		if len(parsedDocs) == 0 {
			// File terbaca tapi tak menghasilkan record (mis. format tak dikenali / kolom judul
			// tak ter-map). Surface eksplisit supaya tidak terasa "Total Records ga sesuai".
			logger.Logf(id, "[Import] ⚠️ File '%s' menghasilkan 0 record (cek format/kolom judul).", fileHeader.Filename)
		}

		fileCount := 0
		fileMissingAbstract := 0
		fileMissingDOI := 0
		dbCount := make(map[string]int)

		for _, doc := range parsedDocs {
			p := model.Paper{
				SessionID:      session.ID,
				Title:          doc.Title,
				Abstract:       doc.Abstract,
				DOI:            doc.DOI,
				Year:           doc.Year,
				Authors:        doc.Authors,
				Database:       doc.Database,
				Journal:        doc.Journal,
				DocumentType:   doc.DocumentType,
				Keywords:       doc.Keywords,
				IndexKeywords:  doc.IndexKeywords,
				Affiliations:   doc.Affiliations,
				Volume:         doc.Volume,
				Issue:          doc.Issue,
				PageStart:      doc.PageStart,
				PageEnd:        doc.PageEnd,
				ISSN:           doc.ISSN,
				ISBN:           doc.ISBN,
				Publisher:      doc.Publisher,
				Language:       doc.Language,
				FundingDetails: doc.FundingDetails,
				CitedBy:        doc.CitedBy,
				ConferenceName: doc.ConferenceName,
				EID:            doc.EID,
				PubMedID:       doc.PubMedID,
				References:     doc.References,
				Status:         "PENDING", // Initial state
			}
			allPapers = append(allPapers, p)
			fileCount++
			dbCount[doc.Database]++

			if strings.TrimSpace(doc.Abstract) == "" {
				fileMissingAbstract++
			}
			if strings.TrimSpace(doc.DOI) == "" {
				fileMissingDOI++
			}
		}

		// Determine most common database for this file
		fileDatabase := ""
		maxCount := 0
		for db, cnt := range dbCount {
			if cnt > maxCount {
				maxCount = cnt
				fileDatabase = db
			}
		}

		// Aggregate per-database totals
		for db, cnt := range dbCount {
			perDatabase[db] += cnt
		}

		totalMissingAbstract += fileMissingAbstract
		totalMissingDOI += fileMissingDOI

		perFileBreakdown = append(perFileBreakdown, fileBreakdown{
			Filename:        fileHeader.Filename,
			Count:           fileCount,
			Database:        fileDatabase,
			MissingAbstract: fileMissingAbstract,
			MissingDOI:      fileMissingDOI,
		})

		// Log per-file details
		logger.Logf(id, "[Import] File '%s': %d papers (Database: %s, Missing abstract: %d, Missing DOI: %d)",
			fileHeader.Filename, fileCount, fileDatabase, fileMissingAbstract, fileMissingDOI)
	}

	if len(allPapers) == 0 {
		sendJSONError(w, http.StatusBadRequest, "No valid papers extracted from files")
		return
	}

	// NOTE: Do NOT dedup here. Deduplication is the pipeline's job (Langkah 4.2,
	// m4_mining.go) which is the single source of truth for the PRISMA audit:
	// it reports records-identified-per-database, duplicates removed, and unique
	// survivors. Pre-deduping at import silently strips records before the audit
	// sees them, so the audit then shows post-import survivors as the identified
	// counts and "0 duplicates removed" — corrupting the PRISMA flow. Insert all
	// raw records (with their source-DB attribution) and let the pipeline dedup.
	totalImported := len(allPapers)
	logger.Logf(id, "[Import] Total: %d papers imported dari %d file (dedup dijalankan di Langkah 4.2)", totalImported, len(perFileBreakdown))

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
		"total":   totalImported,
		"status":  session.Status,
		"breakdown": map[string]interface{}{
			"per_file":         perFileBreakdown,
			"per_database":     perDatabase,
			"missing_abstract": totalMissingAbstract,
			"missing_doi":      totalMissingDOI,
		},
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
		// Superset of disagreements: also surfaces agreed-UNCERTAIN records so every
		// non-terminal paper can be resolved before M5 closes (PRISMA completeness).
		papers, err = h.mongoRepo.GetUnresolvedScreeningPapers(r.Context(), id)
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
		"message": "Resolusi konflik tersimpan",
		"status":  session.Status,
	})
}

// ResolvePICOAudit applies the user's decision on each PICO-audit slipped-through paper
// (EXCLUDE = accept the audit, KEEP = override with justification), then recomputes the
// Module 5 summary so the PRISMA numbers reflect the corrected inclusion set.
func (h *SessionHandler) ResolvePICOAudit(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}
	var payload struct {
		Resolutions []struct {
			PaperID  string `json:"paper_id"`
			Decision string `json:"decision"` // "EXCLUDE" | "KEEP"
			Note     string `json:"note"`
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
	if session.PICOAuditLog == nil {
		sendJSONError(w, http.StatusBadRequest, "Tidak ada PICO audit untuk sesi ini")
		return
	}

	// reason code per flagged paper, for correct exclusion-table attribution.
	reasonByID := map[string]string{}
	for _, s := range session.PICOAuditLog.Slipped {
		reasonByID[s.PaperID] = s.ReasonCode
	}

	resMap := map[string]string{}
	for _, r := range payload.Resolutions {
		if r.PaperID == "" || (r.Decision != "EXCLUDE" && r.Decision != "KEEP") {
			continue
		}
		resMap[r.PaperID] = r.Decision
		if r.Decision == "EXCLUDE" {
			note := strings.TrimSpace(r.Note)
			if note == "" {
				note = "PICO audit override: EXCLUDE"
			}
			rc := reasonByID[r.PaperID]
			if rc == "" {
				rc = "OTHER"
			}
			if e := h.mongoRepo.ExcludePaperWithReason(ctx, id, r.PaperID, rc, "[PICO-AUDIT] "+note); e != nil {
				sendJSONError(w, http.StatusInternalServerError, "Gagal mengupdate keputusan: "+e.Error())
				return
			}
		}
	}

	for i := range session.PICOAuditLog.Slipped {
		s := &session.PICOAuditLog.Slipped[i]
		if dec, ok := resMap[s.PaperID]; ok {
			s.Actioned = true
			s.Resolution = dec
		}
	}

	// Recompute the Module 5 summary with the corrected inclusion set (audit is reused,
	// not re-run, because PICOAuditLog already exists).
	session.Status = "M5_STEP4_REVIEW_HASIL"
	if err := h.mongoRepo.UpdateSession(ctx, session); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal mengupdate sesi: "+err.Error())
		return
	}
	h.pipeline.ExecuteAsync(ctx, session.ID)
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"message": "Koreksi PICO audit tersimpan, ringkasan Modul 5 dihitung ulang",
		"status":  session.Status,
	})
}

// RerunPICOAudit forces a fresh full-coverage PICO-consistency audit over the current
// INCLUDE set by clearing the stored audit, so M5_STEP4_REVIEW_HASIL re-runs it. Use
// after corrections to re-verify the cleaned inclusion set.
func (h *SessionHandler) RerunPICOAudit(w http.ResponseWriter, req *http.Request) {
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
	if !strings.HasPrefix(session.Status, "M5_STEP4") {
		sendJSONError(w, http.StatusBadRequest, "Audit ulang hanya tersedia pada tahap akhir Modul 5 (M5_STEP4)")
		return
	}
	session.PICOAuditLog = nil // force a fresh full-coverage audit on recompute
	session.Status = "M5_STEP4_REVIEW_HASIL"
	// $unset pico_audit_log explicitly: UpdateSession cannot clear an omitempty nil
	// pointer (it is dropped from $set), so the rerun would silently reuse the old audit.
	if err := h.mongoRepo.SaveSessionUnsetting(ctx, session, "pico_audit_log"); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal mereset audit PICO: "+err.Error())
		return
	}
	h.pipeline.ExecuteAsync(ctx, session.ID)
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"message": "Audit ulang PICO dimulai (cakupan penuh atas semua paper INCLUDE)",
		"status":  session.Status,
	})
}

// SaveAuditScopeRules stores the researcher's PICO scope clarifications (HITL) on the
// session and forces a fresh full-coverage audit so every INCLUDE is re-judged uniformly
// against the updated rules. This is the generalizable, multi-tenant mechanism: each
// session defines its own boundary rulings instead of hardcoding review-specific ones.
func (h *SessionHandler) SaveAuditScopeRules(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}
	var payload struct {
		Rules string `json:"rules"`
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
	if !strings.HasPrefix(session.Status, "M5_STEP4") {
		sendJSONError(w, http.StatusBadRequest, "Revisi scope hanya tersedia pada tahap akhir Modul 5 (M5_STEP4)")
		return
	}
	session.AuditScopeRules = strings.TrimSpace(payload.Rules)
	session.PICOAuditLog = nil // force a fresh audit under the new rules
	session.Status = "M5_STEP4_REVIEW_HASIL"
	// Atomic save + $unset of the stale audit (omitempty would otherwise drop the nil).
	if err := h.mongoRepo.SaveSessionUnsetting(ctx, session, "pico_audit_log"); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal menyimpan scope rules: "+err.Error())
		return
	}
	h.pipeline.ExecuteAsync(ctx, session.ID)
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"message": "Aturan scope PICO tersimpan; audit ulang konsisten dijalankan",
		"status":  session.Status,
	})
}

// normalizeFrameworkColumns memvalidasi & merapikan kolom framework hasil edit manusia:
// trim spasi, buang baris berkey kosong, tolak duplikat key (case-insensitive), dan
// pastikan minimal satu kolom tersisa. Murni (tanpa I/O) agar bisa diuji terpisah.
func normalizeFrameworkColumns(cols []model.FrameworkColumn) ([]model.FrameworkColumn, error) {
	cleaned := make([]model.FrameworkColumn, 0, len(cols))
	seen := make(map[string]bool)
	for _, c := range cols {
		key := strings.TrimSpace(c.Key)
		if key == "" {
			continue
		}
		lk := strings.ToLower(key)
		if seen[lk] {
			return nil, fmt.Errorf("Key kolom duplikat: %s", key)
		}
		seen[lk] = true
		cleaned = append(cleaned, model.FrameworkColumn{
			Key:      key,
			Category: strings.TrimSpace(c.Category),
			Desc:     strings.TrimSpace(c.Desc),
		})
	}
	if len(cleaned) == 0 {
		return nil, fmt.Errorf("Framework harus punya minimal satu kolom")
	}
	return cleaned, nil
}

// SaveFrameworkColumns menyimpan daftar kolom framework ekstraksi M7 yang DIEDIT MANUSIA
// (HITL sejati: user menambah/menghapus/mengedit kolom langsung, bukan menebak lewat
// feedback ke LLM). Hanya tersedia di M7_STEP1_WAITING_APPROVAL; menyimpan kolom TIDAK
// memajukan pipeline — user tetap harus klik Approve setelah puas. xAI/multi-tenant:
// kolom berasal dari DATA sesi yang editable, tersimpan di FrameworkSelection.Columns.
func (h *SessionHandler) SaveFrameworkColumns(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}
	var payload struct {
		Columns []model.FrameworkColumn `json:"columns"`
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
	if session.Status != "M7_STEP1_WAITING_APPROVAL" {
		sendJSONError(w, http.StatusBadRequest, "Edit kolom hanya tersedia saat meninjau framework (M7_STEP1_WAITING_APPROVAL)")
		return
	}
	if session.FrameworkSelection == nil {
		sendJSONError(w, http.StatusBadRequest, "Framework belum tersedia untuk sesi ini")
		return
	}

	cleaned, err := normalizeFrameworkColumns(payload.Columns)
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	session.FrameworkSelection.Columns = cleaned
	if err := h.mongoRepo.UpdateSession(ctx, session); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal menyimpan kolom framework: "+err.Error())
		return
	}
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("%d kolom framework tersimpan; klik Setuju untuk mulai ekstraksi", len(cleaned)),
		"columns": cleaned,
	})
}

// normalizePriorReviews merapikan matriks prior-review hasil edit manusia: trim, buang
// baris kosong (author_year kosong), normalisasi verification ke VERIFIED/UNVERIFIED.
// Murni (tanpa I/O) agar bisa diuji terpisah.
func normalizePriorReviews(reviews []model.PriorReview) []model.PriorReview {
	cleaned := make([]model.PriorReview, 0, len(reviews))
	for _, r := range reviews {
		r.AuthorYear = strings.TrimSpace(r.AuthorYear)
		if r.AuthorYear == "" {
			continue
		}
		r.Scope = strings.TrimSpace(r.Scope)
		r.Methodology = strings.TrimSpace(r.Methodology)
		r.KeyFindings = strings.TrimSpace(r.KeyFindings)
		r.Limitations = strings.TrimSpace(r.Limitations)
		r.Selisih = strings.TrimSpace(r.Selisih)
		r.SynthesisNovelty = strings.TrimSpace(r.SynthesisNovelty)
		if strings.EqualFold(strings.TrimSpace(r.Verification), "VERIFIED") {
			r.Verification = "VERIFIED"
		} else {
			r.Verification = "UNVERIFIED"
		}
		cleaned = append(cleaned, r)
	}
	return cleaned
}

// SavePriorReviews menyimpan matriks prior-review yang DIEDIT/DIVERIFIKASI MANUSIA (HITL).
// Karena usulan AI dibuat tanpa web search, peneliti memverifikasi/mengoreksi tiap entri
// (set verification=VERIFIED) sebelum approve. Hanya di M2_STEP2_WAITING_APPROVAL; menyimpan
// TIDAK memajukan pipeline — user tetap klik Approve. xAI/anti-halusinasi.
func (h *SessionHandler) SavePriorReviews(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}
	var payload struct {
		Reviews []model.PriorReview `json:"reviews"`
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
	if session.Status != "M2_STEP2_WAITING_APPROVAL" {
		sendJSONError(w, http.StatusBadRequest, "Edit matriks hanya tersedia saat meninjau Prior Reviews (M2_STEP2_WAITING_APPROVAL)")
		return
	}
	cleaned := normalizePriorReviews(payload.Reviews)
	if session.PriorReviewsMatrix == nil {
		session.PriorReviewsMatrix = &model.PriorReviewsMatrix{}
	}
	session.PriorReviewsMatrix.Reviews = cleaned
	if err := h.mongoRepo.UpdateSession(ctx, session); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal menyimpan matriks: "+err.Error())
		return
	}
	verified := 0
	for _, r := range cleaned {
		if r.Verification == "VERIFIED" {
			verified++
		}
	}
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("%d review tersimpan (%d terverifikasi); klik Setuju untuk lanjut", len(cleaned), verified),
		"reviews": cleaned,
	})
}

// DeleteQdrantPaper menghapus vektor dari Qdrant berdasarkan DOI dan mereset status MongoDB
func (h *SessionHandler) DeleteQdrantPaper(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	var payload struct {
		DOI   string `json:"doi"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if payload.DOI == "" && payload.Title == "" {
		sendJSONError(w, http.StatusBadRequest, "DOI atau Title wajib diisi")
		return
	}

	// 1. Panggil API Qdrant Delete
	qdrantURL := os.Getenv("QDRANT_URL")
	qdrantKey := os.Getenv("QDRANT_API_KEY")
	if qdrantURL != "" {
		var filterKey string
		var filterValue string
		if payload.DOI != "" && payload.DOI != "-" {
			filterKey = "doi"
			filterValue = payload.DOI
		} else {
			filterKey = "title"
			filterValue = payload.Title
		}

		deleteBody := map[string]interface{}{
			"filter": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"key": filterKey,
						"match": map[string]interface{}{
							"value": filterValue,
						},
					},
				},
			},
		}

		bodyBytes, _ := json.Marshal(deleteBody)
		reqQdrant, err := http.NewRequest("POST", fmt.Sprintf("%s/collections/scientific_articles/points/delete", qdrantURL), bytes.NewReader(bodyBytes))
		if err == nil {
			reqQdrant.Header.Set("Content-Type", "application/json")
			if qdrantKey != "" {
				reqQdrant.Header.Set("api-key", qdrantKey)
			}
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(reqQdrant)
			if err == nil {
				defer resp.Body.Close()
			}
		}
	}

	// 2. Update MongoDB slr_papers
	ctx := context.Background()
	collPapers := h.mongoRepo.GetPapersCollection()
	
	filter := bson.M{"session_id": id}
	if payload.DOI != "" && payload.DOI != "-" {
		// handle http and https doi as well
		doiClean := strings.TrimPrefix(payload.DOI, "https://doi.org/")
		doiClean = strings.TrimPrefix(doiClean, "http://doi.org/")
		filter["DOI"] = bson.M{"$regex": primitive.Regex{Pattern: "(?i)" + regexp.QuoteMeta(doiClean)}}
	} else {
		filter["Title"] = bson.M{"$regex": primitive.Regex{Pattern: "(?i)" + regexp.QuoteMeta(payload.Title)}}
	}

	updPapers := bson.M{
		"$set": bson.M{
			"full_text_retrieved": false,
		},
	}
	_, _ = collPapers.UpdateMany(ctx, filter, updPapers)

	// 3. Update MongoDB slr_extractions (Reset QA)
	collExt := h.mongoRepo.GetExtractionCollection()
	updExt := bson.M{
		"$set": bson.M{
			"qa_rated": false,
		},
		"$unset": bson.M{
			"qa_total_score":    "",
			"qa_final_category": "",
			"qa_r1_score":       "",
			"qa_r1_category":    "",
			"qa_r1_reasoning":   "",
			"qa_r1_evidence":    "",
			"qa_r2_score":       "",
			"qa_r2_category":    "",
			"qa_r2_reasoning":   "",
			"qa_r2_evidence":    "",
		},
	}
	_, _ = collExt.UpdateMany(ctx, filter, updExt)

	sendJSONResponse(w, http.StatusOK, map[string]string{
		"message": "Berhasil menghapus data dari Qdrant dan mereset status MongoDB.",
	})
}

// GetExtractions mengembalikan daftar hasil ekstraksi Modul 7
func (h *SessionHandler) GetExtractions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	ctx := context.Background()
	coll := h.mongoRepo.GetExtractionCollection()
	cur, err := coll.Find(ctx, bson.M{"session_id": id})
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to get extractions: "+err.Error())
		return
	}

	var results []bson.M
	if err := cur.All(ctx, &results); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to decode extractions: "+err.Error())
		return
	}

	// Ubah ObjectID menjadi string untuk mempermudah JSON marshalling
	for i := range results {
		if oid, ok := results[i]["_id"].(primitive.ObjectID); ok {
			results[i]["_id"] = oid.Hex()
		}
	}

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"extractions": results,
	})
}

// GetAmbiguousExtractions mengembalikan data ekstraksi yang masih memiliki ambiguitas
func (h *SessionHandler) GetAmbiguousExtractions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}

	ctx := context.Background()
	coll := h.mongoRepo.GetExtractionCollection()
	// Filter where "ambiguous" array is not empty
	filter := bson.M{
		"session_id": id,
		"ambiguous":  bson.M{"$exists": true, "$ne": bson.A{}},
	}
	cur, err := coll.Find(ctx, filter)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to get ambiguous extractions: "+err.Error())
		return
	}

	var results []bson.M
	if err := cur.All(ctx, &results); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to decode ambiguous extractions: "+err.Error())
		return
	}

	for i := range results {
		if oid, ok := results[i]["_id"].(primitive.ObjectID); ok {
			results[i]["_id"] = oid.Hex()
		}
	}

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"extractions": results,
	})
}

// ResolveExtractionManual menyimpan nilai resolusi manual dari user
func (h *SessionHandler) ResolveExtractionManual(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	extID := req.PathValue("ext_id")
	if id == "" || extID == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID and Ext ID are required")
		return
	}

	var payload struct {
		FieldKey      string `json:"field_key"`
		ResolvedValue string `json:"resolved_value"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	ctx := context.Background()
	coll := h.mongoRepo.GetExtractionCollection()
	
	objID, err := primitive.ObjectIDFromHex(extID)
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid Ext ID format")
		return
	}

	filter := bson.M{"_id": objID, "session_id": id, "fields.key": payload.FieldKey}
	update := bson.M{
		"$set": bson.M{
			"fields.$.value": payload.ResolvedValue,
			"fields.$.status": "REPORTED",
		},
		"$pull": bson.M{
			"ambiguous": payload.FieldKey,
		},
	}
	
	res, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to update extraction: "+err.Error())
		return
	}
	if res.ModifiedCount == 0 {
		filterPush := bson.M{"_id": objID, "session_id": id}
		updatePush := bson.M{
			"$push": bson.M{
				"fields": bson.M{
					"key": payload.FieldKey,
					"value": payload.ResolvedValue,
					"evidence": "Manual Resolution",
					"status": "REPORTED",
				},
			},
			"$pull": bson.M{
				"ambiguous": payload.FieldKey,
			},
		}
		_, errUpdate2 := coll.UpdateOne(ctx, filterPush, updatePush)
		if errUpdate2 != nil {
			sendJSONError(w, http.StatusInternalServerError, "Gagal menambahkan field manual ke DB: "+errUpdate2.Error())
			return
		}
	}

	sendJSONResponse(w, http.StatusOK, map[string]string{
		"message": "Field resolusi manual tersimpan",
	})
}

// ResolveExtractionAuto memanggil LLM untuk meresolusi field secara otomatis
func (h *SessionHandler) ResolveExtractionAuto(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	extID := req.PathValue("ext_id")
	if id == "" || extID == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID and Ext ID are required")
		return
	}

	var payload struct {
		FieldKey string `json:"field_key"`
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

	coll := h.mongoRepo.GetExtractionCollection()
	objID, err := primitive.ObjectIDFromHex(extID)
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid Ext ID format")
		return
	}

	var extDoc bson.M
	if err := coll.FindOne(ctx, bson.M{"_id": objID, "session_id": id}).Decode(&extDoc); err != nil {
		sendJSONError(w, http.StatusNotFound, "Extraction not found")
		return
	}

	var title, doi string
	if t, ok := extDoc["Title"].(string); ok { title = t } else if t, ok := extDoc["title"].(string); ok { title = t }
	if d, ok := extDoc["DOI"].(string); ok { doi = d } else if d, ok := extDoc["doi"].(string); ok { doi = d }

	// Normalize DOI and get from FT index
	doi = strings.TrimPrefix(doi, "https://doi.org/")
	doi = strings.TrimPrefix(doi, "http://doi.org/")
	doi = strings.ToLower(strings.TrimSpace(doi))

	// Setup opDefs
	opDefs := "(operational definitions tidak tersedia)"
	if session.PICODefinitions != nil {
		b, _ := json.Marshal(session.PICODefinitions)
		opDefs = string(b)
	}

	ftIndex, _, _ := modules.BuildFulltextIndex(ctx)
	if ftIndex == nil {
		ftIndex = map[string]string{}
	}
	ft := ftIndex[doi]
	if ft == "" && title != "" {
		// Fallback mencari berdasarkan judul yang dinormalisasi (karena paper ini tidak punya DOI)
		ft = ftIndex["title:"+modules.NormTitle(title)]
	}

	if ft == "" {
		errMsg := fmt.Sprintf("Full-text tidak ditemukan di Qdrant, AI tidak bisa membaca paper ini.\n\nJudul: %s\n\nSilakan baca manual PDF-nya dan gunakan tombol 'Simpan (Manual)', atau jalankan ulang Colab Modul 6 untuk mengimpor paper ini.", title)
		logger.Logf(id, "⚠️ [Auto-Resolve] Full-text tidak ditemukan untuk paper: '%s' (DOI: %s). Anda perlu mengimpor PDF-nya via Modul 6 jika ingin memakai fitur Auto-Resolve.", title, doi)
		sendJSONError(w, http.StatusUnprocessableEntity, errMsg)
		return
	}

	rp1, _ := h.pipeline.GetLLMFactory().RoleProviders(ctx, "reviewer1")
	p, errP := h.pipeline.GetLLMFactory().CreateClient(ctx, rp1)
	if errP != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal inisiasi LLM: "+errP.Error())
		return
	}
	
	// Create extraction agent
	ag := agent.NewExtractionAgent(p) // simple no fallback for auto-resolve or just use primary

	resField, errLLM := ag.AutoResolveField(ctx, opDefs, title, ft, payload.FieldKey)
	if errLLM != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal memproses LLM: "+errLLM.Error())
		return
	}

	// Update DB
	filter := bson.M{"_id": objID, "session_id": id, "fields.key": payload.FieldKey}
	update := bson.M{
		"$set": bson.M{
			"fields.$.value": resField.Value,
			"fields.$.evidence": resField.Evidence,
			"fields.$.status": resField.Status,
		},
		"$pull": bson.M{
			"ambiguous": payload.FieldKey,
		},
	}
	
	res, errUpdate := coll.UpdateOne(ctx, filter, update)
	if errUpdate != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal menyimpan resolusi ke DB: "+errUpdate.Error())
		return
	}

	if res.ModifiedCount == 0 {
		filterPush := bson.M{"_id": objID, "session_id": id}
		updatePush := bson.M{
			"$push": bson.M{
				"fields": bson.M{
					"key": payload.FieldKey,
					"value": resField.Value,
					"evidence": resField.Evidence,
					"status": resField.Status,
				},
			},
			"$pull": bson.M{
				"ambiguous": payload.FieldKey,
			},
		}
		_, errUpdate2 := coll.UpdateOne(ctx, filterPush, updatePush)
		if errUpdate2 != nil {
			sendJSONError(w, http.StatusInternalServerError, "Gagal menambahkan field ke DB: "+errUpdate2.Error())
			return
		}
	}

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"message": "Field auto-resolve berhasil",
		"resolved_value": resField.Value,
		"evidence": resField.Evidence,
		"model_used": ag.ModelName(),
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

	// Self-heal: a paper that is BOTH retrieved (vectorized in Qdrant) AND inaccessible is
	// contradictory. Being in Qdrant means it IS accessible, so clear the inaccessible
	// flag. Runs on every sync, so legacy both-true records fix themselves via this normal
	// UI action (no manual DB editing needed).
	_, _ = coll.UpdateMany(ctx, bson.M{
		"session_id":          id,
		"full_text_retrieved": true,
		"inaccessible":        true,
	}, bson.M{"$set": bson.M{"inaccessible": false}})

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
					"acquisition_date":    time.Now().Format(time.RFC3339),
					"inaccessible":        false, // retrieved & inaccessible are mutually exclusive
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
				update := bson.M{"$set": bson.M{"full_text_retrieved": true, "acquisition_date": time.Now().Format(time.RFC3339), "inaccessible": false}}
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
			// inaccessible & retrieved are mutually exclusive: clear the retrieved flags so
			// a paper can never be counted in both VectorizedCount and InaccessibleCount.
			"full_text_retrieved": false,
			"Full_Text_Retrieved": false,
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
// fullTextExcluded mengembalikan true bila keputusan full-text paper = EXCLUDE
// (mirror finalFullDecision di modul M6).
func fullTextExcluded(p bson.M) bool {
	if fd, _ := p["Final_Decision_Full"].(string); fd != "" {
		return fd == "EXCLUDE"
	}
	d1, _ := p["Screener_1_Decision_Full"].(string)
	d2, _ := p["Screener_2_Decision_Full"].(string)
	return d1 == "EXCLUDE" && d2 == "EXCLUDE"
}

// GetExcludedFullText mengembalikan daftar paper EXCLUDE tahap full-text + reason codes
// sesi (multi-tenant) untuk panel HITL re-code alasan eksklusi (tabel PRISMA lebih bersih).
func (h *SessionHandler) GetExcludedFullText(w http.ResponseWriter, req *http.Request) {
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
	reasonCodes := []string{}
	if session.ScreeningSetup != nil && len(session.ScreeningSetup.ReasonCodes) > 0 {
		reasonCodes = session.ScreeningSetup.ReasonCodes
	}

	coll := h.mongoRepo.GetScreeningCollection()
	cursor, err := coll.Find(ctx, bson.M{"session_id": id, "full_text_retrieved": true})
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal mengambil data")
		return
	}
	var papers []bson.M
	_ = cursor.All(ctx, &papers)

	out := []map[string]interface{}{}
	for _, p := range papers {
		if !fullTextExcluded(p) {
			continue
		}
		title, _ := p["Title"].(string)
		if title == "" {
			title, _ = p["title"].(string)
		}
		doi, _ := p["DOI"].(string)
		if doi == "" {
			doi, _ = p["doi"].(string)
		}
		rc, _ := p["Screener_1_Reason_Code_Full"].(string)
		if rc == "" || rc == "-" {
			rc = "OTHER"
		}
		ev, _ := p["Screener_1_Notes_Full"].(string)
		if i := strings.Index(ev, "Evidence:"); i >= 0 {
			ev = strings.TrimSpace(ev[i+len("Evidence:"):])
		}
		oid, _ := p["_id"].(primitive.ObjectID)
		out = append(out, map[string]interface{}{
			"paper_id":    oid.Hex(),
			"title":       title,
			"doi":         doi,
			"reason_code": rc,
			"evidence":    ev,
		})
	}
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"reason_codes": reasonCodes,
		"papers":       out,
	})
}

// RecodeExclusions menerapkan re-code alasan eksklusi full-text (HITL) lalu meregenerasi
// output Modul 6 agar tabel PRISMA memakai kode baru.
func (h *SessionHandler) RecodeExclusions(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}
	var payload struct {
		Recodes []struct {
			PaperID    string `json:"paper_id"`
			ReasonCode string `json:"reason_code"`
		} `json:"recodes"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	ctx := context.Background()
	n := 0
	for _, rc := range payload.Recodes {
		if strings.TrimSpace(rc.PaperID) == "" || strings.TrimSpace(rc.ReasonCode) == "" {
			continue
		}
		if e := h.mongoRepo.RecodeFullTextExclusion(ctx, id, rc.PaperID, rc.ReasonCode, "[HITL re-code] alasan eksklusi diperbarui manual"); e == nil {
			n++
		}
	}
	// Regenerasi summary M6 dengan kode baru.
	session, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}
	session.Status = "M6_STEP3_REVIEW"
	session.SkipReaudit = true // re-code: jangan re-run audit PICO final saat regen
	if e := h.mongoRepo.UpdateSession(ctx, session); e != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal update sesi")
		return
	}
	h.pipeline.ExecuteAsync(ctx, id)
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{"message": "Re-code diterapkan, menyusun ulang ringkasan", "recoded": n})
}

// SuggestRecodes memulai job background: LLM (role Auditor) mengusulkan reason_code spesifik
// untuk tiap paper EXCLUDE, SATU per SATU, dengan progres ter-log ke Live Log + atribusi
// MODEL. Mengembalikan segera ({started,total}); frontend poll GetRecodeResult untuk hasil.
// AI mengusulkan; HITL memutuskan (tak auto-apply). Anti dobel-run: job berjalan -> tolak.
func (h *SessionHandler) SuggestRecodes(w http.ResponseWriter, req *http.Request) {
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

	// Anti dobel-klik di sisi server: kalau job sesi ini masih berjalan, kembalikan statusnya.
	h.recodeMu.Lock()
	if j, ok := h.recodeJobs[id]; ok && !j.Done {
		total := j.Total
		h.recodeMu.Unlock()
		sendJSONResponse(w, http.StatusAccepted, map[string]interface{}{"started": true, "total": total, "already_running": true})
		return
	}
	h.recodeMu.Unlock()

	coll := h.mongoRepo.GetScreeningCollection()
	cursor, err := coll.Find(ctx, bson.M{"session_id": id, "full_text_retrieved": true})
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal mengambil data")
		return
	}
	var papers []bson.M
	_ = cursor.All(ctx, &papers)

	idx2paper := map[int]string{}
	arr := []map[string]interface{}{}
	i := 0
	for _, p := range papers {
		if !fullTextExcluded(p) {
			continue
		}
		title, _ := p["Title"].(string)
		if title == "" {
			title, _ = p["title"].(string)
		}
		ev, _ := p["Screener_1_Notes_Full"].(string)
		oid, _ := p["_id"].(primitive.ObjectID)
		idx2paper[i] = oid.Hex()
		arr = append(arr, map[string]interface{}{"index": i, "title": title, "evidence": ev})
		i++
	}
	if len(arr) == 0 {
		sendJSONResponse(w, http.StatusOK, map[string]interface{}{"started": false, "total": 0, "suggestions": []interface{}{}})
		return
	}

	picoDef := ""
	if session.PICODefinitions != nil {
		b, _ := json.Marshal(session.PICODefinitions)
		picoDef = string(b)
	}
	codesCSV := ""
	if session.ScreeningSetup != nil {
		codesCSV = strings.Join(session.ScreeningSetup.ReasonCodes, ", ")
	}

	job := &recodeJob{Total: len(arr)}
	h.recodeMu.Lock()
	h.recodeJobs[id] = job
	h.recodeMu.Unlock()

	factory := h.pipeline.GetLLMFactory()
	roles := h.mongoRepo.GetLLMRoles(ctx)

	go func() {
		bg := context.Background()
		logger.Logf(id, "   🤖 [Saran AI] Mulai menganalisis %d paper EXCLUDE (role Auditor: %s, fb %s)...", len(arr), roles.Auditor, roles.AuditorFallback)
		out := []map[string]interface{}{}
		usedModel := ""
		for n, paper := range arr {
			title, _ := paper["title"].(string)
			if len(title) > 70 {
				title = title[:70] + "…"
			}
			logger.Logf(id, "   🤖 [Saran AI] Paper %d/%d: %s", n+1, len(arr), title)
			single, _ := json.Marshal([]map[string]interface{}{paper})
			var got *agent.ExclusionCodeSuggestion
			for _, prov := range []string{roles.Auditor, roles.AuditorFallback} {
				if strings.TrimSpace(prov) == "" {
					continue
				}
				c, e := factory.CreateClient(bg, prov)
				if e != nil {
					continue
				}
				s, e2 := agent.NewScreeningAgent(c).SuggestExclusionCodes(bg, picoDef, codesCSV, string(single))
				if e2 == nil && len(s) > 0 {
					got = &s[0]
					// Atribusi xAI LENGKAP: provider + nama MODEL asli (satu provider bisa
					// banyak model). ModelName() = "openai/<model>" / "claude/<model>" → ambil
					// bagian setelah "/" dan gabung dengan provider role.
					mn := c.ModelName()
					if k := strings.Index(mn, "/"); k >= 0 {
						mn = mn[k+1:]
					}
					usedModel = prov
					if mn != "" {
						usedModel = prov + " / " + mn
					}
					break
				}
			}
			if got != nil {
				logger.Logf(id, "      → %s (via %s)", got.ReasonCode, usedModel)
				if pid, ok := idx2paper[paper["index"].(int)]; ok {
					out = append(out, map[string]interface{}{"paper_id": pid, "suggested_code": got.ReasonCode, "rationale": got.Rationale, "model": usedModel})
				}
			} else {
				logger.Logf(id, "      → gagal (AI tak membalas / kuota)")
			}
			h.recodeMu.Lock()
			job.Progress = n + 1
			h.recodeMu.Unlock()
		}
		h.recodeMu.Lock()
		job.Done = true
		job.Model = usedModel
		job.Suggestions = out
		h.recodeMu.Unlock()
		logger.Logf(id, "   🤖 [Saran AI] Selesai: %d/%d usulan%s.", len(out), len(arr), map[bool]string{true: " via " + usedModel, false: ""}[usedModel != ""])
	}()

	sendJSONResponse(w, http.StatusAccepted, map[string]interface{}{"started": true, "total": len(arr)})
}

// GetRecodeResult mengembalikan status/hasil job saran re-code (untuk polling frontend).
func (h *SessionHandler) GetRecodeResult(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	h.recodeMu.Lock()
	job, ok := h.recodeJobs[id]
	var snap recodeJob
	if ok {
		snap = *job
	}
	h.recodeMu.Unlock()
	if !ok {
		sendJSONResponse(w, http.StatusOK, map[string]interface{}{"found": false})
		return
	}
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"found": true, "done": snap.Done, "model": snap.Model,
		"total": snap.Total, "progress": snap.Progress,
		"suggestions": snap.Suggestions, "error": snap.Error,
	})
}

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

// ResetModul7 mengembalikan sesi ke M6_STEP2_EXTRACTION_WAITING dan mereset QA data.
func (h *SessionHandler) ResetModul7(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID required")
		return
	}

	ctx := context.Background()
	_, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}
	// Unset fields in SLRSession and revert status
	sessionColl := h.mongoRepo.GetSessionCollection()
	_, err = sessionColl.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$unset": bson.M{
			// AMENDEMEN PROTOKOL (lihat CLAUDE.md "Validitas metodologi"): ResetModul7 adalah
			// aksi sengaja untuk menyusun ULANG protokol. Unset framework_selection agar
			// runFrameworkL1 me-regenerate (forceRegen tak relevan: framework nil -> generate).
			// Bedakan dari jalur PRESERVE (re-entry biasa mempertahankan protokol).
			"framework_selection":        "",
			"extraction_framework":       "",
			"extraction_log":             "",
			"qa_threshold":               "",
			"qa_threshold_justification": "",
			"sensitivity_analysis":       "",
			"synthesis_prep":             "",
			"modul7_summary":             "",
		},
		"$set": bson.M{
			"status": "M6_COMPLETE",
		},
	})
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to reset session status and fields")
		return
	}

	// Reset paper QA and Extraction fields
	coll := h.mongoRepo.GetExtractionCollection()
	upd := bson.M{
		"$unset": bson.M{
			"extracted":         "",
			"verified":          "",
			"extracted_data":    "",
			"qa_rated":          "",
			"qa_total_score":    "",
			"qa_final_category": "",
			"qa_r1_score":       "",
			"qa_r1_category":    "",
			"qa_r1_reasoning":   "",
			"qa_r1_evidence":    "",
			"qa_r1_model":       "",
			"qa_r2_score":       "",
			"qa_r2_category":    "",
			"qa_r2_reasoning":   "",
			"qa_r2_evidence":    "",
			"qa_r2_model":       "",
		},
	}
	_, err = coll.UpdateMany(ctx, bson.M{"session_id": id}, upd)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to reset extraction and QA fields")
		return
	}

	// Trigger pipeline
	h.pipeline.ExecuteAsync(ctx, id)

	sendJSONResponse(w, http.StatusOK, map[string]string{"message": "Modul 7 direset!"})
}

// ===== Utility Functions =====
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

// RecalculateQA recalculates ERROR papers that have valid R1+R2 scores without
// restarting the full QA pipeline. Only works when session is at M7_STEP3.
func (h *SessionHandler) RecalculateQA(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID required")
		return
	}

	ctx := context.Background()
	session, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	// Safety check: only allow recalculation when at M7_STEP3 or M7_STEP4
	if !strings.Contains(session.Status, "M7_STEP3") && !strings.Contains(session.Status, "M7_STEP4") {
		sendJSONError(w, http.StatusBadRequest, fmt.Sprintf("Recalculation not allowed in status: %s (must be at M7_STEP3 or M7_STEP4)", session.Status))
		return
	}

	fixedCount, err := modules.RecalculateQAErrors(ctx, h.mongoRepo, session)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Recalculation failed: %v", err))
		return
	}

	msg := fmt.Sprintf("Recalculated %d ERROR papers with valid R1+R2 scores", fixedCount)
	if fixedCount == 0 {
		msg = "No ERROR papers found with valid R1+R2 scores to recalculate"
	}

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"fixed_count": fixedCount,
		"message":     msg,
		"kappa":       session.QAThreshold.Kappa,
	})
}

// ReratePaper re-rates a single paper using dual raters.
// POST /api/sessions/{id}/m7/rerate-paper
func (h *SessionHandler) ReratePaper(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID required")
		return
	}

	var payload struct {
		PaperID string `json:"paper_id"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || payload.PaperID == "" {
		sendJSONError(w, http.StatusBadRequest, "paper_id is required")
		return
	}

	ctx := context.Background()
	session, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	// Safety check: only allow re-rating when at M7_STEP3 or beyond
	if !strings.Contains(session.Status, "M7_STEP3") && !strings.Contains(session.Status, "M7_STEP4") {
		sendJSONError(w, http.StatusBadRequest, fmt.Sprintf("Re-rating not allowed in status: %s (must be at M7_STEP3 or later)", session.Status))
		return
	}

	result, err := modules.RerateSinglePaper(ctx, h.mongoRepo, h.pipeline.GetLLMFactory(), session, payload.PaperID)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Re-rating failed: %v", err))
		return
	}

	sendJSONResponse(w, http.StatusOK, result)
}

// GetQAPrompt returns the system prompt used by QA raters for xAI transparency.
// GET /api/sessions/{id}/m7/qa-prompt
func (h *SessionHandler) GetQAPrompt(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID required")
		return
	}

	ctx := context.Background()
	session, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	if session.QAThreshold == nil {
		sendJSONError(w, http.StatusBadRequest, "QA threshold not configured for this session")
		return
	}

	prompt := modules.BuildQASystemPrompt(session)
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"system_prompt":  prompt,
		"tool":           session.QAThreshold.Tool,
		"categorization": session.QAThreshold.Categorization,
		"threshold":      session.QAThreshold.Threshold,
	})
}

// GetXAILog returns the xAI audit log for a session, optionally filtered by step.
// GET /api/sessions/{id}/xai-log?step=M7_STEP1_FRAMEWORK
func (h *SessionHandler) GetXAILog(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID required")
		return
	}

	ctx := context.Background()
	entries, err := h.mongoRepo.GetXAILog(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	if stepFilter := req.URL.Query().Get("step"); stepFilter != "" {
		var filtered []model.XAIEntry
		for _, e := range entries {
			if strings.HasPrefix(e.Step, stepFilter) || strings.Contains(e.Step, stepFilter) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"xai_log": entries,
	})
}

// EnrichMetadata triggers CrossRef metadata enrichment for extraction docs missing fields.
// POST /api/sessions/{id}/m7/enrich-metadata
func (h *SessionHandler) EnrichMetadata(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID required")
		return
	}

	ctx := context.Background()
	_, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	// Jalankan enrichment di background agar log stream ke WebSocket
	go func() {
		bgCtx := context.Background()
		enriched, err := modules.EnrichMetadataFromCrossRef(bgCtx, h.mongoRepo, id)
		if err != nil {
			logger.Logf(id, "   [Enrich] ERROR: %v", err)
		} else {
			logger.Logf(id, "   [Enrich] SELESAI: %d paper diperkaya metadata-nya.", enriched)
		}
	}()

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"message":        "Enrichment sedang berjalan di background. Lihat Agent Real-Time Logs.",
		"enriched_count": -1, // -1 indicates async/in-progress
	})
}

// ExportRIS generates an RIS file (.ris) compatible with VOSviewer from all screening papers.
// GET /api/sessions/{id}/m8b/export-ris
func (h *SessionHandler) ExportRIS(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID required")
		return
	}

	ctx := context.Background()
	_, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	// Get all screening papers
	papers, err := h.mongoRepo.GetAllScreeningPapers(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to retrieve papers: "+err.Error())
		return
	}

	// Get extraction docs for fallback keywords (subject field from enrichment)
	extKeywordsMap := make(map[string]string) // DOI -> keywords from extraction subject
	extColl := h.mongoRepo.GetExtractionCollection()
	if extColl != nil {
		cur, err := extColl.Find(ctx, bson.M{"session_id": id})
		if err == nil {
			var extDocs []bson.M
			_ = cur.All(ctx, &extDocs)
			for _, doc := range extDocs {
				doi := risGetExtDOI(doc)
				if doi == "" {
					continue
				}
				subj := risExtFieldValue(doc, "subject")
				if subj != "" {
					extKeywordsMap[strings.ToLower(doi)] = subj
				}
			}
		}
	}

	// Read pre-enriched Scopus keywords from extraction docs (field "scopus_keywords")
	scopusKeywordsMap := make(map[string]string)
	if extColl != nil {
		curScopus, err := extColl.Find(ctx, bson.M{"session_id": id, "scopus_keywords": bson.M{"$exists": true, "$ne": ""}})
		if err == nil {
			var scopusDocs []bson.M
			_ = curScopus.All(ctx, &scopusDocs)
			for _, doc := range scopusDocs {
				doi := risGetExtDOI(doc)
				if doi == "" {
					continue
				}
				if kw, ok := doc["scopus_keywords"].(string); ok && kw != "" {
					scopusKeywordsMap[strings.ToLower(doi)] = kw
				}
			}
		}
	}

	// Generate RIS entries
	var buf bytes.Buffer
	for _, p := range papers {
		title := risGetStr(p, "Title", "title")
		authors := risGetStr(p, "Authors", "authors")
		year := risGetStr(p, "Year", "year")
		journal := risGetStr(p, "Journal", "journal")
		doi := risGetStr(p, "DOI", "doi")
		authorKw := risGetStr(p, "Keywords", "keywords")
		indexKw := risGetStr(p, "IndexKeywords", "index_keywords")
		abstract := risGetStr(p, "Abstract", "abstract")

		// Fallback for author keywords
		if authorKw == "" && doi != "" {
			if subj, ok := extKeywordsMap[strings.ToLower(doi)]; ok {
				authorKw = subj
			}
		}
		if authorKw == "" && doi != "" {
			if subj, ok := scopusKeywordsMap[strings.ToLower(doi)]; ok {
				authorKw = subj
			}
		}
		if authorKw == "" {
			if sk := risGetStr(p, "scopus_keywords"); sk != "" {
				authorKw = sk
			}
		}
		if authorKw == "" && title != "" {
			authorKw = risExtractTitleKeywords(title)
		}

		// Determine TY value
		tyValue := "JOUR"
		if strings.Contains(strings.ToLower(journal), "conference") ||
			strings.Contains(strings.ToLower(journal), "proceedings") ||
			strings.Contains(strings.ToLower(journal), "symposium") ||
			strings.Contains(strings.ToLower(journal), "workshop") {
			tyValue = "CONF"
		} else if strings.Contains(strings.ToLower(journal), "book") ||
			strings.Contains(strings.ToLower(journal), "chapter") {
			tyValue = "CHAP"
		}

		buf.WriteString(fmt.Sprintf("TY  - %s\n", tyValue))

		// AU - one line per author
		if authors != "" {
			authorList := risParseAuthors(authors)
			for _, au := range authorList {
				buf.WriteString(fmt.Sprintf("AU  - %s\n", au))
			}
		}

		if title != "" {
			buf.WriteString(fmt.Sprintf("TI  - %s\n", title))
		}
		if journal != "" {
			buf.WriteString(fmt.Sprintf("JO  - %s\n", journal))
		}
		if year != "" {
			buf.WriteString(fmt.Sprintf("PY  - %s\n", year))
		}
		if doi != "" {
			buf.WriteString(fmt.Sprintf("DO  - %s\n", doi))
		}

		// KW - Author Keywords (one per line)
		if authorKw != "" {
			kwList := risParseKeywords(authorKw)
			for _, kw := range kwList {
				buf.WriteString(fmt.Sprintf("KW  - %s\n", kw))
			}
		}

		// ID - Index Keywords (one per line) — VOSviewer reads this as separate keyword source
		if indexKw != "" {
			idList := risParseKeywords(indexKw)
			for _, id := range idList {
				buf.WriteString(fmt.Sprintf("ID  - %s\n", id))
			}
		}

		if abstract != "" {
			buf.WriteString(fmt.Sprintf("AB  - %s\n", abstract))
		}

		buf.WriteString("ER  - \n\n")
	}

	// Return as file download
	w.Header().Set("Content-Type", "application/x-research-info-systems")
	w.Header().Set("Content-Disposition", `attachment; filename="slr_papers.ris"`)
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}

// ExportBibTeX is kept as an alias for backward compatibility, now exports RIS format.
// GET /api/sessions/{id}/m8b/export-bibtex
func (h *SessionHandler) ExportBibTeX(w http.ResponseWriter, req *http.Request) {
	h.ExportRIS(w, req)
}

// EnrichScopusKeywords - DEPRECATED. This endpoint has been replaced by CSV upload.
// POST /api/sessions/{id}/m8b/enrich-scopus-keywords
func (h *SessionHandler) EnrichScopusKeywords(w http.ResponseWriter, req *http.Request) {
	sendJSONError(w, http.StatusGone, "Fitur ini telah diganti dengan Upload CSV Scopus")
}

// UploadScopusCSV accepts a multipart CSV file exported from Scopus, parses it,
// matches rows by DOI to extraction docs, and stores keywords/affiliations/document_type.
// POST /api/sessions/{id}/m8b/upload-scopus-csv
func (h *SessionHandler) UploadScopusCSV(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID required")
		return
	}

	ctx := context.Background()
	_, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	// Parse multipart form (max 10MB)
	if err := req.ParseMultipartForm(10 << 20); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Gagal parse form: "+err.Error())
		return
	}

	file, _, err := req.FormFile("file")
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "File 'file' tidak ditemukan dalam form")
		return
	}
	defer file.Close()

	// Read all content
	rawBytes, err := io.ReadAll(file)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal membaca file: "+err.Error())
		return
	}

	// Strip UTF-8 BOM if present
	rawBytes = bytes.TrimPrefix(rawBytes, []byte("\xef\xbb\xbf"))

	// Parse CSV
	reader := csv.NewReader(bytes.NewReader(rawBytes))
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1 // allow variable field count

	records, err := reader.ReadAll()
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "Gagal parse CSV: "+err.Error())
		return
	}

	if len(records) < 2 {
		sendJSONError(w, http.StatusBadRequest, "CSV kosong atau hanya header")
		return
	}

	// Build header index map
	header := records[0]
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.TrimSpace(col)] = i
	}

	// Check required DOI column
	doiCol, hasDOI := colIdx["DOI"]
	if !hasDOI {
		sendJSONError(w, http.StatusBadRequest, "Kolom 'DOI' tidak ditemukan di header CSV")
		return
	}

	// Get column indices for fields we want
	authorKwCol, hasAuthorKw := colIdx["Author Keywords"]
	indexKwCol, hasIndexKw := colIdx["Index Keywords"]
	affiliationsCol, hasAffiliations := colIdx["Affiliations"]
	docTypeCol, hasDocType := colIdx["Document Type"]

	extColl := h.mongoRepo.GetExtractionCollection()
	if extColl == nil {
		sendJSONError(w, http.StatusInternalServerError, "Extraction collection tidak tersedia")
		return
	}

	matched := 0
	skipped := 0
	totalRows := len(records) - 1 // exclude header

	for _, row := range records[1:] {
		if doiCol >= len(row) {
			skipped++
			continue
		}

		doi := strings.TrimSpace(row[doiCol])
		if doi == "" {
			skipped++
			continue
		}

		// Normalize DOI
		doi = strings.TrimPrefix(doi, "https://doi.org/")
		doi = strings.TrimPrefix(doi, "http://doi.org/")
		doi = strings.TrimSpace(doi)

		if doi == "" {
			skipped++
			continue
		}

		doiLower := strings.ToLower(doi)

		// Extract Author Keywords and Index Keywords SEPARATELY
		var authorKeywords, indexKeywords string
		if hasAuthorKw && authorKwCol < len(row) {
			authorKeywords = strings.TrimSpace(row[authorKwCol])
		}
		if hasIndexKw && indexKwCol < len(row) {
			indexKeywords = strings.TrimSpace(row[indexKwCol])
		}

		// Build update set — store separately for proper RIS export (KW vs ID tags)
		updateSet := bson.M{}
		if authorKeywords != "" {
			updateSet["Keywords"] = authorKeywords
			updateSet["keywords"] = authorKeywords
		}
		if indexKeywords != "" {
			updateSet["IndexKeywords"] = indexKeywords
			updateSet["index_keywords"] = indexKeywords
		}
		// Keep combined scopus_keywords for backward compat
		combined := strings.TrimSpace(authorKeywords + "; " + indexKeywords)
		combined = strings.TrimPrefix(combined, "; ")
		combined = strings.TrimSuffix(combined, "; ")
		if combined != "" {
			updateSet["scopus_keywords"] = combined
		}
		if hasAffiliations && affiliationsCol < len(row) {
			aff := strings.TrimSpace(row[affiliationsCol])
			if aff != "" {
				updateSet["scopus_affiliations"] = aff
			}
		}
		if hasDocType && docTypeCol < len(row) {
			dt := strings.TrimSpace(row[docTypeCol])
			if dt != "" {
				updateSet["scopus_document_type"] = dt
			}
		}

		if len(updateSet) == 0 {
			skipped++
			continue
		}

		// Match by DOI (case-insensitive) in screening (primary for RIS) and extraction collections
		doiFilter := bson.M{
			"session_id": id,
			"$or": bson.A{
				bson.M{"doi": primitive.Regex{Pattern: "^" + regexp.QuoteMeta(doiLower) + "$", Options: "i"}},
				bson.M{"DOI": primitive.Regex{Pattern: "^" + regexp.QuoteMeta(doiLower) + "$", Options: "i"}},
			},
		}
		update := bson.M{"$set": updateSet}

		anyMatched := false

		// Update screening collection (primary — RIS export reads from here)
		screenColl := h.mongoRepo.GetScreeningCollection()
		if res, _ := screenColl.UpdateMany(ctx, doiFilter, update); res != nil && res.MatchedCount > 0 {
			anyMatched = true
		}

		// Update extraction collection (for descriptive analysis)
		if res, _ := extColl.UpdateOne(ctx, doiFilter, update); res != nil && res.MatchedCount > 0 {
			anyMatched = true
		}

		if anyMatched {
			matched++
		} else {
			skipped++
		}
	}

	logger.Logf(id, "[Scopus CSV] Selesai: matched=%d, skipped=%d, total_rows=%d", matched, skipped, totalRows)

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"matched":    matched,
		"skipped":    skipped,
		"total_rows": totalRows,
	})
}

// UploadIEEECSV accepts a multipart CSV file exported from IEEE Xplore, parses it,
// matches rows by DOI to screening+extraction docs, and stores keywords to scopus_keywords.
// POST /api/sessions/{id}/m8b/upload-ieee-csv
func (h *SessionHandler) UploadIEEECSV(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID required")
		return
	}

	ctx := context.Background()
	_, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	// Parse multipart form (max 10MB)
	if err := req.ParseMultipartForm(10 << 20); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Gagal parse form: "+err.Error())
		return
	}

	file, _, err := req.FormFile("file")
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "File 'file' tidak ditemukan dalam form")
		return
	}
	defer file.Close()

	// Read all content
	rawBytes, err := io.ReadAll(file)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal membaca file: "+err.Error())
		return
	}

	// Strip UTF-8 BOM if present
	rawBytes = bytes.TrimPrefix(rawBytes, []byte("\xef\xbb\xbf"))

	// Parse CSV
	reader := csv.NewReader(bytes.NewReader(rawBytes))
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "Gagal parse CSV: "+err.Error())
		return
	}

	if len(records) < 2 {
		sendJSONError(w, http.StatusBadRequest, "CSV kosong atau hanya header")
		return
	}

	// Build header index map
	header := records[0]
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.TrimSpace(col)] = i
	}

	// Check required DOI column
	doiCol, hasDOI := colIdx["DOI"]
	if !hasDOI {
		sendJSONError(w, http.StatusBadRequest, "Kolom 'DOI' tidak ditemukan di header CSV")
		return
	}

	// IEEE keyword columns: "Author Keywords" and "IEEE Terms" (semicolon separated)
	authorKwCol, hasAuthorKw := colIdx["Author Keywords"]
	ieeeTermsCol, hasIEEETerms := colIdx["IEEE Terms"]
	meshTermsCol, hasMeshTerms := colIdx["Mesh_Terms"]

	extColl := h.mongoRepo.GetExtractionCollection()
	if extColl == nil {
		sendJSONError(w, http.StatusInternalServerError, "Extraction collection tidak tersedia")
		return
	}

	matched := 0
	skipped := 0
	totalRows := len(records) - 1

	for _, row := range records[1:] {
		if doiCol >= len(row) {
			skipped++
			continue
		}

		doi := strings.TrimSpace(row[doiCol])
		if doi == "" {
			skipped++
			continue
		}

		// Normalize DOI
		doi = strings.TrimPrefix(doi, "https://doi.org/")
		doi = strings.TrimPrefix(doi, "http://doi.org/")
		doi = strings.TrimSpace(doi)

		if doi == "" {
			skipped++
			continue
		}

		doiLower := strings.ToLower(doi)

		// Extract Author Keywords and Index Keywords (IEEE Terms + Mesh_Terms) SEPARATELY
		var authorKeywords string
		if hasAuthorKw && authorKwCol < len(row) {
			authorKeywords = strings.TrimSpace(row[authorKwCol])
		}
		var indexParts []string
		if hasIEEETerms && ieeeTermsCol < len(row) {
			it := strings.TrimSpace(row[ieeeTermsCol])
			if it != "" {
				indexParts = append(indexParts, it)
			}
		}
		if hasMeshTerms && meshTermsCol < len(row) {
			mt := strings.TrimSpace(row[meshTermsCol])
			if mt != "" {
				indexParts = append(indexParts, mt)
			}
		}
		indexKeywords := strings.Join(indexParts, "; ")

		if authorKeywords == "" && indexKeywords == "" {
			skipped++
			continue
		}

		updateSet := bson.M{}
		if authorKeywords != "" {
			updateSet["Keywords"] = authorKeywords
			updateSet["keywords"] = authorKeywords
		}
		if indexKeywords != "" {
			updateSet["IndexKeywords"] = indexKeywords
			updateSet["index_keywords"] = indexKeywords
		}
		// Keep combined for backward compat
		combined := strings.TrimSpace(authorKeywords + "; " + indexKeywords)
		combined = strings.TrimPrefix(combined, "; ")
		combined = strings.TrimSuffix(combined, "; ")
		if combined != "" {
			updateSet["scopus_keywords"] = combined
		}

		// Match by DOI (case-insensitive) in screening and extraction collections
		doiFilter := bson.M{
			"session_id": id,
			"$or": bson.A{
				bson.M{"doi": primitive.Regex{Pattern: "^" + regexp.QuoteMeta(doiLower) + "$", Options: "i"}},
				bson.M{"DOI": primitive.Regex{Pattern: "^" + regexp.QuoteMeta(doiLower) + "$", Options: "i"}},
			},
		}
		update := bson.M{"$set": updateSet}

		anyMatched := false

		// Update screening collection
		screenColl := h.mongoRepo.GetScreeningCollection()
		if res, _ := screenColl.UpdateMany(ctx, doiFilter, update); res != nil && res.MatchedCount > 0 {
			anyMatched = true
		}

		// Update extraction collection
		if res, _ := extColl.UpdateOne(ctx, doiFilter, update); res != nil && res.MatchedCount > 0 {
			anyMatched = true
		}

		if anyMatched {
			matched++
		} else {
			skipped++
		}
	}

	logger.Logf(id, "[IEEE CSV] Selesai: matched=%d, skipped=%d, total_rows=%d", matched, skipped, totalRows)

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"matched":    matched,
		"skipped":    skipped,
		"total_rows": totalRows,
	})
}

// UploadPubMedTXT accepts a MEDLINE/PubMed tagged format text file, parses it,
// matches records by DOI to screening+extraction docs, and stores keywords to scopus_keywords.
// POST /api/sessions/{id}/m8b/upload-pubmed-txt
func (h *SessionHandler) UploadPubMedTXT(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID required")
		return
	}

	ctx := context.Background()
	_, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	// Parse multipart form (max 10MB)
	if err := req.ParseMultipartForm(10 << 20); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Gagal parse form: "+err.Error())
		return
	}

	file, _, err := req.FormFile("file")
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "File 'file' tidak ditemukan dalam form")
		return
	}
	defer file.Close()

	// Read all content
	rawBytes, err := io.ReadAll(file)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal membaca file: "+err.Error())
		return
	}

	// Strip UTF-8 BOM if present
	rawBytes = bytes.TrimPrefix(rawBytes, []byte("\xef\xbb\xbf"))

	// Parse PubMed/MEDLINE tagged format
	// Records are separated by empty lines
	// Tags: AID/LID for DOI (contains "[doi]"), OT for other terms, MH for MeSH headings
	type pubmedRecord struct {
		DOI            string
		AuthorKeywords []string // OT tags
		MeSHKeywords   []string // MH tags
	}

	var records []pubmedRecord
	var currentDOI string
	var currentAuthorKW []string
	var currentMeSH []string

	scanner := bufio.NewScanner(bytes.NewReader(rawBytes))
	// Increase buffer for long lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentTag string

	for scanner.Scan() {
		line := scanner.Text()

		// Empty line = end of record
		if strings.TrimSpace(line) == "" {
			if currentDOI != "" && (len(currentAuthorKW) > 0 || len(currentMeSH) > 0) {
				records = append(records, pubmedRecord{DOI: currentDOI, AuthorKeywords: currentAuthorKW, MeSHKeywords: currentMeSH})
			}
			currentDOI = ""
			currentAuthorKW = nil
			currentMeSH = nil
			currentTag = ""
			continue
		}

		// MEDLINE format: tag starts at column 0-3, followed by "- " and value
		// Continuation lines start with spaces
		if len(line) >= 6 && line[4] == '-' && line[5] == ' ' {
			currentTag = strings.TrimSpace(line[:4])
			value := strings.TrimSpace(line[6:])

			switch currentTag {
			case "AID", "LID":
				// DOI format: "10.1088/1741-2552/ac28d4 [doi]"
				if strings.HasSuffix(value, "[doi]") {
					doi := strings.TrimSpace(strings.TrimSuffix(value, "[doi]"))
					if doi != "" {
						currentDOI = doi
					}
				}
			case "OT":
				// Other Term (author keyword)
				if value != "" {
					currentAuthorKW = append(currentAuthorKW, value)
				}
			case "MH":
				// MeSH Heading (index keyword)
				// Remove subheadings after / and asterisks
				mh := value
				if idx := strings.Index(mh, "/"); idx > 0 {
					mh = mh[:idx]
				}
				mh = strings.TrimRight(mh, "*")
				mh = strings.TrimSpace(mh)
				if mh != "" {
					currentMeSH = append(currentMeSH, mh)
				}
			}
		}
		// Continuation lines (start with spaces) - we skip them for simplicity
	}
	// Don't forget last record if file doesn't end with empty line
	if currentDOI != "" && (len(currentAuthorKW) > 0 || len(currentMeSH) > 0) {
		records = append(records, pubmedRecord{DOI: currentDOI, AuthorKeywords: currentAuthorKW, MeSHKeywords: currentMeSH})
	}

	if len(records) == 0 {
		sendJSONError(w, http.StatusBadRequest, "Tidak ada record dengan DOI dan keywords ditemukan dalam file PubMed")
		return
	}

	extColl := h.mongoRepo.GetExtractionCollection()
	if extColl == nil {
		sendJSONError(w, http.StatusInternalServerError, "Extraction collection tidak tersedia")
		return
	}

	matched := 0
	skipped := 0
	totalRows := len(records)

	for _, rec := range records {
		doi := strings.TrimPrefix(rec.DOI, "https://doi.org/")
		doi = strings.TrimPrefix(doi, "http://doi.org/")
		doi = strings.TrimSpace(doi)

		if doi == "" {
			skipped++
			continue
		}

		doiLower := strings.ToLower(doi)
		authorKwStr := strings.Join(rec.AuthorKeywords, "; ")
		meshKwStr := strings.Join(rec.MeSHKeywords, "; ")

		if authorKwStr == "" && meshKwStr == "" {
			skipped++
			continue
		}

		updateSet := bson.M{}
		if authorKwStr != "" {
			updateSet["Keywords"] = authorKwStr
			updateSet["keywords"] = authorKwStr
		}
		if meshKwStr != "" {
			updateSet["IndexKeywords"] = meshKwStr
			updateSet["index_keywords"] = meshKwStr
		}
		// Keep combined for backward compat
		combined := strings.TrimSpace(authorKwStr + "; " + meshKwStr)
		combined = strings.TrimPrefix(combined, "; ")
		combined = strings.TrimSuffix(combined, "; ")
		if combined != "" {
			updateSet["scopus_keywords"] = combined
		}

		// Match by DOI (case-insensitive) in screening and extraction collections
		doiFilter := bson.M{
			"session_id": id,
			"$or": bson.A{
				bson.M{"doi": primitive.Regex{Pattern: "^" + regexp.QuoteMeta(doiLower) + "$", Options: "i"}},
				bson.M{"DOI": primitive.Regex{Pattern: "^" + regexp.QuoteMeta(doiLower) + "$", Options: "i"}},
			},
		}
		update := bson.M{"$set": updateSet}

		anyMatched := false

		// Update screening collection
		screenColl := h.mongoRepo.GetScreeningCollection()
		if res, _ := screenColl.UpdateMany(ctx, doiFilter, update); res != nil && res.MatchedCount > 0 {
			anyMatched = true
		}

		// Update extraction collection
		if res, _ := extColl.UpdateOne(ctx, doiFilter, update); res != nil && res.MatchedCount > 0 {
			anyMatched = true
		}

		if anyMatched {
			matched++
		} else {
			skipped++
		}
	}

	logger.Logf(id, "[PubMed TXT] Selesai: matched=%d, skipped=%d, total_records=%d", matched, skipped, totalRows)

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"matched":       matched,
		"skipped":       skipped,
		"total_records": totalRows,
	})
}

// --- RIS helper functions ---

func risGetStr(p map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := p[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func risGetExtDOI(doc bson.M) string {
	if d, ok := doc["doi"].(string); ok && d != "" {
		return d
	}
	if d, ok := doc["DOI"].(string); ok && d != "" {
		return d
	}
	return ""
}

func risExtFieldValue(doc bson.M, keySub string) string {
	arr, ok := doc["fields"].(bson.A)
	if !ok {
		if arr2, ok2 := doc["fields"].([]interface{}); ok2 {
			arr = bson.A(arr2)
		}
	}
	if len(arr) == 0 {
		if arr2, ok := doc["m7_fields"].(bson.A); ok {
			arr = arr2
		} else if arr3, ok3 := doc["m7_fields"].([]interface{}); ok3 {
			arr = bson.A(arr3)
		}
	}
	if len(arr) == 0 {
		return ""
	}
	target := strings.ToLower(keySub)
	for _, it := range arr {
		f, ok := it.(bson.M)
		if !ok {
			continue
		}
		key := ""
		if k, ok := f["key"].(string); ok {
			key = strings.ToLower(k)
		}
		if strings.Contains(key, target) {
			if v, ok := f["value"].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

func risParseAuthors(authors string) []string {
	// Split by semicolons or " and "
	authors = strings.ReplaceAll(authors, " ; ", ";")
	authors = strings.ReplaceAll(authors, "; ", ";")
	authors = strings.ReplaceAll(authors, " and ", ";")
	parts := strings.Split(authors, ";")
	var result []string
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			result = append(result, t)
		}
	}
	if len(result) == 0 && authors != "" {
		result = append(result, strings.TrimSpace(authors))
	}
	return result
}

func risParseKeywords(keywords string) []string {
	// Split by semicolons or pipes
	keywords = strings.ReplaceAll(keywords, "|", ";")
	parts := strings.Split(keywords, ";")
	var result []string
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" && risIsValidKeyword(t) {
			result = append(result, t)
		}
	}
	return result
}

// risIsValidKeyword filters out garbage keywords that produce noise in VOSviewer.
func risIsValidKeyword(kw string) bool {
	// Skip short keywords (less than 3 chars)
	if len(kw) < 3 {
		return false
	}
	// Skip keywords that are only digits
	allDigits := true
	for _, c := range kw {
		if c < '0' || c > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return false
	}
	// Skip [not reported] variants
	lower := strings.ToLower(kw)
	if strings.Contains(lower, "[not reported]") {
		return false
	}
	// Skip extraction artifacts
	if strings.Contains(lower, "subjects per dataset") {
		return false
	}
	return true
}

// risExtractTitleKeywords extracts keywords from a paper title by splitting on spaces/punctuation,
// lowercasing, and filtering stopwords and short words.
func risExtractTitleKeywords(title string) string {
	// Split by non-letter characters
	splitter := regexp.MustCompile(`[^a-zA-Z]+`)
	words := splitter.Split(title, -1)

	stopwords := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "from": true,
		"that": true, "this": true, "are": true, "was": true, "were": true,
		"has": true, "have": true, "will": true, "can": true, "not": true,
		"but": true, "its": true, "their": true, "our": true, "than": true,
		"into": true, "also": true, "each": true, "both": true, "more": true,
		"most": true, "some": true, "all": true, "new": true, "first": true,
		"two": true, "one": true, "very": true, "when": true, "only": true,
		"how": true, "where": true, "what": true, "used": true, "using": true,
		"based": true, "through": true, "under": true, "over": true, "about": true,
		"after": true, "before": true, "during": true, "such": true, "which": true,
		"these": true, "those": true, "other": true, "between": true, "novel": true,
		"proposed": true, "method": true, "approach": true, "paper": true, "study": true,
		"results": true, "show": true, "analysis": true, "via": true,
	}

	var kws []string
	for _, w := range words {
		lower := strings.ToLower(w)
		if len(lower) < 3 {
			continue
		}
		if stopwords[lower] {
			continue
		}
		kws = append(kws, lower)
	}

	return strings.Join(kws, "; ")
}

// DownloadTex returns the manuscript .tex file as a download.
func (h *SessionHandler) DownloadTex(w http.ResponseWriter, req *http.Request) {
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

	if session.Manuscript == nil || session.Manuscript.Latex == "" {
		sendJSONError(w, http.StatusNotFound, "LaTeX manuscript not yet generated")
		return
	}

	w.Header().Set("Content-Type", "text/x-tex")
	w.Header().Set("Content-Disposition", `attachment; filename="manuscript.tex"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(session.Manuscript.Latex))
}

// DownloadBib returns the manuscript .bib file as a download.
func (h *SessionHandler) DownloadBib(w http.ResponseWriter, req *http.Request) {
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

	if session.Manuscript == nil || session.Manuscript.Bibtex == "" {
		sendJSONError(w, http.StatusNotFound, "BibTeX file not yet generated")
		return
	}

	w.Header().Set("Content-Type", "application/x-bibtex")
	w.Header().Set("Content-Disposition", `attachment; filename="references.bib"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(session.Manuscript.Bibtex))
}
