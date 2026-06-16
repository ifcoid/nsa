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
	"regexp"
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
		// Menunggu user menekan tombol sinkronisasi Qdrant di UI, lalu Approve untuk
		// lanjut ke full-text screening (ApproveStep men-set M6_STEP2_FULLTEXT_SCREENING).
		return nil

	// ===== LANGKAH 2: Full-text screening (dual-reviewer + RAG Qdrant) =====
	case "M6_STEP2_FULLTEXT_SCREENING":
		return m.runFullTextScreeningBatch(ctx, session)

	case "M6_STEP2_WAITING_RESOLUTION":
		logger.Log(session.ID, "   [System] Batch full-text dijeda (HITL). Resolusi DISAGREE/UNCERTAIN di UI lalu lanjutkan.")
		return nil

	// ===== LANGKAH 3: Resolve + audit + extraction prep + hasil akhir =====
	case "M6_STEP3_REVIEW":
		return m.buildModul6Outputs(ctx, session)

	case "M6_STEP3_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Modul 6 selesai disusun. Menunggu persetujuan akhir sebelum ke Modul 7.")
		return nil

	case "M6_STEP3_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 6.3] Menyusun ulang output Modul 6 (feedback: '%s')...\n", session.Feedback)
		session.Feedback = ""
		session.Status = "M6_STEP3_REVIEW"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M6_STEP3_APPROVED", "M6_COMPLETE":
		session.Status = "M7_EXTRACTION"
		logger.Log(session.ID, "   [System] Modul 6 SELESAI. Memulai Modul 7 (Data Extraction + QA).")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	default:
		return nil
	}
}

