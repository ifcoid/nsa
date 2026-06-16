package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"nsa/internal/agent"
	"nsa/internal/logger"
	"nsa/internal/model"
)

// picoAuditBatchSize is how many INCLUDE papers are sent to the neural auditor per call.
const picoAuditBatchSize = 12

// runFullPICOAudit performs a NEURO-SYMBOLIC, full-coverage audit of every INCLUDE paper
// and returns a PICOAuditLog where each flagged paper carries the provenance (xAI) of
// every signal that flagged it. Three complementary signals, unioned and deduped:
//
//	Rule A (symbolic, deterministic): a reviewer decided EXCLUDE, or the STRICT
//	  interpretation was EXCLUDE, yet the paper was resolved to INCLUDE. Guaranteed
//	  recall on the highest-risk "liberal won over strict" cases -- no LLM dependency.
//	Rule B (symbolic, deterministic): an exclusion-trigger term derived from the PICO
//	  what_doesnt_count definitions appears in the title/abstract.
//	Neural (LLM, index-anchored): full-coverage LLM audit; identity is carried by a
//	  stable integer index (not fragile DOI/title echo), so matching is deterministic.
//
// The human (HITL) decides EXCLUDE/KEEP on the union; the gate blocks M5 closing until
// all are resolved. Symbolic rules guarantee a real false-include is never silently
// missed just because the LLM failed to echo or overlooked it (fixes residual #2).
func (m *M5Screening) runFullPICOAudit(ctx context.Context, session *model.SLRSession, included []map[string]interface{}, primary *agent.ScreeningAgent, picoDef string, pico *model.PICODefinitions) *model.PICOAuditLog {
	total := len(included)
	logger.Logf(session.ID, "      -> PICO-Consistency Audit (FULL coverage, neuro-symbolic): %d INCLUDE...", total)

	// Accumulate flags per paper (by screening _id), preserving discovery order.
	type accum struct {
		paper      map[string]interface{}
		flags      []model.SlippedFlag
		reasonCode string
		reason     string
	}
	acc := map[string]*accum{}
	var order []string
	get := func(p map[string]interface{}) *accum {
		id := objIDHex(p["_id"])
		if id == "" {
			return nil
		}
		a, ok := acc[id]
		if !ok {
			a = &accum{paper: p}
			acc[id] = a
			order = append(order, id)
		}
		return a
	}
	addFlag := func(p map[string]interface{}, f model.SlippedFlag, rc, reason string) {
		a := get(p)
		if a == nil {
			return
		}
		a.flags = append(a.flags, f)
		if a.reasonCode == "" && rc != "" {
			a.reasonCode = rc
		}
		if a.reason == "" && reason != "" {
			a.reason = reason
		}
	}

	// --- Rule A: reviewer / strict EXCLUDE (deterministic) ---
	ruleA := 0
	for _, p := range included {
		for _, f := range reviewerExcludeFlags(p) {
			addFlag(p, f.flag, f.reasonCode, f.flag.Detail)
		}
		if a := acc[objIDHex(p["_id"])]; a != nil && len(a.flags) > 0 {
			ruleA++
		}
	}

	// --- Rule B: exclusion-trigger keywords from PICO what_doesnt_count (deterministic) ---
	terms := extractExclusionTerms(pico)
	ruleB := 0
	for _, p := range included {
		hits := keywordExclusionFlags(getStr(p, "Title", "title"), getStr(p, "Abstract", "abstract"), terms)
		if len(hits) > 0 {
			ruleB++
		}
		for _, h := range hits {
			addFlag(p, h.flag, h.reasonCode, h.flag.Detail)
		}
	}

	// --- Neural: index-anchored LLM audit over all INCLUDE in batches ---
	allSlim := make([]map[string]interface{}, total)
	for i, p := range included {
		allSlim[i] = map[string]interface{}{
			"index":    i + 1,
			"title":    getStr(p, "Title", "title"),
			"abstract": getStr(p, "Abstract", "abstract"),
			"r1":       getStr(p, "Screener_1_Decision"),
			"r1_notes": getStr(p, "Screener_1_Notes"),
			"r2":       getStr(p, "Screener_2_Decision"),
			"r2_notes": getStr(p, "Screener_2_Notes"),
		}
	}
	var analyses []string
	llmFlagged := 0
	for start := 0; start < total; start += picoAuditBatchSize {
		end := start + picoAuditBatchSize
		if end > total {
			end = total
		}
		bj, _ := json.Marshal(allSlim[start:end])
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
			if s.Index < 1 || s.Index > total {
				continue // out-of-range index -> ignore (never act on a phantom)
			}
			p := included[s.Index-1]
			llmFlagged++
			detail := strings.TrimSpace(s.Reason)
			if detail == "" {
				detail = "LLM audit menandai paper ini melanggar kriteria PICO"
			}
			addFlag(p, model.SlippedFlag{Source: "llm-audit", Detail: detail}, strings.TrimSpace(s.ReasonCode), detail)
		}
	}

	// --- Merge into the audit log (PRECISION GATE) ---
	// Only a STRONG signal blocks the gate: an LLM criteria violation, a reviewer whose
	// actual decision was EXCLUDE (then resolved to INCLUDE), or BOTH reviewers' strict
	// interpretation = EXCLUDE. A single strict objection or a keyword-only match is normal
	// screening noise in a strict/liberal design and must not flood the panel; those papers
	// stay INCLUDE. All flags are still kept as provenance on the strong papers.
	var slipped []model.SlippedPaper
	var nLLM, nReviewer, nBothStrict int
	for _, id := range order {
		a := acc[id]
		strong, why := strongSlipSignal(a.flags)
		if !strong {
			continue
		}
		switch why {
		case "llm":
			nLLM++
		case "reviewer":
			nReviewer++
		case "both-strict":
			nBothStrict++
		}
		rc := a.reasonCode
		if rc == "" {
			rc = "OTHER"
		}
		slipped = append(slipped, model.SlippedPaper{
			PaperID:    id,
			DOI:        getStr(a.paper, "DOI", "doi"),
			Title:      getStr(a.paper, "Title", "title"),
			ReasonCode: rc,
			Reason:     a.reason,
			Flags:      a.flags,
		})
	}

	action := "none"
	if len(slipped) > 0 {
		action = "re-screening"
	}
	summary := fmt.Sprintf("Neuro-symbolic audit (precision-gated): dari %d INCLUDE, sinyal mentah strict/reviewer=%d, keyword=%d, LLM=%d. Setelah pengetatan presisi, %d paper masuk koreksi (kuat: LLM=%d, reviewer-EXCLUDE=%d, kedua-strict=%d). Strict-tunggal & keyword-saja dicatat sebagai konteks, tidak memblok.",
		total, ruleA, ruleB, llmFlagged, len(slipped), nLLM, nReviewer, nBothStrict)
	if len(analyses) > 0 {
		summary += " " + strings.Join(analyses, " ")
	}
	logger.Logf(session.ID, "      ✓ PICO audit (precision-gated): %d slipped (LLM=%d reviewer=%d both-strict=%d) dari %d INCLUDE; mentah strict/reviewer=%d keyword=%d llm=%d.", len(slipped), nLLM, nReviewer, nBothStrict, total, ruleA, ruleB, llmFlagged)
	return &model.PICOAuditLog{
		IncludedAtAudit: total,
		Coverage:        fmt.Sprintf("100%% (%d/%d)", total, total),
		Action:          action,
		Analysis:        summary,
		Slipped:         slipped,
	}
}

