package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/ifcoid/refs"

	"nsa/internal/agent"
	"nsa/internal/llm"
	"nsa/internal/logger"
	"nsa/internal/model"
	"nsa/internal/repository"
)

type M7Extraction struct {
	deps *ModuleDeps
	// ftCache menyimpan indeks full-text Qdrant per-sesi agar tidak dibangun ulang tiap batch.
	// BuildFulltextIndex men-scroll SELURUH koleksi global `scientific_articles` (mahal); tanpa
	// cache ia berjalan ulang di setiap batch 6-paper (puluhan scroll penuh per sesi) sambil
	// senyap → terlihat "lama & log kosong". Key = sessionID. Aman lintas-sesi (sync.Map).
	ftCache sync.Map
}

func NewM7Extraction(deps *ModuleDeps) *M7Extraction {
	return &M7Extraction{deps: deps}
}

func (m *M7Extraction) Name() string { return "M7_EXTRACTION" }

const extractionBatchSize = 6

// maxConsecutiveExtractFails: bila extractor LLM gagal sebanyak ini BERUNTUN (apa pun
// sebabnya: down, rate limit, API key salah, error server, stream kosong), anggap SISTEMIK
// (bukan paper jelek) dan ABORT batch — daripada menggilas seluruh paper jadi ERROR.
const maxConsecutiveExtractFails = 3

// ftCacheTTL membatasi umur indeks full-text yang di-cache. Cukup panjang untuk menampung
// banyak batch dalam satu run, tapi tetap di-rebuild bila run berjalan sangat lama (mis. ada
// ingestion full-text baru di Qdrant). Setiap rebuild di-log agar live log tak pernah gelap.
const ftCacheTTL = 30 * time.Minute

// spotVerifyCallTimeout membatasi satu panggilan verifier pada spot-check 20%. Cukup
// longgar untuk model besar, tapi mencegah satu call nyangkut menahan langkah QA berjam-jam.
const spotVerifyCallTimeout = 8 * time.Minute

// verifierSmokeTimeout membatasi pre-flight smoke-test Reviewer 2 (satu call kecil "ok").
// Pendek: tujuannya mendeteksi provider rusak (404/401/kuota) CEPAT lalu menjeda pipeline,
// bukan menunggu lama. Bila provider sehat, balasan datang dalam beberapa detik.
const verifierSmokeTimeout = 90 * time.Second

type ftCacheEntry struct {
	index   map[string]string
	builtAt time.Time
}

// fulltextIndexCached mengembalikan indeks full-text Qdrant, memakai cache per-sesi lintas
// batch. Selalu memberi log mulai/selesai (+durasi & jumlah entri) supaya user melihat proses
// berjalan, bukan layar log kosong selama scroll koleksi global yang besar.
func (m *M7Extraction) fulltextIndexCached(ctx context.Context, sessionID string) map[string]string {
	if v, ok := m.ftCache.Load(sessionID); ok {
		if e := v.(ftCacheEntry); time.Since(e.builtAt) < ftCacheTTL {
			logger.Logf(sessionID, "      [RAG] Pakai indeks full-text dari cache: %d entri (hemat scroll Qdrant).\n", len(e.index))
			return e.index
		}
	}
	logger.Log(sessionID, "      [RAG] Membangun indeks full-text dari Qdrant (scroll koleksi global)... mohon tunggu.")
	start := time.Now()
	idx, _, err := BuildFulltextIndex(ctx)
	if err != nil {
		logger.Logf(sessionID, "      [RAG] ⚠️ Gagal membangun indeks full-text: %v (lanjut tanpa RAG).\n", err)
	}
	if idx == nil {
		idx = map[string]string{}
	}
	logger.Logf(sessionID, "      [RAG] Indeks full-text siap: %d entri (%.1fs). Di-cache untuk batch berikutnya.\n",
		len(idx), time.Since(start).Seconds())
	m.ftCache.Store(sessionID, ftCacheEntry{index: idx, builtAt: time.Now()})
	return idx
}

// invalidateFulltextCache menghapus indeks cached saat ekstraksi selesai / akan di-ulang,
// agar run berikutnya membaca state Qdrant terbaru dan memori dibebaskan.
func (m *M7Extraction) invalidateFulltextCache(sessionID string) {
	m.ftCache.Delete(sessionID)
}

