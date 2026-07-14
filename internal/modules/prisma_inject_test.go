package modules

import "strings"

import "testing"

func TestInjectPrismaFigure(t *testing.T) {
	fresh := prismaFigureBeginMarker + "\nFRESH_FIGURE n=369\n" + prismaFigureEndMarker

	// (1) marker-based replace
	withMarker := "intro\n" + prismaFigureBeginMarker + "\nOLD n=246\n" + prismaFigureEndMarker + "\noutro"
	out := InjectPrismaFigure(withMarker, fresh)
	if !strings.Contains(out, "FRESH_FIGURE n=369") || strings.Contains(out, "OLD n=246") {
		t.Fatalf("marker replace gagal:\n%s", out)
	}
	if strings.Count(out, prismaFigureBeginMarker) != 1 {
		t.Fatalf("marker terduplikasi:\n%s", out)
	}
	if !strings.HasPrefix(out, "intro\n") || !strings.HasSuffix(out, "outro") {
		t.Fatalf("konteks sekitar rusak:\n%s", out)
	}

	// (2) legacy tanpa marker: figure float ber-\label{fig:prisma}
	legacy := "A\n\\begin{figure}[htbp]\nOLD DIAGRAM n=246\n\\label{fig:prisma}\n\\end{figure}\nB"
	out2 := InjectPrismaFigure(legacy, fresh)
	if !strings.Contains(out2, "FRESH_FIGURE n=369") || strings.Contains(out2, "OLD DIAGRAM") {
		t.Fatalf("legacy replace gagal:\n%s", out2)
	}
	if !strings.HasPrefix(out2, "A\n") || !strings.HasSuffix(out2, "\nB") {
		t.Fatalf("legacy konteks rusak:\n%s", out2)
	}

	// (3) tak ada figur → tak berubah
	none := "manuskrip tanpa PRISMA figure"
	if InjectPrismaFigure(none, fresh) != none {
		t.Fatalf("passthrough gagal")
	}
}
