package modules

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"nsa/internal/model"
)

func TestCountUnactionedSlipped(t *testing.T) {
	if got := countUnactionedSlipped(&model.SLRSession{}); got != 0 {
		t.Fatalf("nil audit log should count 0, got %d", got)
	}
	s := &model.SLRSession{PICOAuditLog: &model.PICOAuditLog{Slipped: []model.SlippedPaper{
		{PaperID: "a", Actioned: true, Resolution: "EXCLUDE"},
		{PaperID: "b", Actioned: false},
		{PaperID: "c", Actioned: false},
	}}}
	if got := countUnactionedSlipped(s); got != 2 {
		t.Fatalf("expected 2 unactioned, got %d", got)
	}
}

func TestRefreshPICOAuditLog_ExcludedMarkedResolved(t *testing.T) {
	log := &model.PICOAuditLog{Slipped: []model.SlippedPaper{
		{PaperID: "keep", Actioned: false}, // still included
		{PaperID: "gone", Actioned: false}, // excluded -> should be marked resolved
	}}
	includedNow := []map[string]interface{}{{"_id": "keep"}}
	refreshPICOAuditLog(log, includedNow)

	byID := map[string]model.SlippedPaper{}
	for _, s := range log.Slipped {
		byID[s.PaperID] = s
	}
	if byID["gone"].Actioned != true || byID["gone"].Resolution != "EXCLUDE" {
		t.Fatalf("paper no longer included should be actioned/EXCLUDE, got %+v", byID["gone"])
	}
	if byID["keep"].Actioned != false {
		t.Fatalf("still-included paper must remain unactioned, got %+v", byID["keep"])
	}
}

func TestFormatPICOAudit(t *testing.T) {
	if got := formatPICOAudit(nil); got == "" {
		t.Fatal("nil log should still render a placeholder")
	}
	log := &model.PICOAuditLog{
		Coverage: "100% (124/124)", Action: "re-screening", Analysis: "ringkasan",
		Slipped: []model.SlippedPaper{
			{Title: "Paper X", ReasonCode: "I-NOMATCH", Reason: "bukan Mamba", Actioned: false},
			{Title: "Paper Y", ReasonCode: "P-NOMATCH", Reason: "ECG", Actioned: true, Resolution: "EXCLUDE"},
		},
	}
	out := formatPICOAudit(log)
	for _, want := range []string{"Coverage: 100% (124/124)", "Slipped-through: 2", "Paper X", "PENDING", "RESOLVED: EXCLUDE", "belum dikoreksi"} {
		if !contains(out, want) {
			t.Errorf("formatPICOAudit missing %q in:\n%s", want, out)
		}
	}
}

func TestReviewerExcludeFlags_RuleA(t *testing.T) {
	// Reviewer 2 said EXCLUDE but the paper is INCLUDE -> must flag (the Paper-10 case).
	p := map[string]interface{}{
		"Screener_1_Decision": "INCLUDE",
		"Screener_2_Decision": "EXCLUDE", "Screener_2_Reason_Code": "P-NOMATCH",
	}
	flags := reviewerExcludeFlags(p)
	if len(flags) != 1 || flags[0].flag.Source != "rule:reviewer-exclude" || flags[0].reasonCode != "P-NOMATCH" {
		t.Fatalf("expected reviewer-exclude flag with P-NOMATCH, got %+v", flags)
	}

	// STRICT=EXCLUDE inside notes even though overall decision was INCLUDE -> must flag.
	p2 := map[string]interface{}{
		"Screener_1_Decision": "INCLUDE",
		"Screener_1_Notes":    "Decision: INCLUDE | Strict: EXCLUDE | Liberal: INCLUDE | Evidence: x",
		"Screener_2_Decision": "INCLUDE",
	}
	flags2 := reviewerExcludeFlags(p2)
	if len(flags2) != 1 || flags2[0].flag.Source != "rule:strict-exclude" {
		t.Fatalf("expected strict-exclude flag, got %+v", flags2)
	}

	// Clean INCLUDE/INCLUDE with no strict objection -> no flag.
	if got := reviewerExcludeFlags(map[string]interface{}{"Screener_1_Decision": "INCLUDE", "Screener_2_Decision": "INCLUDE"}); len(got) != 0 {
		t.Fatalf("clean INCLUDE should not flag, got %+v", got)
	}
}

