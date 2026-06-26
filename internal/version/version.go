// Package version menyimpan identitas build backend (commit nsa) untuk Reproducible Error:
// /diagnostic melaporkannya agar developer tahu BINARY/versi mana yang dipakai user (krusial
// untuk backend yang dijalankan LOKAL — bisa binary lama). Di-set saat build via ldflags:
//
//	garble ... build -ldflags "-X 'nsa/internal/version.Commit=<sha>'" ...
//	go build -ldflags "-X 'nsa/internal/version.Commit=<sha>'" ...
//
// Default "dev" = build lokal / tanpa stamp.
package version

var Commit = "dev"
