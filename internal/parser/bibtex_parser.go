package parser

import (
	"fmt"
	"strings"

	"nsa/internal/model"
)

// ParseBibTeX parses .bib file content and returns valid ProposalRef entries
// along with a list of errors for entries that fail validation.
// Supported entry types: @article, @inproceedings, @book, @phdthesis, @mastersthesis, @misc, @techreport.
// Each entry MUST have: author, title, year, and cite_key to be considered valid.
func ParseBibTeX(content []byte) ([]model.ProposalRef, []error) {
	var refs []model.ProposalRef
	var errs []error

	entries := splitBibEntries(string(content))

	for _, entry := range entries {
		ref, err := parseBibEntry(entry)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		refs = append(refs, ref)
	}

	return refs, errs
}

// bibEntry holds the raw text of a single BibTeX entry along with its type and cite key.
type bibEntry struct {
	entryType string
	citeKey   string
	body      string
}

// splitBibEntries splits raw BibTeX content into individual entry blocks using
// a state-machine approach that tracks brace depth.
func splitBibEntries(content string) []bibEntry {
	var entries []bibEntry

	supportedTypes := map[string]bool{
		"article":       true,
		"inproceedings": true,
		"book":          true,
		"phdthesis":     true,
		"mastersthesis": true,
		"misc":          true,
		"techreport":    true,
	}

	i := 0
	for i < len(content) {
		// Look for @ symbol
		if content[i] != '@' {
			i++
			continue
		}

		// Extract entry type
		i++ // skip '@'
		typeStart := i
		for i < len(content) && content[i] != '{' && content[i] != ' ' && content[i] != '\t' {
			i++
		}
		if i >= len(content) {
			break
		}
		entryType := strings.ToLower(strings.TrimSpace(content[typeStart:i]))

		if !supportedTypes[entryType] {
			// Skip unsupported entry types (e.g. @string, @comment, @preamble)
			i++
			continue
		}

		// Skip whitespace to find opening brace
		for i < len(content) && (content[i] == ' ' || content[i] == '\t') {
			i++
		}
		if i >= len(content) || content[i] != '{' {
			continue
		}
		i++ // skip '{'

		// Extract cite key (everything up to first comma)
		keyStart := i
		for i < len(content) && content[i] != ',' && content[i] != '}' {
			i++
		}
		citeKey := strings.TrimSpace(content[keyStart:i])

		if i >= len(content) || content[i] == '}' {
			// Empty entry
			entries = append(entries, bibEntry{entryType: entryType, citeKey: citeKey, body: ""})
			if i < len(content) {
				i++
			}
			continue
		}
		i++ // skip ','

		// Now read body until matching closing brace (brace depth tracking)
		braceDepth := 1
		bodyStart := i
		for i < len(content) && braceDepth > 0 {
			switch content[i] {
			case '{':
				braceDepth++
			case '}':
				braceDepth--
			}
			i++
		}

		// body excludes the final closing brace
		bodyEnd := i - 1
		if bodyEnd < bodyStart {
			bodyEnd = bodyStart
		}
		body := content[bodyStart:bodyEnd]

		entries = append(entries, bibEntry{entryType: entryType, citeKey: citeKey, body: body})
	}

	return entries
}

// parseBibEntry parses a single BibTeX entry and validates mandatory fields.
func parseBibEntry(entry bibEntry) (model.ProposalRef, error) {
	fields := extractBibFields(entry.body)

	// Extract values
	title := fields["title"]
	author := fields["author"]
	year := fields["year"]
	journal := fields["journal"]
	if journal == "" {
		journal = fields["booktitle"]
	}
	abstract := fields["abstract"]
	doi := fields["doi"]
	keywords := fields["keywords"]

	// Validate mandatory fields
	var missing []string
	if entry.citeKey == "" {
		missing = append(missing, "cite_key")
	}
	if author == "" {
		missing = append(missing, "author")
	}
	if title == "" {
		missing = append(missing, "title")
	}
	if year == "" {
		missing = append(missing, "year")
	}

	if len(missing) > 0 {
		key := entry.citeKey
		if key == "" {
			key = "(unknown)"
		}
		return model.ProposalRef{}, fmt.Errorf("entry %q missing mandatory fields: %s", key, strings.Join(missing, ", "))
	}

	return model.ProposalRef{
		CiteKey:  entry.citeKey,
		Title:    title,
		Authors:  author,
		Year:     year,
		Journal:  journal,
		Abstract: abstract,
		DOI:      doi,
		Keywords: keywords,
	}, nil
}

// extractBibFields parses the body of a BibTeX entry and returns a map of field names to values.
// Handles values wrapped in braces {}, double quotes "", or bare numbers.
func extractBibFields(body string) map[string]string {
	fields := make(map[string]string)

	i := 0
	for i < len(body) {
		// Skip whitespace and commas
		for i < len(body) && (body[i] == ' ' || body[i] == '\t' || body[i] == '\n' || body[i] == '\r' || body[i] == ',') {
			i++
		}
		if i >= len(body) {
			break
		}

		// Extract field name (everything up to '=')
		nameStart := i
		for i < len(body) && body[i] != '=' && body[i] != ',' && body[i] != '}' {
			i++
		}
		if i >= len(body) || body[i] != '=' {
			// Not a valid field assignment, skip
			i++
			continue
		}
		fieldName := strings.ToLower(strings.TrimSpace(body[nameStart:i]))
		i++ // skip '='

		// Skip whitespace after '='
		for i < len(body) && (body[i] == ' ' || body[i] == '\t' || body[i] == '\n' || body[i] == '\r') {
			i++
		}
		if i >= len(body) {
			break
		}

		// Extract field value
		var value string
		if body[i] == '{' {
			// Brace-delimited value
			i++ // skip opening '{'
			valueStart := i
			depth := 1
			for i < len(body) && depth > 0 {
				switch body[i] {
				case '{':
					depth++
				case '}':
					depth--
				}
				i++
			}
			// value ends before the closing brace
			valueEnd := i - 1
			if valueEnd < valueStart {
				valueEnd = valueStart
			}
			value = body[valueStart:valueEnd]
		} else if body[i] == '"' {
			// Quote-delimited value
			i++ // skip opening '"'
			valueStart := i
			for i < len(body) && body[i] != '"' {
				i++
			}
			value = body[valueStart:i]
			if i < len(body) {
				i++ // skip closing '"'
			}
		} else {
			// Bare value (number or macro)
			valueStart := i
			for i < len(body) && body[i] != ',' && body[i] != '}' && body[i] != '\n' && body[i] != '\r' {
				i++
			}
			value = body[valueStart:i]
		}

		value = cleanBibValue(value)
		if fieldName != "" {
			fields[fieldName] = value
		}
	}

	return fields
}

// cleanBibValue cleans a BibTeX field value by removing excess whitespace
// and trimming leading/trailing spaces.
func cleanBibValue(s string) string {
	// Replace newlines and tabs with spaces
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\t", " ")

	// Collapse multiple spaces
	var b strings.Builder
	prevSpace := false
	for _, ch := range s {
		if ch == ' ' {
			if !prevSpace {
				b.WriteRune(ch)
			}
			prevSpace = true
		} else {
			b.WriteRune(ch)
			prevSpace = false
		}
	}

	return strings.TrimSpace(b.String())
}
