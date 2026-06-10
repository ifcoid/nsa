package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"nsa/internal/agent"
	"nsa/internal/logger"
	"nsa/internal/model"
)

const qaBatchSize = 6

// ===== L3: Quality appraisal (tool + threshold + dual-rater kappa + sensitivity) =====

func (m *M7Extraction) runQAL3(ctx context.Context, session *model.SLRSession) error {
	coll := m.deps.MongoRepo.GetExtractionCollection()

	// Fase 1+2: tool selection + threshold 3-lapis (sekali).
	if session.QAThreshold == nil {
		logger.Log(session.ID, "   [Langkah 7.3] Tool selection + threshold justification...")
		brain, err := m.deps.LLMFactory.BrainClient(ctx)
		if err != nil {
			return fmt.Errorf("gemini (brain QA) gagal: %w", err)
		}
		designBreakdown := m.designBreakdownFromExtraction(ctx, session)
		qt, err := agent.NewExtractionAgent(brain).SelectQATool(ctx, designBreakdown, session.Feedback)
		if err != nil {
			return err
		}
		session.QAThreshold = qt
		session.Feedback = "" // Bersihkan feedback setelah dipakai
		logger.Logf(session.ID, "   [System] QA tool: %s, threshold %.0f%%.\n", qt.Tool, qt.Threshold)
		session.Status = "M7_STEP3_QA_TOOL_WAITING_APPROVAL"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	// Fase 3: dual-rater QA per paper (batch).
	cur, err := coll.Find(ctx, bson.M{"session_id": session.ID, "qa_rated": bson.M{"$ne": true}},
		options.Find().SetLimit(int64(qaBatchSize)))
	if err != nil {
		return err
	}
	var batch []bson.M
	_ = cur.All(ctx, &batch)

	if len(batch) > 0 {
		ftIndex, _, _ := BuildFulltextIndex(ctx)
		if ftIndex == nil {
			ftIndex = map[string]string{}
		}
		qp1, qf1 := m.deps.LLMFactory.RoleProviders(ctx, "reviewer1")
		r1, err := m.agentWithFallback(ctx, qp1, qf1)
		if err != nil {
			return fmt.Errorf("QA Rater 1 (%s/%s) gagal: %w", qp1, qf1, err)
		}
		qp2, qf2 := m.deps.LLMFactory.RoleProviders(ctx, "reviewer2")
		r2, err := m.agentWithFallback(ctx, qp2, qf2)
		if err != nil {
			return fmt.Errorf("QA Rater 2 (%s/%s) gagal: %w", qp2, qf2, err)
		}

		var r1Model, r2Model string
		cfg1, _ := m.deps.MongoRepo.GetLLMConfig(ctx, qp1)
		if cfg1 != nil {
			r1Model = cfg1.ProviderName
			if cfg1.DefaultModel != "" {
				r1Model += " (" + cfg1.DefaultModel + ")"
			}
		} else {
			r1Model = qp1
		}

		cfg2, _ := m.deps.MongoRepo.GetLLMConfig(ctx, qp2)
		if cfg2 != nil {
			r2Model = cfg2.ProviderName
			if cfg2.DefaultModel != "" {
				r2Model += " (" + cfg2.DefaultModel + ")"
			}
		} else {
			r2Model = qp2
		}
		tool := session.QAThreshold.Tool
		cat := session.QAThreshold.Categorization
		thr := session.QAThreshold.Threshold

		for i, p := range batch {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			title := getStr(p, "Title")
			doi := getStr(p, "DOI", "doi")
			
			var ft string
			if nd := normalizeDOIForRAG(doi); nd != "" && ftIndex[nd] != "" {
				ft = ftIndex[nd]
			} else if nt := NormTitle(title); nt != "" && ftIndex["title:"+nt] != "" {
				ft = ftIndex["title:"+nt]
			}
			
			logger.Logf(session.ID, "      -> QA [%d/%d] %s\n", i+1, len(batch), getStr(p, "DOI", "doi"))
			upd := bson.M{"qa_rated": true}
			if ft == "" {
				upd["qa_final_category"] = "UNRATED"
				upd["qa_total_score"] = 0
			} else {
				justification := session.QAThreshold.ToolJustification
				s1, e1 := r1.AppraiseQuality(ctx, tool, cat, justification, title, ft)
				time.Sleep(3 * time.Second)
				s2, e2 := r2.AppraiseQuality(ctx, tool, cat, justification, title, ft)
				
				isFatal := false
				var fatalMsg string
				if e1 != nil && strings.Contains(e1.Error(), "provider merespons dengan error") {
					isFatal = true
					fatalMsg = e1.Error()
				} else if e2 != nil && strings.Contains(e2.Error(), "provider merespons dengan error") {
					isFatal = true
					fatalMsg = e2.Error()
				}

				if isFatal {
					session.Status = "M7_STEP3_NEEDS_REVISION"
					session.SystemError = fatalMsg
					logger.Logf(session.ID, "      [FATAL] %s\n", fatalMsg)
					return m.deps.MongoRepo.UpdateSession(ctx, session)
				}

				if e1 != nil || e2 != nil || s1 == nil || s2 == nil {
					upd["qa_final_category"] = "ERROR"
					upd["qa_total_score"] = 0
				} else {
					avg := (s1.TotalScore + s2.TotalScore) / 2
					upd["qa_r1_score"] = s1.TotalScore
					upd["qa_r1_category"] = s1.Category
					upd["qa_r1_reasoning"] = s1.Reasoning
					upd["qa_r1_evidence"] = s1.Evidence
					upd["qa_r1_model"] = r1Model
					upd["qa_r2_score"] = s2.TotalScore
					upd["qa_r2_category"] = s2.Category
					upd["qa_r2_reasoning"] = s2.Reasoning
					upd["qa_r2_evidence"] = s2.Evidence
					upd["qa_r2_model"] = r2Model
					upd["qa_total_score"] = avg
					upd["qa_final_category"] = categoryFor(avg, thr)
				}
				time.Sleep(5 * time.Second)
			}
			_, _ = coll.UpdateByID(ctx, p["_id"], bson.M{"$set": upd})
		}
		session.Status = "M7_STEP3_QA" // loop
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	// Fase 3b + 4: kappa + sensitivity (semua sudah dirating).
	logger.Log(session.ID, "   [Langkah 7.3] Hitung kappa QA + sensitivity analysis...")
	rated := m.allRated(ctx, session)
	kappa, details := qaKappa(rated)
	session.QAThreshold.Kappa = kappa
	session.QAThreshold.KappaDetails = details
	session.SensitivityAnalysis = buildSensitivity(rated, session.QAThreshold.Threshold)

	logger.Logf(session.ID, "   [System] QA kappa %.3f; sensitivity verdict %s.\n", kappa, session.SensitivityAnalysis.Verdict)
	session.Status = "M7_STEP3_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

func (m *M7Extraction) allRated(ctx context.Context, session *model.SLRSession) []bson.M {
	cur, _ := m.deps.MongoRepo.GetExtractionCollection().Find(ctx, bson.M{"session_id": session.ID})
	var all []bson.M
	_ = cur.All(ctx, &all)
	return all
}

func categoryFor(score, threshold float64) string {
	if score >= threshold+10 {
		return "HIGH"
	}
	if score >= threshold {
		return "MODERATE"
	}
	return "LOW"
}

// qaKappa: Cohen's kappa 2-kelas (pass=HIGH/MODERATE, fail=LOW) atas keputusan R1 vs R2.
func qaKappa(docs []bson.M) (float64, *model.QAKappaDetails) {
	var total, bothPass, bothFail, r1PassR2Fail, r1FailR2Pass int
	pass := func(cat string) bool { return cat == "HIGH" || cat == "MODERATE" }
	for _, p := range docs {
		c1 := getStr(p, "qa_r1_category")
		c2 := getStr(p, "qa_r2_category")
		if c1 == "" || c2 == "" {
			continue
		}
		total++
		switch {
		case pass(c1) && pass(c2):
			bothPass++
		case !pass(c1) && !pass(c2):
			bothFail++
		case pass(c1) && !pass(c2):
			r1PassR2Fail++
		default:
			r1FailR2Pass++
		}
	}
	details := &model.QAKappaDetails{
		TotalRated:   total,
		BothPass:     bothPass,
		BothFail:     bothFail,
		R1PassR2Fail: r1PassR2Fail,
		R1FailR2Pass: r1FailR2Pass,
	}
	return cohensKappa(total, bothPass, bothFail, r1PassR2Fail, r1FailR2Pass), details
}

func buildSensitivity(docs []bson.M, threshold float64) *model.SensitivityAnalysis {
	countAtLeast := func(t float64) int {
		n := 0
		for _, p := range docs {
			if sc, ok := toFloat(p["qa_total_score"]); ok && sc >= t {
				n++
			}
		}
		return n
	}
	base := countAtLeast(threshold)
	strict := countAtLeast(threshold + 10)
	loose := countAtLeast(threshold - 10)

	verdict := "ROBUST"
	if abs(strict-base) > 1 || abs(loose-base) > 1 {
		verdict = "CONDITIONALLY ROBUST"
	}
	if (base > 0 && abs(strict-base)*100/maxInt(base, 1) > 30) || abs(loose-base) > maxInt(base/2, 2) {
		verdict = "SENSITIVE"
	}

	reasoning := ""
	switch verdict {
	case "ROBUST":
		reasoning = "Perubahan threshold (±10%) tidak memengaruhi jumlah studi yang lolos secara signifikan. Ini menunjukkan bahwa kesimpulan (pool) riset Anda kuat dan tidak mudah bias oleh penentuan batas kualitas metodologis."
	case "CONDITIONALLY ROBUST":
		reasoning = "Terdapat sedikit fluktuasi pada jumlah studi ketika threshold diubah, namun masih dalam batas wajar. Hasil akhir tetap dapat diandalkan dengan mempertimbangkan batasan tersebut."
	case "SENSITIVE":
		reasoning = "Perubahan batas kualitas mengeksklusi proporsi studi yang sangat besar (lebih dari 30%). Ini mengindikasikan bahwa hasil akhir riset sangat bergantung penuh pada batas kualitas (cutoff) yang Anda pilih."
	}

	sc := []model.SensitivityScenario{
		{Name: "Baseline", Threshold: fmt.Sprintf("%.0f%%", threshold), NIncluded: base, Findings: "set studi acuan"},
		{Name: "Ketat", Threshold: fmt.Sprintf("%.0f%%", threshold+10), NIncluded: strict, Findings: fmt.Sprintf("%+d studi vs baseline", strict-base)},
		{Name: "Longgar", Threshold: fmt.Sprintf("%.0f%%", threshold-10), NIncluded: loose, Findings: fmt.Sprintf("%+d studi vs baseline", loose-base)},
	}
	md := fmt.Sprintf("## Sensitivity Analysis\n\n| Skenario | Threshold | n included | Catatan |\n|---|---|---|---|\n"+
		"| Baseline | %.0f%% | %d | acuan |\n| Ketat | %.0f%% | %d | %+d |\n| Longgar | %.0f%% | %d | %+d |\n\n**Verdict:** %s\n\n**Penjelasan (xAI):** %s",
		threshold, base, threshold+10, strict, strict-base, threshold-10, loose, loose-base, verdict, reasoning)
	return &model.SensitivityAnalysis{Scenarios: sc, Verdict: verdict, Markdown: md}
}

// ===== L4: Synthesis prep + meta-analysis feasibility + summary =====

func (m *M7Extraction) runSynthesisL4(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 7.4] Synthesis preparation + meta-analysis feasibility...")
	docs := m.allRated(ctx, session)

	// Ringkasan untuk LLM.
	designs := tallyExtField(docs, "design")
	geo := tallyExtField(docs, "geographic")
	years := tallyMeta(docs, "Year")
	qaDist := map[string]int{}
	for _, p := range docs {
		qaDist[getStr(p, "qa_final_category")]++
	}
	summary := map[string]interface{}{
		"framework":          frameworkName(session),
		"total_included":     len(docs),
		"design_breakdown":   designs,
		"geographic":         geo,
		"year_distribution":  years,
		"quality_distribution": qaDist,
		"qa_threshold":       session.QAThreshold,
	}
	sumJSON, _ := json.Marshal(summary)

	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return fmt.Errorf("gemini (brain synthesis) gagal: %w", err)
	}
	sp, err := agent.NewExtractionAgent(brain).PrepareSynthesis(ctx, string(sumJSON))
	if err != nil {
		return err
	}
	session.SynthesisPrep = sp

	// modul7_summary
	fwLine := "-"
	if session.FrameworkSelection != nil {
		fwLine = fmt.Sprintf("%s — %s", session.FrameworkSelection.Framework, session.FrameworkSelection.Justification)
	}
	exLog := session.ExtractionLog
	exLine := "tidak tersedia"
	if exLog != nil {
		exLine = fmt.Sprintf("Total %d | verifikasi %d | disagreement %.1f%% | ambiguous %d",
			exLog.TotalExtracted, exLog.VerifiedSample, exLog.DisagreementRate, exLog.AmbiguousCount)
	}
	qa := session.QAThreshold
	qaLine := "tidak tersedia"
	if qa != nil {
		qaLine = fmt.Sprintf("Tool %s | threshold %.0f%% | kappa %.3f | kategori: HIGH %d / MODERATE %d / LOW %d",
			qa.Tool, qa.Threshold, qa.Kappa, qaDist["HIGH"], qaDist["MODERATE"], qaDist["LOW"])
	}
	sens := "tidak tersedia"
	if session.SensitivityAnalysis != nil {
		sens = session.SensitivityAnalysis.Verdict
	}
	md := fmt.Sprintf("=== EXTRACTION + QA SUMMARY (SLR) ===\n\n"+
		"FRAMEWORK: %s\n\n"+
		"EXTRACTION: %s\n\n"+
		"QUALITY ASSESSMENT: %s\n\n"+
		"SENSITIVITY: %s\n\n"+
		"HETEROGENEITY VERDICT: %s\n"+
		"META-ANALYSIS FEASIBILITY: %s\n\n"+
		"DESCRIPTIVE OVERVIEW:\n%s\n\n"+
		"FRAMEWORK-DRIVEN GROUPINGS:\n%s\n\n"+
		"NEXT: Data Analysis + Synthesis (Modul 8)",
		fwLine, exLine, qaLine, sens, sp.HeterogeneityVerdict, sp.MetaFeasibility, sp.DescriptiveOverview, sp.Groupings)
	session.Modul7Summary = &model.Modul7Summary{Markdown: md}

	session.Status = "M7_STEP4_WAITING_APPROVAL"
	logger.Log(session.ID, "   [System] synthesis_prep + modul7_summary tersimpan.")
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

func (m *M7Extraction) designBreakdownFromExtraction(ctx context.Context, session *model.SLRSession) string {
	docs := m.allRated(ctx, session)
	t := tallyExtField(docs, "design")
	if len(t) == 0 {
		return fmt.Sprintf("(design tidak terekstrak; total %d studi)", len(docs))
	}
	s := fmt.Sprintf("Total %d studi. Designs: ", len(docs))
	for k, v := range t {
		s += fmt.Sprintf("%s=%d; ", k, v)
	}
	return s
}

// ===== small helpers =====

func frameworkName(session *model.SLRSession) string {
	if session.FrameworkSelection != nil {
		return session.FrameworkSelection.Framework
	}
	return "CUSTOM"
}

func extFieldValue(p bson.M, keySub string) string {
	arr, ok := p["fields"].(bson.A)
	if !ok {
		return ""
	}
	for _, it := range arr {
		f, ok := it.(bson.M)
		if !ok {
			continue
		}
		k, _ := f["key"].(string)
		if strings.Contains(strings.ToLower(k), strings.ToLower(keySub)) {
			v, _ := f["value"].(string)
			return v
		}
	}
	return ""
}

func tallyExtField(docs []bson.M, keySub string) map[string]int {
	out := map[string]int{}
	for _, p := range docs {
		v := extFieldValue(p, keySub)
		if v == "" || v == "[NOT REPORTED]" {
			continue
		}
		if len(v) > 40 {
			v = v[:40]
		}
		out[v]++
	}
	return out
}

func tallyMeta(docs []bson.M, key string) map[string]int {
	out := map[string]int{}
	for _, p := range docs {
		v := getStr(p, key)
		if v == "" {
			continue
		}
		out[v]++
	}
	return out
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	}
	return 0, false
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
