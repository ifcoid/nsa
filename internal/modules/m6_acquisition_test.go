package modules

import "testing"

func TestIsValidDOI(t *testing.T) {
	valid := []string{
		"10.1109/EMBC58623.2025.11252697",
		"10.1016/j.bspc.2025.108707",
		"https://doi.org/10.3390/brainsci16040421",
		"http://doi.org/10.1038/s41598-025-22684-x",
		" 10.1111/nyas.15288 ",
	}
	invalid := []string{
		"2-s2.0-85171571621", // Scopus EID (the real-data bug)
		"2-s2.0-85203841480",
		"", "-", "arXiv:2101.00001", "10.foo", "10/123", "doi:10.1/x",
	}
	for _, d := range valid {
		if !isValidDOI(d) {
			t.Errorf("expected VALID DOI, got invalid: %q", d)
		}
	}
	for _, d := range invalid {
		if isValidDOI(d) {
			t.Errorf("expected INVALID (non-DOI), got valid: %q", d)
		}
	}
}

func TestArxivPDFURL(t *testing.T) {
	cases := map[string]string{
		"http://arxiv.org/abs/2410.09998":    "http://arxiv.org/pdf/2410.09998.pdf",
		"http://arxiv.org/abs/2208.04166v1":  "http://arxiv.org/pdf/2208.04166v1.pdf",
		"https://arxiv.org/abs/2408.13074v2": "https://arxiv.org/pdf/2408.13074v2.pdf",
	}
	for in, want := range cases {
		if got := arxivPDFURL(in); got != want {
			t.Errorf("arxivPDFURL(%q) = %q, want %q", in, got, want)
		}
	}
	// ID tanpa pola /abs/ tidak bisa dikonversi -> kosong (jangan mengarang URL).
	if got := arxivPDFURL("http://arxiv.org/something/123"); got != "" {
		t.Errorf("expected empty for non-abs id, got %q", got)
	}
}
