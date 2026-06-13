package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"go.mongodb.org/mongo-driver/bson"

	"nsa/internal/agent"
	"nsa/internal/llm"
	"nsa/internal/logger"
	"nsa/internal/model"
)

type M8bBibliometric struct {
	deps *ModuleDeps
}

func NewM8bBibliometric(deps *ModuleDeps) *M8bBibliometric { return &M8bBibliometric{deps: deps} }

func (m *M8bBibliometric) Name() string { return "M8B_BIBLIO" }

func (m *M8bBibliometric) Execute(ctx context.Context, session *model.SLRSession) error {
	logger.Logf(session.ID, ">> [MODUL 8b: BIBLIOMETRIC/SLNA] State: %s\n", session.Status)
	ctx = llm.WithXAIContext(ctx, session.ID, session.Status, "M8bBibliometric")

	switch session.Status {
	case "M8B_INIT", "M8B_BIBLIO":
		session.Status = "M8B_STEP1_THESAURUS"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L1: data prep + thesaurus ----
	case "M8B_STEP1_THESAURUS":
		return m.runThesaurusL1(ctx, session)
	case "M8B_STEP1_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau 'bibliometric_data' (thesaurus + log). Approve / revisi.")
		return nil
	case "M8B_STEP1_NEEDS_REVISION":
		session.BibliometricData = nil
		session.Feedback = ""
		session.Status = "M8B_STEP1_THESAURUS"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M8B_STEP1_APPROVED":
		session.Status = "M8B_STEP2_PARAMS"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L2: 9-parameter table + tunggu user jalankan VOSviewer ----
	case "M8B_STEP2_PARAMS":
		return m.runParamsL2(ctx, session)
	case "M8B_STEP2_WAITING_VOSVIEWER":
		logger.Log(session.ID, "   [System] Jalankan VOSviewer manual (pakai 9-parameter + thesaurus), lalu paste hasilnya di UI.")
		return nil

	// ---- L3: cluster interpretation (dari input VOSviewer user) ----
	case "M8B_STEP3_INTERPRET":
		return m.runInterpretL3(ctx, session)
	case "M8B_STEP3_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau 'cluster_interpretation' (Tier 1-4 + bridge + structural holes). Approve / revisi.")
		return nil
	case "M8B_STEP3_NEEDS_REVISION":
		session.ClusterInterpretation = nil
		session.Feedback = ""
		session.Status = "M8B_STEP3_INTERPRET"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M8B_STEP3_APPROVED":
		session.Status = "M8B_STEP4_INTEGRATION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- L4: SLNA integration + summary ----
	case "M8B_STEP4_INTEGRATION":
		return m.runIntegrationL4(ctx, session)
	case "M8B_STEP4_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau 'slna_integration' + 'modul_bibliometric_summary'. Approve untuk lanjut ke Modul 9.")
		return nil
	case "M8B_STEP4_NEEDS_REVISION":
		session.SLNAIntegration = nil
		session.ModulBibliometricSummary = nil
		session.Feedback = ""
		session.Status = "M8B_STEP4_INTEGRATION"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M8B_STEP4_APPROVED":
		session.Status = "M9_MANUSCRIPT"
		logger.Log(session.ID, "   [System] Modul 8b SELESAI. Lanjut ke Modul 9 (Manuscript Writing).")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	default:
		return nil
	}
}

// ===== L1 =====