func (m *M6Acquisition) processAcquisition(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [M6] Memeriksa ketersediaan PDF Open Access (via Unpaywall & ArXiv)...")

	coll := m.deps.MongoRepo.GetScreeningCollection()

	// Ambil semua paper yang INCLUDE (baik eksplisit dari resolusi, maupun implisit dari agreement R1)
	filter := bson.M{
		"session_id": session.ID,
		"$or": []bson.M{
			{"Final_Decision": "INCLUDE"},
			{"Final_Decision": "", "Screener_1_Decision": "INCLUDE"},
		},
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

	for i, p := range papers {
		if i > 0 && i%10 == 0 {
			logger.Logf(session.ID, "   [M6] Memproses paper %d/%d...", i, len(papers))
		}

		// Jika sudah pernah diproses, skip (bisa untuk retry resume)
		if p["full_text_location"] != nil && p["full_text_location"] != "" {
			continue
		}

		// Title dipakai untuk fallback pencarian arXiv by-title (preprint sering punya
		// DOI berbeda / tanpa DOI terbit). Dokumen bisa menyimpan "title" atau "Title".
		title, _ := p["title"].(string)
		if title == "" {
			title, _ = p["Title"].(string)
		}

		doi, ok := p["DOI"].(string)
		if !ok || doi == "" {
			doi, _ = p["doi"].(string)
		}
		if doi == "" {
			// Tidak ada DOI sama sekali: coba arXiv by-title dulu sebelum menyerah ke HITL.
			if arxivURL := m.checkArxiv(client, "", title); arxivURL != "" {
				m.updatePaperAcquisition(ctx, coll, p["_id"], "arxiv", arxivURL)
				continue
			}
			m.updatePaperAcquisition(ctx, coll, p["_id"], "hitl download", "")
			continue
		}
		if !isValidDOI(doi) {
			// Identifier bukan DOI asli (mis. Scopus EID "2-s2.0-..."): Unpaywall/arXiv-by-DOI
			// gagal diam-diam. Coba arXiv by-title; kalau gagal -> hitl download + tandai.
			if arxivURL := m.checkArxiv(client, "", title); arxivURL != "" {
				logger.Logf(session.ID, "      [M6] '%s' bukan DOI valid tapi preprint ketemu di arXiv (by-title) -> %s", doi, arxivURL)
				m.updatePaperAcquisition(ctx, coll, p["_id"], "arxiv", arxivURL)
				continue
			}
			logger.Logf(session.ID, "      [M6] Identifier bukan DOI valid ('%s') -> hitl download (cari manual)", doi)
			m.updatePaperAcquisition(ctx, coll, p["_id"], "hitl download", "")
			_, _ = coll.UpdateByID(ctx, p["_id"], bson.M{"$set": bson.M{"id_not_doi": true}})
			continue
		}

		// 1. Cek Unpaywall
		oaURL := m.checkUnpaywall(client, doi, email)
		if oaURL != "" {
			m.updatePaperAcquisition(ctx, coll, p["_id"], "unpaywall", oaURL)
			continue
		}

		// 2. Cek arXiv: pertama by-DOI, lalu fallback by-title (preprint sering punya
		//    arXiv-ID sendiri sedangkan DOI terbit IEEE/Springer tidak terdaftar di arXiv).
		arxivURL := m.checkArxiv(client, doi, title)
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
	filter := bson.M{
		"session_id": session.ID,
		"$or": []bson.M{
			{"Final_Decision": "INCLUDE"},
			{"Final_Decision": "", "Screener_1_Decision": "INCLUDE"},
		},
	}
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

// doiPattern matches a real DOI (after stripping any doi.org prefix).
var doiPattern = regexp.MustCompile(`^10\.\d{4,9}/`)

// isValidDOI reports whether s looks like a real DOI. Non-DOI identifiers (e.g. Scopus
// EID "2-s2.0-...") make Unpaywall/arXiv fail silently, so callers skip them.
func isValidDOI(s string) bool {
	s = strings.TrimPrefix(s, "https://doi.org/")
	s = strings.TrimPrefix(s, "http://doi.org/")
	return doiPattern.MatchString(strings.TrimSpace(s))
}

func (m *M6Acquisition) checkUnpaywall(client *http.Client, doi, email string) string {
	if !isValidDOI(doi) {
		return ""
	}
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

// checkArxiv mencari PDF preprint di arXiv. Pertama by-DOI (exact), lalu fallback by-title.
// Fallback ini penting: DOI terbit (IEEE/Springer) sering TIDAK terdaftar di arXiv walau
// preprint-nya ada dengan arXiv-ID sendiri -> by-DOI gagal diam-diam. Kandidat by-title
// hanya diterima bila judulnya mirip (titleSim >= 0.8) agar tidak salah-match paper lain.
func (m *M6Acquisition) checkArxiv(client *http.Client, doi, title string) string {
	// 1. By-DOI: DOI yang sama persis -> entri pertama valid tanpa cek judul.
	if isValidDOI(doi) {
		if entries := m.arxivQuery(client, "doi:"+doi); len(entries) > 0 {
			if u := arxivPDFURL(entries[0].ID); u != "" {
				return u
			}
		}
	}

	// 2. Fallback by-title (dengan guard similarity untuk cegah false-match).
	t := strings.TrimSpace(title)
	if t == "" {
		return ""
	}
	nt := NormTitle(t)
	for _, e := range m.arxivQuery(client, `ti:"`+t+`"`) {
		if titleSim(nt, NormTitle(e.Title)) >= 0.8 {
			if u := arxivPDFURL(e.ID); u != "" {
				return u
			}
		}
	}
	return ""
}

type arxivEntry struct {
	ID    string `xml:"id"`
	Title string `xml:"title"`
}

// arxivQuery menjalankan satu query ke arXiv API dan mengembalikan entri (max 5).
func (m *M6Acquisition) arxivQuery(client *http.Client, searchQuery string) []arxivEntry {
	params := url.Values{}
	params.Set("search_query", searchQuery)
	params.Set("max_results", "5")
	req, _ := http.NewRequest("GET", "http://export.arxiv.org/api/query?"+params.Encode(), nil)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var feed struct {
		Entries []arxivEntry `xml:"entry"`
	}
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil
	}
	return feed.Entries
}

// arxivPDFURL mengubah arXiv abs-ID menjadi URL PDF.
// Contoh: http://arxiv.org/abs/2101.00001v1 -> http://arxiv.org/pdf/2101.00001v1.pdf
func arxivPDFURL(id string) string {
	pdfURL := strings.Replace(id, "/abs/", "/pdf/", 1)
	if pdfURL == "" || pdfURL == id {
		return ""
	}
	return pdfURL + ".pdf"
}
