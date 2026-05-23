package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nsa/internal/agent"
	"nsa/internal/logger"
	"nsa/internal/model"
)

type M2Pico struct {
	deps *ModuleDeps
}

func NewM2Pico(deps *ModuleDeps) *M2Pico {
	return &M2Pico{deps: deps}
}

func (m *M2Pico) Name() string { return "M2_PICO" }

func (m *M2Pico) Execute(ctx context.Context, session *model.SLRSession) error {
	logger.Logf(session.ID, ">> [MODUL 2: PICO] Memproses State: %s\n", session.Status)

	switch session.Status {

	// =========================================================================
	// LANGKAH 1: TENTUKAN TOPIK + KLASIFIKASI TIPE GAP
	// =========================================================================
	case "M2_STEP1_TOPIC_GAP":
		logger.Log(session.ID, "   [Langkah 2.1] Menganalisis Topik Mentah & Mengklasifikasi tipe GAP...")
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		gapAgent := agent.NewGapAgent(llmBrain)
		suggestions, err := gapAgent.GenerateSuggestedTopics(ctx, session.Topic)
		if err != nil { return err }

		session.SuggestedTopics = suggestions
		session.Status = "M2_STEP1_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] DIJEDA. Menunggu Anda memilih 1 dari 3 topik yang disarankan.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_STEP1_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi masih dikunci. Silakan setujui di UI Frontend.")
		return nil

	case "M2_STEP1_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 2.1] Mencari ulang saran Topik berdasarkan feedback: '%s'\n", session.Feedback)
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		gapAgent := agent.NewGapAgent(llmBrain)
		topicContext := fmt.Sprintf("Topik awal: %s\n\n[INSTRUKSI REVISI DARI PENELITI]:\n%s\nTolong buatkan 3 saran topik BARU yang berbeda dari sebelumnya dan selaras dengan instruksi revisi ini.", session.Topic, session.Feedback)
		
		suggestions, err := gapAgent.GenerateSuggestedTopics(ctx, topicContext)
		if err != nil { return err }

		session.SuggestedTopics = suggestions
		session.Feedback = ""
		session.Status = "M2_STEP1_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] 3 Topik Baru berhasil disarankan. DIJEDA kembali menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_STEP1_APPROVED":
		if session.SelectedTopic == nil {
			logger.Log(session.ID, "   [System] ERROR: Field 'selected_topic' belum ditemukan di MongoDB. Silakan isi terlebih dahulu.")
			return nil
		}
		
		logger.Logf(session.ID, "   [Langkah 2.1] Topik '%s' (Tipe GAP: %s) telah disetujui. Melanjutkan ke Analisis Prior Reviews...\n", session.SelectedTopic.Name, session.SelectedTopic.Type)
		
		// Sinkronisasi field topic lama agar rapi
		session.Topic = session.SelectedTopic.Name
		session.Status = "M2_STEP2_PRIOR_REVIEWS"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 2: REVIEW OF PRIOR REVIEWS (MATRIKS)
	// =========================================================================
	case "M2_STEP2_PRIOR_REVIEWS":
		logger.Log(session.ID, "   [Langkah 2.2] Menganalisis literatur review terdahulu (Matriks)...")
		if session.SelectedTopic == nil {
			logger.Log(session.ID, "   [System] ERROR: Field 'selected_topic' kosong. Anda tidak bisa melanjutkan ke Langkah 2 tanpa Topik.")
			return nil
		}

		// RAG Context Injeksi
		topicContext := fmt.Sprintf("Judul: %s\nKesenjangan (Gap): %s\nTipe: %s (%s)\nBukti: %s\nAlasannya Mengapa Penting: %s", 
			session.SelectedTopic.Name, session.SelectedTopic.Gap, session.SelectedTopic.Type, session.SelectedTopic.TypeReason, session.SelectedTopic.Evidence, session.SelectedTopic.Importance)

		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		priorAgent := agent.NewPriorReviewAgent(llmBrain)
		matrix, err := priorAgent.GenerateMatrix(ctx, topicContext)
		if err != nil { return err }

		session.PriorReviewsMatrix = matrix
		session.Status = "M2_STEP2_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] Matriks Prior Reviews berhasil disusun. DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_STEP2_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi masih dikunci. Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Lihat document sesi Anda, buka field 'prior_reviews_matrix'.")
		logger.Log(session.ID, "   2. Verifikasi tabel 'reviews' dan 'synthesis_novelty'.")
		logger.Log(session.ID, "   3a. Jika SUDAH sesuai, ubah 'status' menjadi 'M2_STEP2_APPROVED' lalu Update.")
		logger.Log(session.ID, "   3b. Jika TIDAK sesuai, ubah 'status' menjadi 'M2_STEP2_NEEDS_REVISION' dan isi keluhan Anda di field 'feedback' lalu Update.")
		return nil

	case "M2_STEP2_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 2.2] Memperbaiki Matriks Prior Reviews berdasarkan feedback: '%s'\n", session.Feedback)
		if session.SelectedTopic == nil {
			return fmt.Errorf("selected_topic kosong")
		}

		// RAG Context + Instruksi Revisi
		topicContext := fmt.Sprintf("Judul: %s\nKesenjangan (Gap): %s\nTipe: %s (%s)\nBukti: %s\nAlasannya Mengapa Penting: %s\n\n[INSTRUKSI REVISI DARI PENELITI]:\n%s\nTolong cari ulang literatur review yang lebih tepat / perbaiki matriks sebelumnya sesuai dengan keluhan di atas.", 
			session.SelectedTopic.Name, session.SelectedTopic.Gap, session.SelectedTopic.Type, session.SelectedTopic.TypeReason, session.SelectedTopic.Evidence, session.SelectedTopic.Importance, session.Feedback)

		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		priorAgent := agent.NewPriorReviewAgent(llmBrain)
		matrix, err := priorAgent.GenerateMatrix(ctx, topicContext)
		if err != nil { return err }

		session.PriorReviewsMatrix = matrix
		session.Feedback = "" // Bersihkan feedback setelah direvisi
		session.Status = "M2_STEP2_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] Matriks Prior Reviews berhasil direvisi. DIJEDA kembali menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_STEP2_APPROVED":
		logger.Log(session.ID, "   [Langkah 2.2] Matriks Prior Reviews disetujui! Lanjut ke penyusunan PICO...")
		session.Status = "M2_STEP3_PICO"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 3: PICO FRAMEWORK + OPERATIONAL DEFINITIONS + TERMINOLOGI KANONIKAL
	// =========================================================================
	case "M2_STEP3_PICO":
		logger.Log(session.ID, "   [Langkah 2.3] Menyusun PICO Framework 3-Lapis...")
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		// RAG Context: Selected Topic
		topicContext := session.Topic
		if session.SelectedTopic != nil {
			topicContext = fmt.Sprintf("Judul: %s\nKesenjangan (Gap): %s\nTipe: %s (%s)\nBukti: %s\nAlasannya Mengapa Penting: %s", 
				session.SelectedTopic.Name, session.SelectedTopic.Gap, session.SelectedTopic.Type, session.SelectedTopic.TypeReason, session.SelectedTopic.Evidence, session.SelectedTopic.Importance)
		}

		// RAG Context: Prior Reviews Matrix
		priorMatrixContext := "Belum ada matrix prior reviews"
		if session.PriorReviewsMatrix != nil {
			matrixBytes, _ := json.MarshalIndent(session.PriorReviewsMatrix, "", "  ")
			priorMatrixContext = string(matrixBytes)
		}

		picoAgent := agent.NewPicoAgent(llmBrain)
		picoResult, err := picoAgent.Analyze(ctx, topicContext, priorMatrixContext)
		if err != nil { return err }

		session.PICODefinitions = picoResult
		session.Status = "M2_STEP3_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] PICO 3-Lapis berhasil disusun. DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_STEP3_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi masih dikunci. Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Lihat document sesi Anda, buka field 'pico_definitions'.")
		logger.Log(session.ID, "   2. Verifikasi 3 lapisan PICO (Operational Definitions & Canonical Term).")
		logger.Log(session.ID, "   3a. Jika SUDAH sesuai, ubah 'status' menjadi 'M2_STEP3_APPROVED' lalu Update.")
		logger.Log(session.ID, "   3b. Jika TIDAK sesuai, ubah 'status' menjadi 'M2_STEP3_NEEDS_REVISION' dan isi keluhan Anda di field 'feedback' lalu Update.")
		return nil

	case "M2_STEP3_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 2.3] Memperbaiki PICO berdasarkan feedback: '%s'\n", session.Feedback)

		topicContext := session.Topic
		if session.SelectedTopic != nil {
			topicContext = fmt.Sprintf("Judul: %s\nKesenjangan (Gap): %s\nTipe: %s (%s)\nBukti: %s\nAlasannya Mengapa Penting: %s", 
				session.SelectedTopic.Name, session.SelectedTopic.Gap, session.SelectedTopic.Type, session.SelectedTopic.TypeReason, session.SelectedTopic.Evidence, session.SelectedTopic.Importance)
		}
		topicContext += fmt.Sprintf("\n\n[INSTRUKSI REVISI DARI PENELITI]:\n%s\nTolong perbaiki PICO sebelumnya sesuai instruksi revisi ini.", session.Feedback)

		priorMatrixContext := ""
		if session.PriorReviewsMatrix != nil {
			matrixBytes, _ := json.MarshalIndent(session.PriorReviewsMatrix, "", "  ")
			priorMatrixContext = string(matrixBytes)
		}

		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		picoAgent := agent.NewPicoAgent(llmBrain)
		picoResult, err := picoAgent.Analyze(ctx, topicContext, priorMatrixContext)
		if err != nil { return err }

		session.PICODefinitions = picoResult
		session.Feedback = ""
		session.Status = "M2_STEP3_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] PICO 3-Lapis berhasil direvisi. DIJEDA kembali menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_STEP3_APPROVED":
		logger.Log(session.ID, "   [Langkah 2.3] PICO 3-Lapis disetujui! Menyiapkan template Batasan/Filter untuk pra-Langkah 4...")
		
		// Inisialisasi template kosong untuk diisi user
		session.ScopeFilters = &model.ScopeFilters{
			RentangTahun: "[ISI DI SINI, contoh: 2018-2023]",
			Geografis:    "[ISI DI SINI, contoh: Global / Asia Tenggara]",
			Sektor:       "[ISI DI SINI, contoh: Pendidikan / Kesehatan]",
			Bahasa:       "[ISI DI SINI, contoh: English only]",
			Lainnya:      "[ISI DI SINI, contoh: Hanya Jurnal Peer-Reviewed]",
		}
		session.Status = "M2_STEP3_5_WAITING_FILTERS"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_STEP3_5_WAITING_FILTERS":
		logger.Log(session.ID, "   [System] Sesi dikunci. Anda WAJIB melengkapi parameter filter dasar riset!")
		logger.Log(session.ID, "   Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Buka document sesi Anda, cari object 'scope_filters'.")
		logger.Log(session.ID, "   2. Ganti semua teks '[ISI DI SINI...]' dengan parameter riset Anda.")
		logger.Log(session.ID, "   3. Ubah 'status' menjadi 'M2_STEP3_5_FILTERS_PROVIDED' lalu Update.")
		return nil

	case "M2_STEP3_5_FILTERS_PROVIDED":
		// Validasi isian user
		if session.ScopeFilters == nil {
			session.Status = "M2_STEP3_5_WAITING_FILTERS"
			logger.Log(session.ID, "   [Error] Object 'scope_filters' tidak ditemukan. Mengembalikan status untuk diisi.")
			return m.deps.MongoRepo.UpdateSession(ctx, session)
		}

		f := session.ScopeFilters
		if strings.Contains(f.RentangTahun, "[ISI DI SINI") || strings.Contains(f.Geografis, "[ISI DI SINI") || 
		   strings.Contains(f.Sektor, "[ISI DI SINI") || strings.Contains(f.Bahasa, "[ISI DI SINI") {
			
			session.Status = "M2_STEP3_5_WAITING_FILTERS"
			logger.Log(session.ID, "   [Error] Masih ada isian filter yang menggunakan placeholder default '[ISI DI SINI...]'.")
			logger.Log(session.ID, "   [System] Sistem tidak bisa lanjut. Mohon isi data dengan lengkap lalu set status ke M2_STEP3_5_FILTERS_PROVIDED lagi.")
			return m.deps.MongoRepo.UpdateSession(ctx, session)
		}

		logger.Log(session.ID, "   [Validasi] Batasan/Filter berhasil divalidasi sistem! Lanjut menyusun Kriteria Scope (Langkah 4)...")
		session.Status = "M2_STEP4_SCOPE"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 4: JUSTIFIKASI BATASAN SCOPE (3-LAPIS)
	// =========================================================================
	case "M2_STEP4_SCOPE":
		logger.Log(session.ID, "   [Langkah 2.4] Merumuskan Justifikasi Batasan Scope 3-Lapis...")
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")
		filtersBytes, _ := json.MarshalIndent(session.ScopeFilters, "", "  ")

		scopeAgent := agent.NewScopeAgent(llmBrain)
		justifications, rawOutput, err := scopeAgent.GenerateJustifications(ctx, string(picoBytes), string(filtersBytes))
		if err != nil { return err }

		logger.Log(session.ID, "   [LLM Raw Output]:\n" + rawOutput)

		session.ScopeJustifications = justifications
		session.Status = "M2_STEP4_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] Justifikasi Batasan Scope berhasil disusun. DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_STEP4_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi masih dikunci. Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Lihat document sesi Anda, buka field 'scope_justifications'.")
		logger.Log(session.ID, "   2. Verifikasi 3 lapisan justifikasi (Teoretis, Metodologis, Praktis).")
		logger.Log(session.ID, "   3a. Jika SUDAH sesuai, ubah 'status' menjadi 'M2_STEP4_APPROVED' lalu Update.")
		logger.Log(session.ID, "   3b. Jika TIDAK sesuai, ubah 'status' menjadi 'M2_STEP4_NEEDS_REVISION' dan isi keluhan Anda di field 'feedback' lalu Update.")
		return nil

	case "M2_STEP4_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 2.4] Memperbaiki Justifikasi Scope berdasarkan feedback: '%s'\n", session.Feedback)

		picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")
		filtersBytes, _ := json.MarshalIndent(session.ScopeFilters, "", "  ")

		// Tambahkan feedback ke input
		filtersContext := string(filtersBytes) + fmt.Sprintf("\n\n[INSTRUKSI REVISI DARI PENELITI]:\n%s\nTolong perbaiki justifikasi sebelumnya sesuai instruksi revisi ini.", session.Feedback)

		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		scopeAgent := agent.NewScopeAgent(llmBrain)
		justifications, rawOutput, err := scopeAgent.GenerateJustifications(ctx, string(picoBytes), filtersContext)
		if err != nil { return err }

		logger.Log(session.ID, "   [LLM Raw Output]:\n" + rawOutput)

		session.ScopeJustifications = justifications
		session.Feedback = ""
		session.Status = "M2_STEP4_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] Justifikasi Batasan Scope direvisi. DIJEDA kembali menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_STEP4_APPROVED":
		logger.Log(session.ID, "   [Langkah 2.4] Justifikasi Scope disetujui! Lanjut ke penyusunan Research Questions...")
		session.Status = "M2_STEP5_RQ"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 5: FORMULASIKAN RESEARCH QUESTIONS
	// =========================================================================
	case "M2_STEP5_RQ":
		logger.Log(session.ID, "   [Langkah 2.5] Memformulasikan Research Questions (RQ)...")
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		// RAG Context Gathering
		topicContext := "Belum ada topik"
		if session.SelectedTopic != nil {
			topicBytes, _ := json.MarshalIndent(session.SelectedTopic, "", "  ")
			topicContext = string(topicBytes)
		}
		matrixContext := "Belum ada matriks"
		if session.PriorReviewsMatrix != nil {
			matrixBytes, _ := json.MarshalIndent(session.PriorReviewsMatrix, "", "  ")
			matrixContext = string(matrixBytes)
		}
		picoContext := "Belum ada PICO"
		if session.PICODefinitions != nil {
			picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")
			picoContext = string(picoBytes)
		}
		scopeContext := "Belum ada Justifikasi Scope"
		if len(session.ScopeJustifications) > 0 {
			scopeBytes, _ := json.MarshalIndent(session.ScopeJustifications, "", "  ")
			scopeContext = string(scopeBytes)
		}

		rqAgent := agent.NewRQAgent(llmBrain)
		rqs, err := rqAgent.GenerateRQ(ctx, topicContext, matrixContext, picoContext, scopeContext)
		if err != nil { return err }

		session.ResearchQuestions = rqs
		session.Status = "M2_STEP5_WAITING_APPROVAL"
		
		// Deteksi Orphan
		adaOrphan := false
		for _, rq := range rqs {
			if rq.IsOrphan {
				adaOrphan = true
				break
			}
		}

		if adaOrphan {
			logger.Log(session.ID, "   [WARNING] Ditemukan RQ yang berstatus 'RQ-orphan' (Tidak ter-trace ke PICO/GAP)!")
			logger.Log(session.ID, "   [WARNING] Anda diwajibkan merevisinya sebelum lanjut ke Langkah 6!")
		} else {
			logger.Log(session.ID, "   [System] Research Questions berhasil diformulasikan. DIJEDA menunggu persetujuan manusia.")
		}

		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_STEP5_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi masih dikunci. Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Buka document sesi Anda, cari array 'research_questions'.")
		logger.Log(session.ID, "   2. Verifikasi 1 Primary RQ dan 3 Secondary RQs beserta traceability-nya.")
		logger.Log(session.ID, "   3a. Jika SUDAH sempurna (dan is_orphan: false semua), ubah 'status' menjadi 'M2_STEP5_APPROVED' lalu Update.")
		logger.Log(session.ID, "   3b. Jika TIDAK sesuai (atau ada orphan), ubah 'status' menjadi 'M2_STEP5_NEEDS_REVISION' dan isi keluhan Anda di field 'feedback' lalu Update.")
		return nil

	case "M2_STEP5_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 2.5] Memperbaiki RQ berdasarkan feedback: '%s'\n", session.Feedback)

		topicContext := "Belum ada topik"
		if session.SelectedTopic != nil {
			topicBytes, _ := json.MarshalIndent(session.SelectedTopic, "", "  ")
			topicContext = string(topicBytes)
		}
		matrixContext := "Belum ada matriks"
		if session.PriorReviewsMatrix != nil {
			matrixBytes, _ := json.MarshalIndent(session.PriorReviewsMatrix, "", "  ")
			matrixContext = string(matrixBytes)
		}
		picoContext := "Belum ada PICO"
		if session.PICODefinitions != nil {
			picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")
			picoContext = string(picoBytes)
		}
		scopeContext := "Belum ada Justifikasi Scope"
		if len(session.ScopeJustifications) > 0 {
			scopeBytes, _ := json.MarshalIndent(session.ScopeJustifications, "", "  ")
			scopeContext = string(scopeBytes)
		}

		// Tambahkan feedback
		scopeContext += fmt.Sprintf("\n\n[INSTRUKSI REVISI DARI PENELITI]:\n%s\nTolong perbaiki Research Questions sebelumnya agar terhindar dari orphan dan selaras dengan keluhan di atas.", session.Feedback)

		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		rqAgent := agent.NewRQAgent(llmBrain)
		rqs, err := rqAgent.GenerateRQ(ctx, topicContext, matrixContext, picoContext, scopeContext)
		if err != nil { return err }

		session.ResearchQuestions = rqs
		session.Feedback = ""
		session.Status = "M2_STEP5_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] Research Questions direvisi. DIJEDA kembali menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_STEP5_APPROVED":
		logger.Log(session.ID, "   [Langkah 2.5] Research Questions disetujui! Lanjut ke Validasi FINER (Langkah 6)...")
		session.Status = "M2_STEP6_FINER_CHECK"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// =========================================================================
	// LANGKAH 6: CEK FINER + NOVELTY + INTERNAL COHERENCE + HASIL AKHIR
	// =========================================================================
	case "M2_STEP6_FINER_CHECK":
		logger.Log(session.ID, "   [Langkah 2.6] Melakukan validasi akhir FINER, Novelty & Internal Coherence...")
		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		rqsBytes, _ := json.MarshalIndent(session.ResearchQuestions, "", "  ")
		matrixBytes, _ := json.MarshalIndent(session.PriorReviewsMatrix, "", "  ")
		picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")
		scopeBytes, _ := json.MarshalIndent(session.ScopeJustifications, "", "  ")

		finerAgent := agent.NewFinerAgent(llmBrain)
		result, err := finerAgent.ValidateFiner(ctx, string(rqsBytes), string(matrixBytes), string(picoBytes), string(scopeBytes))
		if err != nil { return err }

		session.FinerNoveltyCheck = &result.Check
		session.Modul2Summary = &result.Summary
		session.Status = "M2_STEP6_WAITING_APPROVAL"
		
		logger.Log(session.ID, "   [System] Dokumen 'finer_novelty_check' dan 'modul2_summary' berhasil disusun.")
		if !result.Check.IsPass {
			logger.Log(session.ID, "   [WARNING] Evaluasi FINER mengindikasikan FAIL/TIDAK LULUS. Silakan cek rekomendasi revisi di database!")
		}
		logger.Log(session.ID, "   [System] DIJEDA menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_STEP6_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Sesi masih dikunci. Silakan buka MongoDB Compass:")
		logger.Log(session.ID, "   1. Buka document sesi Anda, cari 'finer_novelty_check' dan 'modul2_summary'.")
		logger.Log(session.ID, "   2. Verifikasi evaluasi FINER dan ringkasan Modul 2.")
		logger.Log(session.ID, "   3a. Jika SUDAH lulus dan sempurna, ubah 'status' menjadi 'M2_STEP6_APPROVED' lalu Update.")
		logger.Log(session.ID, "   3b. Jika BUTUH REVISI, ubah 'status' menjadi 'M2_STEP6_NEEDS_REVISION' dan isi keluhan Anda di field 'feedback'.")
		return nil

	case "M2_STEP6_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 2.6] Memperbaiki FINER & Summary berdasarkan feedback: '%s'\n", session.Feedback)
		
		rqsBytes, _ := json.MarshalIndent(session.ResearchQuestions, "", "  ")
		matrixBytes, _ := json.MarshalIndent(session.PriorReviewsMatrix, "", "  ")
		picoBytes, _ := json.MarshalIndent(session.PICODefinitions, "", "  ")
		scopeBytes, _ := json.MarshalIndent(session.ScopeJustifications, "", "  ")
		
		scopeContext := string(scopeBytes) + fmt.Sprintf("\n\n[INSTRUKSI REVISI DARI PENELITI UNTUK EVALUASI FINER]:\n%s\nPerbaiki evaluasi FINER atau ringkasan Modul 2 sesuai dengan arahan ini.", session.Feedback)

		llmBrain, err := m.deps.LLMFactory.CreateClient(ctx, "gemini")
		if err != nil { return err }

		finerAgent := agent.NewFinerAgent(llmBrain)
		result, err := finerAgent.ValidateFiner(ctx, string(rqsBytes), string(matrixBytes), string(picoBytes), scopeContext)
		if err != nil { return err }

		session.FinerNoveltyCheck = &result.Check
		session.Modul2Summary = &result.Summary
		session.Feedback = ""
		session.Status = "M2_STEP6_WAITING_APPROVAL"
		
		fmt.Println("   [System] Evaluasi FINER direvisi. DIJEDA kembali menunggu persetujuan manusia.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M2_STEP6_APPROVED":
		fmt.Println("   [Langkah 2.6] FINER disetujui! MODUL 2 SELESAI.")
		fmt.Println("   [System] Mentransfer data ke Modul 3 (Search Strategy).")
		session.Status = "M3_INIT" // Transisi ke modul 3
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	default:
		// Jika status diawali "M2_" namun belum terdaftar spesifik
		logger.Logf(session.ID, "   [Modul 2] Sub-status %s tidak dikenali atau belum diimplementasikan.\n", session.Status)
	}

	return nil
}
