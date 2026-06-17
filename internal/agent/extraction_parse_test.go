package agent

import (
	"context"
	"testing"
)

// stubClient mengembalikan raw yang sudah ditentukan, untuk menguji ketahanan parser.
type stubClient struct{ raw string }

func (s stubClient) Generate(_ context.Context, _, _ string) (string, error) { return s.raw, nil }
func (s stubClient) ModelName() string                                       { return "stub/model" }

func TestExtractPaper_RecoversTruncatedFieldsArray(t *testing.T) {
	// Kasus nyata: wrapper objek ke-truncate, CleanJSONResponse menyisakan array `fields`
	// saja. Parser lama gagal ("cannot unmarshal array") & MEMBUANG hasil ekstraksi.
	raw := `[
	  {"key":"id","value":"S01","evidence":"p.1","status":"REPORTED"},
	  {"key":"ssm_variant","value":"Mamba","evidence":"Methods p.3","status":"REPORTED"}
	]`
	ag := NewExtractionAgent(stubClient{raw: raw})
	res, err := ag.ExtractPaper(context.Background(), "[]", "", "T", "full text")
	if err != nil {
		t.Fatalf("expected recovery, got error: %v", err)
	}
	if len(res.Fields) != 2 {
		t.Fatalf("expected 2 recovered fields, got %d", len(res.Fields))
	}
	if res.Fields[1].Value != "Mamba" {
		t.Errorf("field data lost on recovery: %+v", res.Fields[1])
	}
}

func TestExtractPaper_RecoversArrayOfResults(t *testing.T) {
	raw := `[{"fields":[{"key":"id","value":"S02","evidence":"","status":"REPORTED"}],"coverage":"COMPLETE"}]`
	ag := NewExtractionAgent(stubClient{raw: raw})
	res, err := ag.ExtractPaper(context.Background(), "[]", "", "T", "ft")
	if err != nil {
		t.Fatalf("expected recovery of result-array, got error: %v", err)
	}
	if len(res.Fields) != 1 || res.Fields[0].Value != "S02" {
		t.Fatalf("wrong recovery: %+v", res.Fields)
	}
}

func TestExtractPaper_RejectsNonFieldArray(t *testing.T) {
	// Array yang BUKAN field (tanpa Key) tidak boleh "berhasil" jadi field kosong.
	raw := `["foo","bar"]`
	ag := NewExtractionAgent(stubClient{raw: raw})
	if _, err := ag.ExtractPaper(context.Background(), "[]", "", "T", "ft"); err == nil {
		t.Fatal("expected error for non-field array, got nil")
	}
}

func TestNormalizeCoverage(t *testing.T) {
	cases := []struct {
		in, wantTok, wantNote string
	}{
		{"COMPLETE", "COMPLETE", ""},
		{"PARTIAL", "PARTIAL", ""},
		{"PARTIAL – core metadata reported; efficiency missing", "PARTIAL", "core metadata reported; efficiency missing"},
		{"Partial: key quantitative efficiency missing", "PARTIAL", "key quantitative efficiency missing"},
		{"INCOMPLETE - no results", "INCOMPLETE", "no results"},
		{"some unknown blob", "PARTIAL", "some unknown blob"},
		{"", "", ""},
	}
	for _, c := range cases {
		tok, note := NormalizeCoverage(c.in)
		if tok != c.wantTok || note != c.wantNote {
			t.Errorf("NormalizeCoverage(%q) = (%q,%q); want (%q,%q)", c.in, tok, note, c.wantTok, c.wantNote)
		}
	}
}
