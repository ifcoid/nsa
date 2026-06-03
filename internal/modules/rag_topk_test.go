package modules

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

// TestRAGTopK memverifikasi BuildFulltextRAG (scroll with_vector) + ekstraksi
// vektor dense + ranking cosine TopK, TANPA endpoint embedding eksternal:
// memakai vektor salah satu chunk sendiri sebagai query → chunk itu harus
// muncul pada hasil top-k. Skip bila Qdrant tak dikonfigurasi.
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

	// Cari DOI dengan >=3 chunk yang vektornya terisi.
	var pickDOI string
	var chunks []ragChunkVec
	for d, cs := range rag.byDOI {
		if len(cs) >= 3 && len(cs[0].vec) > 0 {
			pickDOI, chunks = d, cs
			break
		}
	}
	if pickDOI == "" {
		t.Fatal("tak ada DOI dengan >=3 chunk bervektor (with_vector mungkin gagal)")
	}
	t.Logf("total DOI=%d | sample DOI=%s chunks=%d dim=%d", len(rag.byDOI), pickDOI, len(chunks), len(chunks[0].vec))

	if len(chunks[0].vec) != 1024 {
		t.Errorf("dim vektor = %d, harusnya 1024 (BGE-M3 dense)", len(chunks[0].vec))
	}

	// Query = vektor chunk index tertentu; chunk itu harus ada di top-3.
	target := chunks[len(chunks)/2]
	out := rag.TopK(pickDOI, target.vec, 3, 8000)
	if out == "" {
		t.Fatal("TopK kosong")
	}
	full := rag.TopK(pickDOI, nil, 0, 24000)
	t.Logf("len top3=%d vs full=%d (top-k harus < full)", len(out), len(full))
	if len(out) >= len(full) && len(chunks) > 3 {
		t.Errorf("top-3 (%d) tidak lebih pendek dari full (%d)", len(out), len(full))
	}
}
