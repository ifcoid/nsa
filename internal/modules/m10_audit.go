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

	// C12: triangulasi verifikasi klaim (neuro-symbolic) — apakah 3 sumber terpakai?
	if ms != nil && len(ms.ClaimVerifications) > 0 {
		total := len(ms.ClaimVerifications)
		ver, neo := 0, 0
		for _, c := range ms.ClaimVerifications {
			if c.Sources >= 2 {
				ver++
			}
			if c.Neo4jVerified {
				neo++
			}
		}
		pct := 0
		if total > 0 {
			pct = ver * 100 / total
		}
		switch {
		case neo == 0:
			add("C12", "Integritas", "Triangulasi verifikasi klaim (3-sumber)", "WARN",
				fmt.Sprintf("%d/%d klaim terverifikasi ≥2 sumber (%d%%), TAPI Neo4j tak berkontribusi (0) — triangulasi berjalan 2-sumber (Qdrant+MongoDB).", ver, total, pct),
				"Aktifkan Neo4j/AuraDB (set NEO4JURI/USER/PASSWORD, restart) lalu jalankan ulang M9 untuk triangulasi 3-sumber penuh — memperkuat defensibilitas Q1.")
		case pct < 60:
			add("C12", "Integritas", "Triangulasi verifikasi klaim (3-sumber)", "WARN",
				fmt.Sprintf("Hanya %d/%d klaim (%d%%) terverifikasi ≥2 sumber (Neo4j aktif, kontribusi %d).", ver, total, pct, neo),
				"Tinjau klaim berdukungan <2 sumber (lihat gerbang M9 / Paket Reproducibility); kuatkan atau lemahkan sesuai bukti.")
		default:
			add("C12", "Integritas", "Triangulasi verifikasi klaim (3-sumber)", "PASS",
				fmt.Sprintf("%d/%d klaim (%d%%) terverifikasi ≥2 sumber; Neo4j aktif (kontribusi %d). Triangulasi 3-sumber penuh.", ver, total, pct, neo), "")
		}
	}

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

	// Rakit artefak submisi DETERMINISTIK dari state sesi (tanpa LLM → reproducible).
	report.ProtocolMarkdown = buildProtocolDoc(session)
	report.ReproPackageMarkdown = buildReproPackage(session, identified, screenedTotal, extractedCount, qaRatedCount)
	return report
}

// --- Perakit artefak submisi (deterministik) ---------------------------------

func mdEsc(s string) string { return strings.TrimSpace(s) }