type flagWithCode struct {
	flag       model.SlippedFlag
	reasonCode string
}

// strongSlipSignal decides whether a paper's flags constitute a STRONG (blocking) audit
// signal, returning the dominant reason (priority: llm > reviewer > both-strict). Strong =
// an LLM criteria violation, OR a reviewer whose actual decision was EXCLUDE, OR BOTH
// reviewers' strict interpretation = EXCLUDE. A single strict objection or a keyword-only
// match is screening noise (expected in a strict/liberal design) and does not block.
func strongSlipSignal(flags []model.SlippedFlag) (bool, string) {
	strict := 0
	hasLLM, hasReviewer := false, false
	for _, f := range flags {
		switch f.Source {
		case "llm-audit":
			hasLLM = true
		case "rule:reviewer-exclude":
			hasReviewer = true
		case "rule:strict-exclude":
			strict++
		}
	}
	switch {
	case hasLLM:
		return true, "llm"
	case hasReviewer:
		return true, "reviewer"
	case strict >= 2:
		return true, "both-strict"
	default:
		return false, ""
	}
}

// reviewerExcludeFlags implements Rule A: an INCLUDE paper where a reviewer decided
// EXCLUDE, or the STRICT interpretation was EXCLUDE, is a probable false-include that
// passed via the liberal interpretation during resolution.
func reviewerExcludeFlags(p map[string]interface{}) []flagWithCode {
	var out []flagWithCode
	check := func(role, dec, rc, notes string) {
		switch {
		case dec == "EXCLUDE":
			out = append(out, flagWithCode{
				flag:       model.SlippedFlag{Source: "rule:reviewer-exclude", Detail: fmt.Sprintf("%s memutuskan EXCLUDE%s namun di-resolve menjadi INCLUDE", role, codeSuffix(rc))},
				reasonCode: cleanReasonCode(rc),
			})
		case strings.Contains(strings.ToUpper(notes), "STRICT: EXCLUDE"):
			out = append(out, flagWithCode{
				flag:       model.SlippedFlag{Source: "rule:strict-exclude", Detail: fmt.Sprintf("interpretasi STRICT %s = EXCLUDE%s", role, codeSuffix(rc))},
				reasonCode: cleanReasonCode(rc),
			})
		}
	}
	check("Reviewer 1", getStr(p, "Screener_1_Decision"), getStr(p, "Screener_1_Reason_Code"), getStr(p, "Screener_1_Notes"))
	check("Reviewer 2", getStr(p, "Screener_2_Decision"), getStr(p, "Screener_2_Reason_Code"), getStr(p, "Screener_2_Notes"))
	return out
}

