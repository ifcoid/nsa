package modules

import (
	"strings"
	"testing"
)

// TestAssembleFulltextKeepsAll: paper muat budget → semua chunk dipakai, tak ada potongan.
func TestAssembleFulltextKeepsAll(t *testing.T) {
	chunks := []ragChunk{
		{idx: 0, content: "abstract text", section: "Abstract"},
		{idx: 1, content: "methods text", section: "Methods"},
		{idx: 2, content: "results text", section: "Results"},
	}
	out := assembleFulltext(chunks, 10000)
	for _, want := range []string{"abstract text", "methods text", "results text"} {
		if !strings.Contains(out, want) {
			t.Fatalf("hasil tak memuat %q: %s", want, out)
		}
	}
	if strings.Contains(out, "dipotong") {
		t.Fatalf("tak boleh ada penanda potong saat muat budget: %s", out)
	}
}

// TestAssembleFulltextPrioritizesTailSections: INTI PERBAIKAN. Saat melebihi budget,
// chunk Results/Discussion di EKOR dokumen harus tetap lolos meski Intro/Related panjang;
// head-truncate lama akan membuang justru Results/Discussion ini.
func TestAssembleFulltextPrioritizesTailSections(t *testing.T) {
	filler := strings.Repeat("x", 400)
	chunks := []ragChunk{
		{idx: 0, content: "OPENER " + filler, section: "Abstract"},
		{idx: 1, content: "INTRO " + filler, section: "Introduction"},
		{idx: 2, content: "RELATED " + filler, section: "Related Work"},
		{idx: 3, content: "BACKGROUND " + filler, section: "Background"},
		{idx: 4, content: "METHODS " + filler, section: "Methods"},
		{idx: 5, content: "RESULTS accuracy 92% " + filler, section: "Results"},
		{idx: 6, content: "DISCUSSION ITR 5.39 bits " + filler, section: "Discussion"},
	}
	// Budget hanya cukup untuk ~4 chunk → memaksa pembuangan.
	out := assembleFulltext(chunks, 4*420)

	if !strings.Contains(out, "RESULTS accuracy 92%") {
		t.Errorf("Results (ekor) terbuang — regresi truncation: %s", out)
	}
	if !strings.Contains(out, "DISCUSSION ITR 5.39 bits") {
		t.Errorf("Discussion (ekor) terbuang — regresi truncation: %s", out)
	}
	if !strings.Contains(out, "OPENER") {
		t.Errorf("pembuka tak dijamin masuk: %s", out)
	}
	if !strings.Contains(out, "dipotong") {
		t.Errorf("penanda potong wajib ada saat melebihi budget: %s", out)
	}
	// Output harus tetap urut chunk_index: OPENER sebelum RESULTS sebelum DISCUSSION.
	if iO, iR, iD := strings.Index(out, "OPENER"), strings.Index(out, "RESULTS"), strings.Index(out, "DISCUSSION"); !(iO < iR && iR < iD) {
		t.Errorf("urutan chunk_index rusak (OPENER=%d RESULTS=%d DISCUSSION=%d)", iO, iR, iD)
	}
}

// TestAssembleFulltextEmptySectionFallback: data lama tanpa section_header tetap
// terisi urut dokumen (tak crash, tak kosong).
func TestAssembleFulltextEmptySectionFallback(t *testing.T) {
	filler := strings.Repeat("y", 400)
	chunks := []ragChunk{
		{idx: 0, content: "A " + filler},
		{idx: 1, content: "B " + filler},
		{idx: 2, content: "C " + filler},
	}
	out := assembleFulltext(chunks, 2*420)
	if !strings.Contains(out, "A ") {
		t.Fatalf("chunk pembuka harus tetap masuk pada fallback: %s", out)
	}
}
