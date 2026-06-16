package modules

import (
	"errors"
	"strings"
	"testing"
)

func TestFullTextKappaReport(t *testing.T) {
	// Degenerate: semua 6 sepakat INCLUDE (kasus batch-12 user) -> TAK TERDEFINISI, bukan 1.000.
	k, defined, note := fullTextKappaReport(6, 6, 0, 0, 0)
	if defined || k != 0 {
		t.Errorf("all-INCLUDE harus tak-terdefinisi (k=0), got k=%v defined=%v", k, defined)
	}
	if !strings.Contains(note, "TAK TERDEFINISI") {
		t.Errorf("note degenerate harus jelas, got %q", note)
	}

	// n=0 -> tak terdefinisi.
	if _, d, _ := fullTextKappaReport(0, 0, 0, 0, 0); d {
		t.Error("n=0 harus tak-terdefinisi")
	}

	// Ada variansi nyata: 10 paper, 6 bothInc, 2 bothExc, 1+1 disagree -> κ terdefinisi.
	k2, d2, note2 := fullTextKappaReport(10, 6, 2, 1, 1)
	if !d2 {
		t.Fatalf("kasus bervariansi harus terdefinisi, note=%q", note2)
	}
	if k2 <= 0 || k2 >= 1 {
		t.Errorf("κ wajar di (0,1), got %v", k2)
	}
}

func TestIsFatalAuthErr(t *testing.T) {
	fatal := []string{
		"fatal error dari provider (HTTP 401): Invalid API Key",
		"HTTP 403: forbidden",
		"unauthorized",
		"invalid_api_key",
		"Authentication failed",
	}
	transient := []string{
		"HTTP 429: rate limit exceeded",
		"context deadline exceeded",
		"HTTP 500: internal server error",
		"HTTP 503 service unavailable",
		"parsing JSON: empty response",
		"",
	}
	for _, s := range fatal {
		if !isFatalAuthErr(errors.New(s)) {
			t.Errorf("expected FATAL-AUTH for %q", s)
		}
	}
	for _, s := range transient {
		if s == "" {
			if isFatalAuthErr(nil) {
				t.Errorf("nil error must not be fatal-auth")
			}
			continue
		}
		if isFatalAuthErr(errors.New(s)) {
			t.Errorf("expected NON-fatal (retryable) for %q", s)
		}
	}
}
