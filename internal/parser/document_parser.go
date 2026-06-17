package parser

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"io"
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

// ParseFile reads the content of a file based on its filename/extension and extracts ParsedDocuments
func ParseFile(filename string, content []byte) ([]ParsedDocument, error) {
	filename = strings.ToLower(filename)

	// Strip UTF-8 BOM if present (common in Windows exports from Scopus, etc.)
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))

	var docs []ParsedDocument
	var err error
	switch {
	case strings.HasSuffix(filename, ".csv"):
		docs, err = parseCSV(content)
	case strings.HasSuffix(filename, ".bib"), strings.HasSuffix(filename, ".bibtex"):
		docs, err = parseBibTeX(content)
	case strings.HasSuffix(filename, ".nbib"):
		docs, err = parseNBIB(content)
	case strings.HasSuffix(filename, ".txt"):
		// Detect content format for .txt files (PubMed/NBIB, RIS, BibTeX, or CSV)
		docs, err = parseTxtByContent(content)
	default:
		docs, err = parseCSV(content)
	}

	// Safety net against silent total-loss: a known export format misnamed by extension
	// (e.g. an IEEE Xplore BibTeX dump saved as .txt/.csv, or a Scopus CSV saved as .txt)
	// must NOT vanish without a trace. If the extension-based parser found nothing but the
	// file clearly has content, retry by sniffing the actual content format.
	if len(docs) == 0 && len(bytes.TrimSpace(content)) > 0 {
		if sniffed, serr := parseTxtByContent(content); len(sniffed) > 0 {
			return sniffed, serr
		}
	}
	return docs, err
}

// parseTxtByContent detects the format of a .txt file by inspecting its content.
// It supports PubMed/NBIB tagged format, RIS format, and falls back to CSV.
func parseTxtByContent(content []byte) ([]ParsedDocument, error) {
	// Check for PubMed/NBIB format markers (tagged format with 4-char tags)
	if bytes.Contains(content, []byte("PMID- ")) ||
		bytes.Contains(content, []byte("TI  - ")) ||
		bytes.Contains(content, []byte("FAU - ")) ||
		bytes.Contains(content, []byte("AU  - ")) ||
		bytes.Contains(content, []byte("AB  - ")) {
		return parseNBIB(content)
	}

	// Check for RIS format (used by IEEE and others)
	if bytes.Contains(content, []byte("TY  - ")) ||
		(bytes.Contains(content, []byte("T1  - ")) && bytes.Contains(content, []byte("ER  - "))) {
		return parseRIS(content)
	}

	// Check for BibTeX format. IEEE Xplore's "Download > BibTeX" produces @ARTICLE{...}
	// entries; users often save these as .txt. Without this branch the file falls through
	// to CSV and silently yields ZERO records (the whole database vanishes from the count).
	if bibtexEntryRe.Match(content) {
		return parseBibTeX(content)
	}

	// Fallback to CSV
	return parseCSV(content)
}

// bibtexEntryRe matches a BibTeX entry header like "@article{", "@inproceedings {",
// tolerant of leading whitespace and case (IEEE uses uppercase @ARTICLE).
var bibtexEntryRe = regexp.MustCompile(`(?i)@(article|inproceedings|incollection|inbook|book|booklet|conference|manual|mastersthesis|misc|phdthesis|proceedings|techreport|unpublished)\s*\{`)

// readCSVRecords membaca header + records dengan satu mode kutip (lazy/strict).
// Per-baris + SKIP baris ParseError (bukan break): satu baris rusak tidak membuang sisanya.
// encoding/csv pulih ke baris berikutnya setelah ParseError. Hanya io.EOF / error non-parse
// yang mengakhiri loop. Mengembalikan (headers, records, jumlahDilewati).
func readCSVRecords(content []byte, comma rune, lazy bool) ([]string, [][]string, int) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.Comma = comma
	reader.LazyQuotes = lazy
	reader.FieldsPerRecord = -1 // izinkan jumlah field bervariasi

	headers, err := reader.Read()
	if err != nil {
		return nil, nil, 0
	}

	var records [][]string
	skipped := 0
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				skipped++
				continue // baris rusak — lewati, lanjut baca sisanya
			}
			break // error non-parse (mis. I/O) — tak bisa dipulihkan
		}
		records = append(records, row)
	}
	return headers, records, skipped
}

