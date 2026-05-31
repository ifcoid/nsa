package parser

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"regexp"
	"strings"
)

// ParsedDocument represents the normalized fields extracted from any file format
type ParsedDocument struct {
	Title        string
	Abstract     string
	DOI          string
	Year         string
	Authors      string
	Database     string
	Journal      string
	DocumentType string
}

// ParseFiles reads the content of a file based on its filename/extension and extracts ParsedDocuments
func ParseFile(filename string, content []byte) ([]ParsedDocument, error) {
	filename = strings.ToLower(filename)
	
	if strings.HasSuffix(filename, ".csv") {
		return parseCSV(content)
	} else if strings.HasSuffix(filename, ".bib") || strings.HasSuffix(filename, ".bibtex") {
		return parseBibTeX(content)
	} else if strings.HasSuffix(filename, ".nbib") || strings.HasSuffix(filename, ".txt") {
		// Attempt NBIB parsing, fallback to CSV if it fails or looks like CSV
		if bytes.Contains(content, []byte("TI  - ")) || bytes.Contains(content, []byte("PMID- ")) {
			return parseNBIB(content)
		}
		// Try CSV fallback for txt
		return parseCSV(content)
	}
	
	// Default fallback to CSV
	return parseCSV(content)
}

func parseCSV(content []byte) ([]ParsedDocument, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1 // Allow variable number of fields
	
	// Detect delimiter. Scopus sometimes uses comma, sometimes semicolon.
	firstLine := string(content)
	if idx := strings.Index(firstLine, "\n"); idx != -1 {
		firstLine = firstLine[:idx]
	}
	if strings.Count(firstLine, ";") > strings.Count(firstLine, ",") {
		reader.Comma = ';'
	} else {
		reader.Comma = ','
	}

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return []ParsedDocument{}, nil
	}

	headers := records[0]
	headerMap := make(map[string]int)
	for i, h := range headers {
		h = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(h, "\"", "")))
		headerMap[h] = i
	}

	// Map known variants
	titleIdx := getIdx(headerMap, "title", "document title", "article title")
	absIdx := getIdx(headerMap, "abstract")
	doiIdx := getIdx(headerMap, "doi")
	yearIdx := getIdx(headerMap, "year", "publication year", "pub_year")
	authIdx := getIdx(headerMap, "authors", "author", "author(s)")
	dbIdx := getIdx(headerMap, "database", "source", "publisher")
	typeIdx := getIdx(headerMap, "document type", "type", "item type", "document identifier")
	journalIdx := getIdx(headerMap, "source title", "journal", "publication title", "conference")

	var docs []ParsedDocument
	for _, row := range records[1:] {
		doc := ParsedDocument{}
		if titleIdx != -1 && titleIdx < len(row) { doc.Title = row[titleIdx] }
		if absIdx != -1 && absIdx < len(row) { doc.Abstract = row[absIdx] }
		if doiIdx != -1 && doiIdx < len(row) { doc.DOI = row[doiIdx] }
		if yearIdx != -1 && yearIdx < len(row) { doc.Year = row[yearIdx] }
		if authIdx != -1 && authIdx < len(row) { doc.Authors = row[authIdx] }
		if dbIdx != -1 && dbIdx < len(row) { doc.Database = row[dbIdx] }
		if typeIdx != -1 && typeIdx < len(row) { doc.DocumentType = row[typeIdx] }
		if journalIdx != -1 && journalIdx < len(row) { doc.Journal = row[journalIdx] }
		
		// Heuristic to guess Database if empty
		if doc.Database == "" {
			if strings.Contains(strings.ToLower(doc.Title), "ieee") || getIdx(headerMap, "ieee terms") != -1 {
				doc.Database = "IEEE"
			} else if getIdx(headerMap, "eid") != -1 { // Scopus signature
				doc.Database = "Scopus"
			} else if getIdx(headerMap, "pmid") != -1 {
				doc.Database = "PubMed"
			} else {
				doc.Database = "Unknown CSV"
			}
		}

		if doc.Title != "" {
			docs = append(docs, doc)
		}
	}
	return docs, nil
}

