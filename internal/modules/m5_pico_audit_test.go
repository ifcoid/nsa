package modules

import (
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
		{PaperID: "keep", Actioned: false},     // still included
		{PaperID: "gone", Actioned: false},     // excluded -> should be marked resolved
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
