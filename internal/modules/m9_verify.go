package modules

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"nsa/internal/logger"
	"nsa/internal/model"
)

// citePattern matches \cite{key} or \cite{key1, key2} patterns in LaTeX text.
var citePattern = regexp.MustCompile(`\\cite\{([^}]+)\}`)

// claimCiteEntry represents a single claim+citation pair extracted from the draft text.
type claimCiteEntry struct {
	Claim string
	Key   string
}

// parseCiteClaims extracts claim text preceding each \cite{key} from the draft.
// For multi-key citations like \cite{a, b}, each key is treated as a separate entry
// sharing the same claim text.
func parseCiteClaims(draft string) []claimCiteEntry {
	matches := citePattern.FindAllStringIndex(draft, -1)
	if len(matches) == 0 {
		return nil
	}

	var entries []claimCiteEntry
	for _, loc := range matches {
		start, end := loc[0], loc[1]
		// Extract the citation keys from the match
		keyStr := draft[start:end]
		inner := citePattern.FindStringSubmatch(keyStr)
		if len(inner) < 2 {
			continue
		}

		// Extract claim text: take the preceding sentence/clause (up to 300 chars back)
		claimStart := start - 300
		if claimStart < 0 {
			claimStart = 0
		}
		preceding := draft[claimStart:start]

		// Find the last sentence boundary (period, newline, or section start)
		claim := extractClaimFromPreceding(preceding)
		if claim == "" {
			continue
		}

		// Split multi-key citations (e.g., "smith2023, lee2024")
		keys := strings.Split(inner[1], ",")
		for _, k := range keys {
			k = strings.TrimSpace(k)
			if k != "" {
				entries = append(entries, claimCiteEntry{Claim: claim, Key: k})
			}
		}
	}
	return entries
}

// extractClaimFromPreceding extracts the claim sentence from text preceding a \cite{}.
func extractClaimFromPreceding(text string) string {
	text = strings.TrimRight(text, " \t")

	// Look for the last sentence boundary: period followed by space, or newline
	lastBound := -1
	for i := len(text) - 1; i >= 0; i-- {
		if text[i] == '\n' {
			lastBound = i
			break
		}
		if text[i] == '.' && i < len(text)-1 && (text[i+1] == ' ' || text[i+1] == '\n') {
			lastBound = i + 1
			break
		}
	}

	var claim string
	if lastBound >= 0 {
		claim = text[lastBound:]
	} else {
		claim = text
	}

	claim = strings.TrimSpace(claim)
	// Remove leading LaTeX commands that are structural, not claim content
	if strings.HasPrefix(claim, "\\section") || strings.HasPrefix(claim, "\\subsection") {
		return ""
	}
	return claim
}

// verifyClaims cross-references claims in the draft against Qdrant (semantic search),
// Neo4j (paper relations), and MongoDB (extraction key_findings). Each claim is scored
// by how many sources can confirm support for the cited paper.
func (m *M9Manuscript) verifyClaims(ctx context.Context, session *model.SLRSession, draft string, citations []PaperCitation) []VerificationResult {
	entries := parseCiteClaims(draft)
	if len(entries) == 0 {
		return nil
	}

	// Build lookup maps for citations
	citationByKey := make(map[string]PaperCitation)
	for _, c := range citations {
		citationByKey[c.Key] = c
	}

	logger.Logf(session.ID, "      [Verify] Checking %d claim-citation pairs across 3 sources...", len(entries))

	var results []VerificationResult
	for _, entry := range entries {
		paper, found := citationByKey[entry.Key]
		if !found {
			// Citation key not in catalog, mark as unverifiable
			results = append(results, VerificationResult{
				Claim:       entry.Claim,
				CitationKey: entry.Key,
				Sources:     0,
			})
			continue
		}

		vr := VerificationResult{
			Claim:       entry.Claim,
			CitationKey: entry.Key,
		}

		// Source 1: Qdrant semantic search -- check if the claim has semantic match to cited paper
		vr.QdrantVerified = m.verifyViaQdrant(ctx, entry.Claim, paper.DOI)

		// Source 2: Neo4j -- check if the paper has relevant relations in the knowledge graph
		vr.Neo4jVerified = m.verifyViaNeo4j(ctx, session.ID, paper.DOI, entry.Claim)

		// Source 3: MongoDB extraction docs -- check if key_findings contains claim-related terms
		vr.MongoVerified = m.verifyViaMongo(ctx, session.ID, entry.Claim, paper.DOI)

		// Count verified sources
		if vr.QdrantVerified {
			vr.Sources++
		}
		if vr.Neo4jVerified {
			vr.Sources++
		}
		if vr.MongoVerified {
			vr.Sources++
		}

		results = append(results, vr)
	}

	verified := 0
	for _, r := range results {
		if r.Sources >= 2 {
			verified++
		}
	}
	logger.Logf(session.ID, "      [Verify] %d/%d claims verified (2+ sources)", verified, len(results))

	return results
}

