package modules

import (
	"regexp"
	"sort"
	"strings"
)

// CiteGuardStats reports what the deterministic citation guard changed in a section.
type CiteGuardStats struct {
	Total      int               // total \cite key occurrences seen
	Valid      int               // keys already present in the catalog
	Remapped   int               // invalid keys remapped to a real catalog key
	Dropped    int               // invalid keys dropped (no confident match)
	RemapPairs map[string]string // invalidKey -> catalogKey (for logging)
	DroppedSet map[string]bool   // invalid keys that were dropped (for logging)
}

// citeGuardStopwords are generic tokens that must NOT drive a citation remap.
// They appear in many titles and would cause misattribution if treated as distinctive.
var citeGuardStopwords = map[string]bool{
	"based": true, "using": true, "model": true, "models": true, "study": true,
	"review": true, "systematic": true, "deep": true, "learning": true, "neural": true,
	"network": true, "networks": true, "method": true, "methods": true, "approach": true,
	"analysis": true, "data": true, "edge": true, "real": true, "time": true, "task": true,
	"cross": true, "multi": true, "modal": true, "modality": true, "gap": true, "gaps": true,
	"deployment": true, "efficiency": true, "efficient": true, "linear": true,
	"benchmark": true, "performance": true, "framework": true, "decoding": true,
	"signal": true, "signals": true, "brain": true, "interface": true, "computer": true,
	"state": true, "space": true, "with": true, "from": true, "this": true, "that": true,
	"survey": true, "novel": true, "robust": true, "domain": true, "feature": true,
}