func (m *M8bBibliometric) runThesaurusL1(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 8b.1] Data prep + thesaurus construction...")
	papers, _ := m.deps.MongoRepo.GetAllScreeningPapers(ctx, session.ID)

	// Kumpulkan keyword mentah (frekuensi) dari field Keywords.
	freq := map[string]int{}
	for _, p := range papers {
		kw := getStr(p, "Keywords", "keywords")
		for _, raw := range strings.FieldsFunc(kw, func(r rune) bool { return r == ';' || r == ',' || r == '|' }) {
			t := strings.ToLower(strings.TrimSpace(raw))
			if len(t) > 1 {
				freq[t]++
			}
		}
	}
	kwFromKeywords := len(freq)

	// ---- Fallback source 1: extraction "subject" field ----
	kwFromSubjects := 0
	extColl := m.deps.MongoRepo.GetExtractionCollection()
	if extColl != nil {
		cur, err := extColl.Find(ctx, bson.M{"session_id": session.ID})
		if err == nil {
			var extDocs []bson.M
			_ = cur.All(ctx, &extDocs)
			for _, doc := range extDocs {
				subj := extFieldValue(doc, "subject")
				if subj == "" {
					continue
				}
				for _, raw := range strings.FieldsFunc(subj, func(r rune) bool { return r == ';' || r == ',' || r == '|' }) {
					t := strings.ToLower(strings.TrimSpace(raw))
					if len(t) > 1 {
						if _, exists := freq[t]; !exists {
							kwFromSubjects++
						}
						freq[t]++
					}
				}
			}
		}
	}

	// ---- Fallback source 2: extract meaningful terms from Title ----
	kwFromTitles := 0
	for _, p := range papers {
		title := getStr(p, "Title", "title")
		if title == "" {
			continue
		}
		for _, term := range extractTitleTerms(title) {
			if _, exists := freq[term]; !exists {
				kwFromTitles++
			}
			freq[term]++
		}
	}

	// ---- Fallback source 3: extract terms from Abstract ----
	kwFromAbstracts := 0
	for _, p := range papers {
		abs := getStr(p, "Abstract", "abstract")
		if abs == "" {
			continue
		}
		for _, term := range extractTitleTerms(abs) {
			if _, exists := freq[term]; !exists {
				kwFromAbstracts++
			}
			freq[term]++
		}
	}

	logger.Logf(session.ID, "   [Biblio] Keyword sources: Keywords=%d, Subjects=%d, Titles=%d, Abstracts=%d\n",
		kwFromKeywords, kwFromSubjects, kwFromTitles, kwFromAbstracts)
	// Ambil sampel top-150 untuk LLM.
	type kv struct {
		k string
		v int
	}
	arr := make([]kv, 0, len(freq))
	for k, v := range freq {
		arr = append(arr, kv{k, v})
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].v != arr[j].v {
			return arr[i].v > arr[j].v
		}
		return arr[i].k < arr[j].k
	})
	var sb strings.Builder
	for i, e := range arr {
		if i >= 150 {
			break
		}
		fmt.Fprintf(&sb, "%s (%d)\n", e.k, e.v)
	}

	bd := &model.BibliometricData{
		RecordsAnalyzed: len(papers),
		Approach:        "VOSviewer direct",
	}
	if brain, err := m.deps.LLMFactory.BrainClient(ctx); err == nil {
		if th, e := agent.NewBibliometricAgent(brain).BuildThesaurus(ctx, sb.String()); e == nil && th != nil {
			bd.ThesaurusKeywords = th.Keywords
			bd.ThesaurusAuthors = th.Authors
			if th.Approach != "" {
				bd.Approach = th.Approach
			}
		} else if e != nil {
			logger.Logf(session.ID, "   [WARN] BuildThesaurus gagal: %v\n", e)
		}
	} else {
		return fmt.Errorf("brain (M8b) gagal dimuat: %w", err)
	}

	mergedTerms := strings.Count(bd.ThesaurusKeywords, "\n")
	bd.LogMarkdown = fmt.Sprintf("## Bibliometric Log\n\n- Records dianalisis: **%d**\n- Thesaurus entries (keywords): ~%d\n- Unique raw keywords: %d\n- Approach: %s",
		len(papers), mergedTerms, len(freq), bd.Approach)
	session.BibliometricData = bd

	logger.Logf(session.ID, "   [System] Thesaurus tersusun (%d entri, %d records). DIJEDA.\n", mergedTerms, len(papers))
	session.Status = "M8B_STEP1_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// ===== L2 =====

func (m *M8bBibliometric) runParamsL2(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 8b.2] Menyusun 9-parameter VOSviewer...")
	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return fmt.Errorf("brain (M8b params) gagal: %w", err)
	}
	records := 0
	if session.BibliometricData != nil {
		records = session.BibliometricData.RecordsAnalyzed
	}
	rqJSON, _ := json.Marshal(session.ResearchQuestions)
	params, err := agent.NewBibliometricAgent(brain).GenerateVOSParams(ctx, string(rqJSON), records)
	if err != nil {
		return err
	}
	session.VOSViewerParams = params
	logger.Log(session.ID, "   [System] 9-parameter tersusun. Menunggu user menjalankan VOSviewer + paste hasil.")
	session.Status = "M8B_STEP2_WAITING_VOSVIEWER"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// ===== L3 =====

func (m *M8bBibliometric) runInterpretL3(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 8b.3] Interpretasi cluster (Tier 1-4)...")
	if strings.TrimSpace(session.BibliometricInput) == "" {
		logger.Log(session.ID, "   [System] Input VOSviewer kosong; kembali menunggu.")
		session.Status = "M8B_STEP2_WAITING_VOSVIEWER"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}
	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return fmt.Errorf("brain (M8b interpret) gagal: %w", err)
	}
	ci, err := agent.NewBibliometricAgent(brain).InterpretClusters(ctx, session.BibliometricInput)
	if err != nil {
		return err
	}
	session.ClusterInterpretation = ci
	session.Status = "M8B_STEP3_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// ===== L4 =====

