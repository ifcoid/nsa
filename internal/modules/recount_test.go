package modules

import "testing"

// ComputeIdentification harus meniru dedup M4 (DOI exact ∪ title+year) untuk koreksi PRISMA.
func TestComputeIdentification(t *testing.T) {
	recs := []IdRecord{
		{Title: "A", DOI: "10.1/x", Year: "2020", Database: "Scopus"},
		{Title: "A different", DOI: "10.1/x", Year: "2020", Database: "WoS"}, // DOI dup
		{Title: "B paper", DOI: "", Year: "2021", Database: "Scopus"},
		{Title: "B  Paper", DOI: "", Year: "2021", Database: "WoS"}, // title+year dup (spasi/kapital)
		{Title: "C", DOI: "10.1/c", Year: "2022", Database: "Scopus"},
	}
	d := ComputeIdentification(recs)
	if got := len(recs); got != 5 {
		t.Fatalf("total records %d != 5", got)
	}
	if d.TotalDuplicates != 2 || d.TotalUnique != 3 {
		t.Fatalf("dup=%d unik=%d, mau dup=2 unik=3", d.TotalDuplicates, d.TotalUnique)
	}
	if d.PrimaryMatch != 1 || d.SecondaryMatch != 1 {
		t.Fatalf("primary(DOI)=%d secondary(title)=%d, mau 1/1", d.PrimaryMatch, d.SecondaryMatch)
	}
	if d.PerDatabaseTotal["Scopus"] != 3 || d.PerDatabaseTotal["WoS"] != 2 {
		t.Fatalf("per-db salah: %+v", d.PerDatabaseTotal)
	}
}
