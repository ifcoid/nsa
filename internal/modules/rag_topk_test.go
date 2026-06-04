package modules

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

// TestRAGTopK memverifikasi BuildFulltextRAG (scroll with_vector) + ekstraksi
// vektor dense + section-aware selectContext, TANPA endpoint embedding eksternal:
// memakai vektor salah satu chunk sendiri sebagai query. Skip bila Qdrant tak diset.
func TestRAGTopK(t *testing.T) {
	_ = godotenv.Load("../../.env")
	if os.Getenv("QDRANT_URL") == "" && os.Getenv("QDRANT_ENDPOINT") == "" {
		t.Skip("QDRANT belum diset; skip")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	rag, ok, err := BuildFulltextRAG(ctx)
	if err != nil {
		t.Fatalf("BuildFulltextRAG: %v", err)
	}
	if !ok || rag == nil {
		t.Fatal("RAG tidak tersedia")
	}
	t.Logf("groups=%d | doi-keyed=%d | title-index=%d", len(rag.byKey), len(rag.doiToKey), len(rag.titles))

	// Cari DOI dengan grup >=3 chunk bervektor.
	var pickDOI string
	var g *ragGroup
	for doi := range rag.doiToKey {
		if gg := rag.groupForDOI(doi); gg != nil && len(gg.chunks) >= 3 && len(gg.chunks[0].vec) > 0 {
			pickDOI, g = doi, gg
			break
		}
	}
	if g == nil {
		t.Fatal("tak ada DOI dengan >=3 chunk bervektor (with_vector mungkin gagal)")
	}
	t.Logf("sample DOI=%s chunks=%d dim=%d", pickDOI, len(g.chunks), len(g.chunks[0].vec))
	if len(g.chunks[0].vec) != 1024 {
		t.Errorf("dim vektor = %d, harusnya 1024 (BGE-M3 dense)", len(g.chunks[0].vec))
	}

	qv := g.chunks[len(g.chunks)/2].vec
	out := rag.TopK(pickDOI, qv, 14, 12000)
	if out == "" {
		t.Fatal("TopK kosong")
	}
	full := rag.TopK(pickDOI, nil, 0, 24000)
	t.Logf("len section-aware=%d vs full=%d", len(out), len(full))
	if len(out) > 12000+5000 {
		t.Errorf("section-aware (%d) jauh melebihi budget 12000", len(out))
	}
}
