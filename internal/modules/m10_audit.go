package modules

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nsa/internal/logger"
	"nsa/internal/model"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// M10Audit = Modul 10: Audit & Defensibility Gate (pra-submisi Q1).
// NEURO-SYMBOLIC: cek inti DETERMINISTIK dari state DB (bukan LLM). HITL: manusia
// meng-atestasi sebelum COMPLETED. Menutup celah Q1 (rekonsiliasi angka, coverage,
// ambang κ, GRADE, kelengkapan manuskrip, akurasi disclosure AI).
type M10Audit struct {
	deps *ModuleDeps
}

func NewM10Audit(deps *ModuleDeps) *M10Audit { return &M10Audit{deps: deps} }

func (m *M10Audit) Name() string { return "Modul 10: Audit & Defensibility Gate" }

func (m *M10Audit) Execute(ctx context.Context, session *model.SLRSession) error {
	switch session.Status {
	case "M10_STEP1_AUDIT":
		return m.runAudit(ctx, session)

	case "M10_STEP1_WAITING_APPROVAL":
		logger.Log(session.ID, "   [Modul 10] Menunggu ATESTASI peneliti atas laporan audit pra-submisi.")
		return nil

	case "M10_STEP1_NEEDS_REVISION":
		// Jalankan ulang audit (mis. setelah user memperbaiki manuskrip/koreksi).
		session.Feedback = ""
		session.Status = "M10_STEP1_AUDIT"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	case "M10_STEP1_APPROVED":
		// Rekam atestasi HITL lalu tutup pipeline.
		if session.AuditReport != nil {
			session.AuditReport.AttestedAt = time.Now().Format(time.RFC3339)
			if strings.TrimSpace(session.AuditReport.AttestedBy) == "" {
				session.AuditReport.AttestedBy = "peneliti"
			}
		}
		session.Status = "COMPLETED"
		logger.Log(session.ID, "   [Modul 10] Audit di-atestasi peneliti. Pipeline COMPLETED — manuskrip siap submit.")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	default:
		return nil
	}
}

