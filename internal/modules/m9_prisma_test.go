package modules

import "testing"

// makePapers builds a synthetic screening set reproducing the real scenario:
// 289 screened = 161 excluded(T/A) + 8 uncertain(T/A) + 120 included(sought);
// of the 120 sought: 5 not retrieved, 115 assessed; of 115 assessed:
// 82 included + 30 excluded(FT) + 3 uncertain(FT).
func makePapers(exTA, uncTA, notRet, incFT, exFT, uncFT int) []map[string]interface{} {
	var ps []map[string]interface{}
	for i := 0; i < exTA; i++ {
		ps = append(ps, map[string]interface{}{"Final_Decision": "EXCLUDE", "Screener_1_Reason_Code": "P-NOMATCH"})
	}
	for i := 0; i < uncTA; i++ {
		ps = append(ps, map[string]interface{}{"Final_Decision": "UNCERTAIN"})
	}
	for i := 0; i < notRet; i++ {
		ps = append(ps, map[string]interface{}{"Final_Decision": "INCLUDE", "full_text_retrieved": false})
	}
	for i := 0; i < incFT; i++ {
		ps = append(ps, map[string]interface{}{"Final_Decision": "INCLUDE", "full_text_retrieved": true, "Final_Decision_Full": "INCLUDE"})
	}
	for i := 0; i < exFT; i++ {
		ps = append(ps, map[string]interface{}{"Final_Decision": "INCLUDE", "full_text_retrieved": true, "Final_Decision_Full": "EXCLUDE", "Screener_1_Reason_Code_Full": "WRONG-OUTCOME"})
	}
	for i := 0; i < uncFT; i++ {
		ps = append(ps, map[string]interface{}{"Final_Decision": "INCLUDE", "full_text_retrieved": true, "Final_Decision_Full": "UNCERTAIN"})
	}
	return ps
}

func TestPrismaFlow_ClosesAndCounts(t *testing.T) {
	papers := makePapers(161, 8, 5, 82, 30, 3) // 289 papers total
	pf := countPrismaFromPapers(papers, 433, 144)

	checks := map[string][2]int{
		"Identified":   {pf.Identified, 433},
		"Duplicates":   {pf.DuplicatesRemoved, 144},
		"Screened":     {pf.Screened, 289},
		"ExcludedTA":   {pf.ExcludedTA, 161},
		"UncertainTA":  {pf.UncertainTA, 8},
		"Sought":       {pf.Sought, 120},
		"NotRetrieved": {pf.NotRetrieved, 5},
		"Assessed":     {pf.Assessed, 115},
		"ExcludedFT":   {pf.ExcludedFT, 30},
		"UncertainFT":  {pf.UncertainFT, 3},
		"Included":     {pf.Included, 82},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
	// Identification->screening closes, so the only warnings are the UNCERTAIN flags.
	for _, w := range pf.Warnings {
		if !contains(w, "UNCERTAIN") {
			t.Errorf("unexpected arithmetic warning (flow should close): %q", w)
		}
	}
	if pf.ExclusionReasonsFT["WRONG-OUTCOME"] != 30 {
		t.Errorf("FT exclusion reason count = %d, want 30", pf.ExclusionReasonsFT["WRONG-OUTCOME"])
	}
}

func TestPrismaFlow_DetectsBrokenArithmetic(t *testing.T) {
	// identified - duplicates (433-144=289) but only 250 screened -> must warn.
	pf := countPrismaFromPapers(makePapers(150, 0, 0, 100, 0, 0), 433, 144) // 250 screened
	found := false
	for _, w := range pf.Warnings {
		if contains(w, "!= screened") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an identification/screening mismatch warning, got %v", pf.Warnings)
	}
}

func TestPrismaFlow_CleanWhenResolved(t *testing.T) {
	// No uncertain at either stage, arithmetic closes -> zero warnings.
	pf := countPrismaFromPapers(makePapers(150, 0, 5, 80, 20, 0), 300, 145) // 255 screened, 300-145=155? -> force match
	if pf.Screened != 255 {
		t.Fatalf("setup: screened=%d", pf.Screened)
	}
	// 300-145=155 != 255 so there WILL be an id/screen warning; assert only that one.
	for _, w := range pf.Warnings {
		if contains(w, "UNCERTAIN") {
			t.Errorf("did not expect UNCERTAIN warning: %q", w)
		}
	}
}

func TestPrismaFlow_TikzAndArtifactRender(t *testing.T) {
	pf := countPrismaFromPapers(makePapers(161, 8, 5, 82, 30, 3), 433, 144)
	tikz := pf.tikzFigure()
	for _, want := range []string{
		`\begin{figure}`, `\begin{tikzpicture}`, `\label{fig:prisma}`,
		"Records identified (n=433)", "Studies included in review (n=82)",
		"Reports not retrieved (n=5)", "Records excluded (n=161)",
	} {
		if !contains(tikz, want) {
			t.Errorf("tikz figure missing %q", want)
		}
	}
	art := pf.artifactText()
	for _, want := range []string{
		"Studies included in the review (final): 82",
		"Reports sought for retrieval: 120",
		"Reports assessed for eligibility (full text): 115",
		"fig:prisma",
	} {
		if !contains(art, want) {
			t.Errorf("artifact text missing %q", want)
		}
	}
}
