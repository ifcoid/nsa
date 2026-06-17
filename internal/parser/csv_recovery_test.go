package parser

import (
	"fmt"
	"strings"
	"testing"
)

// Clean Scopus CSV (escaped quotes, embedded commas/newlines) must parse ALL rows.
func TestScopusCSV_CleanParsesAll(t *testing.T) {
	var b strings.Builder
	b.WriteString("Authors,Title,Year,DOI,Abstract,EID\n")
	for i := 1; i <= 201; i++ {
		abs := "Abstract with \"\"escaped\"\" quotes, commas, and\nan embedded newline."
		b.WriteString(fmt.Sprintf("\"Auth %d\",\"Title %d\",2024,10.1/x%d,\"%s\",2-s2.0-%d\n", i, i, i, abs, i))
	}
	docs, err := parseCSV([]byte(b.String()))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(docs) != 201 {
		t.Errorf("clean Scopus: WANT 201, GOT %d", len(docs))
	}
}

// Invariant: dual-parse NEVER recovers fewer records than lazy-only (the old behavior),
// and never fewer than strict-only. Guards against the LazyQuotes silent-swallow regression.
func TestScopusCSV_DualParseNeverWorse(t *testing.T) {
	inputs := []string{
		"Authors,Title,Abstract\na1,t1,\"he said \"hi\" there\"\na2,t2,\"ok\"\na3,t3,\"x\" extra\"\na4,t4,\"ok\"\n",
		"Authors,Title,Abstract\na1,t1,\"unterminated\na2,t2,\"ok\"\na3,t3,\"a\",b\"c\n",
	}
	for i, in := range inputs {
		_, lazyRecs, _ := readCSVRecords([]byte(in), ',', true)
		_, strictRecs, _ := readCSVRecords([]byte(in), ',', false)
		docs, err := parseCSV([]byte(in))
		if err != nil {
			t.Fatalf("case %d err: %v", i, err)
		}
		best := len(lazyRecs)
		if len(strictRecs) > best {
			best = len(strictRecs)
		}
		// parseCSV drops rows with empty Title; titles present here, so docs == best records.
		if len(docs) < best {
			t.Errorf("case %d: dual-parse WORSE: docs=%d lazy=%d strict=%d", i, len(docs), len(lazyRecs), len(strictRecs))
		}
		t.Logf("case %d: docs=%d (lazy=%d strict=%d)", i, len(docs), len(lazyRecs), len(strictRecs))
	}
}

// PubMed NBIB with blank lines INSIDE records (8068dec fix) — guard against 21/82 regression.
func TestPubMedNBIB_BlankLinesInsideRecords(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 82; i++ {
		b.WriteString(fmt.Sprintf("PMID- %d\n", 1000+i))
		b.WriteString(fmt.Sprintf("TI  - Title of paper %d about\n", i))
		b.WriteString("      neural decoding with Mamba\n")
		b.WriteString("\n") // blank line INSIDE record (formatting, not separator)
		b.WriteString("AB  - Abstract text here.\n")
		b.WriteString("FAU - Doe, Jane\n\n")
	}
	docs, err := parseNBIB([]byte(b.String()))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(docs) != 82 {
		t.Errorf("PubMed: WANT 82, GOT %d", len(docs))
	}
}
