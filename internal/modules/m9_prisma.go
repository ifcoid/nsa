package modules

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"nsa/internal/model"
)

// PrismaFlow holds every PRISMA 2020 flow-diagram count, recomputed deterministically
// from the screening collection plus the M4 identification and M6 acquisition logs.
// It is the single authoritative source for the flow numbers in the manuscript: the
// abstract narrative, the Results study-selection paragraph, and the Figure 1 diagram
// all derive from this struct so they can never disagree.
type PrismaFlow struct {
	// Identification
	Identified        int
	DuplicatesRemoved int
	// Screening (title/abstract)
	Screened    int
	ExcludedTA  int
	UncertainTA int
	Sought      int // included at title/abstract = reports sought for retrieval
	// Retrieval + eligibility (full text)
	NotRetrieved int // sought but full text inaccessible
	Assessed     int // full text retrieved and assessed for eligibility
	ExcludedFT   int
	UncertainFT  int
	Included     int // final included studies
	// Full-text exclusion reasons (code -> count)
	ExclusionReasonsFT map[string]int
	// Arithmetic-consistency warnings (empty when the flow closes cleanly)
	Warnings []string
}

// computePrismaFlow recounts the complete PRISMA flow from ground-truth DB records.
// It mirrors the exact classification logic used in M5 (m5_screening.go) and M6
// (m6_fulltext.go) so the assembled flow matches what those modules persisted.
func (m *M9Manuscript) computePrismaFlow(ctx context.Context, session *model.SLRSession) (*PrismaFlow, error) {
	papers, err := m.deps.MongoRepo.GetAllScreeningPapers(ctx, session.ID)
	if err != nil {
		return nil, err
	}

	identified, duplicates := 0, 0
	if session.DataMiningLog != nil {
		if qa := session.DataMiningLog.QualityAudit; qa != nil {
			identified = qa.TotalRecords
		}
		if d := session.DataMiningLog.Dedup; d != nil {
			duplicates = d.TotalDuplicates
		}
	}
	return countPrismaFromPapers(papers, identified, duplicates), nil
}

// countPrismaFromPapers is the pure classification core of computePrismaFlow, split out
// so it can be unit-tested without a database. It mirrors the exact decision logic of
// M5 (m5_screening.go) and M6 (m6_fulltext.go).
func countPrismaFromPapers(papers []map[string]interface{}, identified, duplicates int) *PrismaFlow {
	pf := &PrismaFlow{
		ExclusionReasonsFT: map[string]int{},
		Identified:         identified,
		DuplicatesRemoved:  duplicates,
		Screened:           len(papers),
	}

	for _, p := range papers {
		// --- Title/abstract decision (mirror m5_screening.go:671-690) ---
		taDec := getStr(p, "Final_Decision", "Screener_1_Decision")
		includedAbstract := taDec == "INCLUDE"
		switch taDec {
		case "INCLUDE":
			pf.Sought++
		case "EXCLUDE":
			pf.ExcludedTA++
		default:
			pf.UncertainTA++
		}
		if !includedAbstract {
			continue
		}

		// --- Full-text retrieval + eligibility (mirror m6_fulltext.go:448-471) ---
		retrieved, _ := p["full_text_retrieved"].(bool)
		if !retrieved {
			pf.NotRetrieved++
			continue
		}
		pf.Assessed++
		switch finalFullDecision(p) {
		case "INCLUDE":
			pf.Included++
		case "EXCLUDE":
			pf.ExcludedFT++
			rc := getStr(p, "Screener_1_Reason_Code_Full")
			if rc == "" || rc == "-" {
				rc = "OTHER"
			}
			pf.ExclusionReasonsFT[rc]++
		default:
			pf.UncertainFT++
		}
	}

	pf.validate()
	return pf
}

