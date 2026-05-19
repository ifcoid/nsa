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
		fmt.Println("   [Langkah 4.2] Export & Multi-DB Dedup (Belum diimplementasikan).")
		return nil

	default:
		fmt.Printf("   [Modul 4] Sub-status %s tidak dikenali atau belum diimplementasikan.\n", session.Status)
	}

	return nil
}
