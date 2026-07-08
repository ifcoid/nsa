package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

// Regresi tiket Sindy M8B_STEP4: LLM menaruh tabel markdown multi-baris LANGSUNG di dalam
// nilai string JSON → newline mentah → "invalid character '\n' in string literal".
func TestCleanJSON_EscapesRawNewlineInString(t *testing.T) {
	raw := "```json\n{\n  \"markdown\": \"| A | B |\n|---|---|\n| x | y |\",\n  \"convergent_gaps\": \"gap 1\ngap 2\"\n}\n```"
	cleaned := CleanJSONResponse(raw)
	var m map[string]string
	if err := json.Unmarshal([]byte(cleaned), &m); err != nil {
		t.Fatalf("masih gagal parse setelah sanitasi: %v\ncleaned=%q", err, cleaned)
	}
	if !strings.Contains(m["markdown"], "| A | B |") || !strings.Contains(m["markdown"], "\n|---|---|") {
		t.Fatalf("isi markdown (termasuk newline) tak terjaga: %q", m["markdown"])
	}
	if !strings.Contains(m["convergent_gaps"], "gap 1\ngap 2") {
		t.Fatalf("convergent_gaps tak terjaga: %q", m["convergent_gaps"])
	}
}

// JSON yang SUDAH valid (escaped) tak boleh berubah maknanya (idempoten & aman).
func TestCleanJSON_LeavesValidJSONIntact(t *testing.T) {
	raw := `{"markdown":"baris1\nbaris2","tab":"a\tb","q":"dia bilang \"hai\""}`
	var m map[string]string
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &m); err != nil {
		t.Fatalf("JSON valid jadi rusak: %v", err)
	}
	if m["markdown"] != "baris1\nbaris2" || m["tab"] != "a\tb" || m["q"] != `dia bilang "hai"` {
		t.Fatalf("nilai berubah: %+v", m)
	}
}
