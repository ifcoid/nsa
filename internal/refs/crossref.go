// Package refs mengambil metadata referensi dari Crossref (anti-halusinasi).
package refs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// normalizeDOI membersihkan DOI (strip prefix, trim).
func normalizeDOI(d string) string {
	d = strings.TrimSpace(d)
	d = strings.TrimPrefix(d, "https://doi.org/")
	d = strings.TrimPrefix(d, "http://doi.org/")
	return strings.TrimSpace(d)
}

// FetchBibtex mengambil entri BibTeX asli dari Crossref untuk satu DOI.
func FetchBibtex(ctx context.Context, client *http.Client, doi string) (string, error) {
	doi = normalizeDOI(doi)
	if doi == "" {
		return "", fmt.Errorf("DOI kosong")
	}
	u := fmt.Sprintf("https://api.crossref.org/works/%s/transform/application/x-bibtex", url.PathEscape(doi))
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Accept", "application/x-bibtex")
	req.Header.Set("User-Agent", "SLR-Agentic/1.0 (mailto:research@example.com)")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("crossref status %d untuk %s", resp.StatusCode, doi)
	}
	return strings.TrimSpace(string(body)), nil
}

// BibtexResult: hasil pengumpulan BibTeX untuk daftar DOI.
type BibtexResult struct {
	Bibtex   string   // gabungan entri .bib
	Verified int      // jumlah DOI terverifikasi di Crossref
	Total    int      // jumlah DOI diperiksa
	NotFound []string // DOI yang gagal
}

// FetchBibtexBatch mengambil BibTeX untuk banyak DOI (skip yang gagal, dengan jeda sopan).
func FetchBibtexBatch(ctx context.Context, dois []string) BibtexResult {
	client := &http.Client{Timeout: 20 * time.Second}
	var b strings.Builder
	res := BibtexResult{Total: len(dois)}
	seen := map[string]bool{}
	for _, doi := range dois {
		nd := normalizeDOI(doi)
		if nd == "" || seen[nd] {
			continue
		}
		seen[nd] = true
		bib, err := FetchBibtex(ctx, client, nd)
		if err != nil || bib == "" {
			res.NotFound = append(res.NotFound, nd)
			continue
		}
		b.WriteString(bib)
		b.WriteString("\n\n")
		res.Verified++
		time.Sleep(300 * time.Millisecond) // sopan ke Crossref
	}
	res.Bibtex = strings.TrimSpace(b.String())
	return res
}
