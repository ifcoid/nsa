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

func TestIsServerOverloadError(t *testing.T) {
	overload := []string{
		"error dari provider di tengah stream: API Error: 503 Service Unavailable. If it persists, check your inference gateway (cc.freemodel.dev).",
		"429 Too Many Requests",
		"provider overloaded, try again",
		"502 Bad Gateway",
		"500 Internal Server Error",
		"rate limit exceeded",
		"quota exceeded for this model",
	}
	for _, m := range overload {
		if !isServerOverloadError(errors.New(m)) {
			t.Errorf("harus terdeteksi overload server: %q", m)
		}
	}
	notOverload := []string{
		"invalid character 'S' looking for beginning of value",
		"dial tcp 127.0.0.1:8080: connect: connection refused",
		"",
	}
	for _, m := range notOverload {
		if isServerOverloadError(errors.New(m)) {
			t.Errorf("BUKAN overload tapi terdeteksi: %q", m)
		}
	}
	if isServerOverloadError(nil) {
		t.Error("nil bukan overload")
	}
}

// TestIsSystemicLLMError mengunci klasifikasi fail-fast di hot-loop QA: error yang
// AKAN BERULANG identik di tiap paper (down/overload/stream kosong/context overflow/
// provider error) harus menghentikan batch — sedangkan error per-item (JSON rusak) tidak.
func TestIsSystemicLLMError(t *testing.T) {
	systemic := []string{
		// Salwa report 2: 503 dari gateway inference user.
		"gagal setelah 3 retries: gagal membaca stream: error dari provider di tengah stream: API Error: 503 Service Unavailable.",
		// Salwa report 1: stream kosong / context overflow claude-opus via aerolink.
		"gagal setelah 3 retries: stream kosong dari provider (model claude-opus-4-8): server membalas 200 OK tapi tanpa konten — kemungkinan context window model terlampaui",
		"dial tcp 127.0.0.1:8080: connect: connection refused",
		"429 Too Many Requests",
		"provider merespons dengan error: 401 Unauthorized",
	}
	for _, m := range systemic {
		if !isSystemicLLMError(errors.New(m)) {
			t.Errorf("harus terdeteksi SISTEMIK (halt): %q", m)
		}
	}
	perItem := []string{
		"invalid character 'S' looking for beginning of value",
		"gagal parsing JSON dari LLM",
		"",
	}
	for _, m := range perItem {
		if isSystemicLLMError(errors.New(m)) {
			t.Errorf("BUKAN sistemik (per-item) tapi terdeteksi halt: %q", m)
		}
	}
	if isSystemicLLMError(nil) {
		t.Error("nil bukan sistemik")
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