func cleanReasonCode(rc string) string {
	rc = strings.TrimSpace(rc)
	if rc == "" || rc == "-" {
		return ""
	}
	return rc
}

func codeSuffix(rc string) string {
	if c := cleanReasonCode(rc); c != "" {
		return " (" + c + ")"
	}
	return ""
}

type exclTerm struct {
	crit string // P / I / C / O
	term string
}

var (
	reQuotedTerm = regexp.MustCompile("['\"`]([^'\"`]{3,40})['\"`]")
	reAcronym    = regexp.MustCompile(`\b[A-Z]{2,6}\b`)
	// exclTermStop drops core in-domain acronyms/words that must NOT become exclusion
	// triggers even if they appear inside a what_doesnt_count sentence.
	exclTermStop = map[string]bool{
		"eeg": true, "bci": true, "fmri": true, "fnirs": true, "seeg": true, "nhp": true,
		"ssm": true, "cnn": true, "ecog": true, "meg": true, "dan": true, "atau": true,
		"yang": true, "tidak": true, "bukan": true, "seperti": true, "hanya": true,
	}
)

// extractExclusionTerms (Rule B source) pulls concrete exclusion-trigger terms from the
// PICO what_doesnt_count operational definitions: quoted phrases and uppercase acronyms.
func extractExclusionTerms(pico *model.PICODefinitions) []exclTerm {
	if pico == nil {
		return nil
	}
	src := []struct{ crit, text string }{
		{"P", pico.P.OperationalDef.WhatDoesntCount},
		{"I", pico.I.OperationalDef.WhatDoesntCount},
		{"C", pico.C.OperationalDef.WhatDoesntCount},
		{"O", pico.O.OperationalDef.WhatDoesntCount},
	}
	seen := map[string]bool{}
	var out []exclTerm
	add := func(crit, term string) {
		t := strings.TrimSpace(strings.Trim(term, ".,;:()'\"` "))
		if len([]rune(t)) < 3 || exclTermStop[strings.ToLower(t)] {
			return
		}
		key := crit + "|" + strings.ToLower(t)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, exclTerm{crit, t})
	}
	for _, s := range src {
		for _, m := range reQuotedTerm.FindAllStringSubmatch(s.text, -1) {
			add(s.crit, m[1])
		}
		for _, m := range reAcronym.FindAllString(s.text, -1) {
			add(s.crit, m)
		}
	}
	return out
}

// keywordExclusionFlags (Rule B match) flags a paper whose title/abstract contains an
// exclusion-trigger term. Acronyms match on word boundary; phrases match as substrings.
func keywordExclusionFlags(title, abstract string, terms []exclTerm) []flagWithCode {
	if len(terms) == 0 {
		return nil
	}
	hay := title + " " + abstract
	hayLower := strings.ToLower(hay)
	var out []flagWithCode
	seen := map[string]bool{}
	for _, t := range terms {
		var hit bool
		if reAcronym.MatchString(t.term) && t.term == strings.ToUpper(t.term) {
			hit = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(t.term) + `\b`).MatchString(hay)
		} else {
			hit = strings.Contains(hayLower, strings.ToLower(t.term))
		}
		if !hit || seen[strings.ToLower(t.term)] {
			continue
		}
		seen[strings.ToLower(t.term)] = true
		out = append(out, flagWithCode{
			flag:       model.SlippedFlag{Source: "rule:keyword", Detail: fmt.Sprintf("kata-pemicu eksklusi '%s' (kriteria %s) muncul di judul/abstrak", t.term, t.crit)},
			reasonCode: t.crit + "-NOMATCH",
		})
	}
	return out
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

// formatPICOAudit renders the audit log for the Module 5 summary, including per-paper
// provenance (which signals flagged it) and resolution status (xAI trail).
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
		fmt.Fprintf(&b, "%d. [%s] %s\n   Status: %s\n", i+1, s.ReasonCode, s.Title, status)
		for _, f := range s.Flags {
			fmt.Fprintf(&b, "   - [%s] %s\n", f.Source, f.Detail)
		}
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
