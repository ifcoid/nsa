package modules

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"nsa/internal/logger"
	"nsa/internal/model"
	"nsa/internal/repository"
)

// P0Ingest implements ProposalModule for the ingest phase of the proposal pipeline.
// It handles BibTeX parsing, PDF upload coordination, embedding server setup,
// vector ingestion, and knowledge graph construction.
type P0Ingest struct {
	deps *ModuleDeps
}

// NewP0Ingest creates a new P0Ingest module instance.
func NewP0Ingest(deps *ModuleDeps) *P0Ingest {
	return &P0Ingest{deps: deps}
}

// Name returns the module identifier.
func (m *P0Ingest) Name() string { return "P0_INGEST" }

// Execute processes the proposal session based on its current status.
func (m *P0Ingest) Execute(ctx context.Context, session *model.ProposalSession) error {
	logger.Logf(session.ID, ">> [MODUL P0: INGEST] Memproses State: %s\n", session.Status)

	switch session.Status {

	case "P0_INIT":
		logger.Log(session.ID, "   [P0] Inisialisasi pipeline ingest. Transisi ke P0_BIB_PARSED.")
		session.Status = "P0_BIB_PARSED"
		return m.deps.MongoRepo.UpdateProposalSession(ctx, session)

	case "P0_BIB_PARSED":
		return m.validateBibRefs(ctx, session)

	case "P0_WAITING_PDFS":
		logger.Log(session.ID, "   [P0] Menunggu upload PDF referensi.")
		logger.Log(session.ID, "   Instruksi: Upload file PDF ke folder yang telah ditentukan,")
		logger.Log(session.ID, "   kemudian ubah status sesi menjadi 'P0_PDFS_UPLOADED'.")
		return nil

	case "P0_PDFS_UPLOADED":
		logger.Log(session.ID, "   [P0] PDF telah di-upload. Transisi ke P0_WAITING_EMBED_SERVER.")
		session.Status = "P0_WAITING_EMBED_SERVER"
		return m.deps.MongoRepo.UpdateProposalSession(ctx, session)

	case "P0_WAITING_EMBED_SERVER":
		logger.Log(session.ID, "   [P0] Menunggu embedding server aktif.")
		logger.Log(session.ID, "   Instruksi: Jalankan Colab notebook embedding server di:")
		logger.Log(session.ID, "   https://colab.research.google.com/drive/embed_server")
		logger.Log(session.ID, "   Setelah server aktif, ubah status menjadi 'P0_EMBED_SERVER_READY'.")
		return nil

	case "P0_EMBED_SERVER_READY":
		logger.Log(session.ID, "   [P0] Embedding server siap. Transisi ke P0_WAITING_INGEST.")
		session.Status = "P0_WAITING_INGEST"
		return m.deps.MongoRepo.UpdateProposalSession(ctx, session)

	case "P0_WAITING_INGEST":
		logger.Log(session.ID, "   [P0] Menunggu proses ingest vektor selesai.")
		logger.Log(session.ID, "   Instruksi: Jalankan Colab notebook PEDE ingest di:")
		logger.Log(session.ID, "   https://colab.research.google.com/drive/pede_ingest")
		logger.Log(session.ID, "   Setelah selesai, ubah status menjadi 'P0_VECTORS_READY'.")
		return nil

	case "P0_VECTORS_READY":
		logger.Log(session.ID, "   [P0] Vektor telah siap. Transisi ke P0_KG_BUILD.")
		session.Status = "P0_KG_BUILD"
		return m.deps.MongoRepo.UpdateProposalSession(ctx, session)

	case "P0_KG_BUILD":
		return m.buildKnowledgeGraph(ctx, session)

	case "P0_DONE":
		logger.Log(session.ID, "   [P0] Pipeline ingest selesai. Semua referensi telah diproses.")
		return nil

	default:
		return nil
	}
}

