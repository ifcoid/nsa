package http

import (
	"testing"

	"nsa/internal/model"
)

func TestNormalizePriorReviews_TrimDropEmptyAndVerification(t *testing.T) {
	in := []model.PriorReview{
		{AuthorYear: "  Smith dkk. (2021) ", Scope: " EEG ", Verification: "verified"},
		{AuthorYear: "   ", KeyFindings: "baris kosong harus dibuang"},
		{AuthorYear: "Lee (2022)", Verification: ""},
		{AuthorYear: "Wang (2020)", Verification: "GARBAGE"},
	}
	out := normalizePriorReviews(in)
	if len(out) != 3 {
		t.Fatalf("baris berauthor kosong harus dibuang; ingin 3, dapat %d", len(out))
	}
	if out[0].AuthorYear != "Smith dkk. (2021)" || out[0].Scope != "EEG" {
		t.Errorf("trim gagal: %+v", out[0])
	}
	if out[0].Verification != "VERIFIED" {
		t.Errorf("verified (case-insensitive) harus jadi VERIFIED, dapat %q", out[0].Verification)
	}
	// Kosong / nilai tak dikenal -> default UNVERIFIED (anti-halusinasi).
	if out[1].Verification != "UNVERIFIED" || out[2].Verification != "UNVERIFIED" {
		t.Errorf("verification non-VERIFIED harus jadi UNVERIFIED; dapat %q, %q", out[1].Verification, out[2].Verification)
	}
}

func TestNormalizePriorReviews_Empty(t *testing.T) {
	if out := normalizePriorReviews(nil); len(out) != 0 {
		t.Fatalf("nil -> kosong, dapat %d", len(out))
	}
}