func (m *M7Extraction) Execute(ctx context.Context, session *model.SLRSession) error {
	logger.Logf(session.ID, ">> [MODUL 7: EXTRACTION + QA] State: %s\n", session.Status)
	ctx = llm.WithXAIContext(ctx, session.ID, session.Status, "M7Extraction")

	switch session.Status {
	case "M7_EXTRACTION", "M7_INIT":
		session.Status = "M7_STEP1_FRAMEWORK"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L1: Framework selection + template + pre-populate extraction ----
	case "M7_STEP1_FRAMEWORK":
		// Re-entry (mis. balik dari M6): PRESERVE protokol bila sudah ada.
		return m.runFrameworkL1(ctx, session, false)
	case "M7_STEP1_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau 'framework_selection' (framework + kolom template). Approve / revisi.")
		return nil
	case "M7_STEP1_NEEDS_REVISION":
		// Revisi framework EKSPLISIT oleh manusia -> regenerate protokol (forceRegen=true).
		logger.Logf(session.ID, "   [Revisi 7.1] Menyusun ulang framework (feedback: '%s')\n", session.Feedback)
		session.Feedback = ""
		return m.runFrameworkL1(ctx, session, true)
	case "M7_STEP1_APPROVED":
		session.Status = "M7_STEP2_EXTRACTION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L2: Systematic extraction (RAG) + 20% spot-verification ----
	case "M7_STEP2_EXTRACTION":
		return m.runExtractionL2(ctx, session)
	case "M7_STEP2_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau hasil ekstraksi (collection slr_extraction) + 'extraction_log'. Approve / revisi.")
		return nil
	case "M7_STEP2_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 7.2] Ekstraksi ulang (feedback: '%s')\n", session.Feedback)
		m.invalidateFulltextCache(session.ID) // re-extract: paksa indeks Qdrant dibangun ulang
		_, _ = m.deps.MongoRepo.GetExtractionCollection().UpdateMany(ctx,
			bson.M{"session_id": session.ID}, bson.M{"$set": bson.M{"extracted": false, "verified": false}})
		_, _ = m.deps.MongoRepo.GetSessionCollection().UpdateOne(ctx,
			bson.M{"_id": session.ID}, bson.M{"$unset": bson.M{"extraction_log": ""}})
		session.ExtractionLog = nil
		session.Feedback = ""
		session.Status = "M7_STEP2_EXTRACTION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP2_REEXTRACT_FAILED":
		// Self-heal hemat: re-extract HANYA paper yang gagal/kosong (ERROR, EMPTY_RESULT,
		// NO_FULLTEXT_RAG, atau coverage kosong) — PERTAHANKAN 67 paper yang sudah baik.
		// Hindari nuke-semua (re-approve framework + 13 menit + risiko 429). runExtractionL2
		// hanya mengambil paper extracted!=true, jadi cukup reset flag pada yg gagal.
		failFilter := bson.M{"session_id": session.ID, "coverage": bson.M{"$in": bson.A{"ERROR", "EMPTY_RESULT", "NO_FULLTEXT_RAG", ""}}}
		nFail, _ := m.deps.MongoRepo.GetExtractionCollection().CountDocuments(ctx, failFilter)
		logger.Logf(session.ID, "   [Self-heal 7.2] Re-extract %d paper gagal/kosong (paper baik dipertahankan)\n", nFail)
		_, _ = m.deps.MongoRepo.GetExtractionCollection().UpdateMany(ctx, failFilter,
			bson.M{"$set": bson.M{"extracted": false, "verified": false}})
		// Unset extraction_log agar spot-verify + statistik dihitung ulang setelah re-extract.
		_, _ = m.deps.MongoRepo.GetSessionCollection().UpdateOne(ctx,
			bson.M{"_id": session.ID}, bson.M{"$unset": bson.M{"extraction_log": ""}})
		session.ExtractionLog = nil
		session.Status = "M7_STEP2_EXTRACTION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP2_REVERIFY":
		// Ulangi HANYA spot-verification (mis. setelah memperbaiki provider Reviewer 2 yang
		// 404/locked) TANPA re-ekstrak. Data ekstraksi dipertahankan. Pre-flight smoke-test akan
		// menjeda lagi bila provider masih rusak — jadi user dapat umpan-balik cepat.
		logger.Log(session.ID, "   [Re-verify 7.2] Mengulang spot-verification (provider Reviewer 2 diperbaiki)...")
		return m.spotVerifyL2(ctx, session, false)
	case "M7_STEP2_VERIFY_BLOCKED":
		// Gerbang HITL: pipeline dijeda karena provider Reviewer 2 tak bisa dipakai. Tahan di
		// sini sampai user memperbaiki provider lalu 'Ulangi Verifikasi' (M7_STEP2_REVERIFY) —
		// atau memilih lanjut tanpa QA (M7_STEP2_VERIFY_SKIP). Lihat 'system_error' utk detail.
		logger.Log(session.ID, "   [System] ⛔ QA Reviewer 2 terblokir. Perbaiki provider di Pengaturan LLM lalu 'Ulangi Verifikasi', atau pilih 'Lanjut tanpa verifikasi'.")
		return nil
	case "M7_STEP2_VERIFY_SKIP":
		// User SADAR memilih lanjut tanpa QA dual-rater (provider Reviewer 2 tak tersedia).
		// Lewati verifikasi; didokumentasikan sebagai limitation. Data ekstraksi dipertahankan.
		logger.Log(session.ID, "   [System] Lanjut TANPA QA Reviewer 2 atas keputusan user (didokumentasikan sebagai limitation).")
		return m.spotVerifyL2(ctx, session, true)
	case "M7_STEP2_APPROVED":
		session.Status = "M7_STEP3_QA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L3: Quality appraisal (tool + threshold + dual-rater kappa + sensitivity) ----
	case "M7_STEP3_QA":
		return m.runQAL3(ctx, session)
	case "M7_STEP3_QA_TOOL_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau pilihan QA Tool & Threshold. Approve / revisi.")
		return nil
	case "M7_STEP3_QA_TOOL_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 7.3] Pemilihan ulang QA Tool (feedback: '%s')\n", session.Feedback)
		_, _ = m.deps.MongoRepo.GetSessionCollection().UpdateOne(ctx,
			bson.M{"_id": session.ID}, bson.M{"$unset": bson.M{"qa_threshold_justification": ""}})
		session.QAThreshold = nil
		// Feedback JANGAN dikosongkan di sini agar bisa dibaca oleh runQAL3
		session.Status = "M7_STEP3_QA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP3_QA_TOOL_APPROVED":
		session.Status = "M7_STEP3_QA_CALIBRATION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L3 Calibration: anchor examples + pilot batch + kappa check ----
	case "M7_STEP3_QA_CALIBRATION":
		return m.runQACalibration(ctx, session)
	case "M7_STEP3_QA_CALIBRATION_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau hasil kalibrasi QA (anchors + pilot kappa). Approve untuk lanjut full rating.")
		return nil
	case "M7_STEP3_QA_CALIBRATION_APPROVED":
		// Reset pilot papers qa_rated flag so they get re-rated in full batch.
		coll := m.deps.MongoRepo.GetExtractionCollection()
		_, _ = coll.UpdateMany(ctx,
			bson.M{"session_id": session.ID, "qa_calibration_pilot": true},
			bson.M{"$set": bson.M{"qa_rated": false}})
		session.Status = "M7_STEP3_QA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP3_QA_CALIBRATION_LOW_KAPPA":
		logger.Log(session.ID, "   [System] Kalibrasi QA kappa rendah. Pilih: retry kalibrasi atau lanjutkan (force proceed).")
		return nil
	case "M7_STEP3_QA_CALIBRATION_RETRY":
		// User wants to retry calibration. Reset pilot papers and re-run.
		coll := m.deps.MongoRepo.GetExtractionCollection()
		_, _ = coll.UpdateMany(ctx,
			bson.M{"session_id": session.ID, "qa_calibration_pilot": true},
			bson.M{"$unset": bson.M{"qa_calibration_pilot": ""}, "$set": bson.M{"qa_rated": false}})
		session.Status = "M7_STEP3_QA_CALIBRATION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP3_QA_CALIBRATION_FORCE_PROCEED":
		// User wants to proceed despite low kappa.
		coll := m.deps.MongoRepo.GetExtractionCollection()
		_, _ = coll.UpdateMany(ctx,
			bson.M{"session_id": session.ID, "qa_calibration_pilot": true},
			bson.M{"$set": bson.M{"qa_rated": false}})
		session.Status = "M7_STEP3_QA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP3_QA_BLOCKED":
		// Gerbang HITL: QA dijeda karena rater provider GAGAL SISTEMIK (rate-limit/overload/
		// ResourceExhausted, context overflow, atau endpoint tak terjangkau) — akan berulang
		// identik di tiap paper. Tahan di sini (passive, TIDAK me-reset apa pun) sampai user
		// memperbaiki/ganti provider rater di Pengaturan LLM lalu 'Ulangi QA' (reviseStep →
		// M7_STEP3_QA: re-attempt HANYA paper ERROR, pertahankan rating & kalibrasi yang sudah
		// ada via ResetQAErrors). Lihat 'system_error' utk detail + nama model. BEDA TEGAS dari
		// M7_STEP3_NEEDS_REVISION (reset PENUH: wipe semua qa_rated + tool + kalibrasi).
		logger.Log(session.ID, "   [System] ⛔ QA dijeda: rater provider gagal sistemik. Perbaiki/ganti provider rater di Pengaturan LLM lalu 'Ulangi QA' (lihat system_error).")
		return nil
	case "M7_STEP3_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau 'qa_threshold_justification' + 'sensitivity_analysis'. Approve / revisi.")
		return nil
	case "M7_STEP3_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi 7.3] QA ulang (feedback: '%s')\n", session.Feedback)
		_, _ = m.deps.MongoRepo.GetExtractionCollection().UpdateMany(ctx,
			bson.M{"session_id": session.ID}, bson.M{"$set": bson.M{"qa_rated": false}})
		_, _ = m.deps.MongoRepo.GetSessionCollection().UpdateOne(ctx,
			bson.M{"_id": session.ID}, bson.M{"$unset": bson.M{"qa_threshold_justification": "", "sensitivity_analysis": ""}})
		session.QAThreshold = nil
		session.SensitivityAnalysis = nil
		// Feedback JANGAN dikosongkan di sini agar bisa dibaca oleh runQAL3
		session.Status = "M7_STEP3_QA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP3_APPROVED":
		session.Status = "M7_STEP4_SYNTHESIS_PREP"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L4: Synthesis prep + meta-analysis feasibility + summary ----
	case "M7_STEP4_SYNTHESIS_PREP":
		return m.runSynthesisL4(ctx, session)
	case "M7_STEP4_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau 'synthesis_prep' + 'modul7_summary'. Approve untuk lanjut ke Modul 8.")
		return nil
	case "M7_STEP4_NEEDS_REVISION":
		session.Feedback = ""
		session.Status = "M7_STEP4_SYNTHESIS_PREP"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP4_APPROVED":
		session.Status = "M7_STEP5_GRAPH_EXTRACTION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L5: Knowledge Graph Extraction (Neuro-Symbolic / GraphRAG) ----
	case "M7_STEP5_GRAPH_EXTRACTION":
		return m.runGraphExtractionL5(ctx, session)
	case "M7_STEP5_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau hasil ekstraksi Knowledge Graph Neo4j. Approve untuk lanjut ke Modul 8.")
		return nil
	case "M7_STEP5_NEEDS_REVISION":
		session.Feedback = ""
		session.Status = "M7_STEP5_GRAPH_EXTRACTION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M7_STEP5_APPROVED":
		session.Status = "M8_SYNTHESIS"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	default:
		return nil
	}
}

