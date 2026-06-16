package http

import (
	"testing"

	"nsa/internal/model"
)

func TestModuleNum(t *testing.T) {
	cases := map[string]int{
		"M5_STEP4_APPROVED": 5,
		"M6_INIT":           6,
		"M9_GROUPB_WAITING": 9,
		"M8B_CLUSTER":       8,
		"COMPLETED":         -1,
		"":                  -1,
		"M_FOO":             -1,
	}
	for in, want := range cases {
		if got := moduleNum(in); got != want {
			t.Errorf("moduleNum(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestIsBackwardToM5(t *testing.T) {
	if !isBackwardToM5("M9_GROUPB_WAITING_APPROVAL", "M5_STEP3_WAITING_RESOLUTION") {
		t.Error("M9 -> M5 should be a backward jump")
	}
	if !isBackwardToM5("M6_INIT", "M5_STEP3_BATCH_SCREENING") {
		t.Error("M6 -> M5 should be a backward jump")
	}
	if isBackwardToM5("M5_STEP4_APPROVED", "M5_STEP3_WAITING_RESOLUTION") {
		t.Error("M5 -> M5 (within module) is NOT a downstream-invalidating jump")
	}
	if isBackwardToM5("M9_GROUPB", "M7_STEP3_QA") {
		t.Error("target M7 is not M5")
	}
}

func TestInvalidateDownstreamForRescreen(t *testing.T) {
	s := &model.SLRSession{
		Status:          "M9_GROUPB_WAITING_APPROVAL",
		Manuscript:      &model.Manuscript{Abstract: "stale abstract", Latex: "stale tex"},
		RescreenPending: false,
	}
	invalidateDownstreamForRescreen(s)
	if s.Manuscript != nil {
		t.Error("stale manuscript should be cleared")
	}
	if !s.RescreenPending {
		t.Error("RescreenPending should be set")
	}
}