func parseCSV(content []byte) ([]ParsedDocument, error) {
	// Deteksi delimiter dari baris pertama. Scopus kadang comma, kadang semicolon.
	firstLine := string(content)
	if idx := strings.Index(firstLine, "\n"); idx != -1 {
		firstLine = firstLine[:idx]
	}
	comma := ','
	if strings.Count(firstLine, ";") > strings.Count(firstLine, ",") {
		comma = ';'
	}

	// LazyQuotes=true DIAM-DIAM menelan baris saat kutip tak seimbang (mis. abstrak Scopus
	// dgn kutip nyasar) -> under-count parah (201 jadi 139). LazyQuotes=false memunculkan
	// ParseError yg kita SKIP per-baris, jadi baris valid lain tetap terbaca utuh. Tapi file
	// yg penuh kutip-nyasar bisa membuat strict men-skip banyak. Maka: coba KEDUA mode, pakai
	// yang memulihkan record TERBANYAK (jaminan tidak pernah lebih buruk dari sebelumnya).
	headers, records, _ := readCSVRecords(content, comma, false) // strict
	if hL, rL, _ := readCSVRecords(content, comma, true); len(rL) > len(records) {
		headers, records = hL, rL
	}
	if headers == nil {
		return []ParsedDocument{}, nil
	}

	headerMap := make(map[string]int)
	for i, h := range headers {
		h = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(h, "\"", "")))
		h = strings.TrimPrefix(h, "\ufeff") // Strip any remaining BOM chars
		headerMap[h] = i
	}

	if len(records) == 0 {
		return []ParsedDocument{}, nil
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
	for _, row := range records {
		doc := ParsedDocument{}
		if titleIdx != -1 && titleIdx < len(row) {
			doc.Title = row[titleIdx]
		}
		if absIdx != -1 && absIdx < len(row) {
			doc.Abstract = row[absIdx]
		}
		if doiIdx != -1 && doiIdx < len(row) {
			doc.DOI = row[doiIdx]
		}
		if yearIdx != -1 && yearIdx < len(row) {
			doc.Year = row[yearIdx]
		}
		if authIdx != -1 && authIdx < len(row) {
			doc.Authors = row[authIdx]
		}
		if dbIdx != -1 && dbIdx < len(row) {
			doc.Database = row[dbIdx]
		}
		if typeIdx != -1 && typeIdx < len(row) {
			doc.DocumentType = row[typeIdx]
		}
		if journalIdx != -1 && journalIdx < len(row) {
			doc.Journal = row[journalIdx]
		}
		if keywordsIdx != -1 && keywordsIdx < len(row) {
			doc.Keywords = row[keywordsIdx]
		}
		if indexKwIdx != -1 && indexKwIdx < len(row) {
			doc.IndexKeywords = row[indexKwIdx]
		}
		if affiliationsIdx != -1 && affiliationsIdx < len(row) {
			doc.Affiliations = row[affiliationsIdx]
		}
		if volumeIdx != -1 && volumeIdx < len(row) {
			doc.Volume = row[volumeIdx]
		}
		if issueIdx != -1 && issueIdx < len(row) {
			doc.Issue = row[issueIdx]
		}
		if pageStartIdx != -1 && pageStartIdx < len(row) {
			doc.PageStart = row[pageStartIdx]
		}
		if pageEndIdx != -1 && pageEndIdx < len(row) {
			doc.PageEnd = row[pageEndIdx]
		}
		if issnIdx != -1 && issnIdx < len(row) {
			doc.ISSN = row[issnIdx]
		}
		if isbnIdx != -1 && isbnIdx < len(row) {
			doc.ISBN = row[isbnIdx]
		}
		if publisherIdx != -1 && publisherIdx < len(row) {
			doc.Publisher = row[publisherIdx]
		}
		if languageIdx != -1 && languageIdx < len(row) {
			doc.Language = row[languageIdx]
		}
		if fundingIdx != -1 && fundingIdx < len(row) {
			doc.FundingDetails = row[fundingIdx]
		}
		if citedByIdx != -1 && citedByIdx < len(row) {
			doc.CitedBy = row[citedByIdx]
		}
		if confNameIdx != -1 && confNameIdx < len(row) {
			doc.ConferenceName = row[confNameIdx]
		}
		if eidIdx != -1 && eidIdx < len(row) {
			doc.EID = row[eidIdx]
		}
		if pubmedIdx != -1 && pubmedIdx < len(row) {
			doc.PubMedID = row[pubmedIdx]
		}
		if referencesIdx != -1 && referencesIdx < len(row) {
			doc.References = row[referencesIdx]
		}

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

	// Split content into individual BibTeX entries using a state machine
	entries := splitBibEntriesRaw(string(content))

	entryRegex := regexp.MustCompile(`(?i)@([a-zA-Z]+)\s*\{([^,]*),`)

	for _, entry := range entries {
		// Extract entry type and cite key from the header
		headerMatch := entryRegex.FindStringSubmatch(entry)
		if headerMatch == nil {
			continue
		}

		docType := headerMatch[1]

		// Skip non-document entries (metadata entries)
		docTypeLower := strings.ToLower(docType)
		if docTypeLower == "string" || docTypeLower == "comment" || docTypeLower == "preamble" {
			continue
		}

		// Parse all fields using brace-counting
		fields := extractAllBibFields(entry)

		doc := ParsedDocument{
			DocumentType: docType,
		}

		doc.Title = fields["title"]
		doc.Abstract = fields["abstract"]
		doc.DOI = fields["doi"]
		doc.Year = fields["year"]
		doc.Authors = fields["author"]
		doc.Journal = fields["journal"]
		if doc.Journal == "" {
			doc.Journal = fields["booktitle"]
		}
		doc.Volume = fields["volume"]
		doc.Issue = fields["number"]
		if doc.Issue == "" {
			doc.Issue = fields["issue"]
		}
		doc.ISSN = fields["issn"]
		doc.ISBN = fields["isbn"]
		doc.Publisher = fields["publisher"]
		doc.Keywords = fields["keywords"]
		doc.Language = fields["language"]
		doc.EID = fields["eid"]
		doc.PubMedID = fields["pmid"]

		// Handle pages field (may be range like "1--10" or "1-10")
		if pages := fields["pages"]; pages != "" {
			pages = strings.ReplaceAll(pages, "--", "-")
			parts := strings.SplitN(pages, "-", 2)
			doc.PageStart = strings.TrimSpace(parts[0])
			if len(parts) > 1 {
				doc.PageEnd = strings.TrimSpace(parts[1])
			}
		}

		// Detect Database from content heuristics
		doc.Database = detectBibDatabase(doc, fields)

		if doc.Title != "" {
			docs = append(docs, doc)
		}
	}

	return docs, nil
}

// splitBibEntriesRaw splits BibTeX content into individual entry strings using brace counting.
// Returns raw string entries for the ParsedDocument parser (separate from splitBibEntries used by ParseBibTeX).
func splitBibEntriesRaw(content string) []string {
	var entries []string
	i := 0
	for i < len(content) {
		// Find next @ that starts an entry
		atIdx := strings.Index(content[i:], "@")
		if atIdx == -1 {
			break
		}
		start := i + atIdx

		// Find the opening brace of the entry
		braceStart := strings.Index(content[start:], "{")
		if braceStart == -1 {
			break
		}
		braceStart += start

		// Count braces to find the matching close
		depth := 0
		end := braceStart
		for end < len(content) {
			if content[end] == '{' {
				depth++
			} else if content[end] == '}' {
				depth--
				if depth == 0 {
					entries = append(entries, content[start:end+1])
					break
				}
			}
			end++
		}
		if depth != 0 {
			// Unmatched braces, take rest of content as entry
			entries = append(entries, content[start:])
			break
		}
		i = end + 1
	}
	return entries
}

// extractAllBibFields parses all field=value pairs from a BibTeX entry using brace counting.
// Handles nested braces and multi-line values correctly.
func extractAllBibFields(entry string) map[string]string {
	fields := make(map[string]string)

	// Find the first { after @type to skip the header (entry type + cite key)
	firstBrace := strings.Index(entry, "{")
	if firstBrace == -1 {
		return fields
	}

	// Find the first comma after the cite key
	body := entry[firstBrace+1:]
	firstComma := strings.Index(body, ",")
	if firstComma == -1 {
		return fields
	}
	body = body[firstComma+1:]

	// Now parse field = value pairs
	// field names are alphanumeric, followed by =, then value in {} or ""
	fieldRegex := regexp.MustCompile(`(?i)([a-zA-Z_][a-zA-Z0-9_]*)\s*=\s*`)

	for {
		body = strings.TrimSpace(body)
		if body == "" || body == "}" {
			break
		}

		loc := fieldRegex.FindStringSubmatchIndex(body)
		if loc == nil {
			break
		}

		fieldName := strings.ToLower(body[loc[2]:loc[3]])
		rest := body[loc[1]:]

		var value string
		var consumed int

		rest = strings.TrimSpace(rest)
		if len(rest) == 0 {
			break
		}

		if rest[0] == '{' {
			// Extract value using brace counting
			value, consumed = extractBracedValue(rest)
		} else if rest[0] == '"' {
			// Extract value between quotes (no nested brace handling needed)
			endQuote := strings.Index(rest[1:], "\"")
			if endQuote == -1 {
				break
			}
			value = rest[1 : endQuote+1]
			consumed = endQuote + 2
		} else {
			// Bare value (number or macro), read until comma or closing brace
			endIdx := strings.IndexAny(rest, ",}")
			if endIdx == -1 {
				value = strings.TrimSpace(rest)
				consumed = len(rest)
			} else {
				value = strings.TrimSpace(rest[:endIdx])
				consumed = endIdx
			}
		}

		// Clean up the value: collapse whitespace from multi-line values
		value = strings.Join(strings.Fields(value), " ")
		fields[fieldName] = value

		// Move past the consumed value and optional comma
		remaining := rest[consumed:]
		remaining = strings.TrimSpace(remaining)
		if len(remaining) > 0 && remaining[0] == ',' {
			remaining = remaining[1:]
		}
		body = remaining
	}

	return fields
}

// extractBracedValue extracts a value enclosed in braces, handling nested braces correctly.
// Returns the inner content and the total number of characters consumed (including outer braces).
func extractBracedValue(s string) (string, int) {
	if len(s) == 0 || s[0] != '{' {
		return "", 0
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '{' {
			depth++
		} else if s[i] == '}' {
			depth--
			if depth == 0 {
				return s[1:i], i + 1
			}
		}
	}
	// Unmatched, return what we have
	return s[1:], len(s)
}

// detectBibDatabase determines the source database from BibTeX entry content.
func detectBibDatabase(doc ParsedDocument, fields map[string]string) string {
	// Check DOI prefix for IEEE
	if strings.Contains(doc.DOI, "10.1109") {
		return "IEEE"
	}
	// Check for Scopus signature (eid field present)
	if doc.EID != "" || fields["eid"] != "" {
		return "Scopus"
	}
	// Check for PubMed signature (pmid field present)
	if doc.PubMedID != "" || fields["pmid"] != "" {
		return "PubMed"
	}
	return "BibTeX Import"
}

func parseNBIB(content []byte) ([]ParsedDocument, error) {
	var docs []ParsedDocument
	var currentDoc *ParsedDocument

	scanner := bufio.NewScanner(bytes.NewReader(content))
	// Increase scanner buffer for large lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lastTag string

	// Accumulators for multi-value fields
	var otVals []string // OT = Author Keywords
	var mhVals []string // MH = MeSH Headings (Index Keywords)
	var adVals []string // AD = Affiliations
	var grVals []string // GR = Grants/Funding

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

	finalizeRecord := func() {
		if currentDoc != nil && currentDoc.Title != "" {
			flushAccumulators()
			docs = append(docs, *currentDoc)
		}
		currentDoc = nil
		resetAccumulators()
		lastTag = ""
	}

	for scanner.Scan() {
		line := scanner.Text()

		// In PubMed NBIB/MEDLINE format, blank lines are just formatting
		// whitespace within records. Records are separated by PMID- tags
		// or ER (End of Record) tags, NOT by blank lines.
		if line == "" {
			continue
		}

		// Handle ER (End of Record) tag - official MEDLINE record terminator
		if strings.HasPrefix(line, "ER  -") || strings.HasPrefix(line, "ER  ") {
			finalizeRecord()
			continue
		}

		// New record typically starts with PMID
		if strings.HasPrefix(line, "PMID- ") {
			// Finalize previous record if any
			if currentDoc != nil && currentDoc.Title != "" {
				finalizeRecord()
			} else if currentDoc != nil {
				// Discard incomplete record without title
				currentDoc = nil
				resetAccumulators()
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

// parseRIS parses RIS (Research Information Systems) format commonly used by IEEE exports.
func parseRIS(content []byte) ([]ParsedDocument, error) {
	var docs []ParsedDocument
	var currentDoc *ParsedDocument

	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var authors []string
	var keywords []string

	finalizeRecord := func() {
		if currentDoc != nil && currentDoc.Title != "" {
			if len(authors) > 0 {
				currentDoc.Authors = strings.Join(authors, "; ")
			}
			if len(keywords) > 0 {
				currentDoc.Keywords = strings.Join(keywords, "; ")
			}
			docs = append(docs, *currentDoc)
		}
		currentDoc = nil
		authors = nil
		keywords = nil
	}

	for scanner.Scan() {
		line := scanner.Text()

		// End of Record
		if strings.HasPrefix(line, "ER  -") || strings.TrimSpace(line) == "ER  -" {
			finalizeRecord()
			continue
		}

		// Empty line - could be record separator
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Start of new record
		if strings.HasPrefix(line, "TY  - ") {
			if currentDoc != nil {
				finalizeRecord()
			}
			currentDoc = &ParsedDocument{}
			authors = nil
			keywords = nil
			docType := strings.TrimSpace(line[6:])
			currentDoc.DocumentType = docType
			continue
		}

		// If no current doc and we see a valid tag, start one
		if currentDoc == nil {
			if len(line) > 6 && line[2] == ' ' && line[3] == ' ' && line[4] == '-' && line[5] == ' ' {
				currentDoc = &ParsedDocument{}
				authors = nil
				keywords = nil
			} else {
				continue
			}
		}

		var tag, val string
		if len(line) > 6 && line[2] == ' ' && line[3] == ' ' && line[4] == '-' && line[5] == ' ' {
			tag = strings.TrimSpace(line[:2])
			val = strings.TrimSpace(line[6:])
		} else if len(line) > 5 && line[2] == ' ' && line[3] == ' ' && line[4] == '-' {
			// Tag with empty value
			tag = strings.TrimSpace(line[:2])
			val = ""
		} else {
			// Continuation or unrecognized - skip
			continue
		}

		switch tag {
		case "TI", "T1": // Title
			if currentDoc.Title != "" {
				currentDoc.Title += " " + val
			} else {
				currentDoc.Title = val
			}
		case "AB", "N2": // Abstract
			if currentDoc.Abstract != "" {
				currentDoc.Abstract += " " + val
			} else {
				currentDoc.Abstract = val
			}
		case "DO", "DOI": // DOI
			if currentDoc.DOI == "" {
				currentDoc.DOI = val
			}
		case "PY", "Y1", "DA": // Year
			if currentDoc.Year == "" && len(val) >= 4 {
				currentDoc.Year = val[:4]
			}
		case "AU", "A1": // Author
			if val != "" {
				authors = append(authors, val)
			}
		case "KW": // Keywords
			if val != "" {
				keywords = append(keywords, val)
			}
		case "JO", "JF", "T2": // Journal / Conference name
			if currentDoc.Journal == "" {
				currentDoc.Journal = val
			}
		case "VL": // Volume
			if currentDoc.Volume == "" {
				currentDoc.Volume = val
			}
		case "IS": // Issue
			if currentDoc.Issue == "" {
				currentDoc.Issue = val
			}
		case "SP": // Start Page
			if currentDoc.PageStart == "" {
				currentDoc.PageStart = val
			}
		case "EP": // End Page
			if currentDoc.PageEnd == "" {
				currentDoc.PageEnd = val
			}
		case "SN": // ISSN/ISBN
			if currentDoc.ISSN == "" {
				currentDoc.ISSN = val
			}
		case "PB": // Publisher
			if currentDoc.Publisher == "" {
				currentDoc.Publisher = val
			}
		case "LA": // Language
			if currentDoc.Language == "" {
				currentDoc.Language = val
			}
		case "DB": // Database
			if currentDoc.Database == "" {
				currentDoc.Database = val
			}
		}
	}

	// Finalize last record
	finalizeRecord()

	// Set database for entries without explicit DB tag
	for i := range docs {
		if docs[i].Database == "" {
			// Detect based on DOI or other heuristics
			if strings.Contains(docs[i].DOI, "10.1109") {
				docs[i].Database = "IEEE"
			} else {
				docs[i].Database = "RIS Import"
			}
		}
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