// ===== L1 =====

func (m *M7Extraction) runFrameworkL1(ctx context.Context, session *model.SLRSession, forceRegen bool) error {
	// VALIDITAS SLR (lihat CLAUDE.md): protokol ekstraksi ditetapkan a priori & diterapkan
	// SERAGAM ke semua studi. Bila protokol SUDAH ADA dan ini BUKAN revisi framework eksplisit
	// (forceRegen), JANGAN regenerate — protokol yang berubah mengikuti data = HARKing/
	// inkonsisten. Pertahankan protokol + data ekstraksi lama; cukup sinkronkan set INCLUDE
	// terbaru (mis. setelah koreksi include/exclude di M6) lalu ekstraksi inkremental hanya
	// memproses paper baru.
	if !forceRegen && session.FrameworkSelection != nil && len(session.FrameworkSelection.Columns) > 0 {
		fwName := session.FrameworkSelection.Framework
		logger.Logf(session.ID, "   [Langkah 7.1] Protokol ekstraksi DIPERTAHANKAN (framework '%s', %d kolom) — TIDAK di-regenerate (validitas SLR). Sinkron set INCLUDE terbaru...\n",
			fwName, len(session.FrameworkSelection.Columns))
		included := m.finalIncludedPapers(ctx, session)
		m.enrichDocTypes(ctx, session.ID, included)
		coll := m.deps.MongoRepo.GetExtractionCollection()
		added := 0
		for _, p := range included {
			paperID := ""
			if oid, ok := p["_id"].(interface{ Hex() string }); ok {
				paperID = oid.Hex()
			}
			if paperID == "" {
				continue
			}
			// Upsert: paper BARU -> insert (extracted=false, masuk antrean). Paper LAMA ->
			// JANGAN disentuh ($setOnInsert): pertahankan data ekstraksinya (LLM non-deterministik,
			// re-ekstraksi paper tak berubah merusak reproducibility).
			res, _ := coll.UpdateOne(ctx,
				bson.M{"session_id": session.ID, "paper_id": paperID},
				bson.M{"$setOnInsert": bson.M{
					"session_id": session.ID, "paper_id": paperID,
					"Title":     getStr(p, "Title", "title"),
					"Author":    getStr(p, "Authors", "authors"),
					"Year":      getStr(p, "Year", "year"),
					"Journal":   getStr(p, "Journal", "journal"),
					"DOI":       getStr(p, "DOI", "doi"),
					"extracted": false, "qa_rated": false,
				}},
				options.Update().SetUpsert(true))
			if res != nil && res.UpsertedCount > 0 {
				added++
			}
		}
		logger.Logf(session.ID, "   [System] Protokol lama dipakai ulang; %d paper baru masuk antrean ekstraksi (paper lama + datanya dipertahankan). Menunggu persetujuan.\n", added)
		session.Status = "M7_STEP1_WAITING_APPROVAL"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	logger.Log(session.ID, "   [Langkah 7.1] Rekomendasi framework + template ekstraksi...")

	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return m.deps.llmError(ctx, "brain", "Memuat client framework M7", err)
	}
	ag := agent.NewExtractionAgent(brain)

	picoJSON := "(tidak tersedia)"
	if session.PICODefinitions != nil {
		b, _ := json.Marshal(session.PICODefinitions)
		picoJSON = string(b)
	}
	rqJSON := "(tidak tersedia)"
	if len(session.ResearchQuestions) > 0 {
		b, _ := json.Marshal(session.ResearchQuestions)
		rqJSON = string(b)
	}

	included := m.finalIncludedPapers(ctx, session)
	m.enrichDocTypes(ctx, session.ID, included) // isi document_type (CrossRef) -> breakdown akurat
	designBreakdown := docTypeBreakdown(included)

	fw, err := ag.RecommendFramework(ctx, picoJSON, rqJSON, designBreakdown)
	if err != nil {
		// xAI: sebut role + provider + model + tindakan perbaikan, bukan error mentah
		// "stream kosong dari provider" yang tak memberi tahu config mana yang salah.
		return m.deps.llmError(ctx, "brain", "Rekomendasi framework", err)
	}
	session.FrameworkSelection = fw

	// xAI: atribusi model untuk tampilan = provider + NAMA MODEL asli.
	if lbl := m.deps.roleLabel(ctx, "brain"); lbl != "belum dikonfigurasi" {
		fw.ModelUsed = lbl
	}

	// Pre-populate koleksi extraction (idempotent untuk sesi ini).
	coll := m.deps.MongoRepo.GetExtractionCollection()
	_, _ = coll.DeleteMany(ctx, bson.M{"session_id": session.ID})
	var docs []interface{}
	for _, p := range included {
		paperID := ""
		if oid, ok := p["_id"].(interface{ Hex() string }); ok {
			paperID = oid.Hex()
		}
		docs = append(docs, bson.M{
			"session_id": session.ID,
			"paper_id":   paperID,
			"Title":      getStr(p, "Title", "title"),
			"Author":     getStr(p, "Authors", "authors"),
			"Year":       getStr(p, "Year", "year"),
			"Journal":    getStr(p, "Journal", "journal"),
			"DOI":        getStr(p, "DOI", "doi"),
			"extracted":  false,
			"qa_rated":   false,
		})
	}
	if len(docs) > 0 {
		_, _ = coll.InsertMany(ctx, docs)
	}
	logger.Logf(session.ID, "   [System] Framework '%s' dipilih, %d paper INCLUDE di-prepopulate.\n", fw.Framework, len(docs))

	session.Status = "M7_STEP1_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// ===== L2 =====

// modelLabel mengembalikan atribusi model xAI lengkap "Provider (model)" dari LLMConfig
// suatu provider role, bukan ModelName() mentah (yg bisa dobel-prefix "openai/openai/...").
// Fallback ke ID provider bila config tak ada. Dipakai konsisten di L1/L2/QA.
func (m *M7Extraction) modelLabel(ctx context.Context, providerID string) string {
	return m.deps.providerLabel(ctx, providerID)
}

func (m *M7Extraction) runExtractionL2(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 7.2] Systematic extraction (RAG Qdrant)...")
	coll := m.deps.MongoRepo.GetExtractionCollection()

	totalCount, _ := coll.CountDocuments(ctx, bson.M{"session_id": session.ID})
	if totalCount == 0 {
		logger.Log(session.ID, "   [System] Tidak ada paper untuk diekstrak. Lanjut approval.")
		session.ExtractionLog = &model.ExtractionLog{}
		session.Status = "M7_STEP2_WAITING_APPROVAL"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	cur, err := coll.Find(ctx, bson.M{"session_id": session.ID, "extracted": bson.M{"$ne": true}},
		options.Find().SetLimit(int64(extractionBatchSize)))
	if err != nil {
		return err
	}
	var batch []bson.M
	_ = cur.All(ctx, &batch)

	// Semua sudah diekstrak -> spot-verify (20%) lalu approval.
	if len(batch) == 0 {
		if session.ExtractionLog == nil {
			return m.spotVerifyL2(ctx, session, false)
		}
		m.invalidateFulltextCache(session.ID) // ekstraksi tuntas: bebaskan indeks cached
		session.Status = "M7_STEP2_WAITING_APPROVAL"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	colsJSON := "[]"
	if session.FrameworkSelection != nil {
		b, _ := json.Marshal(session.FrameworkSelection.Columns)
		colsJSON = string(b)
	}
	opDefs := m.opDefs(session)

	ftIndex := m.fulltextIndexCached(ctx, session.ID)

	doneCount, _ := coll.CountDocuments(ctx, bson.M{"session_id": session.ID, "extracted": true})
	logger.Logf(session.ID, "   [Info] Progres ekstraksi: %d/%d selesai. Memproses batch %d paper berikutnya (tiap paper ~10-60 dtk via LLM).\n",
		doneCount, totalCount, len(batch))

	rp1, rf1 := m.deps.LLMFactory.RoleProviders(ctx, "reviewer1")
	leadAg, err := m.agentWithFallback(ctx, rp1, rf1)
	if err != nil {
		return m.deps.llmError(ctx, "reviewer1", "Memuat extractor utama", err)
	}
	// xAI: atribusi model konsisten = provider role + NAMA MODEL asli (bukan ModelName()
	// mentah "openai/openai/gpt-oss-120b" yang dobel-prefix & menyesatkan). Samakan dgn M7 L1/QA.
	extractorModel := m.modelLabel(ctx, rp1)

	consecutiveFails := 0 // gagal LLM beruntun -> sistemik -> abort (lihat maxConsecutiveExtractFails)
	for i, p := range batch {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		title := getStr(p, "Title")
		doi := getStr(p, "DOI", "doi")
		logger.Logf(session.ID, "      -> Extract [%d/%d] %s\n", i+1, len(batch), doi)

		var ft string
		if nd := normalizeDOIForRAG(doi); nd != "" && ftIndex[nd] != "" {
			ft = ftIndex[nd]
		} else if nt := NormTitle(title); nt != "" && ftIndex["title:"+nt] != "" {
			ft = ftIndex["title:"+nt]
		}

		update := bson.M{"extracted": true}
		if ft == "" {
			logger.Logf(session.ID, "         [RAG] Full-text TIDAK ada di Qdrant — ditandai NO_FULLTEXT_RAG (perlu ekstraksi manual).\n")
			update["coverage"] = "NO_FULLTEXT_RAG"
			update["notes"] = "Full-text tidak tersedia di Qdrant; perlu ekstraksi manual."
		} else {
			logger.Logf(session.ID, "         [LLM] Memanggil extractor (%s)... bisa 10-60 dtk, mohon tunggu.\n", extractorModel)
			res, e := leadAg.ExtractPaper(ctx, colsJSON, opDefs, title, ft)
			if e != nil && isLLMConnectivityError(e) {
				// Error KONEKTIVITAS = sistemik (server LLM mati / base URL salah). Percuma
				// meneruskan 5 paper lain × 3 retry yang pasti gagal identik, dan menandai
				// semua ERROR malah mengotori state. Abort batch dgn pesan actionable; paper
				// ini TIDAK ditulis (tetap extracted=false) agar re-extract bersih saat hidup.
				return fmt.Errorf("Ekstraktor LLM role Reviewer 1 (%s) tidak bisa dihubungi: %v — server LLM mati atau base URL provider salah. Nyalakan server-nya / perbaiki provider Reviewer 1 di Pengaturan LLM, lalu Resume. (Paper belum ditandai ERROR; akan diekstrak ulang otomatis saat server hidup.)", extractorModel, e)
			}
			if e != nil {
				consecutiveFails++
				// Gagal BERUNTUN (rate limit, API key salah, error server, stream kosong, dll)
				// = kemungkinan SISTEMIK, bukan paper jelek. Abort sebelum menggilas sisa paper
				// jadi ERROR. Paper ini & sisanya TIDAK ditulis ERROR -> re-extract bersih saat
				// provider beres (Resume).
				if consecutiveFails >= maxConsecutiveExtractFails {
					hint := ""
					if isContextOverflowError(e) {
						// Pola khas: smoke test (prompt mungil) HIJAU, tapi ekstraksi (full-text
						// besar) gagal "stream kosong". Arahkan ke akar yang benar: context window.
						hint = " PETUNJUK: error 'stream kosong'/context menandakan full-text paper kemungkinan MELEBIHI context window model — ganti Reviewer 1 ke model context BESAR (mis. Gemini / model >=128k token), bukan sekadar provider lain. (Smoke test hijau memakai prompt mungil, jadi tak menjamin sanggup prompt full-text.)"
					}
					return fmt.Errorf("Ekstraksi dihentikan: extractor LLM role Reviewer 1 (%s) gagal %d paper beruntun (terakhir: %v) — kemungkinan provider down / rate-limit / API key salah / context window terlampaui.%s Perbaiki provider Reviewer 1 di Pengaturan LLM lalu Resume. (Sisa paper belum ditandai ERROR.)", extractorModel, consecutiveFails, e, hint)
				}
				logger.Logf(session.ID, "         [!] gagal extract: %v (ditandai ERROR; %d/%d beruntun)\n", e, consecutiveFails, maxConsecutiveExtractFails)
				update["coverage"] = "ERROR"
				update["notes"] = "Ekstraksi gagal: " + e.Error()
			} else if len(res.Fields) == 0 {
				// Silent-empty: parse sukses tapi LLM tak mengembalikan satu field pun.
				// JANGAN sembunyikan sbg "selesai" — tandai agar terlihat & bisa re-extract HITL.
				consecutiveFails = 0 // LLM merespons (walau kosong) -> bukan kegagalan sistemik
				logger.Logf(session.ID, "         [!] hasil kosong (0 field) — ditandai EMPTY_RESULT\n")
				update["coverage"] = "EMPTY_RESULT"
				update["notes"] = "LLM mengembalikan hasil kosong (0 field) walau full-text tersedia; perlu ekstraksi ulang."
				update["model_extraction"] = extractorModel
			} else {
				consecutiveFails = 0
				cov, covNote := agent.NormalizeCoverage(res.Coverage)
				update["fields"] = res.Fields
				update["key_findings"] = res.KeyFindings
				update["qa_red_flags"] = res.QARedFlags
				update["ambiguous"] = res.Ambiguous
				update["coverage"] = cov
				if covNote != "" {
					update["notes"] = covNote
				}
				update["nr_count"] = countNotReported(res.Fields)
				update["model_extraction"] = extractorModel
				logger.Logf(session.ID, "         [✓] Ekstraksi sukses: %d field, coverage=%s (%s).\n", len(res.Fields), cov, extractorModel)
			}
			time.Sleep(5 * time.Second)
		}
		_, _ = coll.UpdateByID(ctx, p["_id"], bson.M{"$set": update})

		// Post-extraction enrichment: fill NOT_REPORTED design/geographic from CrossRef
		if update["fields"] != nil {
			// Convert []agent.ExtractedField to bson.A for the enrichment function
			if extractedFields, ok := update["fields"].([]agent.ExtractedField); ok {
				fieldsBsonA := make(bson.A, len(extractedFields))
				for fi, ef := range extractedFields {
					fieldsBsonA[fi] = bson.M{
						"key":      ef.Key,
						"value":    ef.Value,
						"evidence": ef.Evidence,
						"status":   ef.Status,
					}
				}
				enrichDoc := bson.M{
					"_id":    p["_id"],
					"DOI":    doi,
					"fields": fieldsBsonA,
				}
				EnrichNotReportedFields(ctx, coll, enrichDoc, session.ID)
			}
		}
	}

	session.Status = "M7_STEP2_EXTRACTION" // loop batch berikutnya / verifikasi
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// spotVerifyL2 memverifikasi 20% sampel + field AMBIGUOUS (extractor 2).
func (m *M7Extraction) spotVerifyL2(ctx context.Context, session *model.SLRSession, skipVerification bool) error {
	logger.Log(session.ID, "   [Langkah 7.2] Spot-verification 20% (extractor 2)...")
	coll := m.deps.MongoRepo.GetExtractionCollection()

	// Hanya paper dengan data nyata: kecualikan gagal/kosong agar tidak diverifikasi sia-sia
	// dan tidak ikut dihitung sbg "extracted". (EMPTY_RESULT/"" = hasil kosong, perlu re-extract.)
	cur, _ := coll.Find(ctx, bson.M{"session_id": session.ID, "coverage": bson.M{"$nin": bson.A{"NO_FULLTEXT_RAG", "ERROR", "EMPTY_RESULT", ""}}})
	var all []bson.M
	_ = cur.All(ctx, &all)

	total := len(all)
	sample := total / 5 // 20%
	if sample < 1 && total > 0 {
		sample = 1
	}

	opDefs := m.opDefs(session)
	vp, vf := m.deps.LLMFactory.RoleProviders(ctx, "reviewer2")
	verifierModel := m.modelLabel(ctx, vp)

	disagree, checked, ambiguous, verifyFails := 0, 0, 0, 0
	consecVerifyFails := 0
	var lastVerifyErr string

	if skipVerification {
		// User SADAR memilih lanjut tanpa QA dual-rater (provider Reviewer 2 tak tersedia).
		// Verifikasi dilewati; di tail dicatat sbg LIMITATION metodologis (bukan "acceptable").
		logger.Logf(session.ID, "   [Langkah 7.2] User memilih LANJUT tanpa QA Reviewer 2 — verifikasi DILEWATI (didokumentasikan sebagai limitation).\n")
	} else {
		// PRE-FLIGHT (peringatan awal): pastikan provider Reviewer 2 BENAR-BENAR bisa dipanggil
		// SEBELUM memulai loop ~4 dtk × N paper. Bila 404/401/kuota → JEDA pipeline di gerbang
		// khusus & beri user waktu memperbaiki provider, alih-alih memanggil verifier yang pasti
		// gagal lalu menampilkan "0 diverifikasi" yang menyesatkan di akhir.
		verAg, loadErr := m.agentWithFallback(ctx, vp, vf)
		smokeErr := loadErr
		if verAg != nil && smokeErr == nil {
			logger.Logf(session.ID, "   [Pre-flight 7.2] Menguji koneksi nyata ke Reviewer 2 (%s)... ~beberapa detik.\n", verifierModel)
			sctx, scancel := context.WithTimeout(ctx, verifierSmokeTimeout)
			smokeErr = verAg.SmokeTest(sctx)
			scancel()
		}
		if smokeErr != nil {
			return m.blockVerifier(ctx, session, total, verifierModel, smokeErr)
		}
		logger.Logf(session.ID, "   [Pre-flight 7.2] ✅ Reviewer 2 (%s) siap. Verifikasi ~%d dari %d paper (20%% sampel + semua AMBIGUOUS); ~4 dtk/paper.\n",
			verifierModel, sample, total)

		ftIndex := m.fulltextIndexCached(ctx, session.ID)
		for i := 0; i < total; i++ {
			// Spot-verify = QA sampling non-kritikal. Jika plafon waktu run tercapai (atau run
			// di-stop), JANGAN jadikan fatal: hentikan verifikasi dgn rapi & lanjut ke approval —
			// hasil ekstraksi sudah tersimpan. (Dulu di sini `return ctx.Err()` membuat SELURUH
			// pipeline jadi _ERROR hanya karena satu langkah QA kehabisan waktu.)
			if ctx.Err() != nil {
				logger.Logf(session.ID, "   [WARN] Verifikasi dihentikan di %d/%d (batas waktu run / stop). Lanjut ke approval dgn ekstraksi yang sudah tersimpan.\n", i, total)
				break
			}
			p := all[i]
			amb, _ := p["ambiguous"].(bson.A)
			isAmbiguous := len(amb) > 0
			ambiguous += len(amb)
			if i >= sample && !isAmbiguous {
				continue
			}
			doi := normalizeDOIForRAG(getStr(p, "DOI", "doi"))
			ft := ftIndex[doi]
			if ft == "" {
				logger.Logf(session.ID, "      -> Verify [%d/%d] %s — full-text tak ada di RAG, dilewati.\n", i+1, total, doi)
				continue
			}
			logger.Logf(session.ID, "      -> Verify [%d/%d] %s — memanggil verifier (%s)... ~4 dtk.\n", i+1, total, doi, verifierModel)
			priorJSON, _ := json.Marshal(p["fields"])
			// Batasi tiap panggilan verifier: satu call yang nyangkut tak boleh menahan langkah QA
			// berjam-jam (Generate sendiri retry 3×@60mnt). Timeout → di-skip, bukan fatal.
			vctx, vcancel := context.WithTimeout(ctx, spotVerifyCallTimeout)
			res, e := verAg.VerifyExtraction(vctx, opDefs, getStr(p, "Title"), ft, string(priorJSON))
			vcancel()
			checked++
			if e != nil {
				verifyFails++
				consecVerifyFails++
				lastVerifyErr = e.Error()
				logger.Logf(session.ID, "         [!] verifikasi dilewati (gagal/timeout): %v\n", e)
				// Gagal BERUNTUN tanpa satu pun sukses = verifier sistemik bermasalah (mis. model
				// 404 / key salah / kuota habis MID-RUN setelah lolos pre-flight). JEDA pipeline di
				// gerbang khusus dgn error PENUH — beri user kesempatan memperbaiki provider, bukan
				// lanjut ke approval dengan QA yang menyesatkan (0 berhasil).
				if consecVerifyFails >= maxConsecutiveExtractFails && (checked-verifyFails) == 0 {
					logger.Logf(session.ID, "   [WARN] Verifier gagal %d kali beruntun tanpa satu pun sukses — JEDA pipeline. Periksa provider Reviewer 2 di Pengaturan LLM.\n", consecVerifyFails)
					return m.blockVerifier(ctx, session, total, verifierModel,
						fmt.Errorf("verifier gagal %d kali beruntun tanpa satu pun sukses; error terakhir: %s", consecVerifyFails, lastVerifyErr))
				}
			} else if res != nil && res.Disagree {
				consecVerifyFails = 0
				disagree++
				logger.Logf(session.ID, "         [≠] Disagreement — ditandai untuk ditinjau (HITL).\n")
				_, _ = coll.UpdateByID(ctx, p["_id"], bson.M{"$set": bson.M{"verify_disagree": true, "verify_notes": res.Notes}})
			} else {
				consecVerifyFails = 0
				logger.Logf(session.ID, "         [✓] Sesuai (tidak ada disagreement).\n")
			}
			time.Sleep(4 * time.Second)
		}
	}

	// Rate dihitung HANYA atas verifikasi yang BERHASIL (jangan campur kegagalan provider).
	okChecked := checked - verifyFails
	rate := 0.0
	if okChecked > 0 {
		rate = float64(disagree) / float64(okChecked) * 100
	}
	nrNote := "<5%: acceptable; dokumentasi Limitations."
	if skipVerification {
		// User memilih lewati QA dual-rater (provider Reviewer 2 tak tersedia). Ini LIMITATION
		// metodologis yang HARUS didokumentasikan, BUKAN "acceptable".
		nrNote = "⚠ QA dual-rater (Reviewer 2) DILEWATI atas keputusan user (provider tak tersedia). Dokumentasikan sebagai LIMITATION metodologis di manuskrip (PRISMA: tidak ada verifikasi silang ekstraksi)."
	} else if okChecked == 0 && checked > 0 {
		// JANGAN laporkan 0% palsu sebagai "acceptable" saat verifikasi gagal total.
		nrNote = fmt.Sprintf("⚠ Spot-verification GAGAL TOTAL (0/%d berhasil): verifier role Reviewer 2 (%s) error — %s. QA dual-rater TIDAK berjalan; perbaiki provider Reviewer 2 di Pengaturan LLM, lalu re-verify.", checked, verifierModel, clipErr(lastVerifyErr))
	} else if rate > 15 {
		nrNote = ">15%: disarankan full dual-extraction untuk semua studi."
	} else if rate >= 5 {
		nrNote = "5-15%: refine protocol, re-do subset yang di-flag."
	}
	rp1, _ := m.deps.LLMFactory.RoleProviders(ctx, "reviewer1")
	// Hitung paper gagal/kosong yang masih perlu re-extract (surfacing utk tombol HITL).
	failedCount, _ := coll.CountDocuments(ctx, bson.M{"session_id": session.ID,
		"coverage": bson.M{"$in": bson.A{"ERROR", "EMPTY_RESULT", "NO_FULLTEXT_RAG", ""}}})
	// xAI: atribusi model LENGKAP (provider role + nama model) utk header log, samakan per-paper.
	extractorModelLbl := m.modelLabel(ctx, rp1)
	refineModelLbl := m.modelLabel(ctx, vp)
	colsJSON := "[]"
	if session.FrameworkSelection != nil {
		b, _ := json.Marshal(session.FrameworkSelection.Columns)
		colsJSON = string(b)
	}
	systemPrompt := fmt.Sprintf(`Anda Extractor utama untuk Systematic Literature Review.
Ekstrak data per kolom TEMPLATE dari FULL-TEXT artikel (konteks RAG).

TEMPLATE KOLOM (JSON):
%s

OPERATIONAL DEFINITIONS:
%s

ATURAN ANTI-HALUSINASI (WAJIB):
- Simpulkan HANYA dari full-text yang diberikan. Dilarang memakai pengetahuan luar.
- Per field: kutip kalimat pendukung + section ref di "evidence" (mis. "Methods p.5: We surveyed 234...").
- Jika tidak ada di teks: value "[NOT REPORTED]", status "NOT_REPORTED" (JANGAN mengira).
- Borderline: status "AMBIGUOUS" + alasan di evidence.
- Konsisten dengan canonical terminology.
- RED FLAGS QA (sample kecil tanpa power analysis, missing data tak dijelaskan, confounder tak ditangani, outcome tak validated) → ringkas di "qa_red_flags" (awali tiap item 'QA_RED:').

Keluarkan HANYA JSON MURNI tanpa markdown:
{
  "fields": [{"key": "Theory", "value": "...", "evidence": "Intro p.2: ...", "status": "REPORTED"}],
  "key_findings": "1-2 kalimat temuan utama",
  "qa_red_flags": "QA_RED: ... ; QA_RED: ...",
  "ambiguous": ["nama field yang ambiguous"],
  "coverage": "COMPLETE"
}`, colsJSON, opDefs)

	session.ExtractionLog = &model.ExtractionLog{
		TotalExtracted:      total,
		VerifiedSample:      okChecked, // hanya verifikasi yang BERHASIL (bukan termasuk gagal)
		DisagreementRate:    rate,
		AmbiguousCount:      ambiguous,
		NRNote:              nrNote,
		FailedCount:         int(failedCount),
		SystemPrompt:        systemPrompt,
		ModelExtraction:     extractorModelLbl,
		ModelRefineProtocol: refineModelLbl,
	}
	if skipVerification {
		logger.Logf(session.ID, "   [System] Ekstraksi %d paper; QA Reviewer 2 DILEWATI atas keputusan user (limitation); gagal/kosong %d.\n", total, failedCount)
	} else if verifyFails > 0 {
		logger.Logf(session.ID, "   [System] Ekstraksi %d paper; verifikasi %d BERHASIL / %d GAGAL; disagreement %.1f%% (atas yang berhasil); gagal/kosong %d.%s\n",
			total, okChecked, verifyFails, rate, failedCount,
			func() string {
				if okChecked == 0 {
					return " ⚠ QA dual-rater TIDAK berjalan — cek Reviewer 2."
				}
				return ""
			}())
	} else {
		logger.Logf(session.ID, "   [System] Ekstraksi %d paper; verifikasi %d; disagreement %.1f%%; gagal/kosong %d.\n", total, okChecked, rate, failedCount)
	}
	m.invalidateFulltextCache(session.ID) // selesai: bebaskan indeks cached
	// Sampai sini = pre-flight LULUS (atau user pilih skip) → bersihkan error blokir Reviewer 2
	// yang lama agar gerbang/titik-merah tidak nyangkut. (system_error tanpa omitempty → ""
	// benar-benar meng-clear di $set.)
	session.SystemError = ""
	session.Status = "M7_STEP2_WAITING_APPROVAL"

	// Pastikan transisi ke approval TETAP tersimpan walau plafon waktu run sudah tercapai
	// (ctx kedaluwarsa). Tanpa ini, write gagal → pipeline jadi _ERROR padahal ekstraksi beres.
	writeCtx := ctx
	if ctx.Err() != nil {
		var c context.CancelFunc
		writeCtx, c = context.WithTimeout(context.Background(), 30*time.Second)
		defer c()
	}
	return m.deps.MongoRepo.UpdateSession(writeCtx, session)
}

// blockVerifier MENJEDA pipeline di gerbang M7_STEP2_VERIFY_BLOCKED saat provider Reviewer 2
// tak bisa dipakai (pre-flight gagal atau gagal sistemik mid-run). Menyimpan error PENUH
// (tak dipotong) + instruksi solusi agar user tahu PERSIS apa yang harus diperbaiki di
// Pengaturan LLM, lalu menekan "Ulangi Verifikasi". Data ekstraksi yang sudah tersimpan TIDAK
// disentuh (preserve) — gerbang ini hanya menahan kemajuan sampai keputusan/perbaikan manusia.
func (m *M7Extraction) blockVerifier(ctx context.Context, session *model.SLRSession, total int, verifierModel string, cause error) error {
	full := cause.Error()
	logger.Logf(session.ID, "   [BLOCK 7.2] ⛔ QA Reviewer 2 (%s) tidak bisa dijalankan: %v\n", verifierModel, full)
	logger.Log(session.ID, "   [BLOCK 7.2] Pipeline DIJEDA — perbaiki provider Reviewer 2 di Pengaturan LLM (Test Model sampai ✓), lalu klik 'Ulangi Verifikasi'. Ekstraksi sudah tersimpan & AMAN.")
	session.SystemError = fmt.Sprintf("⛔ QA silang Reviewer 2 (%s) tidak bisa dijalankan — pipeline DIJEDA agar Anda sempat memperbaikinya.\n\nDETAIL ERROR: %s\n\nYANG HARUS DIPERBAIKI: buka Pengaturan LLM → role 'Reviewer 2', perbaiki API key / nama model / base URL (klik 'Test Model' sampai ✓ hijau). Penyebab umum: nama model salah/terkunci (404), API key salah (401), atau kuota habis (429).\n\nLALU: klik 'Ulangi Verifikasi (tanpa re-ekstrak)'. Data ekstraksi %d paper sudah TERSIMPAN & AMAN — tidak akan hilang.", verifierModel, full, total)
	// Simpan konteks di extraction_log (verified=0 + error PENUH) agar gerbang UI bisa
	// menampilkan apa yang gagal & nama modelnya, bukan hanya banner generik.
	session.ExtractionLog = &model.ExtractionLog{
		TotalExtracted:      total,
		VerifiedSample:      0,
		VerifierError:       full,
		NRNote:              "QA dual-rater DIJEDA: provider Reviewer 2 error. Perbaiki di Pengaturan LLM lalu Ulangi Verifikasi.",
		ModelRefineProtocol: verifierModel,
	}
	session.Status = "M7_STEP2_VERIFY_BLOCKED"
	writeCtx := ctx
	if ctx.Err() != nil {
		var c context.CancelFunc
		writeCtx, c = context.WithTimeout(context.Background(), 30*time.Second)
		defer c()
	}
	return m.deps.MongoRepo.UpdateSession(writeCtx, session)
}

// ===== Helpers (dipakai L1-L4) =====

func (m *M7Extraction) finalIncludedPapers(ctx context.Context, session *model.SLRSession) []map[string]interface{} {
	all, err := m.deps.MongoRepo.GetAllScreeningPapers(ctx, session.ID)
	if err != nil {
		return nil
	}
	var out []map[string]interface{}
	for _, p := range all {
		retrieved, _ := p["full_text_retrieved"].(bool)
		incAbs := getStr(p, "Final_Decision") == "INCLUDE" ||
			(getStr(p, "Final_Decision") == "" && getStr(p, "Screener_1_Decision") == "INCLUDE")
		if retrieved && incAbs && finalFullDecision(p) == "INCLUDE" {
			out = append(out, p)
		}
	}
	return out
}

func (m *M7Extraction) opDefs(session *model.SLRSession) string {
	if session.PICODefinitions != nil {
		b, _ := json.Marshal(session.PICODefinitions)
		return string(b)
	}
	return "(operational definitions tidak tersedia)"
}

// agentWithFallback membuat ExtractionAgent dengan retry+fallback (primary -> fallback).
func (m *M7Extraction) agentWithFallback(ctx context.Context, primary, fallback string) (*agent.ExtractionAgent, error) {
	p, errP := m.deps.LLMFactory.CreateClient(ctx, primary)
	var fb llm.LLMClient
	if fallback != "" {
		if c, e := m.deps.LLMFactory.CreateClient(ctx, fallback); e == nil {
			fb = c
		}
	}
	if errP != nil {
		if fb != nil {
			return agent.NewExtractionAgent(llm.NewRetryingClient(nil, fb)), nil
		}
		return nil, errP
	}
	return agent.NewExtractionAgent(llm.NewRetryingClient(p, fb)), nil
}

// enrichDocTypes mengisi `document_type` (Journal Article / Conference Paper / dst.) dari
// CrossRef untuk paper INCLUDE yang belum punya — agar breakdown framework + Methods PRISMA
// (jurnal vs konferensi) akurat untuk jurnal Q1. Idempotent (skip yang sudah terisi),
// persisten, dan progres ter-log per-paper ke Live Log (konvensi operasi panjang).
func (m *M7Extraction) enrichDocTypes(ctx context.Context, sessionID string, included []map[string]interface{}) {
	need := 0
	for _, p := range included {
		if getStr(p, "document_type", "Document_Type", "DocumentType") == "" {
			need++
		}
	}
	if need == 0 {
		return
	}
	logger.Logf(sessionID, "   [DocType] Mengisi tipe dokumen via CrossRef untuk %d paper (Q1: breakdown jurnal/konferensi)...", need)
	coll := m.deps.MongoRepo.GetScreeningCollection()
	done := 0
	for _, p := range included {
		if getStr(p, "document_type", "Document_Type", "DocumentType") != "" {
			continue
		}
		done++
		doi := strings.TrimSpace(getStr(p, "DOI", "doi"))
		title := getStr(p, "Title", "title")
		var work *refs.CrossrefWork
		if isValidDOI(doi) {
			if w, e := refs.GetCrossrefWork(doi); e == nil {
				work = w
			}
		}
		if work == nil && title != "" {
			if resp, e := refs.SearchCrossrefWorks(title, 1, 0); e == nil && resp != nil && len(resp.Message.Items) > 0 {
				work = &resp.Message.Items[0]
			}
		}
		if work == nil || work.Type == "" {
			logger.Logf(sessionID, "   [DocType] %d/%d: %s → tak ada metadata CrossRef, lewati", done, need, truncTitle(title, 50))
			time.Sleep(1500 * time.Millisecond)
			continue
		}
		dt := formatStudyType(work.Type)
		p["document_type"] = dt // update in-memory agar breakdown segera akurat
		if oid, ok := p["_id"].(primitive.ObjectID); ok {
			_, _ = coll.UpdateByID(ctx, oid, bson.M{"$set": bson.M{"document_type": dt}})
		}
		logger.Logf(sessionID, "   [DocType] %d/%d: %s → %s", done, need, truncTitle(title, 50), dt)
		time.Sleep(1500 * time.Millisecond)
	}
	logger.Logf(sessionID, "   [DocType] Selesai mengisi tipe dokumen.")
}

func docTypeBreakdown(papers []map[string]interface{}) string {
	counts := map[string]int{}
	for _, p := range papers {
		dt := getStr(p, "Document_Type", "DocumentType", "document_type")
		if dt == "" {
			dt = "Unknown"
		}
		counts[dt]++
	}
	if len(counts) == 0 {
		return fmt.Sprintf("(belum tersedia; total %d paper)", len(papers))
	}
	s := fmt.Sprintf("Total %d paper. Document types: ", len(papers))
	for k, v := range counts {
		s += fmt.Sprintf("%s=%d; ", k, v)
	}
	return s
}

func countNotReported(fields []agent.ExtractedField) int {
	n := 0
	for _, f := range fields {
		if f.Status == "NOT_REPORTED" {
			n++
		}
	}
	return n
}

// ---- L5: Graph Extraction (Neo4j) ----
type ExtractedNode struct {
	ID    string                 `json:"id"`
	Label string                 `json:"label"`
	Props map[string]interface{} `json:"props"`
}

type ExtractedEdge struct {
	SourceID    string                 `json:"source_id"`
	SourceLabel string                 `json:"source_label"`
	TargetID    string                 `json:"target_id"`
	TargetLabel string                 `json:"target_label"`
	Type        string                 `json:"type"`
	Props       map[string]interface{} `json:"props"`
}

type GraphExtractionResponse struct {
	Nodes []ExtractedNode `json:"nodes"`
	Edges []ExtractedEdge `json:"edges"`
}

func (m *M7Extraction) runGraphExtractionL5(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 7.5] Ekstraksi Knowledge Graph (Neo4j) berjalan...")

	if m.deps.Neo4jRepo == nil {
		// Coba reconnect: baca ulang env vars dan coba koneksi sekali lagi
		logger.Log(session.ID, "   [Neo4j] Koneksi nil saat startup, mencoba reconnect...")
		neo4jURI := os.Getenv("NEO4JURI")
		neo4jUser := os.Getenv("NEO4JUSER")
		neo4jPass := os.Getenv("NEO4JPASSWORD")

		if neo4jURI == "" {
			errMsg := fmt.Sprintf("NEO4JURI env var kosong. Pastikan secret NEO4JURI sudah di-set di environment (Fly.io secrets / .env). Error startup sebelumnya: %s", m.deps.Neo4jConnErr)
			logger.Logf(session.ID, "   [ERROR] %s", errMsg)
			return fmt.Errorf("neo4j: %s", errMsg)
		}

		maskedURI := neo4jURI
		if len(maskedURI) > 10 {
			maskedURI = maskedURI[:10] + "..."
		}
		logger.Logf(session.ID, "   [Neo4j] Reconnect attempt: uri=%s, user=%q, pass_len=%d", maskedURI, neo4jUser, len(neo4jPass))

		repo, err := repository.NewNeo4jRepository(neo4jURI, neo4jUser, neo4jPass)
		if err != nil {
			errDetail := fmt.Sprintf("Neo4j reconnect gagal (uri=%s, user=%q): %v. Error startup: %s", maskedURI, neo4jUser, err, m.deps.Neo4jConnErr)
			logger.Logf(session.ID, "   [ERROR] %s", errDetail)
			return fmt.Errorf("neo4j: %s", errDetail)
		}

		// Reconnect berhasil!
		m.deps.Neo4jRepo = repo
		logger.Log(session.ID, "   [Neo4j] Reconnect BERHASIL! Melanjutkan ekstraksi graph...")
	}

	collExt := m.deps.MongoRepo.GetExtractionCollection()
	filter := bson.M{
		"session_id":      session.ID,
		"extracted":       true,
		"qa_rated":        true,
		"graph_extracted": bson.M{"$ne": true},
	}
	opts := options.Find().SetLimit(int64(extractionBatchSize))

	cursor, err := collExt.Find(ctx, filter, opts)
	if err != nil {
		return fmt.Errorf("find unextracted graph papers: %w", err)
	}
	var papers []map[string]interface{}
	if err := cursor.All(ctx, &papers); err != nil {
		return fmt.Errorf("cursor all: %w", err)
	}

	if len(papers) == 0 {
		logger.Log(session.ID, "   [Langkah 7.5] Seluruh ekstraksi Knowledge Graph selesai.")

		// Hitung summary: total papers yang sudah graph_extracted vs total eligible
		totalGraphed, _ := collExt.CountDocuments(ctx, bson.M{
			"session_id":      session.ID,
			"extracted":       true,
			"qa_rated":        true,
			"graph_extracted": true,
		})
		totalEligible, _ := collExt.CountDocuments(ctx, bson.M{
			"session_id": session.ID,
			"extracted":  true,
			"qa_rated":   true,
		})

		session.GraphExtractionSummary = &model.GraphExtractionSummary{
			TotalGraphed:   int(totalGraphed),
			TotalEligible:  int(totalEligible),
			Neo4jConnected: m.deps.Neo4jRepo != nil,
		}

		session.Status = "M7_STEP5_WAITING_APPROVAL"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}

	logger.Logf(session.ID, "   [Graph] Memproses batch %d dokumen untuk GraphRAG...\n", len(papers))

	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return m.deps.llmError(ctx, "brain", "Memuat client GraphRAG M7", err)
	}

	for _, p := range papers {
		objID := p["_id"]
		title, _ := p["Title"].(string)
		if title == "" {
			title, _ = p["title"].(string)
		}
		doi, _ := p["DOI"].(string)
		if doi == "" {
			doi, _ = p["doi"].(string)
		}

		// Ekstraksi menulis ke "fields" (bukan "m7_fields"); baca yang benar agar graph
		// tidak dibangun dari data KOSONG (null). Fallback ke m7_fields utk kompatibilitas.
		fieldData := p["fields"]
		if fieldData == nil {
			fieldData = p["m7_fields"]
		}
		fields, _ := json.Marshal(fieldData)

		sysPrompt := `Anda adalah ahli neuro-symbolic AI yang bertugas membangun Knowledge Graph dari literatur ilmiah.
Tugas Anda adalah membaca hasil ekstraksi sebuah paper, dan mengubahnya menjadi Nodes (simpul) dan Edges (relasi).
ATURAN NODES:
- Wajib sertakan minimal node Paper.
  Node Paper: Label "Paper", ID (DOI atau Title yang di-slug), Props minimal {title, doi}.
- Buat Nodes untuk Entitas penting: "Author", "Method", "Dataset", "Metric", "Conclusion".
- Gunakan ID yang sangat konsisten untuk entitas yang sama (contoh: id="dataset-adni", label="Dataset", props={name: "ADNI"}).

ATURAN EDGES:
- Hubungkan Paper dengan entitas lain.
- Tipe Relasi valid contohnya: WRITTEN_BY, USES_METHOD, USES_DATASET, EVALUATES_METRIC, CONCLUDES.
- Tiap edge butuh source_id, target_id, source_label, target_label, type, dan props.

Hanya keluarkan JSON utuh dengan format:
{
  "nodes": [{"id":"...", "label":"...", "props":{}}],
  "edges": [{"source_id":"...", "source_label":"...", "target_id":"...", "target_label":"...", "type":"...", "props":{}}]
}
Jangan ada tambahan teks markdown (tanpa ` + "```json" + ` ... ` + "```" + `) atau penjelasan.`

		userPrompt := fmt.Sprintf("Ekstrak graf dari paper berikut:\nJudul: %s\nDOI: %s\nHasil Ekstraksi:\n%s", title, doi, string(fields))

		respText, err := brain.Generate(ctx, sysPrompt, userPrompt)
		if err != nil {
			logger.Logf(session.ID, "      [!] Gagal memanggil LLM (role Brain, %s) untuk paper %s: %v\n", m.deps.roleLabel(ctx, "brain"), title, err)
			continue
		}

		// Bersihkan markdown markdown block jika ada
		respText = strings.TrimSpace(respText)
		if strings.HasPrefix(respText, "```json") {
			respText = strings.TrimPrefix(respText, "```json")
			respText = strings.TrimSuffix(respText, "```")
		}
		if strings.HasPrefix(respText, "```") {
			respText = strings.TrimPrefix(respText, "```")
			respText = strings.TrimSuffix(respText, "```")
		}

		var gResp GraphExtractionResponse
		if err := json.Unmarshal([]byte(respText), &gResp); err != nil {
			logger.Logf(session.ID, "      [!] Gagal mem-parsing JSON Graph untuk paper %s. Output LLM:\n%s\n", title, respText)
			continue
		}

		// Convert ke struktur Neo4jRepository
		var rNodes []repository.GraphNode
		var rEdges []repository.GraphEdge

		for _, n := range gResp.Nodes {
			if n.Props == nil {
				n.Props = make(map[string]interface{})
			}
			n.Props["id"] = n.ID
			rNodes = append(rNodes, repository.GraphNode{
				Label:      n.Label,
				Properties: n.Props,
			})
		}
		for _, e := range gResp.Edges {
			rEdges = append(rEdges, repository.GraphEdge{
				SourceNode: repository.GraphNode{Label: e.SourceLabel, Properties: map[string]interface{}{"id": e.SourceID}},
				TargetNode: repository.GraphNode{Label: e.TargetLabel, Properties: map[string]interface{}{"id": e.TargetID}},
				Type:       e.Type,
				Properties: e.Props,
			})
		}

		err = m.deps.Neo4jRepo.SaveKnowledgeGraph(ctx, rNodes, rEdges)
		if err != nil {
			logger.Logf(session.ID, "      [!] Gagal menyimpan Knowledge Graph Neo4j untuk paper %s: %v\n", title, err)
			continue
		}

		_, _ = collExt.UpdateByID(ctx, objID, bson.M{"$set": bson.M{"graph_extracted": true}})
		logger.Logf(session.ID, "      [+] Sukses menyimpan ke Neo4j: %s (%d nodes, %d edges)\n", title, len(rNodes), len(rEdges))
	}

	return nil
}
