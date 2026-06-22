package modules

import "testing"

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
