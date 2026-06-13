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
	Title          string
	Abstract       string
	DOI            string
	Year           string
	Authors        string
	Database       string
	Journal        string
	DocumentType   string
	Keywords       string // Author Keywords (semicolon separated)
	IndexKeywords  string // Index Keywords / IEEE Terms / MeSH
	Affiliations   string
	Volume         string
	Issue          string
	PageStart      string
	PageEnd        string
	ISSN           string
	ISBN           string
	Publisher      string
	Language       string
	FundingDetails string
	CitedBy        string
	ConferenceName string
	EID            string
	PubMedID       string
	References     string
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
	keywordsIdx := getIdx(headerMap, "author keywords")
	indexKwIdx := getIdx(headerMap, "index keywords", "ieee terms", "mesh_terms")
	affiliationsIdx := getIdx(headerMap, "affiliations", "author affiliations")
	volumeIdx := getIdx(headerMap, "volume")
	issueIdx := getIdx(headerMap, "issue")
	pageStartIdx := getIdx(headerMap, "start page", "page start")
	pageEndIdx := getIdx(headerMap, "end page", "page end")
	issnIdx := getIdx(headerMap, "issn")
	isbnIdx := getIdx(headerMap, "isbn", "isbns")
	publisherIdx := getIdx(headerMap, "publisher")
	languageIdx := getIdx(headerMap, "language", "language of original document")
	fundingIdx := getIdx(headerMap, "funding details", "funding information")
	citedByIdx := getIdx(headerMap, "cited by", "article citation count")
	confNameIdx := getIdx(headerMap, "conference name")
	eidIdx := getIdx(headerMap, "eid")
	pubmedIdx := getIdx(headerMap, "pubmed id")
	referencesIdx := getIdx(headerMap, "references")

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
		if keywordsIdx != -1 && keywordsIdx < len(row) { doc.Keywords = row[keywordsIdx] }
		if indexKwIdx != -1 && indexKwIdx < len(row) { doc.IndexKeywords = row[indexKwIdx] }
		if affiliationsIdx != -1 && affiliationsIdx < len(row) { doc.Affiliations = row[affiliationsIdx] }
		if volumeIdx != -1 && volumeIdx < len(row) { doc.Volume = row[volumeIdx] }
		if issueIdx != -1 && issueIdx < len(row) { doc.Issue = row[issueIdx] }
		if pageStartIdx != -1 && pageStartIdx < len(row) { doc.PageStart = row[pageStartIdx] }
		if pageEndIdx != -1 && pageEndIdx < len(row) { doc.PageEnd = row[pageEndIdx] }
		if issnIdx != -1 && issnIdx < len(row) { doc.ISSN = row[issnIdx] }
		if isbnIdx != -1 && isbnIdx < len(row) { doc.ISBN = row[isbnIdx] }
		if publisherIdx != -1 && publisherIdx < len(row) { doc.Publisher = row[publisherIdx] }
		if languageIdx != -1 && languageIdx < len(row) { doc.Language = row[languageIdx] }
		if fundingIdx != -1 && fundingIdx < len(row) { doc.FundingDetails = row[fundingIdx] }
		if citedByIdx != -1 && citedByIdx < len(row) { doc.CitedBy = row[citedByIdx] }
		if confNameIdx != -1 && confNameIdx < len(row) { doc.ConferenceName = row[confNameIdx] }
		if eidIdx != -1 && eidIdx < len(row) { doc.EID = row[eidIdx] }
		if pubmedIdx != -1 && pubmedIdx < len(row) { doc.PubMedID = row[pubmedIdx] }
		if referencesIdx != -1 && referencesIdx < len(row) { doc.References = row[referencesIdx] }
		
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
	
	// Accumulators for multi-value fields
	var otVals []string  // OT = Author Keywords
	var mhVals []string  // MH = MeSH Headings (Index Keywords)
	var adVals []string  // AD = Affiliations
	var grVals []string  // GR = Grants/Funding
	
	flushAccumulators := func() {
		if currentDoc == nil {
			return
		}
		if len(otVals) > 0 {
			currentDoc.Keywords = strings.Join(otVals, "; ")
		}
		if len(mhVals) > 0 {
			currentDoc.IndexKeywords = strings.Join(mhVals, "; ")
		}
		if len(adVals) > 0 {
			currentDoc.Affiliations = strings.Join(adVals, "; ")
		}
		if len(grVals) > 0 {
			currentDoc.FundingDetails = strings.Join(grVals, "; ")
		}
	}
	
	resetAccumulators := func() {
		otVals = nil
		mhVals = nil
		adVals = nil
		grVals = nil
	}
	
	for scanner.Scan() {
		line := scanner.Text()
		
		if line == "" {
			continue
		}
		
		// New record typically starts with PMID
		if strings.HasPrefix(line, "PMID- ") {
			if currentDoc != nil && currentDoc.Title != "" {
				flushAccumulators()
				docs = append(docs, *currentDoc)
			}
			currentDoc = &ParsedDocument{Database: "PubMed"}
			resetAccumulators()
			currentDoc.PubMedID = strings.TrimSpace(line[6:])
			lastTag = "PMID"
			continue
		}
		
		if currentDoc == nil {
			// In case it doesn't start with PMID, initialize on first valid tag
			if len(line) > 4 && line[4] == '-' {
				currentDoc = &ParsedDocument{Database: "PubMed"}
				resetAccumulators()
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
		case "AID": // Article Identifier, fallback DOI
			if strings.HasSuffix(val, "[doi]") && currentDoc.DOI == "" {
				currentDoc.DOI = strings.TrimSpace(strings.TrimSuffix(val, "[doi]"))
			}
		case "PT": // Publication Type
			if currentDoc.DocumentType == "" {
				currentDoc.DocumentType = val
			}
		case "OT": // Other Term (Author Keywords)
			otVals = append(otVals, val)
		case "MH": // MeSH Heading (Index Keywords)
			// Strip asterisk prefix and take before slash
			cleaned := strings.TrimPrefix(val, "*")
			if slashIdx := strings.Index(cleaned, "/"); slashIdx != -1 {
				cleaned = cleaned[:slashIdx]
			}
			mhVals = append(mhVals, strings.TrimSpace(cleaned))
		case "AD": // Affiliation
			adVals = append(adVals, val)
		case "LA": // Language
			if currentDoc.Language == "" {
				currentDoc.Language = val
			}
		case "GR": // Grant Number / Funding
			grVals = append(grVals, val)
		case "JT": // Journal Title
			if currentDoc.Journal == "" {
				currentDoc.Journal = val
			}
		case "VI": // Volume
			if currentDoc.Volume == "" {
				currentDoc.Volume = val
			}
		case "IP": // Issue/Part
			if currentDoc.Issue == "" {
				currentDoc.Issue = val
			}
		case "PG": // Pagination
			if currentDoc.PageStart == "" {
				parts := strings.SplitN(val, "-", 2)
				currentDoc.PageStart = strings.TrimSpace(parts[0])
				if len(parts) > 1 {
					currentDoc.PageEnd = strings.TrimSpace(parts[1])
				}
			}
		case "IS": // ISSN
			if currentDoc.ISSN == "" {
				currentDoc.ISSN = val
			}
		}
	}
	
	if currentDoc != nil && currentDoc.Title != "" {
		flushAccumulators()
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
