package modules

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"nsa/internal/agent"
	"nsa/internal/latex"
	"nsa/internal/logger"
	"nsa/internal/model"
)

const promptCoherence = `Anda editor manuskrip. Lakukan COHERENCE AUDIT lintas-section sebuah systematic review.
Laporkan tiap subcheck sebagai PASS atau daftar issue spesifik + saran perbaikan (Markdown):
A. Repetisi (Intro↔Discussion↔Conclusions; prior reviews Intro 5.2 vs Discussion 3.5 harus beda framing; gap Discussion 3.6 vs Future Research harus beda; implications Discussion vs Conclusions tak identik).
B. Terminologi kanonikal konsisten + gaya SLR ("systematic review/extraction/synthesis/PICO"); tidak ada drift ScR ("scoping review/charting/PCC").
C. Bahasa sesuai jalur (Jalur A tanpa "pooled effect"; Jalur B tanpa vote counting tanpa kualifikasi).
D. AI-mention leak: scan "AI/Claude/LLM/GPT/Pass 1-2/AI-assisted" di semua section (KECUALI AI Declaration) — tiap match = issue.
E. Konsistensi numerik (N, κ_TA, κ_FT, κ_extract, κ_rob, %, GRADE) antar Methods/Results/Abstract.
F. Internal vocabulary/provenance leak (outputs/, .xlsx, "Modul X", session id, draft v1).
G. Training-workflow voice leak.
H. Indonesian calque ("It is known that", "Many studies have" opener).
I. Geographic claim consistency (Title/Abstract/Intro/Methods/Discussion).
J. GRADE/RoB hedging consistency.
K. AI-STYLE TELLS (anti-ciri-AI): tandai (HARUS NOL di prosa/judul/abstract/heading): em-dash "—"/en-dash "–"; EMOJI & IKON/SIMBOL dekoratif (✅ ⚠️ ❌ ✓ ✗ → ➔ ★ ● ◆ 🎯 🚀 dsb — kecuali ✓/⚠/✗ di dalam tabel checklist PRISMA formal); transisi klise bertumpuk (Moreover/Furthermore/In addition/Notably/"It is worth noting"); pola terlalu rapi "not only X but also Y"; kata over-AI (delve/leverage/underscore/pivotal/realm/tapestry/intricate); kutip keriting. Saran ganti ke gaya akademisi manusia.
Output ringkas per huruf A-K.`

const promptPrisma = `Susun PRISMA 2020 27-ITEM COMPLIANCE CHECK (supplementary) sebagai tabel Markdown:
| # | Item | Section | Status |
Status: ✓ COVERED / ⚠ PARTIAL / ✗ MISSING. Untuk PARTIAL/MISSING beri rekomendasi fix singkat.
Item 1 Title, 2 Abstract, 3-4 Intro (rationale, objectives), 5-19 Methods, 20-22 Results, 23-25 Discussion+Conclusions (incl. 24a-c limitations: study/review/missing-data), 26-27 Other (registration, support, COI, data availability). Nilai berdasarkan section yang diberikan.`

const aiDeclaration = `AI tools were used solely to assist with language refinement, grammar checking, and readability of the manuscript. AI was not used for any analytic decision, study screening, data extraction, risk-of-bias rating, evidence synthesis, or methodological judgement; those were performed by the named reviewers, extractors, and raters. All scholarly content, methodological decisions, interpretations, and final wording are the responsibility of the author(s).`

const aiDeclarationID = `Bantuan AI digunakan semata untuk penyempurnaan bahasa, pemeriksaan tata bahasa, dan keterbacaan naskah. AI tidak digunakan untuk keputusan analitis apa pun, penyaringan studi, ekstraksi data, penilaian risiko bias, sintesis bukti, maupun pertimbangan metodologis; seluruhnya dilakukan oleh reviewer, extractor, dan rater yang disebutkan. Seluruh isi ilmiah, keputusan metodologis, interpretasi, dan kata akhir adalah tanggung jawab penulis.`