// prismaContext returns the validated PRISMA flow artifact block for injection into
// the manuscript prompts, or "" if the flow cannot be computed (the prompts then fall
// back to the title/abstract-only flow from the M5 exclusion table).
func (m *M9Manuscript) prismaContext(ctx context.Context, session *model.SLRSession) string {
	pf, err := m.computePrismaFlow(ctx, session)
	if err != nil {
		return ""
	}
	// Sertakan jejak koreksi include/exclude HITL agar narasi Methods/Results melaporkan
	// deviasi protokol + audit (angka PRISMA sendiri sudah dihitung ulang dari DB).
	return pf.artifactText() + prismaCorrectionsNote(session.ScreeningCorrections)
}

// validate records any arithmetic inconsistency so reviewers see it instead of a
// silently broken flow. PRISMA 2020 requires every record to be accounted for.
func (pf *PrismaFlow) validate() {
	if pf.Identified > 0 && pf.Identified-pf.DuplicatesRemoved != pf.Screened {
		pf.Warnings = append(pf.Warnings, fmt.Sprintf(
			"identified(%d) - duplicates(%d) = %d != screened(%d)",
			pf.Identified, pf.DuplicatesRemoved, pf.Identified-pf.DuplicatesRemoved, pf.Screened))
	}
	if pf.Screened != pf.ExcludedTA+pf.UncertainTA+pf.Sought {
		pf.Warnings = append(pf.Warnings, fmt.Sprintf(
			"screened(%d) != excludedTA(%d) + uncertainTA(%d) + sought(%d)",
			pf.Screened, pf.ExcludedTA, pf.UncertainTA, pf.Sought))
	}
	if pf.Sought != pf.NotRetrieved+pf.Assessed {
		pf.Warnings = append(pf.Warnings, fmt.Sprintf(
			"sought(%d) != notRetrieved(%d) + assessed(%d)",
			pf.Sought, pf.NotRetrieved, pf.Assessed))
	}
	if pf.Assessed != pf.ExcludedFT+pf.UncertainFT+pf.Included {
		pf.Warnings = append(pf.Warnings, fmt.Sprintf(
			"assessed(%d) != excludedFT(%d) + uncertainFT(%d) + included(%d)",
			pf.Assessed, pf.ExcludedFT, pf.UncertainFT, pf.Included))
	}
	if pf.UncertainTA > 0 {
		pf.Warnings = append(pf.Warnings, fmt.Sprintf(
			"%d record UNCERTAIN di title/abstract belum diselesaikan (PRISMA mensyaratkan include/exclude)", pf.UncertainTA))
	}
	if pf.UncertainFT > 0 {
		pf.Warnings = append(pf.Warnings, fmt.Sprintf(
			"%d record UNCERTAIN di full-text belum diselesaikan", pf.UncertainFT))
	}
}

// sortedReasonsFT returns "CODE (n=count)" fragments ordered by descending count
// for stable, deterministic rendering.
func (pf *PrismaFlow) sortedReasonsFT() []string {
	type kv struct {
		code  string
		count int
	}
	pairs := make([]kv, 0, len(pf.ExclusionReasonsFT))
	for c, n := range pf.ExclusionReasonsFT {
		pairs = append(pairs, kv{c, n})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].code < pairs[j].code
	})
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, fmt.Sprintf("%s (n=%d)", p.code, p.count))
	}
	return out
}

