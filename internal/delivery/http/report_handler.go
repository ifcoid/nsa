package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"nsa/internal/latex"
	"nsa/internal/model"
)

// GenerateReport merakit SATU laporan SLR Markdown UTUH dari MongoDB (lihat GENERATEREPORT.md):
// naratif per-modul + PRISMA (dihitung ulang dari slr_screening) + ekstraksi per-paper +
// lampiran transparansi (atribusi model, audit koreksi, verifikasi). Read-only; mengembalikan
// file .md untuk diunduh. TIDAK menulis ke DB.
func (h *SessionHandler) GenerateReport(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}
	ctx := context.Background()
	s, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}
	md := h.buildReportMarkdown(ctx, s)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="laporan_slr.md"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(md))
}

// buildReportMarkdown merakit laporan SLR Markdown UTUH dari MongoDB (dipakai versi .md & .tex).
func (h *SessionHandler) buildReportMarkdown(ctx context.Context, s *model.SLRSession) string {
	var b strings.Builder
	wln := func(format string, a ...interface{}) { b.WriteString(fmt.Sprintf(format, a...)); b.WriteString("\n") }
	sec := func(title, md string) { // tampilkan hanya bila ada isi
		md = strings.TrimSpace(md)
		if md == "" {
			return
		}
		wln("\n## %s\n", title)
		wln("%s", md)
	}

	// ── Cover ──────────────────────────────────────────────────────────────────
	wln("# Laporan SLR — %s", strSafe(s.Topic, "(tanpa topik)"))
	wln("\n> Dihasilkan dari MongoDB (`slr_sessions`/`slr_screening`/`slr_extraction`). "+
		"Status sesi: **%s** · Bahasa manuskrip: %s · Sesi: `%s`", s.Status, strSafe(s.ManuscriptLang, "id"), s.ID)
	wln("> Semua angka pada PRISMA & ringkasan ekstraksi dihitung ulang dari keputusan FINAL di "+
		"database (ground-truth), transparan & dapat direplikasi.")

	// ── 1. Latar & teori ───────────────────────────────────────────────────────
	if s.Foundation != nil {
		sec("1. Latar Belakang & Dasar Teori", s.Foundation.TheoryMarkdown)
	}
	if s.SelectedTopic != nil {
		t := s.SelectedTopic
		sec("Topik & Gap Terpilih", fmt.Sprintf("**%s**\n\n- **Gap:** %s\n- **Tipe:** %s (%s)\n- **Bukti:** %s\n- **Urgensi:** %s",
			t.Name, t.Gap, t.Type, t.TypeReason, t.Evidence, t.Importance))
	}

	// ── 2. Prior reviews (novelty) ─────────────────────────────────────────────
	if s.PriorReviewsMatrix != nil && len(s.PriorReviewsMatrix.Reviews) > 0 {
		var t strings.Builder
		t.WriteString("| Author (Tahun) | Scope | Metodologi | Selisih | Verifikasi |\n|---|---|---|---|---|\n")
		for _, r := range s.PriorReviewsMatrix.Reviews {
			v := r.Verification
			if v == "" {
				v = "UNVERIFIED"
			}
			t.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
				mdCell(r.AuthorYear), mdCell(r.Scope), mdCell(r.Methodology), mdCell(r.Selisih), v))
		}
		if g := strings.TrimSpace(s.PriorReviewsMatrix.SearchGuidance); g != "" {
			t.WriteString("\n*Panduan pencarian verifikasi:* " + g + "\n")
		}
		sec("2. Review Terdahulu & Posisi Novelty", t.String())
	}

	// ── 3. Protokol: PICO, RQ, kriteria ────────────────────────────────────────
	if s.PICODefinitions != nil {
		sec("3. Protokol — Definisi PICO", "```json\n"+jsonPretty(s.PICODefinitions)+"\n```")
	}
	if len(s.ResearchQuestions) > 0 {
		sec("Pertanyaan Penelitian (RQ)", "```json\n"+jsonPretty(s.ResearchQuestions)+"\n```")
	}
	if len(s.InclusionCriteria) > 0 || len(s.ExclusionCriteria) > 0 {
		var t strings.Builder
		t.WriteString("**Inklusi:**\n")
		for _, c := range s.InclusionCriteria {
			t.WriteString("- " + c + "\n")
		}
		t.WriteString("\n**Eksklusi:**\n")
		for _, c := range s.ExclusionCriteria {
			t.WriteString("- " + c + "\n")
		}
		sec("Kriteria Inklusi/Eksklusi", t.String())
	}
	if strings.TrimSpace(s.AuditScopeRules) != "" {
		sec("Aturan Scope/Batas (HITL, audit konsistensi)", s.AuditScopeRules)
	}

	// ── 4. Ringkasan naratif per-modul ─────────────────────────────────────────
	if s.Modul2Summary != nil {
		sec("Modul 2 — Ringkasan (PICO/RQ/Scope)", s.Modul2Summary.Markdown)
	}
	if s.Modul3Summary != nil {
		sec("Modul 3 — Strategi Pencarian", s.Modul3Summary.Markdown)
	}
	if s.Modul4Summary != nil {
		sec("Modul 4 — Identifikasi & Dedup", s.Modul4Summary.Markdown)
	}
	if s.Modul5Summary != nil {
		sec("Modul 5 — Screening Judul/Abstrak", s.Modul5Summary.Markdown)
	}
	if s.Modul6Summary != nil {
		sec("Modul 6 — Akuisisi & Screening Full-Text", s.Modul6Summary.Markdown)
	}
	if s.FulltextKappa > 0 {
		sec("Reliabilitas Antar-Penilai (Full-text)", fmt.Sprintf("Cohen's kappa (full-text): **%.3f**", s.FulltextKappa))
	}
	if s.Modul7Summary != nil {
		sec("Modul 7 — Ekstraksi & QA (ringkasan)", s.Modul7Summary.Markdown)
	}
	if s.Modul8Summary != nil {
		sec("Modul 8 — Analisis & Sintesis (ringkasan)", s.Modul8Summary.Markdown)
	}

	// ── 5. PRISMA flow ─────────────────────────────────────────────────────────
	prisma := h.reportPrisma(ctx, s)
	sec("5. PRISMA Flow (dihitung ulang dari database)", prisma)
	if s.Manuscript != nil && strings.TrimSpace(s.Manuscript.PrismaFlow) != "" {
		sec("PRISMA Flow (versi tervalidasi manuskrip)", "```\n"+strings.TrimSpace(s.Manuscript.PrismaFlow)+"\n```")
	}

	// ── 6. Ekstraksi data ──────────────────────────────────────────────────────
	if s.FrameworkSelection != nil {
		fw := s.FrameworkSelection
		var t strings.Builder
		t.WriteString(fmt.Sprintf("**Framework:** %s  ·  **Model:** %s\n\n", fw.Framework, strSafe(fw.ModelUsed, "-")))
		t.WriteString("| Kolom (key) | Kategori | Deskripsi |\n|---|---|---|\n")
		for _, c := range fw.Columns {
			t.WriteString(fmt.Sprintf("| %s | %s | %s |\n", mdCell(c.Key), mdCell(c.Category), mdCell(c.Desc)))
		}
		sec("6. Protokol Ekstraksi (framework, stabil)", t.String())
	}
	if s.ExtractionLog != nil {
		el := s.ExtractionLog
		verifyNote := ""
		if el.VerifiedSample == 0 && el.TotalExtracted > 0 {
			verifyNote = "  ⚠ **Verifikasi tidak berjalan (0 berhasil) — disagreement bukan hasil valid.**"
		}
		sec("Ringkasan Ekstraksi & Verifikasi (QA)", fmt.Sprintf(
			"- Total diekstrak: **%d**\n- Diverifikasi (Reviewer 2): **%d**%s\n- Disagreement: **%.1f%%**\n"+
				"- Field ambigu: %d\n- Gagal/kosong: %d\n- Model ekstraksi: %s\n- Model verifikasi: %s\n- Catatan: %s",
			el.TotalExtracted, el.VerifiedSample, verifyNote, el.DisagreementRate, el.AmbiguousCount,
			el.FailedCount, strSafe(el.ModelExtraction, "-"), strSafe(el.ModelRefineProtocol, "-"), strSafe(el.NRNote, "-")))
	}
	sec("Tabel Ekstraksi per-Paper", h.reportExtractionTable(ctx, s))

	// ── 7. Sintesis & GRADE ────────────────────────────────────────────────────
	if s.SynthesisPathDecision != nil {
		sec("7. Keputusan Jalur Sintesis", "```json\n"+jsonPretty(s.SynthesisPathDecision)+"\n```")
	}
	if s.SynthesisResults != nil {
		sec("Hasil Sintesis", fmt.Sprintf("*Jalur: %s · Model: %s*\n\n%s",
			s.SynthesisResults.Path, strSafe(s.SynthesisResults.ModelUsed, "-"), s.SynthesisResults.Markdown))
	}
	if s.GradeEvidence != nil {
		g := s.GradeEvidence
		sec("GRADE — Certainty of Evidence", fmt.Sprintf("%s\n\n**Robustness:** %s — %s\n\n%s",
			g.TableMarkdown, g.RobustnessVerdict, g.RobustnessSummary, g.ConfidenceStatements))
	}
	if s.InterpretationPackage != nil {
		sec("Interpretasi Terpadu (Modul 8 → Modul 9)", s.InterpretationPackage.Markdown)
	}
	if s.ModulBibliometricSummary != nil {
		sec("Bibliometrik / SLNA (opsional)", jsonPretty(s.ModulBibliometricSummary))
	}

	// ── 8. Manuskrip final ─────────────────────────────────────────────────────
	if s.Manuscript != nil {
		m := s.Manuscript
		if strings.TrimSpace(m.Final) != "" {
			sec("8. Manuskrip Final (naratif lengkap)", m.Final)
		} else {
			// rakit dari per-section bila final belum ada
			for _, p := range []struct{ t, c string }{
				{"Abstract", m.Abstract}, {"Introduction", m.Introduction}, {"Methods", m.Methods},
				{"Results", m.Results}, {"Discussion", m.Discussion}, {"Conclusions", m.Conclusions},
				{"Future Research", m.FutureResearch},
			} {
				sec("Manuskrip — "+p.t, p.c)
			}
		}
		sec("Daftar Pustaka", m.References)
		sec("Kepatuhan PRISMA 2020 (checklist 27-item)", m.PrismaChecklist)
		sec("Audit Koherensi Manuskrip", m.CoherenceAudit)
	}

	// ── 9. Lampiran transparansi ───────────────────────────────────────────────
	sec("9. Lampiran — Transparansi & Audit", h.reportTransparency(ctx, s))

	wln("\n---\n*Laporan ini dihasilkan otomatis dari database. Untuk replikasi penuh, ekspor "+
		"`slr_sessions`, `slr_screening`, `slr_extraction` (lihat GENERATEREPORT.md).*")
	return b.String()
}

