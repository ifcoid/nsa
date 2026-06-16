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
