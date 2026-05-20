package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/agent"
	"nsa/internal/model"
	"strings"
	"time"
)

type M4Mining struct {
	deps *ModuleDeps
}

func NewM4Mining(deps *ModuleDeps) *M4Mining {
	return &M4Mining{deps: deps}
}

func (m *M4Mining) Name() string {
	return "M4_Mining"
}

func (m *M4Mining) Execute(ctx context.Context, session *model.SLRSession) error {
	switch session.Status {
	// =========================================================================
	// LANGKAH 1: EKSEKUSI FINAL SEARCH + SANITY CHECK
	// =========================================================================
	case "M4_INIT", "M4_STEP1_FINAL_SEARCH":
		fmt.Println("   [Langkah 4.1] Inisialisasi Eksekusi Final Search & Sanity Check...")
		
		// Setup dokumen kosong untuk user input
		if session.DataMiningLog == nil {
			session.DataMiningLog = &model.DataMiningLog{
				InitialSample: model.InitialSearchSample{
					Database:            "Scopus",
					TotalHitsPreFilter:  "[ISI DISINI, misal: 2500]",
					TotalHitsPostFilter: "[ISI DISINI, misal: 350]",
					DateExecuted:        time.Now().Format("2006-01-02"),
					SampleTitles:        []string{"[ISI JUDUL SAMPEL 1]", "[ISI JUDUL SAMPEL 2]"},
					KeyPapersFound:      []string{"[ISI JUDUL PAPER KUNCI YANG DITEMUKAN]"},
					KeyPapersMissing:    []string{"[ISI JUDUL PAPER KUNCI YANG TIDAK DITEMUKAN (TULIS 'NIHIL' BILA TIDAK ADA)]"},
				},
			}
		}

		session.Status = "M4_STEP1_WAITING_INPUT"
		fmt.Println("   [System] Templat 'data_mining_log.initial_sample' berhasil dibuat.")
		fmt.Println("   [System] DIJEDA menunggu input manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP1_WAITING_INPUT":
		fmt.Println("   [System] Sesi dikunci. Silakan eksekusi di Scopus lalu buka MongoDB Compass:")
		fmt.Println("   1. Cari dokumen 'data_mining_log.initial_sample'.")
		fmt.Println("   2. Ganti SEMUA placeholder '[ISI DISINI...]' dengan data aktual dari hasil Scopus Anda.")
		fmt.Println("   3. Jika sudah lengkap terisi, ubah 'status' menjadi 'M4_STEP1_EVALUATE' dan Update.")
		return nil

	case "M4_STEP1_EVALUATE":
		fmt.Println("   [Langkah 4.1] Mengevaluasi Sanity Check hasil pencarian awal...")
		
		if session.DataMiningLog == nil || strings.Contains(session.DataMiningLog.InitialSample.TotalHitsPreFilter, "[ISI") {
			fmt.Println("   [ERROR] Data input belum diisi lengkap! Pastikan tidak ada placeholder '[ISI DISINI...]' yang tersisa.")
			session.Status = "M4_STEP1_WAITING_INPUT"
			return m.deps.MongoRepo.UpdateSession(ctx, session)
		}

		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		sampleBytes, _ := json.MarshalIndent(session.DataMiningLog.InitialSample, "", "  ")
		scopeBytes, _ := json.MarshalIndent(session.ScopeJustifications, "", "  ")

		dmAgent := agent.NewDataMiningAgent(llmBrain)
		verdict, err := dmAgent.SanityCheck(ctx, string(sampleBytes), string(scopeBytes))
		if err != nil { return err }

		session.DataMiningLog.SanityCheck = verdict
		session.Status = "M4_STEP1_WAITING_APPROVAL"

		fmt.Println("   [System] Evaluasi Sanity Check berhasil disusun.")
		fmt.Println("   [System] DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP1_WAITING_APPROVAL":
		fmt.Println("   [System] Sesi dikunci. Silakan buka MongoDB Compass:")
		fmt.Println("   1. Periksa 'data_mining_log.sanity_check'.")
		fmt.Println("   2. Baca 'volume_analysis', 'decision', dan 'recommendation'.")
		fmt.Println("   3a. Jika 'PROCEED' dan Anda setuju, ubah 'status' menjadi 'M4_STEP1_APPROVED'.")
		fmt.Println("   3b. Jika 'REVISE' atau Anda ingin merevisi kueri, ubah 'status' ke 'M4_STEP1_NEEDS_REVISION'. Sistem akan melempar Anda KEMBALI ke Modul 3 Langkah 3 secara otomatis.")
		return nil

	case "M4_STEP1_NEEDS_REVISION":
		fmt.Println("   [System] Mengembalikan status riset ke perbaikan Search String (Modul 3).")
		session.Status = "M3_STEP3_NEEDS_REVISION" 
		// Set feedback agar agen tahu kenapa kita balik
		session.Feedback = fmt.Sprintf("Sanity check gagal di Modul 4. Rekomendasi: %s", session.DataMiningLog.SanityCheck.Recommendation)
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP1_APPROVED":
		fmt.Println("   [Langkah 4.1] Sanity Check disetujui! Lanjut ke Export & Deduplikasi...")
		session.Status = "M4_STEP2_EXPORT"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP2_EXPORT":
		fmt.Println("   [Langkah 4.2] Persiapan Export & Import MongoDB...")
		fmt.Println("   [System] INSTRUKSI UNTUK PENELITI:")
		fmt.Println("   1. Export seluruh hasil pencarian ke bentuk CSV dari masing-masing database (Scopus, IEEE, PubMed).")
		fmt.Println("   2. [PENTING] Buka CSV tersebut di Excel, tambahkan kolom baru bernama 'Database' dan isi sesuai sumbernya (misal: 'Scopus' untuk semua baris Scopus).")
		fmt.Println("   3. Pastikan kolom-kolom inti tercantum: Title, Abstract, Year, DOI, Document Type.")
		fmt.Println("   4. Buka MongoDB Compass.")
		fmt.Println("   5. Buat collection baru bernama 'slr_papers'.")
		fmt.Println("   6. Gunakan fitur 'Add Data -> Import File' untuk MENGGABUNGKAN (menumpuk) semua CSV tersebut ke dalam 'slr_papers'.")
		fmt.Println("   7. Setelah semua CSV ditumpuk, ubah status menjadi 'M4_STEP2_PROCESS' dan Update.")
		
		session.Status = "M4_STEP2_WAITING_IMPORT"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP2_WAITING_IMPORT":
		fmt.Println("   [System] Menunggu Anda mengimpor data CSV ke collection 'slr_papers' di MongoDB.")
		fmt.Println("   Ubah status ke 'M4_STEP2_PROCESS' jika sudah siap diproses deduplikasi otomatis.")
		return nil

	case "M4_STEP2_PROCESS":
		fmt.Println("   [Langkah 4.2] Memproses Basic Quality Audit, Multi-DB Deduplication, dan PICO Preview...")
		
		// 1. Fetch Papers
		papersColl := m.deps.MongoRepo.GetPapersCollection()
		cursor, err := papersColl.Find(ctx, map[string]interface{}{})
		if err != nil {
			fmt.Println("   [ERROR] Gagal membaca collection 'slr_papers'. Pastikan sudah dibuat.")
			session.Status = "M4_STEP2_WAITING_IMPORT"
			return m.deps.MongoRepo.UpdateSession(ctx, session)
		}

		var rawPapers []map[string]interface{}
		if err := cursor.All(ctx, &rawPapers); err != nil { return err }

		if len(rawPapers) == 0 {
			fmt.Println("   [ERROR] Collection 'slr_papers' KOSONG. Silakan import CSV Anda dulu.")
			session.Status = "M4_STEP2_WAITING_IMPORT"
			return m.deps.MongoRepo.UpdateSession(ctx, session)
		}

		fmt.Printf("   [Info] Ditemukan %d records untuk diproses.\n", len(rawPapers))

		// 2. Variables for Audit & Dedup
		audit := model.BasicQualityAudit{
			TotalRecords: len(rawPapers),
			YearDistribution: make(map[string]int),
			DocTypes: make(map[string]int),
		}
		dedup := model.DedupBreakdown{TotalUnique: 0, TotalDuplicates: 0}

		seenDOIs := make(map[string]bool)
		seenTitles := make(map[string]bool)

		var sampleToPreview []map[string]string // Untuk 20 sampel LLM
		var uniquePapers []interface{}          // Untuk menampung post-dedup

		for _, p := range rawPapers {
			// Ekstrak fields dengan toleransi kapitalisasi
			title := getStringField(p, "Title", "title", "TITLE")
			doi := getStringField(p, "DOI", "doi")
			abs := getStringField(p, "Abstract", "abstract")
			year := getStringField(p, "Year", "year")
			dtype := getStringField(p, "Document Type", "document_type", "Document type")

			if abs == "" || abs == "[No abstract available]" { audit.MissingAbstract++ }
			if doi == "" { audit.MissingDOI++ }
			if year != "" { audit.YearDistribution[year]++ }
			if dtype != "" { audit.DocTypes[dtype]++ }

			// Dedup Logic
			isDup := false
			if doi != "" && seenDOIs[doi] {
				dedup.PrimaryMatch++
				isDup = true
			} else if doi != "" {
				seenDOIs[doi] = true
			}

			normTitle := strings.ToLower(strings.ReplaceAll(title, " ", ""))
			titleYearKey := normTitle + "_" + year
			if !isDup && normTitle != "" && seenTitles[titleYearKey] {
				dedup.SecondaryMatch++
				isDup = true
			} else if !isDup && normTitle != "" {
				seenTitles[titleYearKey] = true
			}

			if isDup {
				dedup.TotalDuplicates++
			} else {
				dedup.TotalUnique++
				uniquePapers = append(uniquePapers, p)
				// Kumpulkan 20 sample untuk PICO Preview
				if len(sampleToPreview) < 20 && title != "" && abs != "" {
					sampleToPreview = append(sampleToPreview, map[string]string{
						"title": title,
						"abstract": abs,
					})
				}
			}
		}

		// 3. PICO Consistency Preview
		fmt.Printf("   [Info] Deduplikasi selesai: %d unik, %d duplikat. Menjalankan LLM PICO Preview...\n", dedup.TotalUnique, dedup.TotalDuplicates)
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		sampleBytes, _ := json.Marshal(sampleToPreview)
		picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")

		dmAgent := agent.NewDataMiningAgent(llmBrain)
		picoVerdict, err := dmAgent.PicoConsistencyPreview(ctx, string(sampleBytes), string(picoBytes))
		if err != nil { return err }

		// 4. Save unique papers to post-dedup collection
		if len(uniquePapers) > 0 {
			fmt.Println("   [Info] Menyimpan hasil post-dedup ke collection 'slr_papers_post_dedup'...")
			postDedupColl := m.deps.MongoRepo.GetPostDedupCollection()
			postDedupColl.Drop(ctx) // reset jika re-run
			_, errIns := postDedupColl.InsertMany(ctx, uniquePapers)
			if errIns != nil {
				fmt.Printf("   [ERROR] Gagal menyimpan ke slr_papers_post_dedup: %v\n", errIns)
			}
		}

		// 5. Save to DB
		session.DataMiningLog.QualityAudit = &audit
		session.DataMiningLog.Dedup = &dedup
		session.DataMiningLog.PICOPreview = picoVerdict
		
		session.Status = "M4_STEP2_WAITING_APPROVAL"
		fmt.Println("   [System] Hasil Audit, Deduplikasi, dan PICO Preview berhasil disimpan.")
		fmt.Println("   [System] DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP2_WAITING_APPROVAL":
		fmt.Println("   [System] Sesi dikunci. Silakan buka MongoDB Compass:")
		fmt.Println("   1. Buka document sesi Anda, cek 'data_mining_log' bagian quality, dedup, & pico_preview.")
		fmt.Println("   2. Jika 'match_counts_pct' > 60% dan verdict PROCEED L3, ubah status ke 'M4_STEP2_APPROVED'.")
		fmt.Println("   3. Jika 'match_counts_pct' < 30%, ubah status ke 'M4_STEP2_NEEDS_REVISION'.")
		return nil

	case "M4_STEP2_NEEDS_REVISION":
		fmt.Println("   [System] PICO Consistency gagal. Mengembalikan riset ke Modul 3 (Perbaikan Keywords/AVOID List).")
		session.Status = "M3_STEP3_NEEDS_REVISION"
		session.Feedback = "PICO Consistency sangat buruk (banyak noise). Tolong revisi Search String dan ketatkan AVOID list."
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP2_APPROVED":
		fmt.Println("   [Langkah 4.2] Export & Deduplikasi disetujui! Lanjut ke Setup Screening...")
		session.Status = "M4_STEP3_SETUP_SCREENING"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP3_SETUP_SCREENING":
		fmt.Println("   [Langkah 4.3] Setup Screening Database (Belum diimplementasikan).")
		return nil

	default:
		fmt.Printf("   [Modul 4] Sub-status %s tidak dikenali atau belum diimplementasikan.\n", session.Status)
	}

	return nil
}

func getStringField(doc map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if val, ok := doc[k]; ok && val != nil {
			if strVal, isStr := val.(string); isStr {
				return strVal
			}
		}
	}
	return ""
}
