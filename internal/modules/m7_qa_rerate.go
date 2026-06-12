package modules

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"nsa/internal/agent"
	"nsa/internal/llm"
	"nsa/internal/logger"
	"nsa/internal/model"
	"nsa/internal/repository"
)

// ReratePaperResult holds the result of a single paper re-rating.
type ReratePaperResult struct {
	PaperID       string  `json:"paper_id"`
	Title         string  `json:"title"`
	R1Score       float64 `json:"r1_score"`
	R1Category    string  `json:"r1_category"`
	R1Reasoning   string  `json:"r1_reasoning"`
	R1Evidence    string  `json:"r1_evidence"`
	R1Model       string  `json:"r1_model"`
	R2Score       float64 `json:"r2_score"`
	R2Category    string  `json:"r2_category"`
	R2Reasoning   string  `json:"r2_reasoning"`
	R2Evidence    string  `json:"r2_evidence"`
	R2Model       string  `json:"r2_model"`
	FinalScore    float64 `json:"final_score"`
	FinalCategory string  `json:"final_category"`
}

// RerateSinglePaper re-rates a single paper using dual raters and updates it in MongoDB.
// paperID can be a MongoDB ObjectID hex string or a DOI.
func RerateSinglePaper(ctx context.Context, mongoRepo *repository.MongoRepository, factory *llm.LLMFactory, session *model.SLRSession, paperID string) (*ReratePaperResult, error) {
	if session.QAThreshold == nil {
		return nil, fmt.Errorf("session has no QA threshold configured")
	}

	coll := mongoRepo.GetExtractionCollection()
	tool := session.QAThreshold.Tool
	cat := session.QAThreshold.Categorization
	thr := session.QAThreshold.Threshold
	justification := session.QAThreshold.ToolJustification

	// Try to find paper by ObjectID first, then by DOI
	var paper bson.M
	if oid, err := primitive.ObjectIDFromHex(paperID); err == nil {
		res := coll.FindOne(ctx, bson.M{"_id": oid, "session_id": session.ID})
		if res.Err() == nil {
			_ = res.Decode(&paper)
		}
	}
	if paper == nil {
		// Try by DOI
		res := coll.FindOne(ctx, bson.M{
			"session_id": session.ID,
			"$or": []bson.M{
				{"DOI": paperID},
				{"doi": paperID},
			},
		})
		if res.Err() != nil {
			return nil, fmt.Errorf("paper not found: %s", paperID)
		}
		_ = res.Decode(&paper)
	}
	if paper == nil {
		return nil, fmt.Errorf("paper not found: %s", paperID)
	}

	title := getStr(paper, "Title", "title")
	doi := getStr(paper, "DOI", "doi")

	// Get fulltext from RAG index
	ftIndex, _, _ := BuildFulltextIndex(ctx)
	if ftIndex == nil {
		ftIndex = map[string]string{}
	}

	var ft string
	if nd := normalizeDOIForRAG(doi); nd != "" && ftIndex[nd] != "" {
		ft = ftIndex[nd]
	} else if nt := NormTitle(title); nt != "" && ftIndex["title:"+nt] != "" {
		ft = ftIndex["title:"+nt]
	}

	if ft == "" {
		return nil, fmt.Errorf("fulltext not available for paper: %s (DOI: %s)", title, doi)
	}

	// Set up dual raters
	qp1, qf1 := factory.RoleProviders(ctx, "reviewer1")
	r1, err := buildAgentWithFallback(ctx, factory, qp1, qf1)
	if err != nil {
		return nil, fmt.Errorf("QA Rater 1 (%s/%s): %w", qp1, qf1, err)
	}
	qp2, qf2 := factory.RoleProviders(ctx, "reviewer2")
	r2, err := buildAgentWithFallback(ctx, factory, qp2, qf2)
	if err != nil {
		return nil, fmt.Errorf("QA Rater 2 (%s/%s): %w", qp2, qf2, err)
	}

	// Get model names for transparency
	var r1Model, r2Model string
	cfg1, _ := mongoRepo.GetLLMConfig(ctx, qp1)
	if cfg1 != nil {
		r1Model = cfg1.ProviderName
		if cfg1.DefaultModel != "" {
			r1Model += " (" + cfg1.DefaultModel + ")"
		}
	} else {
		r1Model = qp1
	}
	cfg2, _ := mongoRepo.GetLLMConfig(ctx, qp2)
	if cfg2 != nil {
		r2Model = cfg2.ProviderName
		if cfg2.DefaultModel != "" {
			r2Model += " (" + cfg2.DefaultModel + ")"
		}
	} else {
		r2Model = qp2
	}

	// Run dual rating
	logger.Logf(session.ID, "   [Rerate] Rating paper: %s (DOI: %s)\n", title, doi)

	// Build enhanced justification with rubric + anchors
	enhancedJustification := justification
	if session.QAThreshold.QARubric != "" {
		enhancedJustification += "\n\n[RUBRIK OPERASIONAL PENILAIAN]:\n" + session.QAThreshold.QARubric
	}
	if session.QACalibration != nil && len(session.QACalibration.Anchors) > 0 {
		enhancedJustification += "\n\n" + formatAnchorContext(session.QACalibration.Anchors)
	}

	s1, e1 := r1.AppraiseQuality(ctx, tool, cat, enhancedJustification, title, ft)
	time.Sleep(3 * time.Second)
	s2, e2 := r2.AppraiseQuality(ctx, tool, cat, enhancedJustification, title, ft)

	if e1 != nil && e2 != nil {
		return nil, fmt.Errorf("both raters failed: R1=%v, R2=%v", e1, e2)
	}

	result := &ReratePaperResult{
		Title:   title,
		R1Model: r1Model,
		R2Model: r2Model,
	}

	// Determine paper_id for response
	if oid, ok := paper["_id"].(primitive.ObjectID); ok {
		result.PaperID = oid.Hex()
	} else if idStr, ok := paper["_id"].(string); ok {
		result.PaperID = idStr
	}

	upd := bson.M{"qa_rated": true}

	if s1 != nil {
		result.R1Score = s1.TotalScore
		result.R1Category = s1.Category
		result.R1Reasoning = s1.Reasoning
		result.R1Evidence = s1.Evidence
		upd["qa_r1_score"] = s1.TotalScore
		upd["qa_r1_category"] = s1.Category
		upd["qa_r1_reasoning"] = s1.Reasoning
		upd["qa_r1_evidence"] = s1.Evidence
		upd["qa_r1_model"] = r1Model
	}
	if s2 != nil {
		result.R2Score = s2.TotalScore
		result.R2Category = s2.Category
		result.R2Reasoning = s2.Reasoning
		result.R2Evidence = s2.Evidence
		upd["qa_r2_score"] = s2.TotalScore
		upd["qa_r2_category"] = s2.Category
		upd["qa_r2_reasoning"] = s2.Reasoning
		upd["qa_r2_evidence"] = s2.Evidence
		upd["qa_r2_model"] = r2Model
	}

	// Compute final
	var r1Score, r2Score float64
	var hasR1, hasR2 bool
	if s1 != nil {
		r1Score = s1.TotalScore
		hasR1 = true
	}
	if s2 != nil {
		r2Score = s2.TotalScore
		hasR2 = true
	}

	if hasR1 && hasR2 {
		avg := (r1Score + r2Score) / 2
		finalCat := categoryFor(avg, thr, cat)
		upd["qa_total_score"] = avg
		upd["qa_final_category"] = finalCat
		result.FinalScore = avg
		result.FinalCategory = finalCat
	} else if hasR1 {
		upd["qa_total_score"] = r1Score
		upd["qa_final_category"] = categoryFor(r1Score, thr, cat)
		result.FinalScore = r1Score
		result.FinalCategory = categoryFor(r1Score, thr, cat)
	} else if hasR2 {
		upd["qa_total_score"] = r2Score
		upd["qa_final_category"] = categoryFor(r2Score, thr, cat)
		result.FinalScore = r2Score
		result.FinalCategory = categoryFor(r2Score, thr, cat)
	} else {
		upd["qa_final_category"] = "ERROR"
		upd["qa_total_score"] = 0
		result.FinalCategory = "ERROR"
	}

	// Update paper in MongoDB
	_, err = coll.UpdateByID(ctx, paper["_id"], bson.M{"$set": upd})
	if err != nil {
		return nil, fmt.Errorf("failed to update paper: %w", err)
	}

	logger.Logf(session.ID, "   [Rerate] Done: R1=%.1f(%s) R2=%.1f(%s) Final=%.1f(%s)\n",
		result.R1Score, result.R1Category, result.R2Score, result.R2Category, result.FinalScore, result.FinalCategory)

	return result, nil
}