// verifyViaQdrant checks if semantic search results for the claim text include
// a result matching the cited paper's DOI.
func (m *M9Manuscript) verifyViaQdrant(ctx context.Context, claim, doi string) bool {
	if doi == "" || claim == "" {
		return false
	}
	results := SemanticSearch(ctx, claim, 10)
	if len(results) == 0 {
		return false
	}
	normalizedDOI := normalizeDOIForRAG(doi)
	for _, r := range results {
		if normalizeDOIForRAG(r.DOI) == normalizedDOI {
			return true
		}
	}
	return false
}

// verifyViaNeo4j checks if the paper has relations in the knowledge graph that are
// relevant to the claim. It uses both QueryPaperRelations (for the specific paper)
// and QueryClaimEvidence (for claim-related entities) to determine if the paper's
// graph data supports the claim text.
func (m *M9Manuscript) verifyViaNeo4j(ctx context.Context, sessionID, doi, claim string) bool {
	if m.deps.Neo4jRepo == nil || doi == "" {
		return false
	}

	// First, get the paper's relations in the knowledge graph
	relations, err := m.deps.Neo4jRepo.QueryPaperRelations(ctx, sessionID, doi)
	if err != nil || len(relations) == 0 {
		return false
	}

	// Check if any relation target names or types have word overlap with the claim
	if claim != "" {
		claimWords := extractMeaningfulWords(claim)
		if len(claimWords) > 0 {
			for _, rel := range relations {
				targetWords := extractMeaningfulWords(rel.TargetName)
				typeWords := extractMeaningfulWords(rel.RelationType)
				allRelWords := append(targetWords, typeWords...)
				claimSet := make(map[string]bool)
				for _, w := range claimWords {
					claimSet[w] = true
				}
				for _, w := range allRelWords {
					if claimSet[w] {
						return true
					}
				}
			}
		}
	}

	// Additionally, use QueryClaimEvidence to check if claim keywords match
	// entity names in the graph that connect to this paper
	if claim != "" {
		// Extract key terms from the claim for graph search
		claimWords := extractMeaningfulWords(claim)
		if len(claimWords) > 0 {
			// Use the first few meaningful words as search terms
			searchTerm := ""
			limit := 3
			if len(claimWords) < limit {
				limit = len(claimWords)
			}
			for i := 0; i < limit; i++ {
				if i > 0 {
					searchTerm += " "
				}
				searchTerm += claimWords[i]
			}

			evidence, err := m.deps.Neo4jRepo.QueryClaimEvidence(ctx, searchTerm)
			if err == nil && len(evidence) > 0 {
				normalizedDOI := normalizeDOIForRAG(doi)
				for _, ev := range evidence {
					if normalizeDOIForRAG(ev.DOI) == normalizedDOI {
						return true
					}
				}
			}
		}
	}

	// Fall back: if the paper has relations but none overlap with the claim,
	// still return false to avoid false positives
	return false
}

