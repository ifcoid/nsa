//go:build ignore

// Probe end-to-end Modul 6 Langkah 2-3 terhadap MongoDB + Qdrant + LLM asli.
// Membuat SESI BARU terisolasi (__m6_e2e_probe__), menyalin beberapa paper
// full_text_retrieved=true yang SUDAH ADA (read-only terhadap sesi lain) ke sesi
// probe, lalu menjalankan full-text screening (L2) + outputs (L3).
// TIDAK menghapus sesi/paper milik sesi lain. Hanya membersihkan sisa probe sendiri.
//
// Jalankan: go run scratch/m6_e2e_probe.go
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"nsa/internal/llm"
	"nsa/internal/model"
	"nsa/internal/modules"
	"nsa/internal/orchestrator"
	"nsa/internal/repository"
)

const probeID = "__m6_e2e_probe__"
const copyN = 3

func sval(p bson.M, keys ...string) string {
	for _, k := range keys {
		if v, ok := p[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// normDOI menormalkan DOI sama seperti BuildFulltextIndex (lowercase + strip prefix).
func normDOI(d string) string {
	d = strings.TrimPrefix(d, "https://doi.org/")
	d = strings.TrimPrefix(d, "http://doi.org/")
	return strings.ToLower(strings.TrimSpace(d))
}

func writeResult(s string) {
	_ = os.WriteFile("scratch/m6_e2e_result.txt", []byte(s), 0644)
}

func main() {
	_ = godotenv.Load()
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "slr_agentic_db"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 14*time.Minute)
	defer cancel()

	repo, err := repository.NewMongoRepository(uri, dbName)
	if err != nil {
		fmt.Printf("❌ Mongo: %v\n", err)
		writeResult("❌ M6 E2E gagal konek MongoDB: " + err.Error())
		os.Exit(1)
	}
	fmt.Printf("✅ MongoDB tersambung (db=%s)\n", dbName)

	rawClient, _ := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	defer rawClient.Disconnect(ctx)
	sessColl := rawClient.Database(dbName).Collection("slr_sessions")
	scrColl := repo.GetScreeningCollection()

	// 1. Cek provider wajib (zhipu, groq) tidak placeholder.
	for _, pid := range []string{"zhipu", "groq"} {
		cfg, e := repo.GetLLMConfig(ctx, pid)
		if e != nil || cfg.APIKey == "" || strings.HasPrefix(cfg.APIKey, "GANTI_DENGAN") {
			msg := fmt.Sprintf("⚠️ Provider '%s' belum dikonfigurasi (placeholder/kosong). Probe M6 dibatalkan.", pid)
			fmt.Println(msg)
			writeResult(msg)
			os.Exit(2)
		}
	}
	fmt.Println("✅ Provider zhipu & groq siap.")

	// 2. Cari paper tervektorisasi (full_text_retrieved=true) milik sesi mana pun — READ ONLY.
	cur, err := scrColl.Find(ctx, bson.M{"full_text_retrieved": true}, options.Find().SetLimit(int64(copyN*4)))
	if err != nil {
		fmt.Printf("❌ Find sumber: %v\n", err)
		os.Exit(1)
	}
	var src []bson.M
	_ = cur.All(ctx, &src)
	if len(src) == 0 {
		msg := "⚠️ Tidak ada paper full_text_retrieved=true di slr_screening. Sinkronkan Qdrant dulu. Probe M6 dibatalkan."
		fmt.Println(msg)
		writeResult(msg)
		os.Exit(2)
	}
	fmt.Printf("✅ Ditemukan %d paper tervektorisasi (kandidat salinan).\n", len(src))

	// Bangun indeks RAG Qdrant, lalu utamakan paper yang DOI-nya sudah ADA isinya di Qdrant
	// (karena Qdrant baru terisi sebagian via PEDE).
	ftIndex, ragAvail, ragErr := modules.BuildFulltextIndex(ctx)
	if ragErr != nil {
		fmt.Printf("   [warn] BuildFulltextIndex error: %v\n", ragErr)
	}
	if ragAvail {
		fmt.Printf("✅ Indeks RAG Qdrant: %d DOI memiliki konten.\n", len(ftIndex))
		var withContent []bson.M
		var without []bson.M
		for _, p := range src {
			if c, ok := ftIndex[normDOI(sval(p, "DOI", "doi"))]; ok && strings.TrimSpace(c) != "" {
				withContent = append(withContent, p)
			} else {
				without = append(without, p)
			}
		}
		src = append(withContent, without...) // yang ada konten didahulukan
		fmt.Printf("   %d dari kandidat punya konten RAG (didahulukan).\n", len(withContent))
	} else {
		fmt.Println("   [warn] QDRANT env belum diset; paper akan jadi pending-manual.")
	}

	srcSession := sval(src[0], "session_id")

	// 3. Bersihkan sisa probe SENDIRI (bukan sesi lain).
	_, _ = sessColl.DeleteOne(ctx, bson.M{"_id": probeID})
	_, _ = scrColl.DeleteMany(ctx, bson.M{"session_id": probeID})

	// 4. Salin hingga copyN paper ke sesi probe.
	copied := 0
	var docs []interface{}
	for _, p := range src {
		if copied >= copyN {
			break
		}
		doi := sval(p, "DOI", "doi")
		if doi == "" {
			continue // butuh DOI agar bisa RAG dari Qdrant
		}
		docs = append(docs, bson.M{
			"session_id":          probeID,
			"Title":               sval(p, "Title", "title"),
			"Abstract":            sval(p, "Abstract", "abstract"),
			"Keywords":            sval(p, "Keywords", "keywords"),
			"Authors":             sval(p, "Authors", "authors"),
			"Year":                sval(p, "Year", "year"),
			"Journal":             sval(p, "Journal", "journal"),
			"DOI":                 doi,
			"doi":                 doi,
			"full_text_retrieved": true,
			"Final_Decision":      "",
			"Screener_1_Decision": "INCLUDE", // eligible untuk full-text screening
		})
		copied++
	}
	if copied == 0 {
		msg := "⚠️ Paper tervektorisasi tidak punya DOI; tak bisa RAG. Probe M6 dibatalkan."
		fmt.Println(msg)
		writeResult(msg)
		os.Exit(2)
	}
	if _, err := scrColl.InsertMany(ctx, docs); err != nil {
		fmt.Printf("❌ InsertMany: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Menyalin %d paper ke sesi probe %s.\n", copied, probeID)

	// 5. Buat sesi probe (status langsung M6_STEP2_FULLTEXT_SCREENING). Ambil PICO dari sesi sumber.
	probe := &model.SLRSession{
		ID:     probeID,
		Topic:  "E2E probe Modul 6 (full-text screening)",
		Status: "M6_STEP2_FULLTEXT_SCREENING",
	}
	if srcSession != "" {
		if ss, e := repo.GetSession(ctx, srcSession); e == nil && ss != nil {
			probe.PICODefinitions = ss.PICODefinitions
			probe.ScreenerBriefing = ss.ScreenerBriefing
		}
	}
	if err := repo.UpdateSession(ctx, probe); err != nil {
		fmt.Printf("❌ Buat sesi probe: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Sesi probe dibuat. Menjalankan pipeline M6 L2->L3...\n\n")

	// 6. Jalankan pipeline langkah demi langkah.
	factory := llm.NewLLMFactory(repo)
	pipeline := orchestrator.NewSLRPipeline(repo, factory)

	finalStatus := ""
	for i := 0; i < 30; i++ {
		if err := pipeline.Execute(ctx, probeID); err != nil {
			fmt.Printf("❌ Execute: %v\n", err)
			finalStatus = "ERROR: " + err.Error()
			break
		}
		s, _ := repo.GetSession(ctx, probeID)
		fmt.Printf("→ iter %d: status=%s\n", i+1, s.Status)
		finalStatus = s.Status

		if s.Status == "M6_STEP2_WAITING_RESOLUTION" {
			// Auto-resolve: INCLUDE bila salah satu reviewer INCLUDE, else EXCLUDE.
			dis, _ := repo.GetDisagreedFullTextPapers(ctx, probeID)
			for _, p := range dis {
				hex := ""
				if oid, ok := p["_id"].(interface{ Hex() string }); ok {
					hex = oid.Hex()
				}
				fd := "EXCLUDE"
				if sval(p, "Screener_1_Decision_Full") == "INCLUDE" || sval(p, "Screener_2_Decision_Full") == "INCLUDE" {
					fd = "INCLUDE"
				}
				_ = repo.UpdateScreeningPaperResolutionFull(ctx, probeID, hex, fd, "[E2E auto-resolve]")
			}
			fmt.Printf("   [probe] auto-resolve %d kasus, lanjut.\n", len(dis))
			s.Status = "M6_STEP2_FULLTEXT_SCREENING"
			_ = repo.UpdateSession(ctx, s)
			continue
		}
		if s.Status == "M6_STEP3_WAITING_APPROVAL" || strings.Contains(s.Status, "ERROR") || s.Status == "COMPLETED" {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	// 7. Laporan.
	final, _ := repo.GetSession(ctx, probeID)
	var sb strings.Builder
	fmt.Fprintf(&sb, "M6 E2E selesai. Status akhir: %s\n", finalStatus)
	fmt.Fprintf(&sb, "Paper diuji: %d | Full-text kappa: %.3f | Batch log: %d\n", copied, final.FulltextKappa, len(final.FulltextScreeningLog))

	all, _ := repo.GetAllScreeningPapers(ctx, probeID)
	inc, exc, unc := 0, 0, 0
	for _, p := range all {
		switch sval(p, "Final_Decision_Full", "Screener_1_Decision_Full") {
		case "INCLUDE":
			inc++
		case "EXCLUDE":
			exc++
		default:
			unc++
		}
	}
	fmt.Fprintf(&sb, "Decisions full-text -> INCLUDE: %d, EXCLUDE: %d, lainnya: %d\n", inc, exc, unc)
	if final.Modul6Summary != nil {
		m := final.Modul6Summary.Markdown
		if len(m) > 900 {
			m = m[:900] + "…"
		}
		fmt.Fprintf(&sb, "\n--- modul6_summary (potongan) ---\n%s\n", m)
	}
	fmt.Fprintf(&sb, "\n(Sesi probe '%s' DIPERTAHANKAN untuk inspeksi; hapus manual bila perlu.)", probeID)

	out := sb.String()
	fmt.Println("\n================ HASIL ================")
	fmt.Println(out)
	writeResult(out)
}
