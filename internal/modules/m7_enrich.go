package modules

import (
	"context"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/ifcoid/refs"
	"nsa/internal/logger"
	"nsa/internal/repository"
)

// EnrichMetadataFromCrossRef queries the extraction collection for docs that have a DOI
// but missing/empty fields, then populates them from CrossRef metadata.
// It returns the number of documents enriched.
func EnrichMetadataFromCrossRef(ctx context.Context, mongoRepo *repository.MongoRepository, sessionID string) (int, error) {
	coll := mongoRepo.GetExtractionCollection()

	// Find docs with DOI but nil/empty fields
	filter := bson.M{
		"session_id": sessionID,
		"$or": bson.A{
			bson.M{"fields": bson.M{"$exists": false}},
			bson.M{"fields": nil},
			bson.M{"fields": bson.A{}},
			bson.M{"fields": bson.M{"$size": 0}},
		},
	}

	cur, err := coll.Find(ctx, filter)
	if err != nil {
		return 0, err
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return 0, err
	}

	enriched := 0
	for _, doc := range docs {
		if ctx.Err() != nil {
			return enriched, ctx.Err()
		}

		doi := getStr(doc, "DOI", "doi")
		if doi == "" {
			continue
		}

		// Normalize DOI (strip URL prefix)
		doi = strings.TrimPrefix(doi, "https://doi.org/")
		doi = strings.TrimPrefix(doi, "http://doi.org/")
		doi = strings.TrimSpace(doi)

		if doi == "" || doi == "-" {
			continue
		}

		logger.Logf(sessionID, "   [Enrich] CrossRef lookup: %s", doi)

		work, err := refs.GetCrossrefWork(doi)
		if err != nil {
			logger.Logf(sessionID, "   [Enrich] CrossRef error for %s: %v", doi, err)
			time.Sleep(1 * time.Second)
			continue
		}

		fieldsArray := buildFieldsFromCrossRef(work)

		update := bson.M{
			"$set": bson.M{
				"fields":        fieldsArray,
				"enriched_from": "crossref",
			},
		}
		_, err = coll.UpdateByID(ctx, doc["_id"], update)
		if err != nil {
			logger.Logf(sessionID, "   [Enrich] MongoDB update error: %v", err)
			continue
		}

		enriched++

		// Optionally enrich with Scopus if API key is available
		scopusKey := os.Getenv("SCOPUS_API_KEY")
		if scopusKey != "" {
			enrichWithScopus(doi, scopusKey, sessionID)
		}

		// Rate limiting: sleep 1-2 seconds between CrossRef calls
		time.Sleep(1500 * time.Millisecond)
	}

	if enriched > 0 {
		logger.Logf(sessionID, "   [Enrich] Enriched %d docs from CrossRef metadata.", enriched)
	}
	return enriched, nil
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
