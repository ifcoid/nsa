package modules

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/ifcoid/refs"
	"nsa/internal/logger"
	"nsa/internal/repository"
)

// hasStudyDesign checks whether a doc already has a non-empty study_design value
// in its fields array. Returns true if enrichment can be skipped.
func hasStudyDesign(doc bson.M) bool {
	arr, ok := doc["fields"].(primitive.A)
	if !ok {
		return false
	}
	for _, it := range arr {
		f, ok := it.(bson.M)
		if !ok {
			continue
		}
		k, _ := f["key"].(string)
		if strings.Contains(strings.ToLower(k), "design") || strings.Contains(strings.ToLower(k), "study_type") {
			v, _ := f["value"].(string)
			if v != "" && v != "[NOT REPORTED]" {
				return true
			}
		}
	}
	return false
}

// EnrichMetadataFromCrossRef queries ALL extraction docs for this session, checks
// which ones are missing study_design data, and enriches them from CrossRef.
// It tries DOI lookup first, then falls back to title search.
// It returns the number of documents enriched.
func EnrichMetadataFromCrossRef(ctx context.Context, mongoRepo *repository.MongoRepository, sessionID string) (int, error) {
	coll := mongoRepo.GetExtractionCollection()

	// Query ALL docs for this session
	filter := bson.M{"session_id": sessionID}

	totalDocs, _ := coll.CountDocuments(ctx, filter)
	logger.Logf(sessionID, "   [Enrich] Session: %s, total docs in collection: %d", sessionID, totalDocs)

	cur, err := coll.Find(ctx, filter)
	if err != nil {
		return 0, err
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return 0, err
	}

	// Filter to only docs that need enrichment (missing study_design)
	var needsEnrich []bson.M
	for _, doc := range docs {
		if !hasStudyDesign(doc) {
			needsEnrich = append(needsEnrich, doc)
		}
	}

	logger.Logf(sessionID, "   [Enrich] %d/%d docs need enrichment (missing study_design)", len(needsEnrich), len(docs))

	if len(needsEnrich) == 0 {
		logger.Logf(sessionID, "   [Enrich] All docs already have study_design. Nothing to do.")
		return 0, nil
	}

	enriched := 0
	for i, doc := range needsEnrich {
		if ctx.Err() != nil {
			return enriched, ctx.Err()
		}

		title := getStr(doc, "Title", "title")
		doi := getStr(doc, "DOI", "doi")

		// Normalize DOI (strip URL prefix)
		doi = strings.TrimPrefix(doi, "https://doi.org/")
		doi = strings.TrimPrefix(doi, "http://doi.org/")
		doi = strings.TrimSpace(doi)
		if doi == "-" {
			doi = ""
		}

		logger.Logf(sessionID, "   [Enrich] [%d/%d] Processing: %s (DOI: %s)", i+1, len(needsEnrich), truncTitle(title, 60), doi)

		var work *refs.CrossrefWork

		// Strategy 1: DOI lookup
		if doi != "" {
			w, err := refs.GetCrossrefWork(doi)
			if err != nil {
				logger.Logf(sessionID, "   [Enrich] DOI lookup failed for %s: %v, trying title search...", doi, err)
			} else {
				work = w
			}
		}

		// Strategy 2: Title search fallback
		if work == nil && title != "" {
			logger.Logf(sessionID, "   [Enrich] Searching CrossRef by title: %s", truncTitle(title, 80))
			resp, err := refs.SearchCrossrefWorks(title, 1, 0)
			if err != nil {
				logger.Logf(sessionID, "   [Enrich] Title search failed: %v", err)
			} else if resp != nil && len(resp.Message.Items) > 0 {
				work = &resp.Message.Items[0]
				logger.Logf(sessionID, "   [Enrich] Title search found match (type: %s)", work.Type)
			} else {
				logger.Logf(sessionID, "   [Enrich] Title search returned no results")
			}
		}

		if work == nil {
			logger.Logf(sessionID, "   [Enrich] [%d/%d] No CrossRef data found, skipping", i+1, len(needsEnrich))
			time.Sleep(2 * time.Second)
			continue
		}

		// Build enrichment fields from CrossRef work
		newFields := buildFieldsFromCrossRef(work)
		if len(newFields) == 0 {
			time.Sleep(2 * time.Second)
			continue
		}

		// Merge new fields into existing fields array
		existingArr, _ := doc["fields"].(primitive.A)
		mergedFields := mergeEnrichFields(existingArr, newFields)

		update := bson.M{
			"$set": bson.M{
				"fields":        mergedFields,
				"enriched_from": "crossref",
			},
		}
		_, err := coll.UpdateByID(ctx, doc["_id"], update)
		if err != nil {
			logger.Logf(sessionID, "   [Enrich] MongoDB update error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		enriched++
		logger.Logf(sessionID, "   [Enrich] [%d/%d] Enriched successfully (type: %s)", i+1, len(needsEnrich), work.Type)

		// Optionally enrich with Scopus if API key is available
		if doi != "" {
			scopusKey := os.Getenv("SCOPUS_API_KEY")
			if scopusKey != "" {
				enrichWithScopus(doi, scopusKey, sessionID)
			}
		}

		// Rate limiting: 2 seconds between CrossRef calls (polite pool)
		time.Sleep(2 * time.Second)
	}

	logger.Logf(sessionID, "   [Enrich] Done. Enriched %d/%d docs from CrossRef.", enriched, len(needsEnrich))
	return enriched, nil
}

// mergeEnrichFields merges newly enriched fields into an existing fields array.
// It replaces fields with matching keys that have empty/NOT_REPORTED values,
// and appends new keys that don't exist yet.
func mergeEnrichFields(existing primitive.A, enriched bson.A) bson.A {
	if len(existing) == 0 {
		return enriched
	}

	// Build a map of enriched fields by key
	enrichMap := map[string]bson.M{}
	for _, it := range enriched {
		f, ok := it.(bson.M)
		if !ok {
			continue
		}
		k, _ := f["key"].(string)
		if k != "" {
			enrichMap[k] = f
		}
	}

	// Update existing fields where value is empty or NOT_REPORTED
	merged := make(bson.A, 0, len(existing))
	usedKeys := map[string]bool{}
	for _, it := range existing {
		f, ok := it.(bson.M)
		if !ok {
			merged = append(merged, it)
			continue
		}
		k, _ := f["key"].(string)
		v, _ := f["value"].(string)

		if enrichData, found := enrichMap[k]; found {
			usedKeys[k] = true
			if v == "" || v == "[NOT REPORTED]" {
				merged = append(merged, enrichData)
			} else {
				merged = append(merged, f)
			}
		} else {
			merged = append(merged, f)
		}
	}

	// Append new keys not already in existing
	for k, enrichData := range enrichMap {
		if !usedKeys[k] {
			merged = append(merged, enrichData)
		}
	}

	return merged
}

// truncTitle truncates a title string to maxLen characters for logging.
func truncTitle(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return fmt.Sprintf("%s...", s[:maxLen])
}

// EnrichNotReportedFields enriches specific NOT_REPORTED fields (design, geographic)
// for a single doc after LLM extraction. Called post-extraction in M7 Step 2.
func EnrichNotReportedFields(ctx context.Context, coll *mongo.Collection, doc bson.M, sessionID string) {
	doi := getStr(doc, "DOI", "doi")
	if doi == "" {
		return
	}
	doi = strings.TrimPrefix(doi, "https://doi.org/")
	doi = strings.TrimPrefix(doi, "http://doi.org/")
	doi = strings.TrimSpace(doi)
	if doi == "" || doi == "-" {
		return
	}

	// Check if fields contain NOT_REPORTED for design or geographic
	arr, ok := doc["fields"].(bson.A)
	if !ok {
		if arr2, ok2 := doc["fields"].([]interface{}); ok2 {
			arr = bson.A(arr2)
		}
	}
	if len(arr) == 0 {
		return
	}

	needsDesign := false
	needsGeo := false
	for _, it := range arr {
		f, ok := it.(bson.M)
		if !ok {
			continue
		}
		k, _ := f["key"].(string)
		status, _ := f["status"].(string)
		kLower := strings.ToLower(k)
		if status == "NOT_REPORTED" {
			if strings.Contains(kLower, "design") || strings.Contains(kLower, "study_design") {
				needsDesign = true
			}
			if strings.Contains(kLower, "geographic") || strings.Contains(kLower, "country") || strings.Contains(kLower, "location") {
				needsGeo = true
			}
		}
	}

	if !needsDesign && !needsGeo {
		return
	}

	work, err := refs.GetCrossrefWork(doi)
	if err != nil {
		return
	}

	// Update specific NOT_REPORTED fields from CrossRef data
	updatedArr := make(bson.A, len(arr))
	copy(updatedArr, arr)

	for i, it := range updatedArr {
		f, ok := it.(bson.M)
		if !ok {
			continue
		}
		k, _ := f["key"].(string)
		status, _ := f["status"].(string)
		kLower := strings.ToLower(k)

		if status != "NOT_REPORTED" {
			continue
		}

		if needsDesign && (strings.Contains(kLower, "design") || strings.Contains(kLower, "study_design")) {
			if work.Type != "" {
				updatedArr[i] = bson.M{
					"key":      k,
					"value":    formatStudyType(work.Type),
					"evidence": "CrossRef metadata (type field)",
					"status":   "REPORTED",
				}
			}
		}

		if needsGeo && (strings.Contains(kLower, "geographic") || strings.Contains(kLower, "country") || strings.Contains(kLower, "location")) {
			geo := extractGeographic(work.Author)
			if geo != "" {
				updatedArr[i] = bson.M{
					"key":      k,
					"value":    geo,
					"evidence": "CrossRef author affiliations",
					"status":   "REPORTED",
				}
			}
		}
	}

	_, _ = coll.UpdateByID(ctx, doc["_id"], bson.M{"$set": bson.M{"fields": updatedArr, "enriched_from": "crossref_partial"}})
}

// buildFieldsFromCrossRef constructs a bson.A of field entries from a CrossrefWork.
func buildFieldsFromCrossRef(work *refs.CrossrefWork) bson.A {
	fields := bson.A{}

	// study_design from Type
	if work.Type != "" {
		fields = append(fields, bson.M{
			"key":      "study_design",
			"value":    formatStudyType(work.Type),
			"evidence": "CrossRef metadata (type field)",
			"status":   "REPORTED",
		})
	}

	// geographic from author affiliations
	geo := extractGeographic(work.Author)
	if geo != "" {
		fields = append(fields, bson.M{
			"key":      "geographic",
			"value":    geo,
			"evidence": "CrossRef author affiliations",
			"status":   "REPORTED",
		})
	}

	// publisher
	if work.Publisher != "" {
		fields = append(fields, bson.M{
			"key":      "publisher",
			"value":    work.Publisher,
			"evidence": "CrossRef metadata (publisher field)",
			"status":   "REPORTED",
		})
	}

	// subject
	if len(work.Subject) > 0 {
		fields = append(fields, bson.M{
			"key":      "subject",
			"value":    strings.Join(work.Subject, "; "),
			"evidence": "CrossRef metadata (subject field)",
			"status":   "REPORTED",
		})
	}

	// journal (container-title)
	if len(work.ContainerTitle) > 0 {
		fields = append(fields, bson.M{
			"key":      "journal",
			"value":    work.ContainerTitle[0],
			"evidence": "CrossRef metadata (container-title field)",
			"status":   "REPORTED",
		})
	}

	return fields
}

// formatStudyType converts CrossRef type identifiers to human-readable form.
func formatStudyType(crossrefType string) string {
	typeMap := map[string]string{
		"journal-article":     "Journal Article",
		"proceedings-article": "Conference Paper",
		"book-chapter":        "Book Chapter",
		"book":                "Book",
		"dissertation":        "Dissertation",
		"report":              "Report",
		"dataset":             "Dataset",
		"monograph":           "Monograph",
		"edited-book":         "Edited Book",
		"reference-entry":     "Reference Entry",
		"posted-content":      "Preprint",
	}
	if mapped, ok := typeMap[crossrefType]; ok {
		return mapped
	}
	// Fallback: capitalize and replace hyphens
	return strings.ReplaceAll(strings.Title(crossrefType), "-", " ")
}

// countryAliases maps common alternative names/spellings to canonical country names.
var countryAliases = map[string]string{
	"p. r. china":                  "China",
	"people's republic of china":   "China",
	"pr china":                     "China",
	"p.r. china":                   "China",
	"p.r.china":                    "China",
	"peoples republic of china":    "China",
	"republic of korea":            "South Korea",
	"rok":                          "South Korea",
	"korea":                        "South Korea",
	"united states":                "USA",
	"united states of america":     "USA",
	"u.s.a.":                       "USA",
	"u.s.a":                        "USA",
	"us":                           "USA",
	"united kingdom":               "UK",
	"england":                      "UK",
	"scotland":                     "UK",
	"wales":                        "UK",
	"northern ireland":             "UK",
	"great britain":                "UK",
	"türkiye":                      "Turkey",
	"turkiye":                      "Turkey",
	"russian federation":           "Russia",
	"viet nam":                     "Vietnam",
	"republic of china":            "Taiwan",
	"czech republic":               "Czechia",
	"republic of ireland":          "Ireland",
	"islamic republic of iran":     "Iran",
	"iran, islamic republic of":    "Iran",
	"kingdom of saudi arabia":      "Saudi Arabia",
	"ksa":                          "Saudi Arabia",
	"uae":                          "United Arab Emirates",
	"dprk":                         "North Korea",
	"democratic people's republic of korea": "North Korea",
	"republic of the philippines":  "Philippines",
	"brasil":                       "Brazil",
	"deutschland":                  "Germany",
	"bundesrepublik deutschland":   "Germany",
	"republic of singapore":        "Singapore",
	"hellenic republic":            "Greece",
	"new zealand":                  "New Zealand",
	"south africa":                 "South Africa",
	"sri lanka":                    "Sri Lanka",
	"the netherlands":              "Netherlands",
	"holland":                      "Netherlands",
	"ivory coast":                  "Ivory Coast",
	"cote d'ivoire":                "Ivory Coast",
	"republic of india":            "India",
	"kingdom of thailand":          "Thailand",
	"republic of indonesia":        "Indonesia",
	"federation of malaysia":       "Malaysia",
	"republic of turkey":           "Turkey",
	"republic of poland":           "Poland",
	"hong kong":                    "Hong Kong",
	"hong kong sar":                "Hong Kong",
	"macau":                        "Macau",
	"macao":                        "Macau",
	"republic of south africa":     "South Africa",
	"saudi arabia":                 "Saudi Arabia",
	"south korea":                  "South Korea",
	"north korea":                  "North Korea",
	"costa rica":                   "Costa Rica",
	"puerto rico":                  "Puerto Rico",
	"trinidad and tobago":          "Trinidad and Tobago",
	"papua new guinea":             "Papua New Guinea",
	"dominican republic":           "Dominican Republic",
	"el salvador":                  "El Salvador",
	"burkina faso":                 "Burkina Faso",
	"sierra leone":                 "Sierra Leone",
	"equatorial guinea":            "Equatorial Guinea",
	"bosnia and herzegovina":       "Bosnia and Herzegovina",
	"united arab emirates":         "United Arab Emirates",
}

// validCountries is a set of recognized country names (lowercase -> canonical).
var validCountries = func() map[string]string {
	countries := []string{
		"Afghanistan", "Albania", "Algeria", "Andorra", "Angola",
		"Antigua and Barbuda", "Argentina", "Armenia", "Australia", "Austria",
		"Azerbaijan", "Bahamas", "Bahrain", "Bangladesh", "Barbados",
		"Belarus", "Belgium", "Belize", "Benin", "Bhutan",
		"Bolivia", "Bosnia and Herzegovina", "Botswana", "Brazil", "Brunei",
		"Bulgaria", "Burkina Faso", "Burundi", "Cambodia", "Cameroon",
		"Canada", "Cape Verde", "Central African Republic", "Chad", "Chile",
		"China", "Colombia", "Comoros", "Congo", "Costa Rica",
		"Croatia", "Cuba", "Cyprus", "Czechia", "Denmark",
		"Djibouti", "Dominica", "Dominican Republic", "Ecuador", "Egypt",
		"El Salvador", "Equatorial Guinea", "Eritrea", "Estonia", "Eswatini",
		"Ethiopia", "Fiji", "Finland", "France", "Gabon",
		"Gambia", "Georgia", "Germany", "Ghana", "Greece",
		"Grenada", "Guatemala", "Guinea", "Guinea-Bissau", "Guyana",
		"Haiti", "Honduras", "Hong Kong", "Hungary", "Iceland",
		"India", "Indonesia", "Iran", "Iraq", "Ireland",
		"Israel", "Italy", "Ivory Coast", "Jamaica", "Japan",
		"Jordan", "Kazakhstan", "Kenya", "Kiribati", "Kuwait",
		"Kyrgyzstan", "Laos", "Latvia", "Lebanon", "Lesotho",
		"Liberia", "Libya", "Liechtenstein", "Lithuania", "Luxembourg",
		"Macau", "Madagascar", "Malawi", "Malaysia", "Maldives",
		"Mali", "Malta", "Marshall Islands", "Mauritania", "Mauritius",
		"Mexico", "Micronesia", "Moldova", "Monaco", "Mongolia",
		"Montenegro", "Morocco", "Mozambique", "Myanmar", "Namibia",
		"Nauru", "Nepal", "Netherlands", "New Zealand", "Nicaragua",
		"Niger", "Nigeria", "North Korea", "North Macedonia", "Norway",
		"Oman", "Pakistan", "Palau", "Palestine", "Panama",
		"Papua New Guinea", "Paraguay", "Peru", "Philippines", "Poland",
		"Portugal", "Puerto Rico", "Qatar", "Romania", "Russia",
		"Rwanda", "Saint Kitts and Nevis", "Saint Lucia", "Samoa", "San Marino",
		"Sao Tome and Principe", "Saudi Arabia", "Senegal", "Serbia", "Seychelles",
		"Sierra Leone", "Singapore", "Slovakia", "Slovenia", "Solomon Islands",
		"Somalia", "South Africa", "South Korea", "South Sudan", "Spain",
		"Sri Lanka", "Sudan", "Suriname", "Sweden", "Switzerland",
		"Syria", "Taiwan", "Tajikistan", "Tanzania", "Thailand",
		"Timor-Leste", "Togo", "Tonga", "Trinidad and Tobago", "Tunisia",
		"Turkey", "Turkmenistan", "Tuvalu", "Uganda", "Ukraine",
		"United Arab Emirates", "UK", "USA", "Uruguay", "Uzbekistan",
		"Vanuatu", "Vatican City", "Venezuela", "Vietnam", "Yemen",
		"Zambia", "Zimbabwe",
	}
	m := make(map[string]string, len(countries))
	for _, c := range countries {
		m[strings.ToLower(c)] = c
	}
	return m
}()

// rejectKeywords contains substrings that disqualify a component from being a country.
var rejectKeywords = []string{
	"university", "laboratory", "institute", "school",
	"department", "college", "center", "centre",
	"hospital", "faculty", "division", "program",
	"academy", "research", "science", "technology",
	"corporation", "company", "ltd", "inc",
}

// containsDigit returns true if the string contains any digit character.
func containsDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// shouldRejectComponent returns true if the component should not be considered a country.
func shouldRejectComponent(s string) bool {
	if len(s) < 3 {
		return true
	}
	if containsDigit(s) {
		return true
	}
	lower := strings.ToLower(s)
	for _, kw := range rejectKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// normalizeCountry attempts to map a string to a canonical country name.
// Returns the canonical name and true if found, or empty string and false.
func normalizeCountry(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", false
	}

	// Remove trailing periods and extra whitespace
	trimmed = strings.TrimRight(trimmed, ".")
	trimmed = strings.TrimSpace(trimmed)

	lower := strings.ToLower(trimmed)

	// Check aliases first
	if canonical, ok := countryAliases[lower]; ok {
		return canonical, true
	}

	// Check valid countries list
	if canonical, ok := validCountries[lower]; ok {
		return canonical, true
	}

	return "", false
}

// extractGeographic extracts country/location information from author affiliations.
// It scans all comma-separated components of each affiliation, validates against
// a whitelist of known countries, normalizes aliases, and returns deduplicated
// semicolon-separated country names.
func extractGeographic(authors []refs.CrossrefAuthor) string {
	locations := map[string]bool{}
	for _, a := range authors {
		for _, aff := range a.Affiliation {
			if aff.Name == "" {
				continue
			}
			parts := strings.Split(aff.Name, ",")
			for _, part := range parts {
				component := strings.TrimSpace(part)
				if component == "" {
					continue
				}
				if shouldRejectComponent(component) {
					continue
				}
				if country, ok := normalizeCountry(component); ok {
					locations[country] = true
				}
			}
		}
	}
	if len(locations) == 0 {
		return ""
	}
	var result []string
	for loc := range locations {
		result = append(result, loc)
	}
	sort.Strings(result)
	return strings.Join(result, "; ")
}

// enrichWithScopus attempts to enrich a doc with additional metadata from Scopus.
func enrichWithScopus(doi string, apiKey string, sessionID string) {
	ft, err := refs.RetrieveFullText(doi, apiKey)
	if err != nil {
		logger.Logf(sessionID, "   [Enrich] Scopus error for %s: %v", doi, err)
		return
	}

	coreData := ft.FullTextRetrievalResponse.CoreData
	if coreData.Publisher != "" || coreData.Description != "" {
		logger.Logf(sessionID, "   [Enrich] Scopus data available for %s (publisher: %s)", doi, coreData.Publisher)
	}
}
