package modules

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"nsa/internal/model"
)

// CitationGate validates LaTeX citations against the proposal reference database
// and performs semantic verification using Qdrant vector search.
type CitationGate struct {
	deps *ModuleDeps
}

// NewCitationGate creates a new CitationGate instance.
func NewCitationGate(deps *ModuleDeps) *CitationGate {
	return &CitationGate{deps: deps}
}

// proposalCitePattern matches \cite{key}, \citep{key}, \citet{key} patterns including
// multiple keys separated by commas (e.g. \cite{key1,key2}).
var proposalCitePattern = regexp.MustCompile(`\\cite[pt]?\{([^}]+)\}`)

// ValidateCitations extracts citation keys from the provided LaTeX text,
// cross-checks them against the proposal_refs collection in MongoDB,
// and performs semantic search to detect misattributed claims.
func (cg *CitationGate) ValidateCitations(ctx context.Context, sessionID string, text string) (*model.CitationValidationResult, error) {
	// 1. Extract all citation keys from the text
	keys := extractCiteKeys(text)
	if len(keys) == 0 {
		return &model.CitationValidationResult{
			ValidatedText:       text,
			InvalidCitations:    nil,
			MisattributedClaims: nil,
		}, nil
	}

	// 2. Cross-check keys against MongoDB
	uniqueKeys := uniqueStrings(keys)
	valid, invalid, err := cg.deps.MongoRepo.ValidateCiteKeys(ctx, sessionID, uniqueKeys)
	if err != nil {
		return nil, fmt.Errorf("citation gate: failed to validate cite keys: %w", err)
	}

	// 3. For each valid citation, perform semantic verification.
	// Fetch all refs for the session so we can match results against specific papers.
	allRefs, err := cg.deps.MongoRepo.GetProposalRefs(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("citation gate: failed to fetch proposal refs: %w", err)
	}

	// Build a lookup map from cite_key to ref for matching
	refByCiteKey := make(map[string]model.ProposalRef)
	for _, ref := range allRefs {
		refByCiteKey[ref.CiteKey] = ref
	}

	var misattributed []string
	validSet := make(map[string]bool)
	for _, k := range valid {
		validSet[k] = true
	}

	// Find claim text around each citation for semantic search
	matches := proposalCitePattern.FindAllStringIndex(text, -1)
	for _, loc := range matches {
		citeText := text[loc[0]:loc[1]]
		// Extract key(s) from this particular citation
		subMatches := proposalCitePattern.FindStringSubmatch(citeText)
		if len(subMatches) < 2 {
			continue
		}
		citeKeys := splitCiteKeys(subMatches[1])

		// Extract surrounding claim text (sentence containing the citation)
		claimText := extractClaimContext(text, loc[0], loc[1])
		if claimText == "" {
			continue
		}

		for _, key := range citeKeys {
			key = strings.TrimSpace(key)
			if !validSet[key] {
				continue // skip invalid keys (already reported)
			}

			// Get the specific ref we expect to find
			expectedRef, exists := refByCiteKey[key]
			if !exists {
				continue
			}

			// Semantic search: does the claim match the SPECIFIC cited paper?
			results, ok := SemanticSearch(ctx, claimText, 10)
			if !ok || len(results) == 0 {
				continue // server pencarian hybrid tak tersedia atau tak ada hasil
			}

			// Cek apakah paper yang DISITASI muncul di antara hasil paling relevan.
			// Pakai CUTOFF PERINGKAT (bukan ambang skor absolut) agar tahan terhadap
			// dua skala skor SemanticSearch: cosine [0,1] (mode dense) vs RRF ≈0.01–0.05
			// (mode hybrid). Hasil sudah terurut best-first, jadi indeks = peringkat.
			const citationRankCutoff = 5 // hanya percayai paper di ~5 besar
			found := false
			for rank, r := range results {
				if rank >= citationRankCutoff {
					break
				}
				// Match by DOI if both have it
				if expectedRef.DOI != "" && r.DOI != "" && strings.EqualFold(expectedRef.DOI, r.DOI) {
					found = true
					break
				}
				// Match by title similarity (case-insensitive exact match or containment)
				if expectedRef.Title != "" && r.Title != "" {
					if strings.EqualFold(expectedRef.Title, r.Title) ||
						strings.Contains(strings.ToLower(r.Title), strings.ToLower(expectedRef.Title)) ||
						strings.Contains(strings.ToLower(expectedRef.Title), strings.ToLower(r.Title)) {
						found = true
						break
					}
				}
				// Match by cite_key appearing in snippet or title
				if expectedRef.CiteKey != "" && r.Snippet != "" {
					if strings.Contains(strings.ToLower(r.Snippet), strings.ToLower(expectedRef.CiteKey)) {
						found = true
						break
					}
				}
			}

			if !found {
				misattributed = append(misattributed, fmt.Sprintf("%s (claim: %s)", key, truncateString(claimText, 100)))
			}
		}
	}

	// 4. Build validated text (remove invalid citations)
	validatedText := removeInvalidCitations(text, invalid)

	return &model.CitationValidationResult{
		ValidatedText:       validatedText,
		InvalidCitations:    invalid,
		MisattributedClaims: misattributed,
	}, nil
}

// extractCiteKeys extracts all citation keys from LaTeX text.
func extractCiteKeys(text string) []string {
	var keys []string
	matches := proposalCitePattern.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) >= 2 {
			parts := splitCiteKeys(m[1])
			keys = append(keys, parts...)
		}
	}
	return keys
}

// splitCiteKeys splits comma-separated cite keys.
func splitCiteKeys(raw string) []string {
	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// uniqueStrings returns unique strings from a slice preserving order.
func uniqueStrings(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

// extractClaimContext extracts the sentence surrounding a citation in the text.
func extractClaimContext(text string, start, end int) string {
	// Find sentence boundaries (period, question mark, or newline)
	sentStart := start
	for sentStart > 0 {
		if text[sentStart-1] == '.' || text[sentStart-1] == '?' || text[sentStart-1] == '\n' {
			break
		}
		sentStart--
	}

	sentEnd := end
	for sentEnd < len(text) {
		if text[sentEnd] == '.' || text[sentEnd] == '?' || text[sentEnd] == '\n' {
			sentEnd++
			break
		}
		sentEnd++
	}

	claim := strings.TrimSpace(text[sentStart:sentEnd])
	// Remove the citation command itself from the claim for cleaner semantic search
	claim = proposalCitePattern.ReplaceAllString(claim, "")
	claim = strings.TrimSpace(claim)
	return claim
}

// removeInvalidCitations removes citation commands containing invalid keys from text.
func removeInvalidCitations(text string, invalidKeys []string) string {
	if len(invalidKeys) == 0 {
		return text
	}

	invalidSet := make(map[string]bool)
	for _, k := range invalidKeys {
		invalidSet[k] = true
	}

	result := proposalCitePattern.ReplaceAllStringFunc(text, func(match string) string {
		sub := proposalCitePattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		keys := splitCiteKeys(sub[1])
		var validKeys []string
		for _, k := range keys {
			k = strings.TrimSpace(k)
			if !invalidSet[k] {
				validKeys = append(validKeys, k)
			}
		}
		if len(validKeys) == 0 {
			return "" // Remove entire citation command
		}
		// Reconstruct with only valid keys
		prefix := match[:strings.Index(match, "{")+1]
		return prefix + strings.Join(validKeys, ",") + "}"
	})

	return result
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