// GenerateReportLatex menyajikan LAPORAN sebagai LaTeX (.tex) yang KONSISTEN dengan manuskrip:
// memakai .bib REAL yang SAMA (session.manuscript.bibtex, dari paper catalog ber-cite-guard),
// dengan \nocite{*} agar SEMUA referensi nyata tampil. Integritas: laporan & manuskrip merujuk
// katalog referensi yang identik & nyata (bukan daftar terpisah/halusinasi). Read-only.
func (h *SessionHandler) GenerateReportLatex(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}
	ctx := context.Background()
	s, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}
	md := h.buildReportMarkdown(ctx, s)
	title := "Laporan SLR — " + strSafe(s.Topic, "(tanpa topik)")
	tex := latex.MarkdownToLatex(title, md)

	// Sisipkan bibliografi REAL bersama (identik dgn manuskrip) sebelum \end{document}.
	bib := ""
	if s.Manuscript != nil {
		bib = strings.TrimSpace(s.Manuscript.Bibtex)
	}
	if bib != "" {
		inject := "\n\\clearpage\n\\nocite{*}\n\\bibliographystyle{unsrt}\n\\bibliography{references}\n"
		tex = strings.Replace(tex, "\\end{document}", inject+"\\end{document}", 1)
	}
	w.Header().Set("Content-Type", "application/x-tex; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="laporan_slr.tex"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(tex))
}