// citeTokens splits a string into distinctive lowercase tokens (letters only, len>=4),
// excluding generic stopwords. Used to match an invented citation key to a catalog
// entry by overlap with its key/authors/title.
func citeTokens(s string) map[string]bool {
	out := make(map[string]bool)
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		t := strings.ToLower(cur.String())
		cur.Reset()
		if len(t) >= 4 && !citeGuardStopwords[t] {
			out[t] = true
		}
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

type catalogEntryTokens struct {
	key    string
	tokens map[string]bool
}

// bestCatalogMatch tries to map an invalid citation key to a real catalog key.
// Two signals, both conservative (drop is preferred over misattribution):
//
//	Signal 1 (strong): the invalid key contains a real catalog key as a substring,
//	  e.g. "wang2024femba" contains "wang2024". Longest such match wins.
//	Signal 2 (fallback): distinctive token overlap with the entry's key/authors/title,
//	  e.g. "femba_gap9_deployment" -> token "femba" matches the FEMBA paper title.
//
// Returns "" when there is no clearly distinctive winner.
func bestCatalogMatch(invalidKey string, entries []catalogEntryTokens, df map[string]int) string {
	lowerInvalid := strings.ToLower(invalidKey)

	// Signal 1: substring of a real catalog key (author-year stem decorated by the LLM).
	bestSub := ""
	for _, e := range entries {
		lk := strings.ToLower(e.key)
		if len(lk) >= 6 && strings.Contains(lowerInvalid, lk) && len(lk) > len(bestSub) {
			bestSub = e.key
		}
	}
	if bestSub != "" {
		return bestSub
	}

	// Signal 2: distinctive token overlap.
	kt := citeTokens(invalidKey)
	if len(kt) == 0 {
		return ""
	}
	bestKey, bestScore, second := "", 0.0, 0.0
	bestDistinctive := false
	for _, e := range entries {
		score := 0.0
		distinctive := false
		for t := range kt {
			if e.tokens[t] {
				w := 1.0
				if len(t) >= 5 {
					w += 1.0
				}
				if df[t] == 1 { // token unique to a single catalog entry
					w += 2.0
					distinctive = true
				}
				score += w
			}
		}
		if score > bestScore {
			second, bestScore, bestKey, bestDistinctive = bestScore, score, e.key, distinctive
		} else if score > second {
			second = score
		}
	}
	// Require a distinctive, unambiguous winner.
	if bestKey != "" && bestDistinctive && bestScore >= 3.0 && bestScore > second {
		return bestKey
	}
	return ""
}

var (
	// citeSpaceBeforePunct collapses a space left before punctuation after a \cite{} removal.
	citeSpaceBeforePunct = regexp.MustCompile(`[ \t]+([.,;:)\]])`)
	citeMultiSpace       = regexp.MustCompile(`[ \t]{2,}`)
)

// sanitizeCitations enforces that every \cite{key} references a real catalog key.
// Invalid keys are remapped to the best-matching catalog entry, or dropped when no
// confident match exists. Empty \cite{} commands left after dropping are removed,
// and the leftover whitespace is tidied. This is the deterministic safety net that
// guarantees a compilable bibliography regardless of how capable the Brain model is.
func sanitizeCitations(text string, citations []PaperCitation) (string, CiteGuardStats) {
	stats := CiteGuardStats{RemapPairs: map[string]string{}, DroppedSet: map[string]bool{}}
	if text == "" || len(citations) == 0 {
		return text, stats
	}

	valid := make(map[string]bool, len(citations))
	var entries []catalogEntryTokens
	df := map[string]int{}
	for _, c := range citations {
		if c.Key == "" {
			continue
		}
		valid[c.Key] = true
		toks := citeTokens(c.Key + " " + c.Authors + " " + c.Title)
		entries = append(entries, catalogEntryTokens{key: c.Key, tokens: toks})
		for t := range toks {
			df[t]++
		}
	}

	out := citePattern.ReplaceAllStringFunc(text, func(match string) string {
		inner := citePattern.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		var kept []string
		for _, rk := range strings.Split(inner[1], ",") {
			k := strings.TrimSpace(rk)
			if k == "" {
				continue
			}
			stats.Total++
			if valid[k] {
				stats.Valid++
				kept = appendUniqueKey(kept, k)
				continue
			}
			if mapped := bestCatalogMatch(k, entries, df); mapped != "" {
				stats.Remapped++
				stats.RemapPairs[k] = mapped
				kept = appendUniqueKey(kept, mapped)
			} else {
				stats.Dropped++
				stats.DroppedSet[k] = true
			}
		}
		if len(kept) == 0 {
			return "" // drop the now-empty \cite{}
		}
		return "\\cite{" + strings.Join(kept, ", ") + "}"
	})

	if stats.Dropped > 0 || stats.Remapped > 0 {
		out = citeSpaceBeforePunct.ReplaceAllString(out, "$1")
		out = citeMultiSpace.ReplaceAllString(out, " ")
	}
	return out, stats
}

func appendUniqueKey(keys []string, k string) []string {
	for _, e := range keys {
		if e == k {
			return keys
		}
	}
	return append(keys, k)
}

// buildAllowedKeysList renders a compact, sorted "key — Author Year" list of every
// valid catalog key. Appended to the verification and style-cleanup prompts so the
// LLM corrects \cite{} against the actual bibliography instead of inventing keys.
func buildAllowedKeysList(citations []PaperCitation) string {
	if len(citations) == 0 {
		return ""
	}
	lines := make([]string, 0, len(citations))
	for _, c := range citations {
		if c.Key == "" {
			continue
		}
		author := firstAuthorLabel(c.Authors)
		lines = append(lines, "  "+c.Key+" — "+author+" "+c.Year)
	}
	sort.Strings(lines)
	var b strings.Builder
	b.WriteString("\n\n== ALLOWED CITATION KEYS (gunakan PERSIS, jangan tambah sufiks/ubah) ==\n")
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n== END ALLOWED KEYS ==\n")
	return b.String()
}

// firstAuthorLabel returns a short "Surname" label for the first author, for the
// allowed-keys list (helps the LLM pick the right key without misattributing).
func firstAuthorLabel(authors string) string {
	s := extractFirstSurname(authors)
	if s == "" {
		return strings.TrimSpace(authors)
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
