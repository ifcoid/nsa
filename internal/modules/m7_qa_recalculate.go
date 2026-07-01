package modules

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"nsa/internal/logger"
	"nsa/internal/model"
	"nsa/internal/repository"
)

// RecalculateQAErrors fixes papers stuck in ERROR that have valid R1 AND R2 scores.
// It recalculates the final category and score, then recomputes kappa and sensitivity
// for the session.
//
// Returns (fixed, needRerate): `fixed` = ERROR papers salvaged (both scores present, only
// the final category was missing); `needRerate` = ERROR papers that CANNOT be recalculated
// because at least one rater produced no score (mis. provider LLM gagal/timeout). A paper is
// set to ERROR *justru karena* skornya tak lengkap (lihat m7_qa.go), jadi recalc yang
// mensyaratkan kedua skor >0 memang tak akan menyentuhnya — paper begitu HARUS dinilai ulang
// ("Lanjutkan QA"), bukan di-recalculate. Kedua angka dikembalikan agar handler bisa memberi
// pesan yang actionable (xAI), bukan buntu "0 fixed".
func RecalculateQAErrors(ctx context.Context, mongoRepo *repository.MongoRepository, session *model.SLRSession) (int, int, error) {
	if session.QAThreshold == nil {
		return 0, 0, fmt.Errorf("session has no QA threshold configured")
	}

	coll := mongoRepo.GetExtractionCollection()
	cat := session.QAThreshold.Categorization
	thr := session.QAThreshold.Threshold

	// Ambil SEMUA paper qa_final_category = "ERROR" (tanpa syarat skor), lalu pilah:
	// yang punya kedua skor -> bisa di-recalculate; yang tak lengkap -> perlu re-rating.
	errCur, err := coll.Find(ctx, bson.M{
		"session_id":        session.ID,
		"qa_final_category": "ERROR",
	})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to query ERROR papers: %w", err)
	}

	var errDocs []bson.M
	if err := errCur.All(ctx, &errDocs); err != nil {
		return 0, 0, fmt.Errorf("failed to decode ERROR papers: %w", err)
	}

	fixedCount := 0
	needRerate := 0
	for _, ep := range errDocs {
		r1sc, r1ok := toFloat(ep["qa_r1_score"])
		r2sc, r2ok := toFloat(ep["qa_r2_score"])
		if r1ok && r2ok && r1sc > 0 && r2sc > 0 {
			avg := (r1sc + r2sc) / 2
			newCat := categoryFor(avg, thr, cat)
			fixUpd := bson.M{
				"qa_total_score":    avg,
				"qa_final_category": newCat,
			}
			_, err := coll.UpdateByID(ctx, ep["_id"], bson.M{"$set": fixUpd})
			if err == nil {
				fixedCount++
				logger.Logf(session.ID, "      [Recalc] Fixed ERROR paper %s: R1=%.1f R2=%.1f avg=%.1f -> %s\n",
					getStr(ep, "DOI", "doi"), r1sc, r2sc, avg, newCat)
			}
		} else {
			// Skor tak lengkap: recalc tak bisa menolong; paper ini butuh dinilai ulang.
			needRerate++
		}
	}

	// Recompute kappa and sensitivity from all papers
	allCur, err := coll.Find(ctx, bson.M{"session_id": session.ID})
	if err != nil {
		return fixedCount, needRerate, fmt.Errorf("failed to query all papers for kappa: %w", err)
	}
	var allDocs []bson.M
	_ = allCur.All(ctx, &allDocs)

	kappa, details := qaKappa(allDocs)
	session.QAThreshold.Kappa = kappa
	session.QAThreshold.KappaDetails = details

	// Recompute actual feasibility
	totalRateable := 0
	actualPass := 0
	for _, p := range allDocs {
		c := getStr(p, "qa_final_category")
		if c == "UNRATED" || c == "ERROR" || c == "" {
			continue
		}
		totalRateable++
		if sc, ok := toFloat(p["qa_total_score"]); ok && sc >= thr {
			actualPass++
		}
	}
	if totalRateable > 0 {
		session.QAThreshold.ActualFeasibility = float64(actualPass) / float64(totalRateable) * 100
		session.QAThreshold.ActualFeasibilityNote = fmt.Sprintf("Dari %d studi yang berhasil dinilai, %d (%.1f%%) memenuhi threshold %.0f%%.",
			totalRateable, actualPass, session.QAThreshold.ActualFeasibility, thr)
	}

	// Recompute sensitivity
	session.SensitivityAnalysis = buildSensitivity(allDocs, thr, cat)

	// Persist session updates
	if err := mongoRepo.UpdateSession(ctx, session); err != nil {
		return fixedCount, needRerate, fmt.Errorf("failed to update session: %w", err)
	}

	logger.Logf(session.ID, "   [Recalc] Completed: %d fixed, %d perlu re-rating, kappa=%.3f, sensitivity=%s\n",
		fixedCount, needRerate, kappa, session.SensitivityAnalysis.Verdict)

	return fixedCount, needRerate, nil
}