// runAudit menjalankan seluruh cek simbolik, merangkai AuditReport, dan berhenti di gerbang.
func (m *M10Audit) runAudit(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Modul 10] Menjalankan audit pra-submisi (simbolik/deterministik dari DB)...")

	// --- Kumpulkan angka dari koleksi (deterministik) ---
	extColl := m.deps.MongoRepo.GetExtractionCollection()
	scrColl := m.deps.MongoRepo.GetScreeningCollection()
	sid := session.ID

	cnt := func(coll *mongo.Collection, filter bson.M) int {
		n, err := coll.CountDocuments(ctx, filter)
		if err != nil {
			return -1
		}
		return int(n)
	}
	screenedTotal := cnt(scrColl, bson.M{"session_id": sid})
	extractedCount := cnt(extColl, bson.M{"session_id": sid, "extracted": true})
	qaRatedCount := cnt(extColl, bson.M{"session_id": sid, "qa_rated": true})
	qaErrorCount := cnt(extColl, bson.M{"session_id": sid, "qa_final_category": "ERROR"})

	identified := 0
	if session.DataMiningLog != nil && session.DataMiningLog.QualityAudit != nil {
		identified = session.DataMiningLog.QualityAudit.TotalRecords
	}

	report := buildAuditReport(session, identified, screenedTotal, extractedCount, qaRatedCount, qaErrorCount)
	session.AuditReport = report
	session.Status = "M10_STEP1_WAITING_APPROVAL"
	logger.Logf(session.ID, "   [Modul 10] Audit selesai: %s (PASS=%d WARN=%d FAIL=%d).\n", report.Verdict, report.PassCount, report.WarnCount, report.FailCount)
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// buildAuditReport = MESIN AUDIT SIMBOLIK murni (tanpa I/O) — semua cek deterministik dari
// state sesi + angka yang sudah dihitung. Dipisah agar bisa di-unit-test tanpa DB.
func buildAuditReport(session *model.SLRSession, identified, screenedTotal, extractedCount, qaRatedCount, qaErrorCount int) *model.AuditReport {
	var checks []model.AuditCheck
	add := func(id, cat, name, status, detail, fix string) {
		checks = append(checks, model.AuditCheck{ID: id, Category: cat, Name: name, Status: status, Detail: detail, Fix: fix})
	}

	// ===== PRISMA =====
	// C1: rekonsiliasi angka identification→inclusion (deterministik).
	switch {
	case extractedCount <= 0:
		add("C1", "PRISMA", "Rekonsiliasi angka PRISMA", "FAIL",
			fmt.Sprintf("Studi included final (terekstrak) = %d. Tak ada studi untuk disintesis.", extractedCount),
			"Pastikan Modul 6 menghasilkan minimal 1 studi included full-text sebelum audit.")
	case identified > 0 && screenedTotal > 0 && !(identified >= screenedTotal && screenedTotal >= extractedCount):
		add("C1", "PRISMA", "Rekonsiliasi angka PRISMA", "WARN",
			fmt.Sprintf("Angka tidak monoton: identified=%d, screened=%d, included=%d.", identified, screenedTotal, extractedCount),
			"Periksa alur PRISMA — angka harus identified ≥ screened ≥ included.")
	default:
		add("C1", "PRISMA", "Rekonsiliasi angka PRISMA", "PASS",
			fmt.Sprintf("identified=%d ≥ screened=%d ≥ included=%d (konsisten).", identified, screenedTotal, extractedCount), "")
	}

	// C2: artefak PRISMA (flow + checklist) ada di manuskrip.
	ms := session.Manuscript
	if ms != nil && strings.TrimSpace(ms.PrismaFlow) != "" && strings.TrimSpace(ms.PrismaChecklist) != "" {
		add("C2", "PRISMA", "Artefak PRISMA 2020 (flow + checklist)", "PASS", "PRISMA flow & 27-item checklist tersedia.", "")
	} else {
		add("C2", "PRISMA", "Artefak PRISMA 2020 (flow + checklist)", "FAIL",
			"PRISMA flow dan/atau 27-item checklist belum ada di manuskrip.",
			"Selesaikan Modul 9 (Compile) — menghasilkan prisma_flow & prisma_checklist.")
	}

	// ===== Reliabilitas =====
	// C3: κ kalibrasi QA (dual-rater).
	if cal := session.QACalibration; cal != nil {
		if cal.CalibrationPassed || cal.PilotKappa >= 0.60 {
			add("C3", "Reliabilitas", "Inter-rater reliability QA (Cohen's κ)", "PASS",
				fmt.Sprintf("κ pilot = %.2f (memenuhi ambang ≥0.60 / kalibrasi lulus).", cal.PilotKappa), "")
		} else {
			add("C3", "Reliabilitas", "Inter-rater reliability QA (Cohen's κ)", "WARN",
				fmt.Sprintf("κ pilot = %.2f di bawah 0.60 & kalibrasi belum lulus.", cal.PilotKappa),
				"Jalankan ulang kalibrasi QA (tombol Jalankan Ulang QA) atau dokumentasikan justifikasi di Limitations.")
		}
	} else {
		add("C3", "Reliabilitas", "Inter-rater reliability QA (Cohen's κ)", "WARN",
			"Kalibrasi QA (κ) tidak tercatat.", "Pastikan Modul 7 L3 (QA dual-rater + kalibrasi) dijalankan.")
	}

	// C4: reliabilitas ekstraksi.
	if el := session.ExtractionLog; el != nil && el.ExtractionKappa > 0 {
		st := "PASS"
		fix := ""
		if el.ExtractionKappa < 0.60 {
			st, fix = "WARN", "κ ekstraksi <0.60 — laporkan di Limitations atau tingkatkan verifikasi Reviewer 2."
		}
		add("C4", "Reliabilitas", "Reliabilitas ekstraksi data (κ)", st,
			fmt.Sprintf("κ ekstraksi = %.2f (verified sample %d).", el.ExtractionKappa, el.VerifiedSample), fix)
	} else {
		add("C4", "Reliabilitas", "Reliabilitas ekstraksi data (κ)", "WARN",
			"κ ekstraksi tidak tercatat (verifikasi Reviewer 2 mungkin dilewati).",
			"Jalankan verifikasi ekstraksi (Modul 7 L2) agar reliabilitas terdokumentasi.")
	}

	// ===== Kelengkapan (coverage) =====
	// C5: tiap studi included → diekstrak → di-appraisal (tak ada gap).
	switch {
	case qaErrorCount > 0:
		add("C5", "Kelengkapan", "Coverage QA (tiap studi included dinilai)", "FAIL",
			fmt.Sprintf("%d studi berstatus qa_final_category=ERROR (belum dinilai lengkap R1+R2).", qaErrorCount),
			"Buka Modul 7 L3 → '▶️ Lanjutkan QA' / rate ulang studi ERROR sampai nol.")
	case extractedCount > 0 && qaRatedCount < extractedCount:
		add("C5", "Kelengkapan", "Coverage QA (tiap studi included dinilai)", "WARN",
			fmt.Sprintf("Baru %d dari %d studi terekstrak yang selesai dinilai QA.", qaRatedCount, extractedCount),
			"Selesaikan penilaian QA untuk semua studi included sebelum submit.")
	default:
		add("C5", "Kelengkapan", "Coverage QA (tiap studi included dinilai)", "PASS",
			fmt.Sprintf("%d studi included, %d dinilai QA, 0 ERROR.", extractedCount, qaRatedCount), "")
	}

	// C6: kegagalan ekstraksi.
	if el := session.ExtractionLog; el != nil && el.FailedCount > 0 {
		add("C6", "Kelengkapan", "Kegagalan ekstraksi (ERROR/EMPTY/NO-FULLTEXT)", "WARN",
			fmt.Sprintf("%d studi gagal/kosong saat ekstraksi.", el.FailedCount),
			"Ekstrak ulang studi gagal (Modul 7) atau dokumentasikan sebagai missing-data di Limitations (PRISMA 24c).")
	} else {
		add("C6", "Kelengkapan", "Kegagalan ekstraksi (ERROR/EMPTY/NO-FULLTEXT)", "PASS", "Tak ada kegagalan ekstraksi tercatat.", "")
	}

	// C7: GRADE / certainty of evidence.
	if g := session.GradeEvidence; g != nil && strings.TrimSpace(g.TableMarkdown) != "" {
		add("C7", "Kelengkapan", "GRADE / certainty of evidence", "PASS",
			fmt.Sprintf("Tabel GRADE ada; robustness: %s.", strings.TrimSpace(g.RobustnessVerdict)), "")
	} else {
		add("C7", "Kelengkapan", "GRADE / certainty of evidence", "WARN",
			"Tabel GRADE belum ada.", "Jalankan Modul 8 L3 (GRADE + robustness) untuk menilai certainty.")
	}

	// C8: sintesis.
	if s := session.SynthesisResults; s != nil && strings.TrimSpace(s.Markdown) != "" {
		add("C8", "Kelengkapan", "Sintesis bukti", "PASS", fmt.Sprintf("Sintesis (%s) tersedia.", strings.TrimSpace(s.Path)), "")
	} else {
		add("C8", "Kelengkapan", "Sintesis bukti", "WARN", "Hasil sintesis belum ada.", "Selesaikan Modul 8 (synthesis).")
	}

	// ===== Pelaporan =====
	// C9: kelengkapan section manuskrip.
	if ms != nil {
		var empty []string
		req := map[string]string{"Title": ms.Title, "Abstract": ms.Abstract, "Introduction": ms.Introduction,
			"Methods": ms.Methods, "Results": ms.Results, "Discussion": ms.Discussion, "Conclusions": ms.Conclusions}
		for name, v := range req {
			if strings.TrimSpace(v) == "" {
				empty = append(empty, name)
			}
		}
		if len(empty) == 0 {
			add("C9", "Pelaporan", "Kelengkapan section manuskrip (IMRaD)", "PASS", "Semua section inti terisi.", "")
		} else {
			add("C9", "Pelaporan", "Kelengkapan section manuskrip (IMRaD)", "FAIL",
				"Section kosong: "+strings.Join(empty, ", ")+".",
				"Selesaikan Modul 9 (Group A/B) untuk mengisi section tersebut.")
		}
	} else {
		add("C9", "Pelaporan", "Kelengkapan section manuskrip (IMRaD)", "FAIL", "Manuskrip belum ada.", "Selesaikan Modul 9.")
	}

	// C10: RQ ada & terjawab (heuristik simbolik: jumlah RQ > 0).
	if n := len(session.ResearchQuestions); n > 0 {
		add("C10", "Pelaporan", "Research Questions terdefinisi", "PASS", fmt.Sprintf("%d RQ terdefinisi.", n), "")
	} else {
		add("C10", "Pelaporan", "Research Questions terdefinisi", "WARN", "Tidak ada RQ tersimpan.", "Definisikan RQ di Modul 2.")
	}

	// ===== Integritas (yang paling penting untuk Q1 berbasis-AI) =====
	// C11: AKURASI disclosure AI. Sistem ini MEMAKAI AI untuk skrining/ekstraksi/appraisal/
	// sintesis (via HITL + κ). Disclosure WAJIB menyatakan ini secara akurat — bukan
	// "AI tidak dipakai untuk analisis". Ini butuh atestasi manusia (WARN by design).
	add("C11", "Integritas", "Akurasi disclosure penggunaan AI (COPE/Elsevier)", "WARN",
		"Sistem memakai AI sebagai alat bantu keputusan untuk skrining, ekstraksi, appraisal, dan sintesis (dengan verifikasi manusia HITL + Cohen's κ). Disclosure harus menyatakan ini secara AKURAT.",
		"Pastikan 'AI Assistance Declaration' di manuskrip menyebut peran AI sebagai decision-support ber-HITL + κ (bukan 'AI tidak dipakai untuk analisis'). Jejak xAI tiap keputusan tersimpan & dapat diekspor sebagai supplementary.")

	// --- Rangkum verdict ---
	report := &model.AuditReport{Checks: checks, GeneratedAt: time.Now().Format(time.RFC3339)}
	for _, c := range checks {
		switch c.Status {
		case "PASS":
			report.PassCount++
		case "WARN":
			report.WarnCount++
		case "FAIL":
			report.FailCount++
		}
	}
	switch {
	case report.FailCount > 0:
		report.Verdict = "NOT_READY"
		report.Summary = fmt.Sprintf("%d BLOKER (FAIL), %d peringatan, %d lolos. Perbaiki bloker sebelum submit Q1.", report.FailCount, report.WarnCount, report.PassCount)
	case report.WarnCount > 0:
		report.Verdict = "READY_WITH_WARNINGS"
		report.Summary = fmt.Sprintf("Tak ada bloker. %d peringatan (tinjau + dokumentasikan), %d lolos. Boleh submit setelah atestasi.", report.WarnCount, report.PassCount)
	default:
		report.Verdict = "READY"
		report.Summary = fmt.Sprintf("Semua %d cek lolos. Manuskrip layak submit Q1 setelah atestasi peneliti.", report.PassCount)
	}
	return report
}