func (m *M8bBibliometric) runIntegrationL4(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [Langkah 8b.4] SLNA integration + summary...")
	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return fmt.Errorf("brain (M8b integration) gagal: %w", err)
	}

	clusterMd := ""
	if session.ClusterInterpretation != nil {
		clusterMd = session.ClusterInterpretation.Markdown + "\n\n" + session.ClusterInterpretation.TableMarkdown
	}
	slrSummary := ""
	if session.InterpretationPackage != nil {
		slrSummary = session.InterpretationPackage.Markdown
	} else if session.Modul8Summary != nil {
		slrSummary = session.Modul8Summary.Markdown
	}

	integ, err := agent.NewBibliometricAgent(brain).IntegrateSLNA(ctx, clusterMd, slrSummary)
	if err != nil {
		return err
	}
	session.SLNAIntegration = integ

	// Summary
	recs, thes := 0, ""
	if session.BibliometricData != nil {
		recs = session.BibliometricData.RecordsAnalyzed
		thes = session.BibliometricData.Approach
	}
	paramTbl := ""
	if session.VOSViewerParams != nil {
		paramTbl = session.VOSViewerParams.TableMarkdown
	}
	clusterTbl := ""
	if session.ClusterInterpretation != nil {
		clusterTbl = session.ClusterInterpretation.TableMarkdown
	}
	summary := fmt.Sprintf("=== BIBLIOMETRIC + SLNA SUMMARY ===\n\n"+
		"DATA PREPARATION:\n- Records: %d | Approach: %s\n\n"+
		"VOSVIEWER 9-PARAMETER:\n%s\n\n"+
		"CLUSTER INTERPRETATION:\n%s\n\n"+
		"SLNA INTEGRATION:\n%s\n\n"+
		"CONVERGENT GAPS (HIGH priority → Future Research Modul 9):\n%s\n\n"+
		"NEXT: Manuscript Writing (Modul 9) — sertakan tabel 9-parameter, subseksi Bibliometric Cluster + Integrated SLNA.",
		recs, thes, paramTbl, clusterTbl, integ.Markdown, integ.ConvergentGaps)
	session.ModulBibliometricSummary = &model.ModulBibliometricSummary{Markdown: summary}

	logger.Log(session.ID, "   [System] slna_integration + modul_bibliometric_summary tersimpan.")
	session.Status = "M8B_STEP4_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// ===== Helpers for keyword extraction =====

// academicStopwords contains common English stopwords plus academic filler words.
var academicStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "from": true,
	"that": true, "this": true, "are": true, "was": true, "were": true,
	"been": true, "have": true, "has": true, "will": true, "can": true,
	"not": true, "but": true, "its": true, "their": true, "our": true,
	"than": true, "into": true, "also": true, "each": true, "both": true,
	"more": true, "most": true, "some": true, "all": true, "new": true,
	"first": true, "two": true, "one": true, "very": true, "when": true,
	"only": true, "how": true, "where": true, "what": true, "used": true,
	"using": true, "based": true, "through": true, "under": true, "over": true,
	"about": true, "after": true, "before": true, "during": true, "such": true,
	"which": true, "these": true, "those": true, "other": true, "between": true,
	"use": true, "method": true, "approach": true, "proposed": true, "paper": true,
	"study": true, "results": true, "show": true, "analysis": true, "via": true,
	"novel": true,
}

// splitWordsRe splits text into words by non-letter/non-digit boundaries.
var splitWordsRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// extractTitleTerms extracts meaningful unigrams and bigrams from text.
// It splits by spaces/punctuation, lowercases, filters stopwords and short words (<4 chars),
// then generates bigrams from adjacent non-stopword words.
func extractTitleTerms(text string) []string {
	words := splitWordsRe.Split(text, -1)
	var filtered []string
	for _, w := range words {
		w = strings.ToLower(strings.TrimSpace(w))
		if len(w) < 4 {
			continue
		}
		// Must contain at least one letter
		hasLetter := false
		for _, r := range w {
			if unicode.IsLetter(r) {
				hasLetter = true
				break
			}
		}
		if !hasLetter {
			continue
		}
		if academicStopwords[w] {
			continue
		}
		filtered = append(filtered, w)
	}

	// Collect unigrams
	terms := make([]string, len(filtered))
	copy(terms, filtered)

	// Generate bigrams from adjacent non-stopword words
	for i := 0; i < len(filtered)-1; i++ {
		bigram := filtered[i] + " " + filtered[i+1]
		terms = append(terms, bigram)
	}
	return terms
}
