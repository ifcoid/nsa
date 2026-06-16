package modules

import "testing"

func testCatalog() []PaperCitation {
	return []PaperCitation{
		{Key: "wang2024", Authors: "Wang, L.; Chen, H.", Title: "FEMBA: efficient Mamba EEG deployment on GAP9", Year: "2024"},
		{Key: "chen2024", Authors: "Chen, Y.; Liu, X.", Title: "Geo-Mamba Riemannian manifold neuroimaging", Year: "2024"},
		{Key: "zhang2024", Authors: "Zhang, Q.", Title: "MSGM emotion recognition on Jetson Xavier", Year: "2024"},
		{Key: "li2024", Authors: "Li, P.", Title: "EEGM2 resource constrained BCI variant", Year: "2024"},
	}
}

func TestSanitizeCitations_DecoratedKeyRemap(t *testing.T) {
	// LLM decorated the real author-year key "wang2024" into "wang2024femba".
	in := `FEMBA mendemonstrasikan deployment MCU \cite{wang2024femba}.`
	out, stats := sanitizeCitations(in, testCatalog())
	if got := stats.RemapPairs["wang2024femba"]; got != "wang2024" {
		t.Fatalf("expected remap wang2024femba->wang2024, got %q (out=%q)", got, out)
	}
	if want := `FEMBA mendemonstrasikan deployment MCU \cite{wang2024}.`; out != want {
		t.Fatalf("unexpected output:\n got: %q\nwant: %q", out, want)
	}
}

func TestSanitizeCitations_DescriptiveKeyRemapByTitle(t *testing.T) {
	// Fully invented descriptive key; "femba" token is distinctive to wang2024's title.
	in := `Deployment full-stack pada GAP9 tercatat \cite{femba_gap9_deployment}.`
	out, stats := sanitizeCitations(in, testCatalog())
	if got := stats.RemapPairs["femba_gap9_deployment"]; got != "wang2024" {
		t.Fatalf("expected remap to wang2024, got %q (out=%q)", got, out)
	}
}

func TestSanitizeCitations_GenericKeyDropped(t *testing.T) {
	// "cross_modality_gap" has only generic stopword tokens -> must be dropped, not misattributed.
	in := `Generalisasi belum terbukti \cite{cross_modality_gap}.`
	out, stats := sanitizeCitations(in, testCatalog())
	if stats.Dropped != 1 || !stats.DroppedSet["cross_modality_gap"] {
		t.Fatalf("expected cross_modality_gap dropped, stats=%+v", stats)
	}
	if want := `Generalisasi belum terbukti.`; out != want {
		t.Fatalf("expected empty \\cite removed and space tidied:\n got: %q\nwant: %q", out, want)
	}
}

func TestSanitizeCitations_MultiKeyPartial(t *testing.T) {
	in := `Beberapa studi \cite{chen2024, msgm_jetson_xavier, totally_unknown_concept}.`
	out, stats := sanitizeCitations(in, testCatalog())
	// chen2024 valid; msgm_jetson_xavier -> zhang2024 (distinctive "jetson"/"xavier"/"msgm"); unknown dropped.
	if want := `Beberapa studi \cite{chen2024, zhang2024}.`; out != want {
		t.Fatalf("unexpected output:\n got: %q\nwant: %q\nstats=%+v", out, want, stats)
	}
}

func TestSanitizeCitations_ValidKeysUntouched(t *testing.T) {
	in := `Mamba efisien \cite{wang2024, chen2024} pada BCI \cite{li2024}.`
	out, stats := sanitizeCitations(in, testCatalog())
	if out != in {
		t.Fatalf("valid keys should be untouched:\n got: %q\nwant: %q", out, in)
	}
	if stats.Remapped != 0 || stats.Dropped != 0 || stats.Valid != 3 {
		t.Fatalf("expected 3 valid, 0 remap, 0 drop; got %+v", stats)
	}
}

func TestBuildAllowedKeysList(t *testing.T) {
	out := buildAllowedKeysList(testCatalog())
	for _, want := range []string{"wang2024 — Wang 2024", "chen2024 — Chen 2024", "ALLOWED CITATION KEYS"} {
		if !contains(out, want) {
			t.Fatalf("allowed-keys list missing %q:\n%s", want, out)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
