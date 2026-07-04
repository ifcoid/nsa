package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"go.mongodb.org/mongo-driver/bson/primitive"

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
	}

	// ── 9. Lampiran transparansi ───────────────────────────────────────────────
	sec("9. Lampiran — Transparansi & Audit", h.reportTransparency(ctx, s))

	wln("\n---\n*Laporan ini dihasilkan otomatis dari database. Untuk replikasi penuh, ekspor "+
		"`slr_sessions`, `slr_screening`, `slr_extraction` (lihat GENERATEREPORT.md).*")

	fname := "laporan_slr.md"
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fname))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
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
	if s.SynthesisResults != nil && s.SynthesisResults.ModelUsed != "" {
		b.WriteString("- Sintesis: " + s.SynthesisResults.ModelUsed + "\n")
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

	w2("# Panduan Handoff Cowork-LLM — %s", strSafe(s.Topic, "(tanpa topik)"))
	w2("")
	w2("> Dokumen ini menyerahkan hasil SLR agar dapat DILANJUTKAN/DISEMPURNAKAN bersama LLM lain")
	w2("> (mis. Claude/GPT di sesi terpisah) yang diberi akses **read-only** ke database Anda.")
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
	w2("## 3. Cara meregenerasi / menyempurnakan (untuk cowork LLM)")
	w2("1. **Titik awal**: manuskrip LaTeX final sudah ada — unduh `.tex` + `.bib` dari Ruang Ekspor, jalankan `pdflatex` + `bibtex`.")
	w2("2. **Untuk memperkaya/menulis ulang**: tarik data lengkap dari MongoDB (koleksi di §1). Semua angka PRISMA/κ **dihitung ulang dari keputusan FINAL di DB** — jangan mengarang.")
	w2("3. **Untuk verifikasi klaim/sitasi**: cari full-text di Qdrant `scientific_articles` (semantic search) sebelum menulis klaim; kutip HANYA yang tertaut bukti (lihat `claim_verifications` pada `manuscript`).")
	w2("4. **Ikuti protokol a-priori** (jangan HARKing): framework ekstraksi & kriteria sudah tetap di `slr_sessions`. Jangan ubah protokol mengikuti data.")
	w2("5. **Gaya Q1 / anti-AI-tell**: tanpa em-dash/emoji/kata over-AI; deskripsikan proses AI-assisted-HITL secara TRANSPARAN di Methods (jangan menyamarkan AI sebagai manusia).")
	w2("6. **Panduan detail** (pemetaan field → bagian artikel, gate etika, pertanyaan Scopus AI): ikuti `GENERATEARTIKEL.md` & `GENERATEREPORT.md` (repo `ifcoid/nsa`).")
	w2("")
	w2("## 4. Artefak yang menyertai (dari Ruang Ekspor)")
	w2("- Manuskrip: `.tex` (LaTeX), `.bib` (BibTeX), `.md` (final).")
	w2("- Laporan lengkap `.md` (naratif per-modul + PRISMA + ekstraksi).")
	w2("- Protokol a-priori (PROSPERO/OSF) + Paket Reproducibility (PRISMA-S + κ + provenance).")
	w2("")
	w2("_Dokumen ini di-generate otomatis dari sesi; credential-safe; read-only. Simpan bersama artefak sebagai satu handoff kit._")

	fname := fmt.Sprintf("handoff_%s.md", s.ID)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fname))
	_, _ = w.Write([]byte(b.String()))
}
