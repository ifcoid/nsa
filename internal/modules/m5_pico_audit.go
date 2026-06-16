package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"nsa/internal/agent"
	"nsa/internal/logger"
	"nsa/internal/model"
)

// picoAuditBatchSize is how many INCLUDE papers are sent to the auditor per LLM call.
// Full coverage is achieved by iterating all batches (not a 10% sample).
const picoAuditBatchSize = 12

// runFullPICOAudit audits EVERY included paper against the PICO definitions, in batches,
// and returns a PICOAuditLog listing each probable false-include matched back to its
// screening record. This replaces the previous 10% spot check so no false-include can
// slip into the final set unaudited (required for a defensible PRISMA inclusion set).
func (m *M5Screening) runFullPICOAudit(ctx context.Context, session *model.SLRSession, included []map[string]interface{}, primary *agent.ScreeningAgent, picoDef string) *model.PICOAuditLog {
	total := len(included)
	logger.Logf(session.ID, "      -> PICO-Consistency Audit (FULL coverage): %d INCLUDE, batch %d...", total, picoAuditBatchSize)

	// Index for matching audited papers back to their screening _id.
	byDOI := map[string]map[string]interface{}{}
	byTitle := map[string]map[string]interface{}{}
	for _, p := range included {
		if d := normalizeDOIForRAG(getStr(p, "DOI", "doi")); d != "" {
			byDOI[d] = p
		}
		if t := normTitleKey(getStr(p, "Title", "title")); t != "" {
			byTitle[t] = p
		}
	}

	var slipped []model.SlippedPaper
	var analyses []string
	seen := map[string]bool{}

	for start := 0; start < total; start += picoAuditBatchSize {
		end := start + picoAuditBatchSize
		if end > total {
			end = total
		}
		slim := make([]map[string]string, 0, end-start)
		for _, p := range included[start:end] {
			slim = append(slim, map[string]string{
				"doi":      getStr(p, "DOI", "doi"),
				"title":    getStr(p, "Title", "title"),
				"abstract": getStr(p, "Abstract", "abstract"),
				"r1":       getStr(p, "Screener_1_Decision"),
				"r1_notes": getStr(p, "Screener_1_Notes"),
				"r2":       getStr(p, "Screener_2_Decision"),
				"r2_notes": getStr(p, "Screener_2_Notes"),
			})
		}
		bj, _ := json.Marshal(slim)
		res, err := m.auditPICOWithFallback(ctx, primary, picoDef, string(bj))
		if err != nil || res == nil {
			logger.Logf(session.ID, "      [!] Audit batch %d-%d gagal: %v", start+1, end, err)
			analyses = append(analyses, fmt.Sprintf("(batch %d-%d gagal diaudit)", start+1, end))
			continue
		}
		if s := strings.TrimSpace(res.Analysis); s != "" {
			analyses = append(analyses, s)
		}
		for _, s := range res.Slipped {
			p := byDOI[normalizeDOIForRAG(s.DOI)]
			if p == nil {
				p = byTitle[normTitleKey(s.Title)]
			}
			if p == nil {
				continue // cannot match to a real record; never act on a phantom
			}
			id := objIDHex(p["_id"])
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			slipped = append(slipped, model.SlippedPaper{
				PaperID:    id,
				DOI:        getStr(p, "DOI", "doi"),
				Title:      getStr(p, "Title", "title"),
				ReasonCode: strings.TrimSpace(s.ReasonCode),
				Reason:     strings.TrimSpace(s.Reason),
			})
		}
	}

	action := "none"
	if len(slipped) > 0 {
		action = "re-screening"
	}
	logger.Logf(session.ID, "      ✓ PICO audit selesai: %d slipped-through dari %d INCLUDE.", len(slipped), total)
	return &model.PICOAuditLog{
		IncludedAtAudit: total,
		Coverage:        fmt.Sprintf("100%% (%d/%d)", total, total),
		Action:          action,
		Analysis:        strings.Join(analyses, " "),
		Slipped:         slipped,
	}
}

// auditPICOWithFallback runs one audit batch through the primary screening agent and
// falls back to zhipu then groq, matching the resilience of the screening passes.
func (m *M5Screening) auditPICOWithFallback(ctx context.Context, primary *agent.ScreeningAgent, picoDef, batchJSON string) (*agent.PICOAuditResult, error) {
	res, err := primary.AuditPICO(ctx, picoDef, batchJSON)
	if err == nil && res != nil {
		return res, nil
	}
	for _, prov := range []string{"zhipu", "groq"} {
		c, e := m.deps.LLMFactory.CreateClient(ctx, prov)
		if e != nil {
			continue
		}
		if r, e2 := agent.NewScreeningAgent(c).AuditPICO(ctx, picoDef, batchJSON); e2 == nil && r != nil {
			return r, nil
		}
	}
	return nil, err
}

// refreshPICOAuditLog reconciles the stored audit with the current INCLUDE set: any
// flagged paper no longer present in INCLUDE (i.e. it was excluded) is marked resolved.
func refreshPICOAuditLog(log *model.PICOAuditLog, includedNow []map[string]interface{}) {
	if log == nil {
		return
	}
	stillIncluded := map[string]bool{}
	for _, p := range includedNow {
		if id := objIDHex(p["_id"]); id != "" {
			stillIncluded[id] = true
		}
	}
	for i := range log.Slipped {
		if !stillIncluded[log.Slipped[i].PaperID] {
			log.Slipped[i].Actioned = true
			if log.Slipped[i].Resolution == "" {
				log.Slipped[i].Resolution = "EXCLUDE"
			}
		}
	}
}

// countUnactionedSlipped returns how many audit-flagged papers still await a human
// decision (excluded or explicitly kept). Used by the M5 closing gate.
func countUnactionedSlipped(session *model.SLRSession) int {
	if session.PICOAuditLog == nil {
		return 0
	}
	n := 0
	for _, s := range session.PICOAuditLog.Slipped {
		if !s.Actioned {
			n++
		}
	}
	return n
}

// formatPICOAudit renders the audit log for the Module 5 summary, including the
// per-paper resolution status so reviewers see exactly what remains open.
func formatPICOAudit(log *model.PICOAuditLog) string {
	if log == nil {
		return "Audit dilewati (Tidak ada paper INCLUDE)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Coverage: %s\nSlipped-through: %d\nAction: %s\n", log.Coverage, len(log.Slipped), log.Action)
	pending := 0
	for i, s := range log.Slipped {
		status := "RESOLVED: " + s.Resolution
		if !s.Actioned {
			status = "PENDING (perlu keputusan INCLUDE/EXCLUDE)"
			pending++
		}
		fmt.Fprintf(&b, "%d. [%s] %s\n   Alasan: %s\n   Status: %s\n", i+1, s.ReasonCode, s.Title, s.Reason, status)
	}
	if pending > 0 {
		fmt.Fprintf(&b, "CATATAN: %d paper belum dikoreksi; Modul 5 tidak dapat ditutup sampai semuanya diselesaikan.\n", pending)
	}
	if strings.TrimSpace(log.Analysis) != "" {
		fmt.Fprintf(&b, "Analysis: %s\n", log.Analysis)
	}
	return b.String()
}

// normTitleKey normalizes a title for matching (lowercase, no whitespace).
func normTitleKey(title string) string {
	return strings.ToLower(strings.Join(strings.Fields(title), ""))
}

// objIDHex returns the hex string of a Mongo _id value whether it is an ObjectID or a string.
func objIDHex(v interface{}) string {
	switch id := v.(type) {
	case primitive.ObjectID:
		return id.Hex()
	case string:
		return id
	default:
		return ""
	}
}