func aiDeclFor(session *model.SLRSession) string {
	if l := strings.ToLower(strings.TrimSpace(session.ManuscriptLang)); l == "en" || l == "english" || l == "inggris" {
		return aiDeclaration
	}
	return aiDeclarationID
}

func (m *M9Manuscript) runCompile(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [L10] Compile: .bib (paper catalog) + coherence audit + PRISMA checklist + final + .tex...")
	ms := session.Manuscript
	if ms == nil {
		return fmt.Errorf("manuscript kosong; jalankan group A/B dulu")
	}

	// 1. Build .bib from paper catalog (local entries from metadata).
	citations := m.buildPaperCatalog(ctx, session)
	latexCitations := make([]latex.PaperCitation, len(citations))
	for i, c := range citations {
		latexCitations[i] = latex.PaperCitation{
			Key:     c.Key,
			Authors: c.Authors,
			Title:   c.Title,
			Year:    c.Year,
			Journal: c.Journal,
			DOI:     c.DOI,
		}
	}
	ms.Bibtex = latex.GenerateBibFile(latexCitations)
	logger.Logf(session.ID, "      ✓ .bib generated (%d entries from paper catalog)\n", len(citations))

	// Defense-in-depth: re-run the deterministic citation guard over every section so
	// no \cite{} key can reference an entry absent from the .bib just generated above
	// (covers approved/older drafts and any key that slipped through M9_GROUPB).
	m.sanitizeManuscriptCitations(session, citations)

	// Complete, validated PRISMA 2020 flow (identification -> inclusion) recomputed
	// from ground-truth DB records; backs both the figure and the narrative numbers.
	prismaTikz := ""
	if pf, e := m.computePrismaFlow(ctx, session); e == nil {
		ms.PrismaFlow = pf.artifactText()
		prismaTikz = pf.tikzFigure()
		if len(pf.Warnings) > 0 {
			logger.Logf(session.ID, "      [PRISMA][WARN] flow tidak menutup: %s", strings.Join(pf.Warnings, " | "))
		} else {
			logger.Log(session.ID, "      ✓ PRISMA flow valid (aritmetika menutup) + Figure 1 (TikZ)")
		}
	} else {
		logger.Logf(session.ID, "      [WARN] gagal menghitung PRISMA flow: %v", e)
	}

	// Build references markdown for backward compatibility
	_, metaRefs := m.includedReferences(ctx, session)
	refsMd := fmt.Sprintf("## References\n\n_%d referensi dari paper catalog._\n\n%s\n", len(citations), metaRefs)
	ms.References = refsMd

	// 2. Coherence audit + PRISMA checklist (brain).
	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return m.deps.llmError(ctx, "brain", "Memuat client kompilasi M9", err)
	}
	ag := agent.NewManuscriptAgent(brain)
	allSections := m.allSectionsBundle(ms)
	lang := langDirective(session)

	if audit, e := ag.Write(ctx, promptCoherence+lang, allSections); e == nil {
		ms.CoherenceAudit = audit
		logger.Log(session.ID, "      ✓ Coherence audit")
	} else {
		ms.CoherenceAudit = "Audit gagal: " + e.Error()
		logger.Logf(session.ID, "      [WARN] Coherence audit gagal: %v\n", e)
	}
	if prisma, e := ag.Write(ctx, promptPrisma+lang, allSections+m.artifactBundle(session)); e == nil {
		ms.PrismaChecklist = prisma
		logger.Log(session.ID, "      ✓ PRISMA checklist")
	} else {
		ms.PrismaChecklist = "PRISMA checklist gagal: " + e.Error()
	}

	// 3. Compile master Markdown (16-section order) for backward compatibility.
	ms.Final = m.compileFinal(session)

	// 4. Academic LaTeX document (assembles LaTeX sections directly).
	sections := map[string]string{
		"Introduction":    ms.Introduction,
		"Methods":         ms.Methods,
		"Results":         ms.Results,
		"Discussion":      ms.Discussion,
		"Future Research": ms.FutureResearch,
		"Conclusions":     ms.Conclusions,
	}
	ms.Latex = latex.BuildAcademicLatex(
		session.Topic,
		"",               // author left empty for user to fill
		ms.Abstract,
		m.keywords(session),
		sections,
		"references",
		prismaTikz,
	)

	// 5. modul9_summary.
	ms.Summary = m.compileSummaryFromCatalog(session, len(citations))

	logger.Log(session.ID, "   [System] manuscript_final + .tex + .bib + checklist + audit tersimpan. DIJEDA untuk persetujuan akhir.")
	session.Status = "M9_COMPILE_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// sanitizeManuscriptCitations runs the deterministic citation guard over every
