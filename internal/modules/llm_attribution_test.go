package modules

import (
	"errors"
	"testing"
)

func TestIsLLMConnectivityError(t *testing.T) {
	conn := []string{
		`Post "http://localhost:8080/v1/chat/completions": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`,
		"dial tcp 127.0.0.1:8080: connect: connection refused",
		"lookup api.example.com: no such host",
		"read: connection reset by peer",
	}
	for _, m := range conn {
		if !isLLMConnectivityError(errors.New(m)) {
			t.Errorf("harus terdeteksi konektivitas: %q", m)
		}
	}
	notConn := []string{
		"invalid character 'S' looking for beginning of value",
		"gagal parsing JSON dari LLM",
		"rate limit exceeded (429)",
		"",
	}
	for _, m := range notConn {
		if isLLMConnectivityError(errors.New(m)) {
			t.Errorf("BUKAN konektivitas tapi terdeteksi: %q", m)
		}
	}
	if isLLMConnectivityError(nil) {
		t.Error("nil bukan error konektivitas")
	}
}

func TestRoleDisplay(t *testing.T) {
	cases := map[string]string{
		"brain":              "Brain",
		"reviewer1":          "Reviewer 1",
		"reviewer1_fallback": "Reviewer 1 (fallback)",
		"reviewer2":          "Reviewer 2",
		"reviewer2_fallback": "Reviewer 2 (fallback)",
		"supervisor":         "Supervisor",
		"supervisor_fallback": "Supervisor (fallback)",
		"auditor":            "Auditor",
		"":                   "LLM",
		"custom_role":        "custom_role", // pass-through untuk role tak dikenal
	}
	for in, want := range cases {
		if got := roleDisplay(in); got != want {
			t.Errorf("roleDisplay(%q) = %q, want %q", in, got, want)
		}
	}
}