func TestExtractAndMatchExclusionTerms_RuleB(t *testing.T) {
	pico := &model.PICODefinitions{}
	pico.P.OperationalDef.WhatDoesntCount = "Mengecualikan sinyal periferal non-otak (seperti hanya EMG, ECG, atau EOG)."
	pico.I.OperationalDef.WhatDoesntCount = "Bukan 'RNN/LSTM' atau 'Attention GRU'; hanya Modern SSM."
	terms := extractExclusionTerms(pico)

	has := func(term string) bool {
		for _, x := range terms {
			if strings.EqualFold(x.term, term) {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"ECG", "EMG", "EOG", "GRU", "RNN/LSTM"} {
		if !has(want) {
			t.Errorf("expected exclusion term %q extracted, got %+v", want, terms)
		}
	}
	if has("EEG") {
		t.Error("EEG must be in the stoplist, not an exclusion trigger")
	}

	// Paper 10: abstract mentions ECG -> keyword flag fires with P-NOMATCH.
	hits := keywordExclusionFlags("Modeling Multivariate Biosignals", "uses EEG and ECG and PSG signals", terms)
	if len(hits) == 0 {
		t.Fatal("expected keyword flag for ECG in abstract")
	}
	if hits[0].flag.Source != "rule:keyword" || hits[0].reasonCode != "P-NOMATCH" {
		t.Errorf("expected keyword/P-NOMATCH, got %+v", hits[0])
	}
}

func TestStrongSlipSignal_PrecisionGate(t *testing.T) {
	llm := model.SlippedFlag{Source: "llm-audit"}
	rev := model.SlippedFlag{Source: "rule:reviewer-exclude"}
	strict := model.SlippedFlag{Source: "rule:strict-exclude"}
	kw := model.SlippedFlag{Source: "rule:keyword"}

	cases := []struct {
		name  string
		flags []model.SlippedFlag
		want  bool
		why   string
	}{
		{"llm blocks", []model.SlippedFlag{llm}, true, "llm"},
		{"reviewer-exclude blocks", []model.SlippedFlag{rev}, true, "reviewer"},
		{"both-strict blocks", []model.SlippedFlag{strict, strict}, true, "both-strict"},
		{"single strict = noise", []model.SlippedFlag{strict}, false, ""},
		{"keyword only = noise", []model.SlippedFlag{kw}, false, ""},
		{"single strict + keyword = noise", []model.SlippedFlag{strict, kw}, false, ""},
		{"llm dominates strict", []model.SlippedFlag{strict, strict, llm}, true, "llm"},
		{"reviewer dominates both-strict", []model.SlippedFlag{strict, strict, rev}, true, "reviewer"},
	}
	for _, c := range cases {
		got, why := strongSlipSignal(c.flags)
		if got != c.want || (got && why != c.why) {
			t.Errorf("%s: got (%v,%q), want (%v,%q)", c.name, got, why, c.want, c.why)
		}
	}
}

func TestFormatPICOAudit_ShowsProvenance(t *testing.T) {
	log := &model.PICOAuditLog{
		Coverage: "100% (124/124)", Action: "re-screening",
		Slipped: []model.SlippedPaper{{
			Title: "Paper Z", ReasonCode: "P-NOMATCH", Actioned: false,
			Flags: []model.SlippedFlag{
				{Source: "rule:reviewer-exclude", Detail: "Reviewer 2 memutuskan EXCLUDE"},
				{Source: "rule:keyword", Detail: "kata-pemicu eksklusi 'ECG'"},
			},
		}},
	}
	out := formatPICOAudit(log)
	for _, want := range []string{"rule:reviewer-exclude", "rule:keyword", "ECG", "PENDING"} {
		if !contains(out, want) {
			t.Errorf("provenance output missing %q in:\n%s", want, out)
		}
	}
}

func TestNormTitleKeyAndObjIDHex(t *testing.T) {
	if normTitleKey("  Hello   World  ") != "helloworld" {
		t.Errorf("normTitleKey: got %q", normTitleKey("  Hello   World  "))
	}
	oid := primitive.NewObjectID()
	if objIDHex(oid) != oid.Hex() {
		t.Error("objIDHex(ObjectID) mismatch")
	}
	if objIDHex("abc123") != "abc123" {
		t.Error("objIDHex(string) mismatch")
	}
	if objIDHex(42) != "" {
		t.Error("objIDHex(unknown) should be empty")
	}
}