// buildAgentWithFallback creates an ExtractionAgent with retry+fallback (standalone version for handler use).
func buildAgentWithFallback(ctx context.Context, factory *llm.LLMFactory, primary, fallback string) (*agent.ExtractionAgent, error) {
	p, errP := factory.CreateClient(ctx, primary)
	var fb llm.LLMClient
	if fallback != "" {
		if c, e := factory.CreateClient(ctx, fallback); e == nil {
			fb = c
		}
	}
	if errP != nil {
		if fb != nil {
			return agent.NewExtractionAgent(llm.NewRetryingClient(nil, fb)), nil
		}
		return nil, errP
	}
	return agent.NewExtractionAgent(llm.NewRetryingClient(p, fb)), nil
}

// BuildQASystemPrompt constructs the system prompt used by QA raters for transparency/xAI purposes.
func BuildQASystemPrompt(session *model.SLRSession) string {
	if session.QAThreshold == nil {
		return ""
	}

	justification := session.QAThreshold.ToolJustification

	// Include rubric if available
	if session.QAThreshold.QARubric != "" {
		justification += "\n\n[RUBRIK OPERASIONAL PENILAIAN]:\n" + session.QAThreshold.QARubric
	}

	// Include anchor examples if available
	if session.QACalibration != nil && len(session.QACalibration.Anchors) > 0 {
		justification += "\n\n" + formatAnchorContext(session.QACalibration.Anchors)
	}

	return fmt.Sprintf(`Anda penilai kualitas (rater) Systematic Literature Review.
Nilai kualitas metodologis artikel memakai tool: %s.
Detail/Framework/Justifikasi tool: %s
Kategorisasi ambang: %s

ATURAN: nilai HANYA dari full-text (konteks RAG). Skor 0-100 (dinormalisasi).
Tetapkan category sesuai ambang.

Keluarkan HANYA JSON MURNI tanpa markdown:
{
  "total_score": 78,
  "category": "MODERATE",
  "items_summary": "Domain 1: Participants (18/20) - populasi jelas; Domain 2: Predictors (15/20) - variabel terdefinisi; Domain 3: Outcome (12/20) - kurang detail; Domain 4: Analysis (10/20) - bias selection; Domain 5: Overall (5/20) - moderate risk",
  "reasoning": "Penjelasan logis mengapa paper mendapat skor ini, mengacu ke rubrik per domain dan threshold kategorisasi",
  "evidence": "Kutipan langsung dari teks yang mendukung penilaian tiap domain: '[kutipan 1]' (Domain X), '[kutipan 2]' (Domain Y)"
}

PENTING:
- items_summary HARUS breakdown per domain sesuai rubrik
- evidence HARUS berisi kutipan langsung dari full-text, bukan ringkasan
- total_score = hasil weighted sum sesuai rubrik operasional`,
		session.QAThreshold.Tool,
		justification,
		session.QAThreshold.Categorization)
}