// artifactText renders the validated flow as a labelled artifact block that is injected
// into the manuscript prompts. The header is emphatic so the LLM uses these exact
// numbers verbatim rather than the partial title/abstract-only flow.
func (pf *PrismaFlow) artifactText() string {
	var b strings.Builder
	b.WriteString("\n\n== PRISMA FLOW (FINAL, VALIDATED -- GUNAKAN ANGKA INI PERSIS, JANGAN MENGARANG/MENGHITUNG ULANG) ==\n")
	b.WriteString(fmt.Sprintf("- Records identified (databases/registers): %d\n", pf.Identified))
	b.WriteString(fmt.Sprintf("- Duplicate records removed before screening: %d\n", pf.DuplicatesRemoved))
	b.WriteString(fmt.Sprintf("- Records screened (title/abstract): %d\n", pf.Screened))
	b.WriteString(fmt.Sprintf("- Records excluded at title/abstract: %d\n", pf.ExcludedTA))
	if pf.UncertainTA > 0 {
		b.WriteString(fmt.Sprintf("- Records uncertain/unresolved at title/abstract: %d\n", pf.UncertainTA))
	}
	b.WriteString(fmt.Sprintf("- Reports sought for retrieval: %d\n", pf.Sought))
	b.WriteString(fmt.Sprintf("- Reports not retrieved (full text inaccessible): %d\n", pf.NotRetrieved))
	b.WriteString(fmt.Sprintf("- Reports assessed for eligibility (full text): %d\n", pf.Assessed))
	b.WriteString(fmt.Sprintf("- Reports excluded at full-text: %d\n", pf.ExcludedFT))
	if reasons := pf.sortedReasonsFT(); len(reasons) > 0 {
		b.WriteString("  Full-text exclusion reasons: " + strings.Join(reasons, "; ") + "\n")
	}
	if pf.UncertainFT > 0 {
		b.WriteString(fmt.Sprintf("- Reports uncertain/unresolved at full-text: %d\n", pf.UncertainFT))
	}
	b.WriteString(fmt.Sprintf("- Studies included in the review (final): %d\n", pf.Included))
	b.WriteString("Figure 1 (PRISMA flow) memuat angka-angka ini; narasikan konsisten dan rujuk \"Figure~\\ref{fig:prisma}\".\n")
	if len(pf.Warnings) > 0 {
		b.WriteString("CATATAN KONSISTENSI (jangan tulis di manuskrip, untuk reviewer): " + strings.Join(pf.Warnings, " | ") + "\n")
	}
	b.WriteString("== END PRISMA FLOW ==\n")
	return b.String()
}

// prismaCorrectionsNote merangkai jejak koreksi include/exclude HITL pasca-screening menjadi
// catatan deviasi-protokol untuk konteks manuskrip. PRISMA Figure 1 sudah mencerminkan
// keputusan FINAL (dihitung ulang dari DB); catatan ini memastikan koreksinya TERLAPORKAN
// (provenance/audit Q1: apa yang diubah + alasannya), bukan perubahan diam-diam. "" bila kosong.
func prismaCorrectionsNote(cor []model.ScreeningCorrection) string {
	if len(cor) == 0 {
		return ""
	}
	reincluded, excluded := 0, 0
	for _, c := range cor {
		if c.To == "INCLUDE" {
			reincluded++
		} else if c.To == "EXCLUDE" {
			excluded++
		}
	}
	var b strings.Builder
	b.WriteString("\n\n== KOREKSI SCREENING HITL (deviasi protokol — laporkan di Methods sebagai koreksi keputusan + audit; JANGAN diklaim sebagai perubahan kriteria) ==\n")
	b.WriteString(fmt.Sprintf("- Total koreksi keputusan full-text pasca-screening: %d (%d di-INCLUDE-kan ulang dari EXCLUDE; %d di-EXCLUDE-kan dari INCLUDE).\n", len(cor), reincluded, excluded))
	b.WriteString("- PRISMA Figure 1 & seluruh angka sudah mencerminkan keputusan FINAL. Protokol ekstraksi TIDAK berubah (koreksi keputusan, bukan amendemen protokol).\n")
	b.WriteString("- Tiap koreksi membawa alasan eksplisit (justifikasi metodologis):\n")
	const maxList = 15
	for i, c := range cor {
		if i >= maxList {
			b.WriteString(fmt.Sprintf("  ... dan %d koreksi lain (lihat audit screening_corrections).\n", len(cor)-maxList))
			break
		}
		title := c.Title
		if title == "" {
			title = c.DOI
		}
		b.WriteString(fmt.Sprintf("  - \"%s\" (%s): %s -> %s — %s\n", title, c.DOI, c.From, c.To, c.Reason))
	}
	b.WriteString("== END KOREKSI SCREENING HITL ==\n")
	return b.String()
}

