package modules

import (
	"errors"
	"testing"
)

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
