package parser

import "testing"

// Regression: an IEEE Xplore BibTeX export saved with a .txt extension must route to the
// BibTeX parser, not silently fall through to CSV and yield zero records.
// Akar bug "Total Records ga sesuai": seluruh file IEEE (86 paper) hilang diam-diam.
func TestParseBibTeXSavedAsTxt(t *testing.T) {
	const bib = `@ARTICLE{11362959,
  author={Doe, John and Smith, Jane},
  journal={IEEE Sensors Letters},
  title={A Novel Brain Connectivity Method},
  year={2026},
  volume={10},
  doi={10.1109/LSENS.2026.3657220},}@ARTICLE{11328767,
  author={Lee, Alice},
  title={Lightweight Neural Network for EEG},
  year={2025},
  doi={10.1109/LSENS.2026.3650795},}`

	docs, err := ParseFile("IEEE.txt", []byte(bib))
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs from BibTeX-as-.txt, got %d (likely fell through to CSV)", len(docs))
	}
	if docs[0].Title == "" || docs[0].DOI == "" {
		t.Errorf("first doc missing fields: %+v", docs[0])
	}
	if docs[0].Database != "IEEE" {
		t.Errorf("expected Database=IEEE, got %q", docs[0].Database)
	}
}

// The content-sniffing safety net must also recover a BibTeX dump misnamed as .csv.
func TestParseBibTeXMisnamedAsCSV(t *testing.T) {
	const bib = `@inproceedings{x1,
  title={Some Conference Paper},
  year={2024},
  doi={10.1000/abc},}`
	docs, err := ParseFile("export.csv", []byte(bib))
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("safety net failed: expected 1 doc, got %d", len(docs))
	}
}
