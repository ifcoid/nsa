package modules

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"nsa/internal/logger"
	"nsa/internal/model"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type M6Acquisition struct {
	deps *ModuleDeps
}

func NewM6Acquisition(deps *ModuleDeps) *M6Acquisition {
	return &M6Acquisition{deps: deps}
}

func (m *M6Acquisition) Name() string {
	return "M6_ACQUISITION"
}

func (m *M6Acquisition) Execute(ctx context.Context, session *model.SLRSession) error {
	switch session.Status {

	case "M5_DONE":
		session.Status = "M6_INIT"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M6_INIT":
		logger.Log(session.ID, "   [System] Memulai Modul 6: Full-Text Acquisition...")
		session.Status = "M6_STEP1_ACQUISITION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M6_STEP1_ACQUISITION":
		// Cek semua paper INCLUDE dan cari open access URL-nya
		err := m.processAcquisition(ctx, session)
		if err != nil {
			return err
		}

		session.Status = "M6_STEP1_WAITING_SYNC"
		logger.Log(session.ID, "   [System] Langkah 1 selesai. Menunggu Anda menjalankan proses vektorisasi (PEDE) di Colab dan Sinkronisasi.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M6_STEP1_WAITING_SYNC":
		// Menunggu user menekan tombol sinkronisasi Qdrant di UI.
		// Proses sinkronisasi dilakukan via HTTP handler (session_handler.go)
		return nil

	default:
		return nil
	}
}

func (m *M6Acquisition) processAcquisition(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [M6] Memeriksa ketersediaan PDF Open Access (via Unpaywall & ArXiv)...")

	coll := m.deps.MongoRepo.GetScreeningCollection()

	// Ambil semua paper yang INCLUDE
	filter := bson.M{
		"session_id":     session.ID,
		"Final_Decision": "INCLUDE",
	}

	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		return err
	}

	var papers []bson.M
	if err = cursor.All(ctx, &papers); err != nil {
		return err
	}

	logger.Log(session.ID, fmt.Sprintf("   [M6] Terdapat %d paper INCLUDE yang akan diproses akuisisi.", len(papers)))

	client := &http.Client{Timeout: 15 * time.Second}
	email := "admin@example.com" // Unpaywall requires email

	for _, p := range papers {
		// Jika sudah pernah diproses, skip (bisa untuk retry resume)
		if p["full_text_location"] != nil && p["full_text_location"] != "" {
			continue
		}

		doi, ok := p["DOI"].(string)
		if !ok || doi == "" {
			// Jika tidak ada DOI, langsung hitl download
			m.updatePaperAcquisition(ctx, coll, p["_id"], "hitl download", "")
			continue
		}

		// 1. Cek Unpaywall
		oaURL := m.checkUnpaywall(client, doi, email)
		if oaURL != "" {
			m.updatePaperAcquisition(ctx, coll, p["_id"], "unpaywall", oaURL)
			continue
		}

		// 2. Cek ArXiv via API (jika DOI ditemukan di arXiv)
		arxivURL := m.checkArxiv(client, doi)
		if arxivURL != "" {
			m.updatePaperAcquisition(ctx, coll, p["_id"], "arxiv", arxivURL)
			continue
		}

		// 3. Fallback: Hitl Download
		m.updatePaperAcquisition(ctx, coll, p["_id"], "hitl download", "")
		time.Sleep(500 * time.Millisecond) // rate limit protection
	}

	return m.updateAcquisitionLog(ctx, session, coll)
}

func (m *M6Acquisition) updatePaperAcquisition(ctx context.Context, coll *mongo.Collection, id interface{}, location, url string) {
	update := bson.M{
		"$set": bson.M{
			"full_text_location": location,
			"download_url":       url,
		},
	}
	_, _ = coll.UpdateByID(ctx, id, update)
}

func (m *M6Acquisition) updateAcquisitionLog(ctx context.Context, session *model.SLRSession, coll *mongo.Collection) error {
	filter := bson.M{"session_id": session.ID, "Final_Decision": "INCLUDE"}
	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		return err
	}
	var papers []bson.M
	_ = cursor.All(ctx, &papers)

	var log model.AcquisitionLog
	log.TotalInclude = len(papers)

	for _, p := range papers {
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
	return nil
}

func (m *M6Acquisition) checkUnpaywall(client *http.Client, doi, email string) string {
	urlStr := fmt.Sprintf("https://api.unpaywall.org/v2/%s?email=%s", url.PathEscape(doi), url.QueryEscape(email))
	req, _ := http.NewRequest("GET", urlStr, nil)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return ""
	}

	isOA, ok := data["is_oa"].(bool)
	if ok && isOA {
		bestOA, ok := data["best_oa_location"].(map[string]interface{})
		if ok {
			pdfURL, _ := bestOA["url_for_pdf"].(string)
			if pdfURL != "" {
				return pdfURL
			}
			fallbackURL, _ := bestOA["url"].(string)
			return fallbackURL
		}
	}
	return ""
}

func (m *M6Acquisition) checkArxiv(client *http.Client, doi string) string {
	urlStr := fmt.Sprintf("http://export.arxiv.org/api/query?search_query=doi:%s", url.QueryEscape(doi))
	req, _ := http.NewRequest("GET", urlStr, nil)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	type ArxivEntry struct {
		ID string `xml:"id"`
	}
	type ArxivFeed struct {
		Entries []ArxivEntry `xml:"entry"`
	}
	var feed ArxivFeed
	if err := xml.Unmarshal(body, &feed); err == nil && len(feed.Entries) > 0 {
		// Convert standard arxiv URL to PDF URL
		// Example ID: http://arxiv.org/abs/2101.00001
		pdfURL := strings.Replace(feed.Entries[0].ID, "/abs/", "/pdf/", 1)
		if pdfURL != "" && pdfURL != feed.Entries[0].ID {
			return pdfURL + ".pdf"
		}
	}
	return ""
}