// buildProtocolDoc merakit protokol a-priori gaya PROSPERO yang dapat didaftarkan,
// murni dari data sesi (bisa diedit user via modul terkait → multi-tenant).
func buildProtocolDoc(s *model.SLRSession) string {
	var b strings.Builder
	title := mdEsc(s.Topic)
	if s.SelectedTopic != nil && strings.TrimSpace(s.SelectedTopic.Name) != "" {
		title = mdEsc(s.SelectedTopic.Name)
	}
	fmt.Fprintf(&b, "# Protokol Systematic Literature Review (a-priori, gaya PROSPERO)\n\n")
	fmt.Fprintf(&b, "> Dokumen ini dirakit otomatis dari protokol yang ditetapkan di awal sesi. Cocok untuk pendaftaran (mis. PROSPERO/OSF) sebelum atau saat submisi.\n\n")
	fmt.Fprintf(&b, "## 1. Judul review\n%s\n\n", title)

	fmt.Fprintf(&b, "## 2. Pertanyaan & tujuan review\n")
	if len(s.ResearchQuestions) == 0 {
		b.WriteString("_(belum ada RQ tersimpan)_\n")
	}
	for i, rq := range s.ResearchQuestions {
		fmt.Fprintf(&b, "%d. **[%s]** %s\n", i+1, strings.TrimSpace(rq.Type), mdEsc(rq.Question))
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## 3. Kriteria kelayakan (PICO)\n")
	if p := s.PICODefinitions; p != nil {
		fmt.Fprintf(&b, "- **Population/Problem (P):** %s\n", mdEsc(p.P.Value))
		fmt.Fprintf(&b, "- **Intervention (I):** %s\n", mdEsc(p.I.Value))
		fmt.Fprintf(&b, "- **Comparison (C):** %s\n", mdEsc(p.C.Value))
		fmt.Fprintf(&b, "- **Outcome (O):** %s\n", mdEsc(p.O.Value))
	} else {
		b.WriteString("_(PICO belum ditetapkan)_\n")
	}
	if len(s.InclusionCriteria) > 0 {
		b.WriteString("\n**Kriteria inklusi:**\n")
		for _, c := range s.InclusionCriteria {
			fmt.Fprintf(&b, "- %s\n", mdEsc(c))
		}
	}
	if len(s.ExclusionCriteria) > 0 {
		b.WriteString("\n**Kriteria eksklusi:**\n")
		for _, c := range s.ExclusionCriteria {
			fmt.Fprintf(&b, "- %s\n", mdEsc(c))
		}
	}
	if sf := s.ScopeFilters; sf != nil {
		b.WriteString("\n**Batasan lingkup:** ")
		fmt.Fprintf(&b, "Tahun: %s · Geografis: %s · Sektor: %s · Bahasa: %s%s\n",
			def(sf.RentangTahun), def(sf.Geografis), def(sf.Sektor), def(sf.Bahasa),
			ifStr(strings.TrimSpace(sf.Lainnya) != "", " · Lainnya: "+mdEsc(sf.Lainnya), ""))
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## 4. Sumber informasi & strategi pencarian\n")
	if db := s.DatabaseSelection; db != nil {
		fmt.Fprintf(&b, "**Database:** %s\n\n", def(db.Decision))
		if strings.TrimSpace(db.JustifikasiFinal) != "" {
			fmt.Fprintf(&b, "_Justifikasi:_ %s\n\n", mdEsc(db.JustifikasiFinal))
		}
	}
	if ss := s.SearchString; ss != nil {
		if strings.TrimSpace(ss.ScopusQuery) != "" {
			fmt.Fprintf(&b, "**Kueri (Scopus, kanonik):**\n```\n%s\n```\n", strings.TrimSpace(ss.ScopusQuery))
		}
		for _, a := range ss.AdaptedStrings {
			_ = a // adapted strings ada di paket reproducibility (lengkap per-DB)
		}
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## 5. Ekstraksi data (form)\n")
	if fw := s.FrameworkSelection; fw != nil && len(fw.Columns) > 0 {
		fmt.Fprintf(&b, "Framework: **%s**. Item data yang diekstrak:\n\n", def(fw.Framework))
		b.WriteString("| Kolom | Kategori | Deskripsi |\n|---|---|---|\n")
		for _, c := range fw.Columns {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", mdEsc(c.Key), mdEsc(c.Category), mdEsc(c.Desc))
		}
	} else {
		b.WriteString("_(form ekstraksi belum ditetapkan)_\n")
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## 6. Penilaian risiko bias / kualitas\n")
	if qa := s.QAThreshold; qa != nil {
		fmt.Fprintf(&b, "**Tool:** %s · **Ambang:** %.0f%% · **Kategorisasi:** %s\n\n", def(qa.Tool), qa.Threshold, def(qa.Categorization))
		if strings.TrimSpace(qa.ToolJustification) != "" {
			fmt.Fprintf(&b, "_Justifikasi tool:_ %s\n", mdEsc(qa.ToolJustification))
		}
	} else {
		b.WriteString("_(tool QA belum ditetapkan)_\n")
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## 7. Strategi sintesis data\n")
	if sp := s.SynthesisPathDecision; sp != nil {
		fmt.Fprintf(&b, "Jalur sintesis: **%s**.\n", def(synthPathLabel(sp)))
	} else {
		b.WriteString("Narrative synthesis (default) kecuali homogenitas mendukung meta-analysis.\n")
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## 8. Amandemen protokol\n")
	if n := len(s.ScreeningCorrections); n > 0 {
		fmt.Fprintf(&b, "%d koreksi keputusan include/exclude pasca-screening tercatat (lihat Paket Reproducibility untuk alasan tiap koreksi).\n", n)
	} else {
		b.WriteString("Tidak ada amandemen protokol tercatat; protokol diterapkan seragam ke semua studi.\n")
	}
	return b.String()
}

// buildReproPackage merakit supplementary reproducibility (PRISMA-S + κ + provenance).
func buildReproPackage(s *model.SLRSession, identified, screened, included, qaRated int) string {
	var b strings.Builder
	b.WriteString("# Paket Reproducibility (Supplementary)\n\n")
	b.WriteString("> Dirakit deterministik dari state sesi. Lampirkan sebagai supplementary material untuk memenuhi standar reproducibility Q1 (PRISMA 2020 + PRISMA-S).\n\n")

	fmt.Fprintf(&b, "## A. Ringkasan angka (PRISMA)\n")
	fmt.Fprintf(&b, "| Tahap | n |\n|---|---|\n| Records identified | %d |\n| Records screened | %d |\n| Studies included (final) | %d |\n| Studies appraised (QA) | %d |\n\n", identified, screened, included, qaRated)

	fmt.Fprintf(&b, "## B. Strategi pencarian lengkap (PRISMA-S)\n")
	if ss := s.SearchString; ss != nil {
		if strings.TrimSpace(ss.ScopusQuery) != "" {
			fmt.Fprintf(&b, "**Scopus (kanonik):**\n```\n%s\n```\n", strings.TrimSpace(ss.ScopusQuery))
		}
		for _, a := range ss.AdaptedStrings {
			fmt.Fprintf(&b, "\n**%s:**\n```\n%s\n```\n", mdEsc(a.Database), strings.TrimSpace(a.Query))
		}
		if len(ss.Filters) > 0 {
			b.WriteString("\n**Filter diterapkan:** ")
			var fs []string
			for _, f := range ss.Filters {
				fs = append(fs, mdEsc(f.Filter)+"="+mdEsc(f.Value))
			}
			b.WriteString(strings.Join(fs, "; ") + "\n")
		}
	} else if sl := s.SearchLog; sl != nil {
		fmt.Fprintf(&b, "```\n%s\n```\nDatabases: %s\n", strings.TrimSpace(sl.SearchStringFinal), strings.Join(sl.Databases, ", "))
	} else {
		b.WriteString("_(strategi pencarian tak tersedia)_\n")
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## C. Reliabilitas antar-penilai (Cohen's κ)\n")
	if cal := s.QACalibration; cal != nil {
		fmt.Fprintf(&b, "- Kalibrasi QA (dual-rater): κ = **%.2f** (%s)\n", cal.PilotKappa, ifStr(cal.CalibrationPassed, "lulus", "belum lulus"))
	}
	if el := s.ExtractionLog; el != nil && el.ExtractionKappa > 0 {
		fmt.Fprintf(&b, "- Verifikasi ekstraksi: κ = **%.2f** (sampel %d)\n", el.ExtractionKappa, el.VerifiedSample)
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## D. Form ekstraksi & rubrik QA\n")
	if fw := s.FrameworkSelection; fw != nil && len(fw.Columns) > 0 {
		fmt.Fprintf(&b, "Framework **%s** — %d item data (lihat Protokol §5).\n", def(fw.Framework), len(fw.Columns))
	}
	if qa := s.QAThreshold; qa != nil && strings.TrimSpace(qa.QARubric) != "" {
		fmt.Fprintf(&b, "\n**Rubrik operasional QA (%s, ambang %.0f%%):**\n\n%s\n", def(qa.Tool), qa.Threshold, mdEsc(qa.QARubric))
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## E. Certainty of evidence (GRADE)\n")
	if g := s.GradeEvidence; g != nil && strings.TrimSpace(g.TableMarkdown) != "" {
		fmt.Fprintf(&b, "%s\n\n_Robustness:_ %s. %s\n", strings.TrimSpace(g.TableMarkdown), def(g.RobustnessVerdict), mdEsc(g.RobustnessSummary))
	} else {
		b.WriteString("_(tabel GRADE tak tersedia)_\n")
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## E2. Sintesis (naratif / meta-analisis)\n")
	if sr := s.SynthesisResults; sr != nil && strings.TrimSpace(sr.Markdown) != "" {
		fmt.Fprintf(&b, "_Jalur: %s._\n\n%s\n", def(sr.Path), strings.TrimSpace(sr.Markdown))
		if strings.TrimSpace(sr.ForestPlotScript) != "" {
			fmt.Fprintf(&b, "\n**Skrip forest plot (reproducible):**\n```\n%s\n```\n", strings.TrimSpace(sr.ForestPlotScript))
		}
	} else {
		b.WriteString("_(sintesis tak tersedia)_\n")
	}
	b.WriteString("\n")

	// SLNA/bibliometric = metode reproducibility-kritis (parameter VOSviewer + thesaurus).
	if s.VOSViewerParams != nil || s.BibliometricData != nil || s.SLNAIntegration != nil {
		fmt.Fprintf(&b, "## E3. Bibliometrik / SLNA (parameter & metode)\n")
		if bd := s.BibliometricData; bd != nil {
			fmt.Fprintf(&b, "- Records dianalisis: %d · Pendekatan: %s\n", bd.RecordsAnalyzed, def(bd.Approach))
		}
		if vp := s.VOSViewerParams; vp != nil && strings.TrimSpace(vp.TableMarkdown) != "" {
			fmt.Fprintf(&b, "\n**Parameter VOSviewer (9-parameter, siap-Methods):**\n\n%s\n", strings.TrimSpace(vp.TableMarkdown))
		}
		if si := s.SLNAIntegration; si != nil && strings.TrimSpace(si.Markdown) != "" {
			fmt.Fprintf(&b, "\n**Integrasi SLNA:**\n\n%s\n", strings.TrimSpace(si.Markdown))
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "## F. PRISMA 2020 (flow + checklist)\n")
	if ms := s.Manuscript; ms != nil {
		if strings.TrimSpace(ms.PrismaFlow) != "" {
			fmt.Fprintf(&b, "**Flow:**\n```\n%s\n```\n", strings.TrimSpace(ms.PrismaFlow))
		}
		if strings.TrimSpace(ms.PrismaChecklist) != "" {
			fmt.Fprintf(&b, "\n**Checklist 27-item:**\n\n%s\n", strings.TrimSpace(ms.PrismaChecklist))
		}
		if strings.TrimSpace(ms.CoherenceAudit) != "" {
			fmt.Fprintf(&b, "\n**Audit koherensi manuskrip:**\n\n%s\n", strings.TrimSpace(ms.CoherenceAudit))
		}
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## G. Jejak keputusan (provenance)\n")
	if n := len(s.ScreeningCorrections); n > 0 {
		b.WriteString("Koreksi keputusan include/exclude pasca-screening (dengan alasan):\n\n")
		b.WriteString("| Paper | Dari | Ke | Alasan |\n|---|---|---|---|\n")
		for _, c := range s.ScreeningCorrections {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", mdEsc(firstNonEmpty(c.Title, c.DOI, c.PaperID)), mdEsc(c.From), mdEsc(c.To), mdEsc(c.Reason))
		}
	} else {
		b.WriteString("Tidak ada koreksi keputusan pasca-screening (protokol diterapkan seragam).\n")
	}
	// Bukti triangulasi klaim (neuro-symbolic) — xAI: mana yang lolos ≥2 sumber.
	if ms := s.Manuscript; ms != nil && len(ms.ClaimVerifications) > 0 {
		total := len(ms.ClaimVerifications)
		ver, q, ne, mo := 0, 0, 0, 0
		var weak []model.ClaimVerification
		for _, c := range ms.ClaimVerifications {
			if c.Sources >= 2 {
				ver++
			} else {
				weak = append(weak, c)
			}
			if c.QdrantVerified {
				q++
			}
			if c.Neo4jVerified {
				ne++
			}
			if c.MongoVerified {
				mo++
			}
		}
		fmt.Fprintf(&b, "\n**Verifikasi klaim (triangulasi neuro-symbolic):** %d/%d klaim terverifikasi ≥2 sumber (Qdrant=%d, Neo4j=%d, MongoDB=%d).",
			ver, total, q, ne, mo)
		if ms.ModelUsed != "" {
			fmt.Fprintf(&b, " Model penulis: %s.", mdEsc(ms.ModelUsed))
		}
		b.WriteString("\n")
		if len(weak) > 0 {
			fmt.Fprintf(&b, "\nKlaim dengan dukungan <2 sumber (%d) — ditinjau/dilemahkan saat penulisan (P2):\n\n", len(weak))
			b.WriteString("| Section | Klaim | Sumber cocok |\n|---|---|---|\n")
			for i, c := range weak {
				if i >= 25 {
					fmt.Fprintf(&b, "| … | _(%d klaim lain)_ | |\n", len(weak)-25)
					break
				}
				var src []string
				if c.QdrantVerified {
					src = append(src, "Qdrant")
				}
				if c.Neo4jVerified {
					src = append(src, "Neo4j")
				}
				if c.MongoVerified {
					src = append(src, "MongoDB")
				}
				fmt.Fprintf(&b, "| %s | %s | %s |\n", mdEsc(c.Section), mdEsc(clip(c.Claim, 140)), def(strings.Join(src, "+")))
			}
		}
	}
	b.WriteString("\nJejak lengkap tiap panggilan AI (model, prompt, keluaran) tersimpan pada log xAI sesi dan dapat diekspor.\n\n")

	fmt.Fprintf(&b, "## H. Pernyataan penggunaan AI\n")
	b.WriteString("AI (LLM) dipakai sebagai alat bantu keputusan (decision-support) pada skrining, ekstraksi, penilaian kualitas/risiko bias, dan sintesis — SELALU dengan verifikasi manusia (HITL) di tiap gerbang; dua penilai AI independen dengan κ terukur; aturan simbolik dipadukan dengan penilaian neural (neuro-symbolic); provenance tiap panggilan tersimpan. AI bukan hakim tunggal; keputusan akhir tanggung jawab penulis.\n")
	b.WriteString("\n## I. Ketersediaan data & arsip (Data Availability)\n")
	b.WriteString("Untuk reproducibility permanen, deposit paket ini + protokol + laporan + manuskrip ke **Zenodo** (dapat DOI, versioned) dan daftarkan protokol a-priori di **PROSPERO/OSF**. Sitasi DOI Zenodo pada *Data Availability Statement* manuskrip. Rantai: protokol a-priori → data & keputusan (DB) → paket ini → arsip ber-DOI (Zenodo) → disitasi di artikel. Langkah lengkap ada di dokumen Panduan Handoff (Ruang Ekspor).\n")
	return b.String()
}

// helper kecil
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func def(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return strings.TrimSpace(s)
}
func ifStr(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "-"
}
func synthPathLabel(sp *model.SynthesisPathDecision) string {
	if sp == nil {
		return ""
	}
	return sp.Verdict
}
