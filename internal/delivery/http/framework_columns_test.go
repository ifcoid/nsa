package http

import (
	"testing"

	"nsa/internal/model"
)

func TestNormalizeFrameworkColumns_TrimAndKeepValid(t *testing.T) {
	in := []model.FrameworkColumn{
		{Key: "  accuracy ", Category: " Output ", Desc: "  akurasi absolut "},
		{Key: "delta_accuracy", Category: "Output", Desc: "+%"},
	}
	out, err := normalizeFrameworkColumns(in)
	if err != nil {
		t.Fatalf("tak terduga error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("ingin 2 kolom, dapat %d", len(out))
	}
	if out[0].Key != "accuracy" || out[0].Category != "Output" || out[0].Desc != "akurasi absolut" {
		t.Errorf("trim gagal: %+v", out[0])
	}
}

func TestNormalizeFrameworkColumns_DropEmptyKey(t *testing.T) {
	in := []model.FrameworkColumn{
		{Key: "itr", Category: "Output"},
		{Key: "   ", Category: "Output", Desc: "baris kosong harus dibuang"},
		{Key: "delta_itr", Category: "Output"},
	}
	out, err := normalizeFrameworkColumns(in)
	if err != nil {
		t.Fatalf("tak terduga error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("baris berkey kosong harus dibuang; ingin 2, dapat %d", len(out))
	}
}

func TestNormalizeFrameworkColumns_RejectDuplicateKeyCaseInsensitive(t *testing.T) {
	in := []model.FrameworkColumn{
		{Key: "Accuracy"},
		{Key: "accuracy"},
	}
	if _, err := normalizeFrameworkColumns(in); err == nil {
		t.Fatal("duplikat key (beda kapital) harus ditolak")
	}
}

func TestNormalizeFrameworkColumns_RejectAllEmpty(t *testing.T) {
	in := []model.FrameworkColumn{
		{Key: "  "},
		{Key: ""},
	}
	if _, err := normalizeFrameworkColumns(in); err == nil {
		t.Fatal("framework tanpa kolom valid harus ditolak")
	}
	if _, err := normalizeFrameworkColumns(nil); err == nil {
		t.Fatal("nil columns harus ditolak")
	}
}