// reportPrisma menghitung ulang angka PRISMA inti dari slr_screening (+ data_mining_log).
func (h *SessionHandler) reportPrisma(ctx context.Context, s *model.SLRSession) string {
	papers, err := h.mongoRepo.GetAllScreeningPapers(ctx, s.ID)
	if err != nil {
		return "_(gagal memuat data screening)_"
	}
	identified, duplicates := 0, 0
	if s.DataMiningLog != nil {
		if s.DataMiningLog.QualityAudit != nil {
			identified = s.DataMiningLog.QualityAudit.TotalRecords
		}
		if s.DataMiningLog.Dedup != nil {
			duplicates = s.DataMiningLog.Dedup.TotalDuplicates
		}
	}
	screened := len(papers)
	inclAbs, retrieved, notRetrieved, exFT, includedFinal := 0, 0, 0, 0, 0
	for _, p := range papers {
		if !passedAbstractScreening(p) {
			continue
		}
		inclAbs++
		ret, _ := p["full_text_retrieved"].(bool)
		if !ret {
			notRetrieved++
			continue
		}
		retrieved++
		switch fullTextDecision(p) {
		case "INCLUDE":
			includedFinal++
		case "EXCLUDE":
			exFT++
		}
	}
	return fmt.Sprintf(
		"| Tahap | n |\n|---|---|\n"+
			"| Records identified | %d |\n| Duplicates removed | %d |\n| Records screened (judul/abstrak) | %d |\n"+
			"| Included at title/abstract (sought) | %d |\n| Reports not retrieved | %d |\n"+
			"| Reports assessed (full-text) | %d |\n| Excluded at full-text | %d |\n"+
			"| **Studies included (final)** | **%d** |\n",
		identified, duplicates, screened, inclAbs, notRetrieved, retrieved, exFT, includedFinal)
}

