package modules

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

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

// extractGeographic extracts country/location information from author affiliations.
func extractGeographic(authors []refs.CrossrefAuthor) string {
	locations := map[string]bool{}
	for _, a := range authors {
		for _, aff := range a.Affiliation {
			if aff.Name != "" {
				// Extract last component (usually country) from affiliation
				parts := strings.Split(aff.Name, ",")
				if len(parts) > 0 {
					country := strings.TrimSpace(parts[len(parts)-1])
					if country != "" {
						locations[country] = true
					}
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
