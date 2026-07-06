package modules

import (
	"fmt"
	"testing"
)

// Regresi: parse error yang MELAMPIRKAN respons LLM mentah (Raw: ...) tak boleh
// disalah-klasifikasi sebagai overload/connectivity hanya karena payload paper
// memuat substring polos "500"/"429"/"dial tcp".
func TestErrClassifiers_IgnoreRawPayload(t *testing.T) {
	err := fmt.Errorf("parse QAResult (json: cannot unmarshal object into Go struct field QAResult.items_summary of type string). Raw: {\"total_score\":500,\"note\":\"429 subjects, quota met, dial tcp sample\"}")
	if isServerOverloadError(err) {
		t.Fatalf("parse error salah di-cap overload (500/429/quota di RAW)")
	}
	if isLLMConnectivityError(err) {
		t.Fatalf("parse error salah di-cap connectivity ('dial tcp' di RAW)")
	}
	if isSystemicLLMError(err) {
		t.Fatalf("parse error (post-tolerance) tak boleh dianggap sistemik")
	}
}

// Sinyal ASLI dari provider (di luar Raw) tetap terdeteksi.
func TestErrClassifiers_RealSignalStillDetected(t *testing.T) {
	if !isServerOverloadError(fmt.Errorf("provider merespons dengan error: 429 Too Many Requests")) {
		t.Fatalf("429 asli tak terdeteksi")
	}
	if !isLLMConnectivityError(fmt.Errorf("Post \"https://x/v1\": dial tcp: no such host")) {
		t.Fatalf("connectivity asli tak terdeteksi")
	}
}