// reportExtractionTable membuat tabel ringkas per-paper + blok detail field per paper.
func (h *SessionHandler) reportExtractionTable(ctx context.Context, s *model.SLRSession) string {
	coll := h.mongoRepo.GetExtractionCollection()
	cur, err := coll.Find(ctx, map[string]interface{}{"session_id": s.ID, "extracted": true})
	if err != nil {
		return ""
	}
	var docs []map[string]interface{}
	_ = cur.All(ctx, &docs)
	if len(docs) == 0 {
		return "_(belum ada paper terekstrak)_"
	}
	var b strings.Builder
	b.WriteString("| # | Paper | DOI | Coverage | NR | Verified |\n|---|---|---|---|---|---|\n")
	for i, d := range docs {
		ver := "-"
		if v, ok := d["verified"].(bool); ok && v {
			ver = "✓"
		}
		if vd, ok := d["verify_disagree"].(bool); ok && vd {
			ver = "≠ disagree"
		}
		b.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %v | %s |\n",
			i+1, mdCell(gsStr(d, "Title")), mdCell(gsStr(d, "DOI")), gsStr(d, "coverage"), d["nr_count"], ver))
	}
	// Detail field per paper (transparansi penuh: field, value, status). Evidence -> slr_extraction.
	b.WriteString("\n<details><summary>Detail field per paper (klik buka)</summary>\n")
	for _, d := range docs {
		b.WriteString(fmt.Sprintf("\n**%s** (%s)\n\n", strSafe(gsStr(d, "Title"), "(tanpa judul)"), gsStr(d, "DOI")))
		if arr, ok := d["fields"].(primitive.A); ok {
			b.WriteString("| Field | Nilai | Status |\n|---|---|---|\n")
			for _, fe := range arr {
				m, ok := fe.(primitive.M)
				if !ok {
					continue
				}
				b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", mdCell(gsStr(m, "key")), mdCell(gsStr(m, "value")), gsStr(m, "status")))
			}
		}
		if kf := gsStr(d, "key_findings"); kf != "" {
			b.WriteString("\n*Key findings:* " + kf + "\n")
		}
	}
	b.WriteString("\n</details>\n\n*Provenance per nilai (kutipan + section) tersimpan di "+
		"`slr_extraction.fields[].evidence`.*\n")
	return b.String()
}

// reportTransparency: atribusi model, audit koreksi, status verifikasi, jejak xAI.
func (h *SessionHandler) reportTransparency(ctx context.Context, s *model.SLRSession) string {
	var b strings.Builder
	b.WriteString("**Atribusi model (xAI):** output AI membawa provider + nama model asli.\n\n")
	if s.FrameworkSelection != nil && s.FrameworkSelection.ModelUsed != "" {
		b.WriteString("- Framework ekstraksi: " + s.FrameworkSelection.ModelUsed + "\n")
	}
	if s.ExtractionLog != nil {
		b.WriteString("- Ekstraksi (Reviewer 1): " + strSafe(s.ExtractionLog.ModelExtraction, "-") + "\n")
		b.WriteString("- Verifikasi (Reviewer 2): " + strSafe(s.ExtractionLog.ModelRefineProtocol, "-") + "\n")
	}
	if s.DescriptiveAnalysis != nil && s.DescriptiveAnalysis.ModelUsed != "" {
		b.WriteString("- Analisis deskriptif/heterogenitas: " + s.DescriptiveAnalysis.ModelUsed + "\n")
	}
	if s.SynthesisPathDecision != nil && s.SynthesisPathDecision.ModelUsed != "" {
		b.WriteString("- Keputusan jalur sintesis: " + s.SynthesisPathDecision.ModelUsed + "\n")
	}
	if s.SynthesisResults != nil && s.SynthesisResults.ModelUsed != "" {
		b.WriteString("- Sintesis: " + s.SynthesisResults.ModelUsed + "\n")
	}
	if s.GradeEvidence != nil && s.GradeEvidence.ModelUsed != "" {
		b.WriteString("- GRADE certainty: " + s.GradeEvidence.ModelUsed + "\n")
	}
	if s.InterpretationPackage != nil && s.InterpretationPackage.ModelUsed != "" {
		b.WriteString("- Interpretasi (Modul 8): " + s.InterpretationPackage.ModelUsed + "\n")
	}
	if s.BibliometricData != nil && s.BibliometricData.ModelUsed != "" {
		b.WriteString("- Bibliometrik/thesaurus (Modul 8b): " + s.BibliometricData.ModelUsed + "\n")
	}
	if s.VOSViewerParams != nil && s.VOSViewerParams.ModelUsed != "" {
		b.WriteString("- Parameter VOSviewer (Modul 8b): " + s.VOSViewerParams.ModelUsed + "\n")
	}
	if s.ClusterInterpretation != nil && s.ClusterInterpretation.ModelUsed != "" {
		b.WriteString("- Interpretasi cluster (Modul 8b): " + s.ClusterInterpretation.ModelUsed + "\n")
	}
	if s.SLNAIntegration != nil && s.SLNAIntegration.ModelUsed != "" {
		b.WriteString("- Integrasi SLNA (Modul 8b): " + s.SLNAIntegration.ModelUsed + "\n")
	}
	if s.Manuscript != nil && s.Manuscript.ModelUsed != "" {
		b.WriteString("- Penulisan manuskrip (Modul 9): " + s.Manuscript.ModelUsed + "\n")
	}

	// Triangulasi klaim (xAI neuro-symbolic): ringkasan verifikasi ≥2 sumber.
	if s.Manuscript != nil && len(s.Manuscript.ClaimVerifications) > 0 {
		verified := 0
		for _, cv := range s.Manuscript.ClaimVerifications {
			if cv.Sources >= 2 {
				verified++
			}
		}
		total := len(s.Manuscript.ClaimVerifications)
		b.WriteString(fmt.Sprintf("\n**Verifikasi klaim (triangulasi Qdrant/Neo4j/Mongo):** %d/%d klaim terverifikasi ≥2 sumber.\n", verified, total))
		if verified < total {
			b.WriteString("| Section | Klaim | Sumber | Sitasi |\n|---|---|---|---|\n")
			for _, cv := range s.Manuscript.ClaimVerifications {
				if cv.Sources >= 2 {
					continue
				}
				b.WriteString(fmt.Sprintf("| %s | %s | %d | %s |\n", mdCell(cv.Section), mdCell(cv.Claim), cv.Sources, mdCell(cv.CitationKey)))
			}
		}
	}

	if len(s.ScreeningCorrections) > 0 {
		reincl, excl := 0, 0
		for _, c := range s.ScreeningCorrections {
			if c.To == "INCLUDE" {
				reincl++
			} else if c.To == "EXCLUDE" {
				excl++
			}
		}
		b.WriteString(fmt.Sprintf("\n**Audit koreksi include/exclude (deviasi protokol terdokumentasi):** "+
			"%d total (%d re-include, %d exclude).\n\n", len(s.ScreeningCorrections), reincl, excl))
		b.WriteString("| Paper | Perubahan | Alasan | Tgl |\n|---|---|---|---|\n")
		for _, c := range s.ScreeningCorrections {
			title := c.Title
			if title == "" {
				title = c.DOI
			}
			at := c.At
			if len(at) >= 10 {
				at = at[:10]
			}
			b.WriteString(fmt.Sprintf("| %s | %s → %s | %s | %s |\n", mdCell(title), c.From, c.To, mdCell(c.Reason), at))
		}
	} else {
		b.WriteString("\n_Tidak ada koreksi include/exclude pasca-screening._\n")
	}

	b.WriteString(fmt.Sprintf("\n**Jejak panggilan LLM (xai_log):** %d entri tersimpan di sesi.\n", len(s.XAILog)))
	if s.RescreenPending {
		b.WriteString("\n⚠ `rescreen_pending=true`: artefak hilir mungkin perlu diregenerasi setelah perubahan keputusan.\n")
	}
	return b.String()
}