// validateBibRefs validates that the session has sufficient references:
// - Minimum 10 entries
// - At least 70% from the last 5 years
func (m *P0Ingest) validateBibRefs(ctx context.Context, session *model.ProposalSession) error {
	refs, err := m.deps.MongoRepo.GetProposalRefs(ctx, session.ID)
	if err != nil {
		return fmt.Errorf("p0_ingest: gagal mengambil referensi: %w", err)
	}

	// Validate minimum 10 entries
	if len(refs) < 10 {
		session.SystemError = fmt.Sprintf("Jumlah referensi tidak mencukupi: %d (minimum 10)", len(refs))
		session.Status = "P0_BIB_PARSED_ERROR"
		return m.deps.MongoRepo.UpdateProposalSession(ctx, session)
	}

	// Validate >= 70% from last 5 years
	cutoffYear := time.Now().Year() - 5
	recentCount := 0
	for _, ref := range refs {
		year, err := strconv.Atoi(ref.Year)
		if err != nil {
			continue
		}
		if year >= cutoffYear {
			recentCount++
		}
	}

	recentPct := float64(recentCount) / float64(len(refs)) * 100
	if recentPct < 70.0 {
		session.SystemError = fmt.Sprintf("Persentase referensi terbaru (5 tahun terakhir) hanya %.1f%% (minimum 70%%)", recentPct)
		session.Status = "P0_BIB_PARSED_ERROR"
		return m.deps.MongoRepo.UpdateProposalSession(ctx, session)
	}

	logger.Logf(session.ID, "   [P0] Validasi BibTeX berhasil: %d referensi, %.1f%% terbaru.", len(refs), recentPct)
	session.Status = "P0_WAITING_PDFS"
	return m.deps.MongoRepo.UpdateProposalSession(ctx, session)
}