// manuscript section in-place, guaranteeing each \cite{} resolves to a .bib entry.
func (m *M9Manuscript) sanitizeManuscriptCitations(session *model.SLRSession, citations []PaperCitation) {
	ms := session.Manuscript
	secs := []struct {
		name string
		ptr  *string
	}{
		{"Introduction", &ms.Introduction},
		{"Methods", &ms.Methods},
		{"Results", &ms.Results},
		{"Discussion", &ms.Discussion},
		{"Future Research", &ms.FutureResearch},
		{"Conclusions", &ms.Conclusions},
		{"Abstract", &ms.Abstract},
	}
	totalRemap, totalDrop := 0, 0
	for _, s := range secs {
		cleaned, stats := sanitizeCitations(*s.ptr, citations)
		*s.ptr = cleaned
		totalRemap += stats.Remapped
		totalDrop += stats.Dropped
	}
	if totalRemap > 0 || totalDrop > 0 {
		logger.Logf(session.ID, "      [CiteGuard/compile] %d keys remapped, %d dropped across sections", totalRemap, totalDrop)
	} else {
		logger.Log(session.ID, "      [CiteGuard/compile] all \\cite keys catalog-clean")
	}
}

// includedReferences mengembalikan daftar DOI + daftar referensi terformat (dari metadata included).
func (m *M9Manuscript) includedReferences(ctx context.Context, session *model.SLRSession) ([]string, string) {
	papers, _ := m.deps.MongoRepo.GetAllScreeningPapers(ctx, session.ID)
	type ref struct{ author, year, title, journal, doi string }
	var list []ref
	var dois []string
	for _, p := range papers {
		retrieved, _ := p["full_text_retrieved"].(bool)
		incAbs := getStr(p, "Final_Decision") == "INCLUDE" ||
			(getStr(p, "Final_Decision") == "" && getStr(p, "Screener_1_Decision") == "INCLUDE")
		if !(retrieved && incAbs && finalFullDecision(p) == "INCLUDE") {
			continue
		}
		doi := getStr(p, "DOI", "doi")
		list = append(list, ref{
			author: getStr(p, "Authors", "authors"), year: getStr(p, "Year", "year"),
			title: getStr(p, "Title", "title"), journal: getStr(p, "Journal", "journal"), doi: doi,
		})
		if doi != "" {
			dois = append(dois, doi)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].author != list[j].author {
			return list[i].author < list[j].author
		}
		return list[i].title < list[j].title
	})
	var b strings.Builder
	for _, r := range list {
		line := strings.TrimSpace(fmt.Sprintf("%s (%s). %s. %s.", r.author, r.year, r.title, r.journal))
		if r.doi != "" {
			line += " https://doi.org/" + strings.TrimPrefix(strings.TrimPrefix(r.doi, "https://doi.org/"), "http://doi.org/")
		}
		b.WriteString("- " + line + "\n")
	}
	if b.Len() == 0 {
		b.WriteString("_(Tidak ada studi included untuk direferensikan.)_\n")
	}
	return dois, b.String()
}

func (m *M9Manuscript) allSectionsBundle(ms *model.Manuscript) string {
	return fmt.Sprintf("=== TITLE ===\n%s\n\n=== ABSTRACT ===\n%s\n\n=== INTRODUCTION ===\n%s\n\n=== METHODS ===\n%s\n\n=== RESULTS ===\n%s\n\n=== DISCUSSION ===\n%s\n\n=== FUTURE RESEARCH ===\n%s\n\n=== CONCLUSIONS ===\n%s\n",
		ms.Title, ms.Abstract, ms.Introduction, ms.Methods, ms.Results, ms.Discussion, ms.FutureResearch, ms.Conclusions)
}