// ── helper kecil ──────────────────────────────────────────────────────────────

func jsonPretty(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "(gagal serialisasi)"
	}
	return string(b)
}

func strSafe(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// mdCell merapikan teks untuk satu sel tabel markdown (buang newline, escape pipa).
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	if s == "" {
		return "-"
	}
	return s
}

// HandoffGuide merakit "Panduan Handoff Cowork-LLM": dokumen credential-SAFE yang mengarahkan
// LLM lain (Claude/GPT di sesi/tool lain) untuk meregenerasi/menyempurnakan artikel+laporan
// LaTeX dari DATA sesi ini. TIDAK pernah memuat kredensial asli — hanya placeholder + pointer.
func (h *SessionHandler) HandoffGuide(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}
	s, err := h.mongoRepo.GetSession(context.Background(), id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "slr_agentic_db"
	}
	lang := strSafe(s.ManuscriptLang, "id")

	var b strings.Builder
	w2 := func(f string, a ...interface{}) { b.WriteString(fmt.Sprintf(f, a...)); b.WriteString("\n") }

	w2("# AGENTS.md — Instruksi Proyek untuk Cowork-LLM (Handoff SLR: %s)", strSafe(s.Topic, "(tanpa topik)"))
	w2("")
	w2("> **Untuk agent/LLM yang membaca file ini:** ini adalah Project Instructions (standar")
	w2("> `AGENTS.md`). Anda ditugaskan MELANJUTKAN/MENYEMPURNAKAN Systematic Literature Review di")
	w2("> bawah dengan akses **read-only** ke database peneliti. Ikuti aturan protokol (a-priori,")
	w2("> anti-HARKing, gaya Q1) di §4 secara ketat; jangan mengarang angka/sitasi.")
	w2("> Session: `%s` · Status: %s · Bahasa manuskrip: %s", s.ID, s.Status, lang)
	w2("")
	w2("## ⚠️ Keamanan (WAJIB dibaca)")
	w2("Dokumen ini **TIDAK memuat kredensial** apa pun. Isi placeholder `<...>` di bawah dengan")
	w2("kredensial Anda SENDIRI (jangan commit/kirim ke pihak lain). Semua akses cukup **READ-ONLY**.")
	w2("")
	w2("## 1. Sumber data (pointer sesi ini)")
	w2("| Store | Peran | Lokasi / kunci |")
	w2("|---|---|---|")
	w2("| MongoDB | State SLR + keputusan HITL + provenance | DB `%s`, koleksi `slr_sessions` (`_id=\"%s\"`), `slr_screening`, `slr_extraction` |", dbName, s.ID)
	w2("| Qdrant | Full-text embedding (RAG bukti/sitasi) | collection **`scientific_articles`** (payload: `title`,`doi`,`article_id`,`content`) — GLOBAL lintas-sesi, filter per DOI studi included |")
	w2("| Neo4j (opsional) | Knowledge graph / SLNA | node per studi (label Paper/Author/Method/Dataset/Metric), relasi antar-entitas — filter `session_id=\"%s\"` |", s.ID)
	w2("")
	w2("### 1a. Skema AKTUAL sesi ini (auto-generated — selalu terkini, bukan dokumentasi manual)")
	w2("> Tabel di bawah di-introspeksi LANGSUNG dari dokumen Mongo sesi Anda saat file ini dibuat,")
	w2("> jadi mustahil basi. `✓`=terisi, `✗`=kosong.")
	w2("")
	b.WriteString(h.liveSchemaMarkdown(context.Background(), s.ID))
	w2("")
	w2("## 2. Template koneksi (isi kredensial Anda — JANGAN bagikan)")
	w2("```bash")
	w2("# MongoDB (Atlas / lokal)")
	w2("export MONGO_URI=\"<mongodb+srv://USER:PASS@host/>\"   # read-only user disarankan")
	w2("export DB_NAME=\"%s\"", dbName)
	w2("# Qdrant")
	w2("export QDRANT_URL=\"<https://<id>.<region>.gcp.cloud.qdrant.io:6333>\"")
	w2("export QDRANT_API_KEY=\"<api-key>\"")
	w2("# Neo4j / AuraDB (opsional)")
	w2("export NEO4JURI=\"<neo4j+s://<id>.databases.neo4j.io>\"")
	w2("export NEO4JUSER=\"neo4j\"; export NEO4JPASSWORD=\"<password>\"")
	w2("```")
	w2("")
	w2("## 3. Dua jalur pakai kit ini")
	w2("- **Jalur cepat (TANPA database):** manuskrip LaTeX final SUDAH lengkap — unduh `.tex`+`.bib` dari Ruang Ekspor, compile (lihat §7), lalu suruh cowork-LLM menyempurnakan prosa langsung di `.tex`. Rantai tertutup tanpa akses DB.")
	w2("- **Jalur regen penuh (dengan DB read-only):** untuk menulis ulang/memperkaya dari sumber, sambungkan ke DB (§2) lalu ikuti §4.")
	w2("")
	w2("## 4. Langkah regen penuh (untuk cowork LLM)")
	w2("1. Tarik data lengkap dari MongoDB (koleksi §1). Angka PRISMA/κ **dihitung ulang dari keputusan FINAL di DB** (ground-truth) — jangan mengarang.")
	w2("2. **Verifikasi klaim/sitasi**: cari full-text di Qdrant `scientific_articles` (semantic search) sebelum menulis klaim; kutip HANYA yang tertaut bukti (lihat `claim_verifications` pada `manuscript`).")
	w2("3. **Protokol a-priori** (jangan HARKing): framework ekstraksi & kriteria sudah tetap di `slr_sessions`; jangan ubah protokol mengikuti data.")
	w2("4. **Gaya Q1 / anti-AI-tell**: tanpa em-dash/emoji/kata over-AI; deskripsikan proses AI-assisted-HITL secara TRANSPARAN di Methods (jangan menyamarkan AI sebagai manusia).")
	w2("5. **Panduan detail & pemetaan field → bagian artikel** (repo PUBLIK `ifcoid/nsa`, bisa langsung di-fetch cowork-LLM):")
	w2("   - Menulis artikel Q1: `https://raw.githubusercontent.com/ifcoid/nsa/main/GENERATEARTIKEL.md`")
	w2("   - Peta field → laporan: `https://raw.githubusercontent.com/ifcoid/nsa/main/GENERATEREPORT.md`")
	w2("")
	w2("## 5. Artefak dalam kit ini (dari Ruang Ekspor)")
	w2("- Manuskrip: `.tex` (LaTeX), `.bib` (BibTeX), `.md` (final).")
	w2("- Laporan lengkap `.md` (naratif per-modul + PRISMA + ekstraksi).")
	w2("- Protokol a-priori (PROSPERO/OSF) + Paket Reproducibility (PRISMA-S + κ + provenance).")
	w2("- Figur bibliometrik/SLNA (SVG/PNG + CSV data) — di-generate notebook PEDE, diunggah di Ruang Ekspor.")
	w2("- Dokumen ini (Panduan Handoff).")
	w2("")
	w2("## 6. Deposit Zenodo — arsip reproducible + DOI (disarankan Q1)")
	w2("Agar review benar-benar reproducible & dapat disitasi permanen, deposit kit ke **Zenodo** (dapat DOI, versioned):")
	w2("1. https://zenodo.org → login (bisa via ORCID) → **New upload**.")
	w2("2. Unggah SATU paket: Protokol, Paket Reproducibility, Laporan, manuskrip `.tex`/`.bib`, **figur bibliometrik (SVG/PNG + CSV)**, dan Handoff ini. (Opsional: ekspor read-only data ekstraksi/screening dari Mongo sebagai `.json`/`.csv` — HAPUS kredensial.)")
	w2("3. Metadata: judul, penulis+ORCID, tipe *Dataset*/*Software*, lisensi (mis. CC-BY-4.0), keywords → **Publish** → dapat **DOI** (`10.5281/zenodo.XXXXXXX`).")
	w2("4. **Sitasi DOI itu di manuskrip** pada *Data Availability Statement* (mis. \"Protocol, extraction data, and reproducibility package are openly available at Zenodo, DOI: 10.5281/zenodo.XXXXXXX\") + tautkan registrasi **PROSPERO/OSF** bila ada.")
	w2("5. Revisi berikutnya: fitur *New version* Zenodo (concept-DOI tetap, version-DOI baru) → jejak revisi terarsip.")
	w2("")
	w2("> **Rantai reproducibility tertutup:** Protokol a-priori (PROSPERO) → data & keputusan (DB read-only) → Paket Reproducibility → arsip permanen ber-DOI (Zenodo) → disitasi di manuskrip. Pihak ketiga memverifikasi dari DOI TANPA akses ke sistem asli.")
	w2("")
	w2("## 7. Compile LaTeX → PDF (TinyTeX)")
	w2("Manuskrip DAN laporan konsisten **LaTeX + BibTeX** memakai katalog referensi NYATA yang sama (integritas sitasi). Untuk meng-compile:")
	w2("1. **Install TinyTeX** (distribusi LaTeX ringan ~100MB, sekali pasang):")
	w2("   - macOS/Linux: `wget -qO- \"https://yihui.org/tinytex/install-bin-unix.sh\" | sh`")
	w2("   - Windows (PowerShell): `irm https://yihui.org/tinytex/install-bin-windows.bat -OutFile install.bat; ./install.bat`")
	w2("   - via R: `install.packages('tinytex'); tinytex::install_tinytex()`")
	w2("2. Taruh `.tex` + `references.bib` (untuk laporan) / `.bib` (untuk manuskrip) di folder yang SAMA, lalu:")
	w2("   ```\n   pdflatex <file>.tex\n   bibtex <file>\n   pdflatex <file>.tex\n   pdflatex <file>.tex\n   ```")
	w2("   Urutan itu wajib agar sitasi & daftar pustaka BibTeX ter-resolve; paket yang kurang di-install otomatis oleh TinyTeX. Tanpa install: unggah `.tex`+`.bib` ke **Overleaf** (compile di browser).")
	w2("")
	w2("_Dokumen ini di-generate otomatis dari sesi; credential-safe; read-only. Simpan bersama artefak sebagai satu handoff kit._")

	fname := fmt.Sprintf("handoff_%s.md", s.ID)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fname))
	_, _ = w.Write([]byte(b.String()))
}