// tikzFigure renders a self-contained PRISMA 2020 flow diagram as a TikZ figure float.
// Requires \usepackage{tikz} and the positioning + arrows.meta libraries, which
// BuildAcademicLatex adds when a figure is present.
func (pf *PrismaFlow) tikzFigure() string {
	exFT := fmt.Sprintf("Reports excluded (n=%d)", pf.ExcludedFT)
	if reasons := pf.sortedReasonsFT(); len(reasons) > 0 {
		exFT += `\\ ` + latexEscapeShort(strings.Join(reasons, "; "))
	}
	exTA := fmt.Sprintf("Records excluded (n=%d)", pf.ExcludedTA)
	if pf.UncertainTA > 0 {
		exTA += fmt.Sprintf(`\\ Uncertain/unresolved (n=%d)`, pf.UncertainTA)
	}
	assessed := fmt.Sprintf("Reports assessed for eligibility (n=%d)", pf.Assessed)
	if pf.UncertainFT > 0 {
		assessed += fmt.Sprintf(`\\ Uncertain at full text (n=%d)`, pf.UncertainFT)
	}

	var b strings.Builder
	b.WriteString("\\begin{figure}[htbp]\n\\centering\n")
	b.WriteString("\\begin{tikzpicture}[\n")
	b.WriteString("  box/.style={rectangle, draw, text width=6.2cm, minimum height=0.9cm, align=left, font=\\small, inner sep=4pt},\n")
	b.WriteString("  side/.style={rectangle, draw, text width=5.4cm, minimum height=0.9cm, align=left, font=\\small, inner sep=4pt},\n")
	b.WriteString("  node distance=0.85cm and 1.3cm,\n")
	b.WriteString("  every path/.style={-{Latex[length=2mm]}, thick}\n")
	b.WriteString("]\n")
	b.WriteString(fmt.Sprintf("\\node[box] (identified) {Records identified (n=%d):\\\\ databases and registers};\n", pf.Identified))
	b.WriteString("\\node[box, below=of identified] (screened) {" + fmt.Sprintf("Records screened (n=%d)", pf.Screened) + "};\n")
	b.WriteString("\\node[box, below=of screened] (sought) {" + fmt.Sprintf("Reports sought for retrieval (n=%d)", pf.Sought) + "};\n")
	b.WriteString("\\node[box, below=of sought] (assessed) {" + assessed + "};\n")
	b.WriteString("\\node[box, below=of assessed] (included) {" + fmt.Sprintf("Studies included in review (n=%d)", pf.Included) + "};\n")
	b.WriteString("\\node[side, right=of identified] (dups) {" + fmt.Sprintf("Duplicate records removed (n=%d)", pf.DuplicatesRemoved) + "};\n")
	b.WriteString("\\node[side, right=of screened] (exta) {" + exTA + "};\n")
	b.WriteString("\\node[side, right=of sought] (notret) {" + fmt.Sprintf("Reports not retrieved (n=%d)", pf.NotRetrieved) + "};\n")
	b.WriteString("\\node[side, right=of assessed] (exft) {" + exFT + "};\n")
	b.WriteString("\\draw (identified) -- (screened);\n")
	b.WriteString("\\draw (screened) -- (sought);\n")
	b.WriteString("\\draw (sought) -- (assessed);\n")
	b.WriteString("\\draw (assessed) -- (included);\n")
	b.WriteString("\\draw (identified) -- (dups);\n")
	b.WriteString("\\draw (screened) -- (exta);\n")
	b.WriteString("\\draw (sought) -- (notret);\n")
	b.WriteString("\\draw (assessed) -- (exft);\n")
	b.WriteString("\\end{tikzpicture}\n")
	b.WriteString("\\caption{PRISMA 2020 flow diagram of study identification, screening, eligibility, and inclusion.}\n")
	b.WriteString("\\label{fig:prisma}\n")
	b.WriteString("\\end{figure}\n")
	return b.String()
}

// latexEscapeShort escapes the handful of characters that would break a TikZ node body.
func latexEscapeShort(s string) string {
	r := strings.NewReplacer(
		"&", `\&`, "%", `\%`, "#", `\#`, "_", `\_`, "$", `\$`,
		"{", `\{`, "}", `\}`, "~", `\textasciitilde{}`, "^", `\textasciicircum{}`,
	)
	return r.Replace(s)
}
