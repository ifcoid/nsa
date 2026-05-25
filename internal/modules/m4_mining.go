package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"nsa/internal/agent"
	"nsa/internal/logger"
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
		logger.Log(session.ID, "   [Langkah 4.1] Inisialisasi Eksekusi Final Search & Sanity Check...")
		
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
		logger.Log(session.ID, "   [System] Templat 'data_mining_log.initial_sample' berhasil dibuat.")
		logger.Log(session.ID, "   [System] DIJEDA menunggu input manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP1_WAITING_INPUT":
		logger.Log(session.ID, "   [System] Sesi dikunci. Silakan eksekusi di Scopus lalu buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Cari dokumen 'data_mining_log.initial_sample'.")
		logger.Log(session.ID, "   2. Ganti SEMUA placeholder '[ISI DISINI...]' dengan data aktual dari hasil Scopus Anda.")
		logger.Log(session.ID, "   3. Jika sudah lengkap terisi, ubah 'status' menjadi 'M4_STEP1_EVALUATE' dan Update.")
		return nil

	case "M4_STEP1_EVALUATE":
		logger.Log(session.ID, "   [Langkah 4.1] Mengevaluasi Sanity Check hasil pencarian awal...")
		
		if session.DataMiningLog == nil || strings.Contains(session.DataMiningLog.InitialSample.TotalHitsPreFilter, "[ISI") {
			logger.Log(session.ID, "   [ERROR] Data input belum diisi lengkap! Pastikan tidak ada placeholder '[ISI DISINI...]' yang tersisa.")
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

		logger.Log(session.ID, "   [System] Evaluasi Sanity Check berhasil disusun.")
		logger.Log(session.ID, "   [System] DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP1_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi dikunci. Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Periksa 'data_mining_log.sanity_check'.")
		logger.Log(session.ID, "   2. Baca 'volume_analysis', 'decision', dan 'recommendation'.")
		logger.Log(session.ID, "   3a. Jika 'PROCEED' dan Anda setuju, ubah 'status' menjadi 'M4_STEP1_APPROVED'.")
		logger.Log(session.ID, "   3b. Jika 'REVISE' atau Anda ingin merevisi kueri, ubah 'status' ke 'M4_STEP1_NEEDS_REVISION'. Sistem akan melempar Anda KEMBALI ke Modul 3 Langkah 3 secara otomatis.")
		return nil

	case "M4_STEP1_NEEDS_REVISION":
		logger.Log(session.ID, "   [System] Mengembalikan status riset ke perbaikan Search String (Modul 3).")
		session.Status = "M3_STEP3_NEEDS_REVISION" 
		// Set feedback agar agen tahu kenapa kita balik
		session.Feedback = fmt.Sprintf("Sanity check gagal di Modul 4. Rekomendasi: %s", session.DataMiningLog.SanityCheck.Recommendation)
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP1_APPROVED":
		logger.Log(session.ID, "   [Langkah 4.1] Sanity Check disetujui! Lanjut ke Export & Deduplikasi...")
		session.Status = "M4_STEP2_EXPORT"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP2_EXPORT":
		logger.Log(session.ID, "   [Langkah 4.2] Persiapan Export & Import MongoDB...")
		logger.Log(session.ID, "   [System] INSTRUKSI UNTUK PENELITI:")
		logger.Log(session.ID, "   1. Export seluruh hasil pencarian ke bentuk CSV dari masing-masing database (Scopus, IEEE, PubMed).")
		logger.Log(session.ID, "   2. [PENTING] Buka CSV tersebut di Excel, tambahkan kolom baru bernama 'Database' dan isi sesuai sumbernya (misal: 'Scopus' untuk semua baris Scopus).")
		logger.Log(session.ID, "   3. Pastikan kolom-kolom inti tercantum: Title, Abstract, Year, DOI, Document Type.")
		logger.Log(session.ID, "   4. Buka MongoDB Compass.")
		logger.Log(session.ID, "   5. Buat collection baru bernama 'slr_papers'.")
		logger.Log(session.ID, "   6. Gunakan fitur 'Add Data -> Import File' untuk MENGGABUNGKAN (menumpuk) semua CSV tersebut ke dalam 'slr_papers'.")
		logger.Log(session.ID, "   7. Setelah semua CSV ditumpuk, ubah status menjadi 'M4_STEP2_PROCESS' dan Update.")
		
		session.Status = "M4_STEP2_WAITING_IMPORT"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP2_WAITING_IMPORT":
		logger.Log(session.ID, "   [System] Menunggu Anda mengimpor data CSV ke collection 'slr_papers' di MongoDB.")
		logger.Log(session.ID, "   Ubah status ke 'M4_STEP2_PROCESS' jika sudah siap diproses deduplikasi otomatis.")
		return nil

	case "M4_STEP2_PROCESS":
		logger.Log(session.ID, "   [Langkah 4.2] Memproses Basic Quality Audit, Multi-DB Deduplication, dan PICO Preview...")
		
		// 1. Fetch Papers
		papersColl := m.deps.MongoRepo.GetPapersCollection()
		cursor, err := papersColl.Find(ctx, map[string]interface{}{})
		if err != nil {
			logger.Log(session.ID, "   [ERROR] Gagal membaca collection 'slr_papers'. Pastikan sudah dibuat.")
			session.Status = "M4_STEP2_WAITING_IMPORT"
			return m.deps.MongoRepo.UpdateSession(ctx, session)
		}

		var rawPapers []map[string]interface{}
		if err := cursor.All(ctx, &rawPapers); err != nil { return err }

		if len(rawPapers) == 0 {
			logger.Log(session.ID, "   [ERROR] Collection 'slr_papers' KOSONG. Silakan import CSV Anda dulu.")
			session.Status = "M4_STEP2_WAITING_IMPORT"
			return m.deps.MongoRepo.UpdateSession(ctx, session)
		}

		logger.Logf(session.ID, "   [Info] Ditemukan %d records untuk diproses.\n", len(rawPapers))

		// 2. Variables for Audit & Dedup
		audit := model.BasicQualityAudit{
			TotalRecords: len(rawPapers),
			YearDistribution: make(map[string]int),
			DocTypes: make(map[string]int),
			MissingAbstractSources: make(map[string]int),
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
			dbSource := getStringField(p, "Database", "database", "Source", "source")
			if dbSource == "" { dbSource = "Unknown" }

			if abs == "" || abs == "[No abstract available]" { 
				audit.MissingAbstract++
				audit.MissingAbstractSources[dbSource]++
			}
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
		var picoVerdict *model.PICOPreviewCheck
		if audit.MissingAbstract > 0 || audit.MissingDOI > 0 {
			logger.Log(session.ID, "   [WARNING] Ditemukan data tidak lengkap (Missing Abstract / DOI). Menghentikan proses sebelum memanggil LLM untuk menghemat token.")
			logger.Log(session.ID, "   Silakan perbaiki file CSV Anda dan klik 'Ulangi Import CSV'.")
			picoVerdict = &model.PICOPreviewCheck{
				MatchCountsPct: 0,
				Verdict:        "HALTED_MISSING_DATA",
				Recommendation: "Proses dihentikan otomatis karena ada data abstrak/DOI yang hilang. Wajib melakukan Re-Import CSV yang sudah diperbaiki untuk melanjutkan.",
			}
		} else {
			logger.Logf(session.ID, "   [Info] Deduplikasi selesai: %d unik, %d duplikat. Menjalankan LLM PICO Preview...\n", dedup.TotalUnique, dedup.TotalDuplicates)
			llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
			if err != nil { return err }

			sampleBytes, _ := json.Marshal(sampleToPreview)
			picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")

			dmAgent := agent.NewDataMiningAgent(llmBrain)
			picoVerdict, err = dmAgent.PicoConsistencyPreview(ctx, string(sampleBytes), string(picoBytes))
			if err != nil { return err }
		}

		// 4. Save unique papers to post-dedup collection
		if len(uniquePapers) > 0 {
			logger.Log(session.ID, "   [Info] Menyimpan hasil post-dedup ke collection 'slr_papers_post_dedup'...")
			postDedupColl := m.deps.MongoRepo.GetPostDedupCollection()
			postDedupColl.Drop(ctx) // reset jika re-run
			_, errIns := postDedupColl.InsertMany(ctx, uniquePapers)
			if errIns != nil {
				logger.Logf(session.ID, "   [ERROR] Gagal menyimpan ke slr_papers_post_dedup: %v\n", errIns)
			}
		}

		// 5. Save to DB
		session.DataMiningLog.QualityAudit = &audit
		session.DataMiningLog.Dedup = &dedup
		session.DataMiningLog.PICOPreview = picoVerdict
		
		session.Status = "M4_STEP2_WAITING_APPROVAL"
		logger.Log(session.ID, "   [System] Hasil Audit, Deduplikasi, dan PICO Preview berhasil disimpan.")
		logger.Log(session.ID, "   [System] DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP2_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi dikunci. Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Buka document sesi Anda, cek 'data_mining_log' bagian quality, dedup, & pico_preview.")
		logger.Log(session.ID, "   2. Jika 'match_counts_pct' > 60% dan verdict PROCEED L3, ubah status ke 'M4_STEP2_APPROVED'.")
		logger.Log(session.ID, "   3. Jika 'match_counts_pct' < 30%, ubah status ke 'M4_STEP2_NEEDS_REVISION'.")
		return nil

	case "M4_STEP2_NEEDS_REVISION":
		logger.Log(session.ID, "   [System] PICO Consistency gagal. Mengembalikan riset ke Modul 3 (Perbaikan Keywords/AVOID List).")
		session.Status = "M3_STEP3_NEEDS_REVISION"
		session.Feedback = "PICO Consistency sangat buruk (banyak noise). Tolong revisi Search String dan ketatkan AVOID list."
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP2_APPROVED":
		logger.Log(session.ID, "   [Langkah 4.2] Export & Deduplikasi disetujui! Lanjut ke Setup Screening...")
		session.Status = "M4_STEP3_SETUP_SCREENING"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP3_SETUP_SCREENING":
		logger.Log(session.ID, "   [Langkah 4.3] Men-setup Database Screening & Modul 4 Summary...")
		
		postDedupColl := m.deps.MongoRepo.GetPostDedupCollection()
		cursor, err := postDedupColl.Find(ctx, map[string]interface{}{})
		if err != nil { return err }

		var uniquePapers []map[string]interface{}
		if err := cursor.All(ctx, &uniquePapers); err != nil { return err }

		if len(uniquePapers) == 0 {
			logger.Log(session.ID, "   [ERROR] Collection 'slr_papers_post_dedup' kosong! Lakukan dedup terlebih dahulu.")
			session.Status = "M4_STEP2_WAITING_IMPORT"
			return m.deps.MongoRepo.UpdateSession(ctx, session)
		}

		screeningColl := m.deps.MongoRepo.GetScreeningCollection()
		screeningColl.Drop(ctx)

		var screeningDocs []interface{}
		for _, p := range uniquePapers {
			// Copy field mapping
			title := getStringField(p, "Title", "title", "TITLE")
			abs := getStringField(p, "Abstract", "abstract")
			doi := getStringField(p, "DOI", "doi")
			year := getStringField(p, "Year", "year")
			authors := getStringField(p, "Authors", "authors", "Author")
			keywords := getStringField(p, "Author Keywords", "Index Keywords", "keywords")
			journal := getStringField(p, "Source title", "journal")
			db := getStringField(p, "Database", "source_db", "Source")
			
			doc := map[string]interface{}{
				"session_id": session.ID,
				"Source_DB": db,
				"Authors": authors,
				"Year": year,
				"Title": title,
				"Abstract": abs,
				"Keywords": keywords,
				"DOI": doi,
				"Journal": journal,
				
				// Dual reviewer columns
				"Screener_1_Decision": "",
				"Screener_1_Reason_Code": "",
				"Screener_1_Notes": "",
				"Screener_2_Decision": "",
				"Screener_2_Reason_Code": "",
				"Screener_2_Notes": "",
				"Agreement": "",
				"Conflict_Resolution": "",
				"Final_Decision": "",
				"Full_Text_Retrieved": false,
				"Full_Text_Location": "",
			}
			screeningDocs = append(screeningDocs, doc)
		}

		if len(screeningDocs) > 0 {
			_, err = screeningColl.InsertMany(ctx, screeningDocs)
			if err != nil { return err }
			logger.Logf(session.ID, "   [Info] Berhasil membuat collection 'slr_screening' dengan %d paper siap di-review.\n", len(screeningDocs))
		}

		// Setup embedded criteria
		pCanonical := "N/A"
		pWhatCounts := "N/A"
		pWhatDoesnt := "N/A"
		if session.PICODefinitions != nil {
			pCanonical = session.PICODefinitions.CanonicalTerm.Term
			pWhatCounts = session.PICODefinitions.P.OperationalDef.WhatCounts
			pWhatDoesnt = session.PICODefinitions.P.OperationalDef.WhatDoesntCount
		}

		session.ScreeningSetup = &model.ScreeningSetup{
			SearchDate: time.Now().Format("2006-01-02"),
			PCanonical: pCanonical,
			PWhatCounts: pWhatCounts,
			PWhatDoesnt: pWhatDoesnt,
			ICOWhatCounts: "Lihat dokumen PICO untuk I, C, dan O",
			ReasonCodes: []string{"P-NOMATCH", "I-NOMATCH", "O-NOMATCH", "STUDY-DESIGN", "LANGUAGE", "DUPLICATE", "NO-ABSTRACT", "OTHER"},
		}

		// LLM Summary
		llmBrain, _ := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		sessionDataBytes, _ := json.MarshalIndent(session.DataMiningLog, "", "  ")
		
		dmAgent := agent.NewDataMiningAgent(llmBrain)
		summary, err := dmAgent.GenerateModul4Summary(ctx, string(sessionDataBytes))
		if err == nil {
			session.Modul4Summary = summary
		}

		session.Status = "M4_STEP3_WAITING_APPROVAL"
		logger.Log(session.ID, "   [System] Database screening & Summary Modul 4 telah disiapkan. DIJEDA.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M4_STEP3_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi dikunci. Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Periksa collection 'slr_screening' pastikan data post-dedup masuk beserta kolom reviewer.")
		logger.Log(session.ID, "   2. Periksa dokumen 'screening_setup' & 'modul4_summary'.")
		logger.Log(session.ID, "   3. Ubah status ke 'M4_STEP3_APPROVED' jika semua valid.")
		return nil
		
	case "M4_STEP3_APPROVED":
		logger.Log(session.ID, "   [Langkah 4.3] MODUL 4 SELESAI! Seluruh data siap untuk di-screening.")
		session.Status = "M5_INIT"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	default:
		logger.Logf(session.ID, "   [Modul 4] Sub-status %s tidak dikenali atau belum diimplementasikan.\n", session.Status)
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