// bsonValType mendeskripsikan tipe & keterisian nilai bson secara ringkas (untuk schema live).
func bsonValType(v interface{}) (typ string, populated bool) {
	switch x := v.(type) {
	case nil:
		return "kosong", false
	case string:
		return "teks", strings.TrimSpace(x) != ""
	case bool:
		return "boolean", true
	case int32, int64, int, float64, float32:
		return "angka", true
	case primitive.ObjectID:
		return "id", true
	case primitive.DateTime, primitive.Timestamp:
		return "tanggal", true
	case primitive.A:
		return fmt.Sprintf("array (%d item)", len(x)), len(x) > 0
	case []interface{}:
		return fmt.Sprintf("array (%d item)", len(x)), len(x) > 0
	case primitive.M:
		return fmt.Sprintf("objek (%d field)", len(x)), len(x) > 0
	case map[string]interface{}:
		return fmt.Sprintf("objek (%d field)", len(x)), len(x) > 0
	default:
		return "nilai", v != nil
	}
}

// knownFieldNote memberi keterangan singkat untuk field sesi yang penting (sisanya "-").
var knownFieldNote = map[string]string{
	"topic": "topik riset", "foundation": "fondasi teori (M1)", "pico_definitions": "PICO (M2)",
	"research_questions": "RQ (M2)", "scope_filters": "batas lingkup", "scope_justifications": "justifikasi scope",
	"inclusion_criteria": "kriteria inklusi", "exclusion_criteria": "kriteria eksklusi", "keywords": "keyword PICO (M3)",
	"database_selection": "pilihan database (M3)", "search_string": "search string (M3)", "search_log": "log pencarian",
	"data_mining_log": "identifikasi+dedup (M4)", "screening_setup": "setup screening + reason_codes (M5)",
	"framework_selection": "framework ekstraksi (M7 L1)", "extraction_log": "log+κ ekstraksi (M7 L2)",
	"qa_threshold_justification": "tool+ambang QA (M7 L3)", "qa_calibration": "kalibrasi QA + κ pilot (M7 L3)",
	"synthesis_path_decision": "jalur sintesis (M8)", "synthesis_results": "hasil sintesis (M8)",
	"grade_evidence_table": "GRADE + robustness (M8)", "slna_integration": "SLNA bibliometric (M8b)",
	"manuscript": "manuskrip final + xAI (M9) — lihat sub-field", "audit_report": "audit pra-submisi + artefak (M10) — lihat sub-field",
	"fulltext_kappa": "κ full-text screening (M6)", "manuscript_lang": "bahasa manuskrip",
}