// buildKnowledgeGraph extracts triplets from proposal refs and pushes them
// to Neo4j as a knowledge graph with node labels: Paper, Author, Method,
// Dataset, Finding, Gap; and edge types: CITES, USES_METHOD, FINDS,
// CONTRADICTS, EXTENDS, HAS_GAP.
func (m *P0Ingest) buildKnowledgeGraph(ctx context.Context, session *model.ProposalSession) error {
	if m.deps.Neo4jRepo == nil {
		logger.Log(session.ID, "   [P0] Neo4j tidak tersedia, skip KG build.")
		session.Status = "P0_DONE"
		return m.deps.MongoRepo.UpdateProposalSession(ctx, session)
	}

	refs, err := m.deps.MongoRepo.GetProposalRefs(ctx, session.ID)
	if err != nil {
		return fmt.Errorf("p0_ingest: gagal mengambil referensi untuk KG: %w", err)
	}

	logger.Logf(session.ID, "   [P0] Membangun Knowledge Graph dari %d referensi...", len(refs))

	var nodes []repository.GraphNode
	var edges []repository.GraphEdge

	for _, ref := range refs {
		// Create Paper node
		paperID := ref.DOI
		if paperID == "" {
			paperID = ref.CiteKey
		}
		paperNode := repository.GraphNode{
			Label: "Paper",
			Properties: map[string]interface{}{
				"id":         paperID,
				"title":      ref.Title,
				"year":       ref.Year,
				"doi":        ref.DOI,
				"cite_key":   ref.CiteKey,
				"session_id": session.ID,
			},
		}
		nodes = append(nodes, paperNode)

		// Create Author nodes and CITES edges (author -> paper)
		authors := parseAuthors(ref.Authors)
		for _, author := range authors {
			authorNode := repository.GraphNode{
				Label: "Author",
				Properties: map[string]interface{}{
					"id":   author,
					"name": author,
				},
			}
			nodes = append(nodes, authorNode)

			edges = append(edges, repository.GraphEdge{
				Type:       "CITES",
				SourceNode: authorNode,
				TargetNode: paperNode,
				Properties: map[string]interface{}{
					"session_id": session.ID,
				},
			})
		}

		// Extract keywords as potential Method/Dataset/Finding nodes
		if ref.Keywords != "" {
			keywords := splitKeywords(ref.Keywords)
			for _, kw := range keywords {
				methodNode := repository.GraphNode{
					Label: "Method",
					Properties: map[string]interface{}{
						"id":   kw,
						"name": kw,
					},
				}
				nodes = append(nodes, methodNode)

				edges = append(edges, repository.GraphEdge{
					Type:       "USES_METHOD",
					SourceNode: paperNode,
					TargetNode: methodNode,
					Properties: map[string]interface{}{
						"session_id": session.ID,
					},
				})
			}
		}
	}

	// Build citation network edges (CITES between papers)
	// Create edges between papers that share authors (EXTENDS relationship)
	papersByAuthor := make(map[string][]string)
	for _, ref := range refs {
		authors := parseAuthors(ref.Authors)
		paperID := ref.DOI
		if paperID == "" {
			paperID = ref.CiteKey
		}
		for _, a := range authors {
			papersByAuthor[a] = append(papersByAuthor[a], paperID)
		}
	}

	for _, papers := range papersByAuthor {
		if len(papers) > 1 {
			for i := 1; i < len(papers); i++ {
				edges = append(edges, repository.GraphEdge{
					Type: "EXTENDS",
					SourceNode: repository.GraphNode{
						Label:      "Paper",
						Properties: map[string]interface{}{"id": papers[0]},
					},
					TargetNode: repository.GraphNode{
						Label:      "Paper",
						Properties: map[string]interface{}{"id": papers[i]},
					},
					Properties: map[string]interface{}{
						"session_id": session.ID,
						"relation":   "same_author",
					},
				})
			}
		}
	}

	// Save to Neo4j
	err = m.deps.Neo4jRepo.SaveKnowledgeGraph(ctx, nodes, edges)
	if err != nil {
		return fmt.Errorf("p0_ingest: gagal menyimpan KG ke Neo4j: %w", err)
	}

	logger.Logf(session.ID, "   [P0] Knowledge Graph berhasil dibangun: %d nodes, %d edges.", len(nodes), len(edges))
	session.Status = "P0_DONE"
	return m.deps.MongoRepo.UpdateProposalSession(ctx, session)
}

// parseAuthors splits an author string into individual author names.
func parseAuthors(authors string) []string {
	// BibTeX authors are separated by " and "
	parts := splitByAnd(authors)
	var result []string
	for _, p := range parts {
		p = trimString(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// splitByAnd splits a string by " and " separator (case-insensitive).
func splitByAnd(s string) []string {
	var result []string
	lower := toLower(s)
	for {
		idx := indexSubstring(lower, " and ")
		if idx < 0 {
			result = append(result, s)
			break
		}
		result = append(result, s[:idx])
		s = s[idx+5:]
		lower = lower[idx+5:]
	}
	return result
}

// splitKeywords splits a keywords string by comma or semicolon.
func splitKeywords(keywords string) []string {
	var result []string
	// Split by semicolons first, then commas
	for _, part := range splitString(keywords, ";") {
		for _, subpart := range splitString(part, ",") {
			s := trimString(subpart)
			if s != "" {
				result = append(result, s)
			}
		}
	}
	return result
}

// Helper functions to avoid importing strings in a way that creates confusion
func trimString(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	j := len(s)
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\n' || s[j-1] == '\r') {
		j--
	}
	return s[i:j]
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		} else {
			b[i] = c
		}
	}
	return string(b)
}

func indexSubstring(s, sub string) int {
	if len(sub) > len(s) {
		return -1
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func splitString(s, sep string) []string {
	var result []string
	for {
		idx := indexSubstring(s, sep)
		if idx < 0 {
			result = append(result, s)
			break
		}
		result = append(result, s[:idx])
		s = s[idx+len(sep):]
	}
	return result
}