// verifyViaMongo checks if the extraction doc for the cited paper contains key_findings
// text that has word overlap with the claim.
func (m *M9Manuscript) verifyViaMongo(ctx context.Context, sessionID, claim, doi string) bool {
	if doi == "" || claim == "" {
		return false
	}
	coll := m.deps.MongoRepo.GetExtractionCollection()
	var doc bson.M
	err := coll.FindOne(ctx, bson.M{
		"session_id": sessionID,
		"doi":        doi,
		"extracted":  true,
	}).Decode(&doc)
	if err != nil {
		// Try case-insensitive DOI match
		err = coll.FindOne(ctx, bson.M{
			"session_id": sessionID,
			"extracted":  true,
		}).Decode(&doc)
		if err != nil {
			return false
		}
		docDOI := getStr(doc, "doi", "DOI")
		if normalizeDOIForRAG(docDOI) != normalizeDOIForRAG(doi) {
			return false
		}
	}

	keyFindings := getStr(doc, "key_findings")
	if keyFindings == "" {
		return false
	}

	// Check word overlap between claim and key_findings
	return hasWordOverlap(claim, keyFindings, 3)
}

// hasWordOverlap checks if there are at least minOverlap meaningful words (4+ chars)
// shared between two text strings.
func hasWordOverlap(text1, text2 string, minOverlap int) bool {
	words1 := extractMeaningfulWords(text1)
	words2 := extractMeaningfulWords(text2)

	set2 := make(map[string]bool)
	for _, w := range words2 {
		set2[w] = true
	}

	overlap := 0
	seen := make(map[string]bool)
	for _, w := range words1 {
		if !seen[w] && set2[w] {
			overlap++
			seen[w] = true
			if overlap >= minOverlap {
				return true
			}
		}
	}
	return false
}

// extractMeaningfulWords splits text into lowercase words of 4+ characters,
// filtering out common stop words.
func extractMeaningfulWords(text string) []string {
	stopWords := map[string]bool{
		"that": true, "this": true, "with": true, "from": true, "were": true,
		"been": true, "have": true, "also": true, "which": true, "their": true,
		"more": true, "than": true, "these": true, "they": true, "some": true,
		"into": true, "such": true, "when": true, "most": true, "both": true,
		"each": true, "does": true, "only": true, "over": true, "very": true,
		"will": true, "would": true, "could": true, "should": true, "about": true,
		"other": true, "there": true, "those": true, "being": true, "after": true,
	}

	words := strings.Fields(strings.ToLower(text))
	var meaningful []string
	for _, w := range words {
		// Strip punctuation
		w = strings.Trim(w, ".,;:!?()[]{}\"'`")
		if len(w) >= 4 && !stopWords[w] {
			meaningful = append(meaningful, w)
		}
	}
	return meaningful
}

// formatVerificationResults formats the verification results into a structured
// summary that can be appended to the verification pass prompt.
func formatVerificationResults(results []VerificationResult) string {
	if len(results) == 0 {
		return "\n\n== VERIFICATION RESULTS ==\nNo claims with citations found to verify.\n== END VERIFICATION ==\n"
	}

	var b strings.Builder
	b.WriteString("\n\n== VERIFICATION RESULTS ==\n")

	verified, unverified, weak := 0, 0, 0
	for _, r := range results {
		status := "UNVERIFIED"
		if r.Sources >= 2 {
			status = "VERIFIED"
			verified++
		} else if r.Sources == 1 {
			status = "WEAK"
			weak++
		} else {
			unverified++
		}

		claimPreview := r.Claim
		if len(claimPreview) > 120 {
			claimPreview = claimPreview[:120] + "..."
		}

		b.WriteString(fmt.Sprintf("[%s] \\cite{%s}: %s (sources: %d/3 | qdrant=%v neo4j=%v mongo=%v)\n",
			status, r.CitationKey, claimPreview, r.Sources,
			r.QdrantVerified, r.Neo4jVerified, r.MongoVerified))
	}

	b.WriteString(fmt.Sprintf("\nSUMMARY: %d verified, %d weak (1 source), %d unverified (0 sources) out of %d total claims.\n",
		verified, weak, unverified, len(results)))
	b.WriteString("INSTRUCTIONS: Remove or rephrase UNVERIFIED claims. Strengthen VERIFIED claims with specific evidence. For WEAK claims, add hedging language.\n")
	b.WriteString("== END VERIFICATION ==\n")
	return b.String()
}