// liveSchemaMarkdown men-introspeksi dokumen Mongo AKTUAL (sesi + 1 ekstraksi) lalu memancarkan
// peta field yang SELALU TERKINI — tak bergantung pada dokumentasi manual yang bisa basi.
func (h *SessionHandler) liveSchemaMarkdown(ctx context.Context, id string) string {
	var b strings.Builder
	var raw bson.M
	if err := h.mongoRepo.GetSessionCollection().FindOne(ctx, bson.M{"_id": id}).Decode(&raw); err != nil {
		return "_(schema live tak tersedia: " + err.Error() + ")_\n"
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		if k == "xai_log" || k == "fulltext_screening_log" { // berat/verbose, lewati
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b.WriteString("| Field (`slr_sessions`) | Tipe | Terisi | Keterangan |\n|---|---|---|---|\n")
	for _, k := range keys {
		typ, pop := bsonValType(raw[k])
		note := knownFieldNote[k]
		if note == "" {
			note = "-"
		}
		mark := "✗"
		if pop {
			mark = "✓"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", k, typ, mark, note)
	}
	// Sub-field manuscript & audit_report (artefak inti).
	expand := func(title, key string, notes map[string]string) {
		if m, ok := raw[key].(bson.M); ok && len(m) > 0 {
			sub := make([]string, 0, len(m))
			for sk := range m {
				sub = append(sub, sk)
			}
			sort.Strings(sub)
			fmt.Fprintf(&b, "\n**`%s.*`** (%s):\n\n| Sub-field | Tipe | Terisi | Keterangan |\n|---|---|---|---|\n", key, title)
			for _, sk := range sub {
				typ, pop := bsonValType(m[sk])
				mark := "✗"
				if pop {
					mark = "✓"
				}
				n := notes[sk]
				if n == "" {
					n = "-"
				}
				fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", sk, typ, mark, n)
			}
		}
	}
	expand("manuskrip M9", "manuscript", map[string]string{
		"latex": "LaTeX final (.tex)", "bibtex": "BibTeX (.bib)", "final": "markdown final",
		"model_used": "nama model penulis (xAI)", "claim_verifications": "bukti triangulasi klaim (xAI)",
		"prisma_flow": "PRISMA flow tervalidasi", "prisma_checklist": "PRISMA 27-item", "coherence_audit": "audit koherensi",
	})
	expand("audit M10", "audit_report", map[string]string{
		"checks": "cek simbolik", "verdict": "READY/WITH_WARNINGS/NOT_READY", "protocol_markdown": "protokol PROSPERO",
		"repro_package_markdown": "paket reproducibility", "attested_by": "atestasi peneliti", "attested_at": "waktu atestasi",
	})
	// Satu dokumen ekstraksi (field per-paper).
	var ext bson.M
	if err := h.mongoRepo.GetExtractionCollection().FindOne(ctx, bson.M{"session_id": id}).Decode(&ext); err == nil && len(ext) > 0 {
		ek := make([]string, 0, len(ext))
		for k := range ext {
			ek = append(ek, k)
		}
		sort.Strings(ek)
		b.WriteString("\n**`slr_extraction.*`** (contoh 1 dokumen — struktur per-paper):\n\n| Field | Tipe | Terisi |\n|---|---|---|\n")
		for _, k := range ek {
			typ, pop := bsonValType(ext[k])
			mark := "✗"
			if pop {
				mark = "✓"
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s |\n", k, typ, mark)
		}
	}
	return b.String()
}

// SchemaGuide menyajikan SKEMA LIVE (peta field) sesi sebagai file .md tersendiri —
// selalu terkini karena di-introspeksi dari dokumen Mongo aktual. Read-only, credential-safe.
func (h *SessionHandler) SchemaGuide(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID is required")
		return
	}
	s, err := h.mongoRepo.GetSession(context.Background(), id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}
	var b strings.Builder
	b.WriteString("# Skema Data (Live) — " + strSafe(s.Topic, "(tanpa topik)") + "\n\n")
	b.WriteString("> Peta field di-introspeksi LANGSUNG dari dokumen Mongo sesi `" + s.ID + "` saat file ini dibuat,\n")
	b.WriteString("> jadi SELALU terkini (bukan dokumentasi manual). `✓`=terisi, `✗`=kosong. Read-only; tanpa kredensial.\n\n")
	b.WriteString("Koleksi: `slr_sessions` (`_id=\"" + s.ID + "\"`), `slr_screening`, `slr_extraction`. Qdrant `scientific_articles`; Neo4j filter `session_id`.\n\n")
	b.WriteString(h.liveSchemaMarkdown(context.Background(), s.ID))
	b.WriteString("\n_Untuk pemetaan field → bagian artikel & metodologi Q1: lihat GENERATEREPORT.md / GENERATEARTIKEL.md (repo publik ifcoid/nsa) atau Panduan Handoff._\n")

	fname := fmt.Sprintf("schema_%s.md", s.ID)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fname))
	_, _ = w.Write([]byte(b.String()))
}
