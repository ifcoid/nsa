package agent

import (
	"encoding/json"
	"testing"
)

func unmarshalQA(raw string, q *QAResult) error {
	return json.Unmarshal([]byte(CleanJSONResponse(raw)), q)
}

// Regresi: rater (mis. mistral-large-latest) mengembalikan items_summary sebagai OBJEK
// per-domain, bukan string. Harus di-flatten (deterministik), bukan gagal keras
// ("json: cannot unmarshal object into Go struct field QAResult.items_summary").
func TestQAResult_ItemsSummaryObject(t *testing.T) {
	raw := `{"total_score":78,"category":"HIGH","items_summary":{"Domain 1":"data bersih (25/25)","Domain 2":"desain jelas (18/20)"},"reasoning":"skor tinggi","evidence":"'kutipan'"}`
	var q QAResult
	if err := unmarshalQA(raw, &q); err != nil {
		t.Fatalf("unmarshal gagal: %v", err)
	}
	if q.TotalScore != 78 || q.Category != "HIGH" {
		t.Fatalf("scalar salah: %+v", q)
	}
	want := "Domain 1: data bersih (25/25); Domain 2: desain jelas (18/20)"
	if q.ItemsSummary != want {
		t.Fatalf("items_summary flatten salah:\n got=%q\nwant=%q", q.ItemsSummary, want)
	}
}

func TestQAResult_ItemsSummaryString(t *testing.T) {
	raw := `{"total_score":"80%","category":"HIGH","items_summary":"Domain 1: ok","reasoning":"r","evidence":"e"}`
	var q QAResult
	if err := unmarshalQA(raw, &q); err != nil {
		t.Fatalf("unmarshal gagal: %v", err)
	}
	if q.TotalScore != 80 {
		t.Fatalf("total_score string→float salah: %v", q.TotalScore)
	}
	if q.ItemsSummary != "Domain 1: ok" {
		t.Fatalf("items_summary string salah: %q", q.ItemsSummary)
	}
}