func parseBibTeX(content []byte) ([]ParsedDocument, error) {
	var docs []ParsedDocument
	
	// A very basic regex-based BibTeX parser
	// Matches entries like @article{id, title={...}, abstract={...}}
	entryRegex := regexp.MustCompile(`(?i)@([a-zA-Z]+)\s*\{([^,]+),`)
	
	// Split by entries
	entries := entryRegex.Split(string(content), -1)
	matches := entryRegex.FindAllStringSubmatch(string(content), -1)
	
	if len(matches) == 0 || len(entries) < 2 {
		return docs, nil // no valid bibtex found
	}
	
	for i, match := range matches {
		if i+1 >= len(entries) {
			break
		}
		
		docType := match[1]
		entryContent := entries[i+1]
		
		doc := ParsedDocument{
			DocumentType: docType,
			Database:     "BibTeX Import",
		}
		
		doc.Title = extractBibField(entryContent, "title")
		doc.Abstract = extractBibField(entryContent, "abstract")
		doc.DOI = extractBibField(entryContent, "doi")
		doc.Year = extractBibField(entryContent, "year")
		doc.Authors = extractBibField(entryContent, "author")
		
		if doc.Title != "" {
			docs = append(docs, doc)
		}
	}
	
	return docs, nil
}

func extractBibField(entry string, field string) string {
	// Matches field={value} or field="value"
	re := regexp.MustCompile(`(?i)` + field + `\s*=\s*(?:\{([^{}]*)\}|"([^"]*)")`)
	match := re.FindStringSubmatch(entry)
	if len(match) > 1 {
		if match[1] != "" {
			return strings.TrimSpace(match[1])
		}
		if len(match) > 2 {
			return strings.TrimSpace(match[2])
		}
	}
	return ""
}

func parseNBIB(content []byte) ([]ParsedDocument, error) {
	var docs []ParsedDocument
	var currentDoc *ParsedDocument
	
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var lastTag string
	
	for scanner.Scan() {
		line := scanner.Text()
		
		if line == "" {
			continue
		}
		
		// New record typically starts with PMID
		if strings.HasPrefix(line, "PMID- ") {
			if currentDoc != nil && currentDoc.Title != "" {
				docs = append(docs, *currentDoc)
			}
			currentDoc = &ParsedDocument{Database: "PubMed"}
			lastTag = "PMID"
			continue
		}
		
		if currentDoc == nil {
			// In case it doesn't start with PMID, initialize on first valid tag
			if len(line) > 4 && line[4] == '-' {
				currentDoc = &ParsedDocument{Database: "PubMed"}
			} else {
				continue
			}
		}
		
		var tag, val string
		if len(line) > 6 && line[4] == '-' {
			tag = strings.TrimSpace(line[:4])
			val = strings.TrimSpace(line[6:])
			lastTag = tag
		} else if strings.HasPrefix(line, "      ") && lastTag != "" { // Continuation line
			val = strings.TrimSpace(line)
			tag = lastTag
		} else {
			continue
		}
		
		switch tag {
		case "TI":
			if currentDoc.Title != "" {
				currentDoc.Title += " " + val
			} else {
				currentDoc.Title = val
			}
		case "AB":
			if currentDoc.Abstract != "" {
				currentDoc.Abstract += " " + val
			} else {
				currentDoc.Abstract = val
			}
		case "DP": // Date of Publication (e.g. 2021 May 5)
			if currentDoc.Year == "" && len(val) >= 4 {
				currentDoc.Year = val[:4]
			}
		case "FAU": // Full Author
			if currentDoc.Authors != "" {
				currentDoc.Authors += "; " + val
			} else {
				currentDoc.Authors = val
			}
		case "LID": // Location ID, often contains DOI
			if strings.HasSuffix(val, "[doi]") {
				currentDoc.DOI = strings.TrimSpace(strings.TrimSuffix(val, "[doi]"))
			}
		case "PT": // Publication Type
			if currentDoc.DocumentType == "" {
				currentDoc.DocumentType = val
			}
		}
	}
	
	if currentDoc != nil && currentDoc.Title != "" {
		docs = append(docs, *currentDoc)
	}
	
	return docs, nil
}

func getIdx(headerMap map[string]int, keys ...string) int {
	for _, k := range keys {
		if idx, ok := headerMap[k]; ok {
			return idx
		}
	}
	return -1
}
