//go:build ignore
package main

// runstage.go — menjalankan modul aktif sampai pipeline BERHENTI di gate HITL
// (atau terminal) berikutnya. TIDAK pernah melewati/memajukan gate; jika sesi
// sudah berada di gate, langsung lapor tanpa mengubah apa pun. Ini murni
// "jalankan tahap sampai butuh keputusan manusia".

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"nsa/internal/llm"
	"nsa/internal/orchestrator"
	"nsa/internal/repository"
)

const SID = "disertasi"

func isGate(st string) bool {
	return strings.Contains(st, "WAITING") || strings.Contains(st, "NEEDS_REVISION") ||
		strings.Contains(st, "ERROR") || st == "COMPLETED" ||
		(strings.Contains(st, "DONE") && st != "M5_DONE")
}

// embeddingHealth ping endpoint embedding. Return "" bila sehat / tak dikonfigurasi
// (skip disengaja), atau pesan error bila DOWN. Dipakai untuk gagal-cepat sebelum
// screening agar tidak diam-diam fallback saat tunnel Colab mati.
func embeddingHealth(ctx context.Context) string {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("EMBED_ENDPOINT")), "/ ")
	if base == "" {
		return "" // tidak pakai embedding -> bukan error
	}
	key := strings.TrimSpace(os.Getenv("EMBED_API_KEY"))
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(cctx, "POST", base+"/embeddings",
		strings.NewReader(`{"model":"BAAI/bge-m3","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Sprintf("http %d", resp.StatusCode)
	}
	return ""
}

func main() {
	_ = godotenv.Load()
	repo, err := repository.NewMongoRepository(os.Getenv("MONGO_URI"), os.Getenv("DB_NAME"))
	if err != nil {
		fmt.Println("mongo:", err)
		os.Exit(1)
	}
	factory := llm.NewLLMFactory(repo)
	pipeline := orchestrator.NewSLRPipeline(repo, factory)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()
	start := time.Now()

	s, err := repo.GetSession(ctx, SID)
	if err != nil {
		fmt.Println("getsession:", err)
		os.Exit(1)
	}
	fmt.Printf("FROM: %s\n", s.Status)

	// Monitor: untuk screening L2, pastikan endpoint embedding hidup dulu (top-k).
	// Gagal-cepat kalau mati supaya tak diam-diam degrade ke fallback.
	if strings.HasPrefix(s.Status, "M6_STEP2") {
		if msg := embeddingHealth(ctx); msg != "" {
			fmt.Printf("EMBEDDING DOWN: %s\n", msg)
			fmt.Println("Top-k butuh endpoint embedding. Restart notebook Colab, update EMBED_ENDPOINT di .env, lalu jalankan lagi. (Batch TIDAK dijalankan agar tak degrade diam-diam.)")
			os.Exit(4)
		}
		fmt.Println("[monitor] embedding endpoint OK.")
	}

	if isGate(s.Status) {
		total, screened, _ := repo.GetScreeningProgress(ctx, SID)
		fmt.Printf("ALREADY AT GATE: %s  screened=%d/%d (perlu persetujuan; tidak menjalankan apa pun)\n", s.Status, screened, total)
		return
	}

	var st string
	for {
		if e := pipeline.Execute(ctx, SID); e != nil {
			fmt.Printf("EXEC ERR: %v\n", e)
			if cur, ge := repo.GetSession(ctx, SID); ge == nil && cur != nil {
				fmt.Printf("STATUS: %s\n", cur.Status)
			}
			os.Exit(2)
		}
		cur, e := repo.GetSession(ctx, SID)
		if e != nil {
			fmt.Println("getsession:", e)
			os.Exit(1)
		}
		st = cur.Status
		if isGate(st) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	total, screened, _ := repo.GetScreeningProgress(ctx, SID)
	fmt.Printf("STOP AT GATE: %s  screened=%d/%d  elapsed=%s\n", st, screened, total, time.Since(start).Round(time.Second))
}