func (m *M9Manuscript) compileFinal(session *model.SLRSession) string {
	ms := session.Manuscript
	var b strings.Builder
	w := func(s string) { b.WriteString(s + "\n\n") }

	w("# Title (Alternatives & Recommendation)")
	w(ms.Title)
	w("## Abstract")
	w(ms.Abstract)
	w("## Keywords")
	w(m.keywords(session))
	w("## 1. Introduction")
	w(ms.Introduction)
	w("## 2. Methods")
	w(ms.Methods)
	w("## 3. Results")
	w(ms.Results)
	w("## 4. Discussion")
	w(ms.Discussion)
	w("## 5. Future Research Agenda")
	w(ms.FutureResearch)
	w("## 6. Conclusions")
	w(ms.Conclusions)
	w("## Funding")
	w("_[Diisi penulis]_")
	w("## Conflict of Interest")
	w("_[Diisi penulis]_")
	w("## AI Assistance Declaration")
	w(aiDeclFor(session))
	w(ms.References)
	w("## Figure Captions")
	w(m.figureCaptions(session))
	w("## Supplementary Material")
	w("- PRISMA 2020 27-item checklist\n- Full extraction dataset (anonymized)\n- Screening records\n- PROSPERO protocol [URL — diisi penulis]")
	return strings.TrimSpace(b.String())
}

func (m *M9Manuscript) keywords(session *model.SLRSession) string {
	kw := []string{"systematic review"}
	if session.PICODefinitions != nil && session.PICODefinitions.CanonicalTerm.Term != "" {
		kw = append(kw, session.PICODefinitions.CanonicalTerm.Term)
	}
	if session.FrameworkSelection != nil && session.FrameworkSelection.Framework != "" {
		kw = append(kw, session.FrameworkSelection.Framework+" framework")
	}
	if session.SLNAIntegration != nil {
		kw = append(kw, "systematic literature network analysis")
	}
	return strings.Join(kw, "; ")
}

func (m *M9Manuscript) figureCaptions(session *model.SLRSession) string {
	var b strings.Builder
	b.WriteString("- Figure 1. PRISMA 2020 flow diagram (study selection).\n")
	if session.DescriptiveAnalysis != nil {
		for i, f := range session.DescriptiveAnalysis.Figures {
			cap := fmt.Sprintf("- Figure %d. %s", i+2, f.Name)
			if f.URL != "" {
				cap += " (" + f.URL + ")"
			}
			b.WriteString(cap + "\n")
		}
	}
	return b.String()
}

func (m *M9Manuscript) compileSummaryFromCatalog(session *model.SLRSession, totalCitations int) string {
	path := "-"
	if session.SynthesisPathDecision != nil {
		path = session.SynthesisPathDecision.Verdict
	}
	return fmt.Sprintf("=== MANUSCRIPT WRITING COMPLETE (SLR) ===\n\n"+
		"FINAL DELIVERABLES (di session.manuscript):\n"+
		"- final (manuscript_final.md, 16 section)\n- latex (manuscript_final.tex, academic article with natbib)\n- bibtex (references.bib, %d entries from paper catalog)\n"+
		"- prisma_checklist (27-item)\n- coherence_audit\n- references\n\n"+
		"SECTIONS: Title, Abstract, Introduction, Methods, Results, Discussion, Future Research, Conclusions.\n"+
		"SYNTHESIS PATH: %s | Framework: %s\n"+
		"REFERENCES: %d entries in .bib (from included paper metadata).\n\n"+
		"PRE-SUBMISSION (diisi penulis): Funding, Conflict of Interest, Author/ORCID, Cover letter, PROSPERO URL, Ethical statement.\n"+
		"AI Assistance Declaration: ada (terbatas language/readability).\n\n"+
		"Pipeline SLR SELESAI.",
		totalCitations, path, frameworkName(session), totalCitations)
}


