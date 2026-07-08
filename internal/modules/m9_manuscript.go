package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"nsa/internal/agent"
	"nsa/internal/llm"
	"nsa/internal/logger"
	"nsa/internal/model"
)

type M9Manuscript struct {
	deps *ModuleDeps
}

func NewM9Manuscript(deps *ModuleDeps) *M9Manuscript { return &M9Manuscript{deps: deps} }

func (m *M9Manuscript) Name() string { return "M9_MANUSCRIPT" }

func (m *M9Manuscript) Execute(ctx context.Context, session *model.SLRSession) error {
	logger.Logf(session.ID, ">> [MODUL 9: MANUSCRIPT] State: %s\n", session.Status)
	ctx = llm.WithXAIContext(ctx, session.ID, session.Status, "M9Manuscript")

	switch session.Status {
	case "M9_MANUSCRIPT", "M9_INIT":
		if session.Manuscript == nil {
			session.Manuscript = &model.Manuscript{}
		}
		session.Status = "M9_GROUPA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// Generic revision fallback: reset to Group A (start of M9)
	case "M9_NEEDS_REVISION":
		logger.Logf(session.ID, "   [Revisi M9] Reset ke Group A. Feedback: '%s'\n", session.Feedback)
		session.Manuscript = &model.Manuscript{}
		session.Feedback = ""
		session.Status = "M9_GROUPA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- GROUP A: Methods + Results + Discussion + Future Research ----
	case "M9_GROUPA":
		return m.runGroup(ctx, session, "M9_GROUPA_WAITING_EMBED", m.generateGroupA)
	case "M9_GROUPA_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau Methods/Results/Discussion/Future Research. Approve / revisi.")
		return nil
	case "M9_GROUPA_NEEDS_REVISION":
		session.Feedback = ""
		session.Status = "M9_GROUPA"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M9_GROUPA_APPROVED":
		session.Status = "M9_GROUPB"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// Server verifikasi (BGE-M3 hybrid) mati saat menulis -> tunggu user menyalakan
	// Colab lagi, lalu resume ke group tertunda (lihat session_handler approve).
	case "M9_GROUPA_WAITING_EMBED", "M9_GROUPB_WAITING_EMBED", "M9_COMPILE_WAITING_EMBED":
		logger.Log(session.ID, "   [System] M9 dijeda: server embedding/pencarian BGE-M3 (hybrid) mati. Nyalakan Colab, masukkan endpoint via web, lalu lanjutkan.")
		return nil

	// Provider penulis manuskrip (Brain/Supervisor) gagal SISTEMIK (rate-limit/overload/
	// kuota/koneksi) -> gerbang recoverable (mirror M7_STEP3_QA_BLOCKED). Passive: tahan di
	// sini sampai user perbaiki/ganti provider lalu 'Lanjutkan Manuskrip' (ApproveStep melepas
	// _BLOCKED -> resume fase). TIDAK me-reset manuskrip yang sudah ditulis.
	case "M9_GROUPA_BLOCKED", "M9_GROUPB_BLOCKED", "M9_COMPILE_BLOCKED":
		logger.Log(session.ID, "   [System] ⛔ M9 dijeda: provider penulis manuskrip gagal sistemik. Perbaiki/ganti provider Brain/Supervisor di Pengaturan LLM lalu 'Lanjutkan Manuskrip' (lihat system_error).")
		return nil

	// ---- GROUP B: Introduction + Conclusions + Abstract + Title ----
	case "M9_GROUPB":
		return m.runGroup(ctx, session, "M9_GROUPB_WAITING_EMBED", m.generateGroupB)
	case "M9_GROUPB_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau Introduction/Conclusions/Abstract/Title. Approve / revisi.")
		return nil
	case "M9_GROUPB_NEEDS_REVISION":
		session.Feedback = ""
		session.Status = "M9_GROUPB"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M9_GROUPB_APPROVED":
		session.Status = "M9_COMPILE"
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	// ---- COMPILE: references + audit + prisma + final + latex + summary ----
	case "M9_COMPILE":
		err := m.runCompile(ctx, session)
		var be *EmbedBackendDownError
		if errors.As(err, &be) {
			logger.Logf(session.ID, "   [PAUSE] M9 (compile) dijeda — verifikasi sitasi butuh server hybrid: %s\n", be.Reason)
			session.Status = "M9_COMPILE_WAITING_EMBED"
			session.EmbedError = be.Reason
			return m.deps.MongoRepo.UpdateSession(ctx, session)
		}
		if isSystemicLLMError(err) {
			return m.blockM9Systemic(ctx, session, "M9_COMPILE_BLOCKED", err)
		}
		return err
	case "M9_COMPILE_WAITING_APPROVAL":
		logger.Log(session.ID, "   [System] Tinjau manuscript_final (+.tex/.bib), coherence_audit, PRISMA checklist. Approve untuk menutup pipeline.")
		return nil
	case "M9_COMPILE_NEEDS_REVISION":
		session.Feedback = ""
		session.Status = "M9_COMPILE"
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	case "M9_COMPILE_APPROVED":
		// Manuskrip final siap → masuk GERBANG AUDIT (Modul 10) sebelum COMPLETED.
		session.Status = "M10_STEP1_AUDIT"
		logger.Log(session.ID, "   [System] Manuskrip SLR final siap → menjalankan Audit Pra-Submisi (Modul 10).")
		return m.deps.MongoRepo.UpdateSession(ctx, session)

	default:
		return nil
	}
}

// ===== Multi-pass pipeline =====

// generateWithMultiPass orchestrates a 3-pass generation pipeline:
// Pass 1: Initial draft generation with RAG data + paper catalog.
// Pass 2: Claim verification -- cross-references claims against Qdrant, Neo4j, and MongoDB,
//
//	then asks LLM to remove/flag unverified claims and strengthen verified ones.
//
// Pass 3: Style cleanup -- removes AI-like phrasing, em dashes, and redundancies.
func (m *M9Manuscript) generateWithMultiPass(ctx context.Context, ag *agent.ManuscriptAgent, session *model.SLRSession, section, systemPrompt, userBundle, lang string, citations []PaperCitation) (string, error) {
	// The allowed-keys list is supplied to passes 2 and 3 so the LLM corrects \cite{}
	// against the real bibliography instead of inventing decorated/descriptive keys.
	allowedKeys := buildAllowedKeysList(citations)

	// Pass 1: Generate initial draft
	draft, err := ag.Write(ctx, systemPrompt+lang, userBundle)
	if err != nil {
		return "", fmt.Errorf("multi-pass P1 (draft): %w", err)
	}

	// Pass 2: Verify claims and ask LLM to fix unverified/invalid ones.
	// Bila backend hybrid mati, verifyClaims mengembalikan EmbedBackendDownError -->
	// propagasi ke runGroup supaya M9 MENJEDA (bukan menutup manuskrip dgn
	// verifikasi terdegradasi).
	verResults, err := m.verifyClaims(ctx, session, draft, citations)
	if err != nil {
		return "", err
	}
	// xAI: SIMPAN bukti verifikasi per-section (durable + dapat diekspor), bukan hanya
	// dipakai sekali lalu dibuang. Menutup celah audit M9 (klaim mana yang gagal triangulasi).
	setSectionVerifications(session.Manuscript, section, verResults)
	verSummary := formatVerificationResults(verResults)
	verifiedDraft, err := ag.Write(ctx, promptVerification+lang, draft+allowedKeys+verSummary)
	if err != nil {
		return "", fmt.Errorf("multi-pass P2 (verify): %w", err)
	}

	// Pass 3: Style cleanup (keeps the allowed-keys list in view so style edits
	// never reintroduce or mangle citation keys)
	finalDraft, err := ag.Write(ctx, promptStyleCleanup+lang, verifiedDraft+allowedKeys)
	if err != nil {
		return "", fmt.Errorf("multi-pass P3 (style): %w", err)
	}

	// Pass 4 (deterministic): enforce that every \cite{} resolves to a real catalog
	// key. This is model-agnostic and guarantees a compilable bibliography even if a
	// lower-capability Brain model invented keys that survived passes 1-3.
	cleaned, stats := sanitizeCitations(finalDraft, citations)
	logCiteGuard(session.ID, systemPrompt, stats)
	return cleaned, nil
}

// syncEmbedEnv menjembatani embed_config DB (diisi user via web: "Simpan Endpoint & Lanjut")
// ke ENV yang dibaca jalur RAG/hybrid (requireHybrid → CheckSearchBackend, SemanticSearch).
// Tanpa ini, endpoint tersimpan di Mongo TAPI M9 membaca os.Getenv → "belum diset" → loop di
// WAITING_EMBED walau user sudah memasukkan endpoint valid (lapor balqis). M6 sudah benar
// (baca GetEmbedConfig langsung); ini menyamakan perilaku M9.
func (m *M9Manuscript) syncEmbedEnv(ctx context.Context) {
	ec := m.deps.MongoRepo.GetEmbedConfig(ctx)
	if ec == nil {
		return
	}
	if v := strings.TrimSpace(ec.Endpoint); v != "" {
		os.Setenv("EMBED_ENDPOINT", v)
	}
	if v := strings.TrimSpace(ec.APIKey); v != "" {
		os.Setenv("EMBED_API_KEY", v)
	}
	if v := strings.TrimSpace(ec.Model); v != "" {
		os.Setenv("EMBED_MODEL", v)
	}
}

// setSectionVerifications MENGGANTI entri verifikasi untuk satu section (idempoten saat
// re-run/revisi) lalu menyimpan bukti triangulasi 3-sumber ke manuskrip (xAI durable).
func setSectionVerifications(ms *model.Manuscript, section string, vrs []VerificationResult) {
	if ms == nil {
		return
	}
	kept := ms.ClaimVerifications[:0]
	for _, cv := range ms.ClaimVerifications {
		if cv.Section != section {
			kept = append(kept, cv)
		}
	}
	ms.ClaimVerifications = kept
	for _, vr := range vrs {
		ms.ClaimVerifications = append(ms.ClaimVerifications, model.ClaimVerification{
			Section:        section,
			Claim:          vr.Claim,
			CitationKey:    vr.CitationKey,
			QdrantVerified: vr.QdrantVerified,
			Neo4jVerified:  vr.Neo4jVerified,
			MongoVerified:  vr.MongoVerified,
			Sources:        vr.Sources,
		})
	}
}

// logCiteGuard surfaces what the deterministic citation guard changed so it is
// visible in the approval-stage log (not just silently rewritten).
func logCiteGuard(sessionID, systemPrompt string, stats CiteGuardStats) {
	if stats.Remapped == 0 && stats.Dropped == 0 {
		logger.Logf(sessionID, "      [CiteGuard] %d/%d \\cite keys valid (catalog-clean)", stats.Valid, stats.Total)
		return
	}
	logger.Logf(sessionID, "      [CiteGuard] %d valid, %d remapped, %d dropped (of %d)",
		stats.Valid, stats.Remapped, stats.Dropped, stats.Total)
	for invalid, mapped := range stats.RemapPairs {
		logger.Logf(sessionID, "         remap  \\cite{%s} -> \\cite{%s}", invalid, mapped)
	}
	if len(stats.DroppedSet) > 0 {
		dropped := make([]string, 0, len(stats.DroppedSet))
		for k := range stats.DroppedSet {
			dropped = append(dropped, k)
		}
		logger.Logf(sessionID, "         dropped (no catalog match): %s", strings.Join(dropped, ", "))
	}
}

// ===== Group generators =====

// runGroup menjalankan generator group dan mengubah EmbedBackendDownError menjadi
// JEDA (pause) + status WAITING, sehingga user diberi tahu untuk menyalakan kembali
// server embedding/pencarian — menjamin verifikasi sitasi selalu hybrid (Q1).
func (m *M9Manuscript) runGroup(ctx context.Context, session *model.SLRSession, waitStatus string, fn func(context.Context, *model.SLRSession) error) error {
	err := fn(ctx, session)
	var be *EmbedBackendDownError
	if errors.As(err, &be) {
		logger.Logf(session.ID, "   [PAUSE] M9 dijeda — verifikasi sitasi butuh server hybrid: %s\n", be.Reason)
		session.Status = waitStatus
		session.EmbedError = be.Reason
		return m.deps.MongoRepo.UpdateSession(ctx, session)
	}
	// Kegagalan LLM SISTEMIK (rate-limit/overload/ResourceExhausted/context/koneksi) akan
	// berulang identik tiap bagian → JANGAN grinding retry berkepanjangan (tampak beku, tiket
	// Sindy) → buka gerbang recoverable *_BLOCKED. waitStatus "M9_GROUPA_WAITING_EMBED" →
	// fase "M9_GROUPA" → gerbang "M9_GROUPA_BLOCKED".
	if isSystemicLLMError(err) {
		phase := strings.TrimSuffix(waitStatus, "_WAITING_EMBED")
		return m.blockM9Systemic(ctx, session, phase+"_BLOCKED", err)
	}
	return err
}

// blockM9Systemic membuka GERBANG HITL M9 yang bisa dipulihkan saat provider penulis manuskrip
// (Brain/Supervisor) gagal SISTEMIK — mirror pola M7_STEP3_QA_BLOCKED. Passive: pipeline
// berhenti dgn pesan jelas (sebut model + dugaan akar + langkah), user perbaiki/ganti provider
// lalu 'Lanjutkan Manuskrip' (ApproveStep melepas akhiran _BLOCKED → resume fase tertunda).
func (m *M9Manuscript) blockM9Systemic(ctx context.Context, session *model.SLRSession, blockStatus string, err error) error {
	hint := "perbaiki/ganti provider Brain/Supervisor di Pengaturan LLM lalu 'Lanjutkan Manuskrip'"
	switch {
	case isServerOverloadError(err):
		hint = "provider sedang rate-limit/overload/kuota habis (mis. 429/503/ResourceExhausted) — tunggu beberapa menit ATAU ganti provider Brain/Supervisor ke yang lebih lapang di Pengaturan LLM, lalu 'Lanjutkan Manuskrip'"
	case isContextOverflowError(err):
		hint = "prompt manuskrip melebihi context window model — pakai model Brain context lebih besar di Pengaturan LLM, lalu 'Lanjutkan Manuskrip'"
	case isLLMConnectivityError(err):
		hint = "endpoint provider tak terjangkau — pastikan server/gateway LLM berjalan & base URL benar di Pengaturan LLM, lalu 'Lanjutkan Manuskrip'"
	}
	modelAttr := ""
	if session.Manuscript != nil && session.Manuscript.ModelUsed != "" {
		modelAttr = " (" + session.Manuscript.ModelUsed + ")"
	}
	msg := fmt.Sprintf("Penulisan manuskrip (M9) dijeda: provider penulis%s gagal sistemik — %s. Penyebab ini berulang di tiap bagian, jadi dihentikan agar tak menggiling lama & tampak macet. %s.",
		modelAttr, clipErr(err.Error()), hint)
	session.Status = blockStatus
	session.SystemError = msg
	logger.Logf(session.ID, "   [HALT] %s\n", msg)
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

func (m *M9Manuscript) generateGroupA(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [L1-L4] Menulis Methods, Results, Discussion, Future Research...")
	// Pra-syarat Q1: server verifikasi hybrid harus hidup SEBELUM menulis (hemat
	// token Brain bila ternyata mati -> langsung jeda via runGroup).
	m.syncEmbedEnv(ctx) // jembatani embed_config DB -> ENV sebelum cek hybrid (fix loop WAITING_EMBED)
	if reason := requireHybrid(ctx); reason != "" {
		return &EmbedBackendDownError{Reason: reason}
	}
	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return m.deps.llmError(ctx, "brain", "Memuat client manuskrip M9", err)
	}
	ag := agent.NewManuscriptAgent(brain)
	if session.Manuscript == nil {
		session.Manuscript = &model.Manuscript{}
	}
	session.Manuscript.ModelUsed = brain.ModelName() // atribusi xAI (nama model asli)
	bundle := m.artifactBundle(session)
	lang := langDirective(session)

	// Fetch per-paper data and build citation catalog for structured referencing
	papers := m.fetchExtractionData(ctx, session.ID)
	citations := m.buildPaperCatalog(ctx, session)
	paperBundle := buildPaperDataBundle(papers, citations)

	// Integrate Qdrant fulltext snippets for top papers in the catalog
	fulltextCtx := m.buildFulltextSnippets(ctx, session.ID, citations)

	userCtx := bundle + m.prismaContext(ctx, session) + paperBundle + fulltextCtx

	// Multi-pass: Methods
	methods, err := m.generateWithMultiPass(ctx, ag, session, "Methods", promptMethods, userCtx, lang, citations)
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Methods (3-pass)")

	// Multi-pass: Results
	results, err := m.generateWithMultiPass(ctx, ag, session, "Results", promptResults, userCtx+sectionCtx("METHODS (sudah ditulis)", methods), lang, citations)
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Results (3-pass)")

	// Multi-pass: Discussion
	discussion, err := m.generateWithMultiPass(ctx, ag, session, "Discussion", promptDiscussion, userCtx+sectionCtx("RESULTS (sudah ditulis -- jangan diulang)", results), lang, citations)
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Discussion (3-pass)")

	// Multi-pass: Future Research
	future, err := m.generateWithMultiPass(ctx, ag, session, "Future Research", promptFuture, userCtx+sectionCtx("DISCUSSION (limitations -- agenda harus beda/actionable)", discussion), lang, citations)
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Future Research (3-pass)")

	session.Manuscript.Methods = methods
	session.Manuscript.Results = results
	session.Manuscript.Discussion = discussion
	session.Manuscript.FutureResearch = future
	session.Status = "M9_GROUPA_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

func (m *M9Manuscript) generateGroupB(ctx context.Context, session *model.SLRSession) error {
	logger.Log(session.ID, "   [L5-L9] Menulis Introduction, Conclusions, Abstract, Title...")
	// Pra-syarat Q1: server verifikasi hybrid harus hidup sebelum menulis.
	m.syncEmbedEnv(ctx) // jembatani embed_config DB -> ENV sebelum cek hybrid (fix loop WAITING_EMBED)
	if reason := requireHybrid(ctx); reason != "" {
		return &EmbedBackendDownError{Reason: reason}
	}
	brain, err := m.deps.LLMFactory.BrainClient(ctx)
	if err != nil {
		return m.deps.llmError(ctx, "brain", "Memuat client manuskrip M9", err)
	}
	ag := agent.NewManuscriptAgent(brain)
	ms := session.Manuscript
	if ms != nil {
		ms.ModelUsed = brain.ModelName() // atribusi xAI (nama model asli)
	}
	bundle := m.artifactBundle(session)
	lang := langDirective(session)

	// Fetch per-paper data and build citation catalog for structured referencing
	papers := m.fetchExtractionData(ctx, session.ID)
	citations := m.buildPaperCatalog(ctx, session)
	paperBundle := buildPaperDataBundle(papers, citations)

	// Integrate Qdrant fulltext snippets for top papers in the catalog
	fulltextCtx := m.buildFulltextSnippets(ctx, session.ID, citations)

	userCtx := bundle + m.prismaContext(ctx, session) + paperBundle + fulltextCtx

	// Multi-pass: Introduction
	intro, err := m.generateWithMultiPass(ctx, ag, session, "Introduction", promptIntro, userCtx+sectionCtx("RESULTS (untuk tune preview, JANGAN bocorkan angka spesifik di Intro)", trim(ms.Results, 4000)), lang, citations)
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Introduction (3-pass)")

	// Multi-pass: Conclusions
	conclusions, err := m.generateWithMultiPass(ctx, ag, session, "Conclusions", promptConclusions, userCtx+sectionCtx("DISCUSSION", trim(ms.Discussion, 5000))+sectionCtx("FUTURE RESEARCH", trim(ms.FutureResearch, 2500)), lang, citations)
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Conclusions (3-pass)")

	// Multi-pass: Abstract
	abstract, err := m.generateWithMultiPass(ctx, ag, session, "Abstract", promptAbstract, userCtx+sectionCtx("METHODS", trim(ms.Methods, 3000))+sectionCtx("RESULTS", trim(ms.Results, 4000))+sectionCtx("DISCUSSION", trim(ms.Discussion, 3000)), lang, citations)
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Abstract (3-pass)")

	// Title uses single-pass (no citations to verify in a title)
	title, err := ag.Write(ctx, promptTitle+lang, sectionCtx("ABSTRACT", abstract)+m.geoFrameworkHint(session))
	if err != nil {
		return err
	}
	logger.Log(session.ID, "      ✓ Title")

	ms.Introduction = intro
	ms.Conclusions = conclusions
	ms.Abstract = abstract
	ms.Title = title
	session.Status = "M9_GROUPB_WAITING_APPROVAL"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}

// ===== context helpers =====

// buildFulltextSnippets uses BuildFulltextIndex to get fulltext for included papers,
// then returns a context block with trimmed snippets (first 500 chars) for each paper
// in the catalog that has fulltext available in Qdrant.
func (m *M9Manuscript) buildFulltextSnippets(ctx context.Context, sessionID string, citations []PaperCitation) string {
	if len(citations) == 0 {
		return ""
	}

	index, available, err := BuildFulltextIndex(ctx)
	if !available || err != nil || len(index) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n== RAG FULLTEXT SNIPPETS (evidence from papers) ==\n")
	count := 0
	for _, c := range citations {
		if c.DOI == "" {
			continue
		}
		normalized := normalizeDOIForRAG(c.DOI)
		fulltext, ok := index[normalized]
		if !ok || fulltext == "" {
			continue
		}
		// Trim to first 500 chars
		snippet := fulltext
		if len(snippet) > 500 {
			snippet = snippet[:500] + "..."
		}
		b.WriteString(fmt.Sprintf("[%s] %s\n", c.Key, snippet))
		count++
	}

	if count == 0 {
		return ""
	}
	b.WriteString("== END RAG FULLTEXT ==\n")
	logger.Logf(sessionID, "      [RAG] Included fulltext snippets for %d/%d papers", count, len(citations))
	return b.String()
}

// langDirective menentukan bahasa output manuskrip (default Bahasa Indonesia untuk draft).
func langDirective(session *model.SLRSession) string {
	lang := strings.ToLower(strings.TrimSpace(session.ManuscriptLang))
	if lang == "en" || lang == "english" || lang == "inggris" {
		return "\n\nLANGUAGE: Write the entire section in formal, publication-quality academic English."
	}
	return "\n\nBAHASA: Tulis SELURUH section dalam Bahasa Indonesia akademik formal (kualitas jurnal). Istilah metodologi baku boleh tetap Inggris ('systematic review', 'PRISMA 2020', 'PICO', 'meta-analysis', nama tool RoB seperti RoB 2/NOS/MMAT). Judul karya & daftar referensi JANGAN diterjemahkan."
}

func sectionCtx(label, content string) string {
	if content == "" {
		return ""
	}
	return fmt.Sprintf("\n\n=== %s ===\n%s\n", label, content)
}

func trim(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func (m *M9Manuscript) geoFrameworkHint(session *model.SLRSession) string {
	fw := frameworkName(session)
	path := "JALUR A (narrative)"
	if session.SynthesisPathDecision != nil && session.SynthesisPathDecision.Verdict != "" {
		path = session.SynthesisPathDecision.Verdict
	}
	return fmt.Sprintf("\n\n=== KONTEKS TITLE ===\nFramework: %s | Synthesis path: %s | Topik: %s\n", fw, path, session.Topic)
}

// artifactBundle merangkai artefak M2-M8 sebagai konteks TEKS RINGKAS (bukan JSON penuh)
// agar prompt kecil/fokus (penting untuk backend lambat seperti rprompt/claude headless).
func (m *M9Manuscript) artifactBundle(session *model.SLRSession) string {
	var b strings.Builder
	w := func(label, val string) {
		if strings.TrimSpace(val) != "" {
			fmt.Fprintf(&b, "## %s\n%s\n\n", label, val)
		}
	}
	b.WriteString("=== ARTEFAK SLR (sumber data; pakai angka & kutipan APA ADANYA) ===\n\n")
	w("TOPIC", session.Topic)

	if t := session.SelectedTopic; t != nil {
		w("GAP", fmt.Sprintf("%s | Tipe %s | %s", t.Name, t.Type, t.Gap))
	}
	if p := session.PICODefinitions; p != nil {
		w("PICO", fmt.Sprintf("P: %s\nI: %s\nC: %s\nO: %s\nCanonical term: %s — %s",
			p.P.Value, p.I.Value, p.C.Value, p.O.Value, p.CanonicalTerm.Term, p.CanonicalTerm.Definition))
	}
	if len(session.ResearchQuestions) > 0 {
		var rq strings.Builder
		for _, q := range session.ResearchQuestions {
			fmt.Fprintf(&rq, "- [%s] %s\n", q.Type, q.Question)
		}
		w("RESEARCH QUESTIONS", rq.String())
	}
	if pr := session.PriorReviewsMatrix; pr != nil {
		var s strings.Builder
		for _, r := range pr.Reviews {
			fmt.Fprintf(&s, "- %s | Scope: %s | Method: %s | Findings: %s | Limitations: %s | Selisih: %s\n",
				r.AuthorYear, r.Scope, r.Methodology, r.KeyFindings, r.Limitations, r.Selisih)
		}
		w("PRIOR REVIEWS", s.String())
	}
	if len(session.ScopeJustifications) > 0 {
		var s strings.Builder
		for _, sj := range session.ScopeJustifications {
			fmt.Fprintf(&s, "- %s: T=%s; M=%s; P=%s\n", sj.Name, sj.Theoretical, sj.Methodological, sj.Practical)
		}
		w("SCOPE JUSTIFICATIONS", s.String())
	}
	if sl := session.SearchLog; sl != nil {
		dates, _ := json.Marshal(sl.DateExecuted)
		hits, _ := json.Marshal(sl.TotalHits)
		w("SEARCH", fmt.Sprintf("String: %s\nDatabases: %s\nDate: %s | Hits: %s\nUpdate policy: %s",
			sl.SearchStringFinal, strings.Join(sl.Databases, ", "), string(dates), string(hits), sl.UpdatePolicy))
	}
	if et := session.ExclusionTable; et != nil {
		w("PRISMA FLOW + EXCLUSION + KAPPA", et.FlowNumbers+"\n\n"+et.KappaReport+"\n\n"+et.ExclusionReasons)
	}
	kappaTA := 0.0
	if n := len(session.KalibrasiLog); n > 0 {
		kappaTA = session.KalibrasiLog[n-1].Kappa
	}
	extK, robK := 0.0, 0.0
	if session.ExtractionLog != nil {
		extK = session.ExtractionLog.ExtractionKappa
	}
	if session.QAThreshold != nil {
		robK = session.QAThreshold.Kappa
	}
	w("KAPPA VALUES", fmt.Sprintf("κ_TA=%.3f | κ_FT=%.3f | κ_extract=%.3f | κ_rob=%.3f", kappaTA, session.FulltextKappa, extK, robK))

	if fw := session.FrameworkSelection; fw != nil {
		w("FRAMEWORK", fw.Framework+" — "+fw.Justification)
	}
	if el := session.ExtractionLog; el != nil {
		w("EXTRACTION", fmt.Sprintf("Total %d | verifikasi %d | disagreement %.1f%%", el.TotalExtracted, el.VerifiedSample, el.DisagreementRate))
	}
	if qa := session.QAThreshold; qa != nil {
		w("QUALITY APPRAISAL", fmt.Sprintf("Tool %s | threshold %.0f%% | %s | tool justification: %s", qa.Tool, qa.Threshold, qa.Categorization, qa.ToolJustification))
	}
	if s := session.SensitivityAnalysis; s != nil {
		w("SENSITIVITY", s.Verdict+" — "+s.Markdown)
	}
	if sp := session.SynthesisPathDecision; sp != nil {
		w("SYNTHESIS PATH", sp.Verdict+" — "+sp.Rationale)
	}
	if sr := session.SynthesisResults; sr != nil {
		w("SYNTHESIS RESULTS", sr.Markdown)
	}
	if g := session.GradeEvidence; g != nil {
		w("GRADE EVIDENCE", g.TableMarkdown+"\nRobustness: "+g.RobustnessVerdict+"\n"+g.ConfidenceStatements)
	}
	if d := session.DescriptiveAnalysis; d != nil {
		w("DESCRIPTIVE", d.Markdown+"\nHeterogeneity: "+d.HeterogeneityVerdict+"\n"+d.HeterogeneityNarrative)
	}
	if ip := session.InterpretationPackage; ip != nil {
		w("INTERPRETATION PACKAGE", ip.Markdown)
	}
	if ac := session.AcquisitionLog; ac != nil {
		w("ACQUISITION", fmt.Sprintf("Included %d | inaccessible %d (%.1f%%)", ac.TotalInclude, ac.InaccessibleCount, ac.InaccessiblePct))
	}
	if si := session.SLNAIntegration; si != nil {
		w("SLNA INTEGRATION", si.Markdown+"\nConvergent gaps: "+si.ConvergentGaps)
	}
	return b.String()
}

// buildPaperDataBundle formats per-paper extraction data and citation keys into a
// structured catalog that is appended to the user prompt. This allows the LLM to
// write per-claim citations using \cite{authorYear} keys from the catalog.
func buildPaperDataBundle(papers []ExtractionPaperData, citations []PaperCitation) string {
	if len(citations) == 0 && len(papers) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n== PAPER CATALOG (use \\cite{key} to cite) ==\n")

	// Build a map from DOI to extraction data for quick lookup
	findingsMap := make(map[string]string)
	fieldsMap := make(map[string]string)
	for _, p := range papers {
		if p.DOI != "" {
			if p.KeyFindings != "" {
				findingsMap[p.DOI] = p.KeyFindings
			}
			if len(p.Fields) > 0 {
				var fs []string
				for _, f := range p.Fields {
					if f.Value != "" {
						fs = append(fs, f.Key+": "+f.Value)
					}
				}
				if len(fs) > 0 {
					fieldsMap[p.DOI] = strings.Join(fs, "; ")
				}
			}
		}
	}

	for _, c := range citations {
		line := fmt.Sprintf("[%s]: %s (%s). %s.", c.Key, c.Authors, c.Year, c.Title)
		if c.Journal != "" {
			line += " " + c.Journal + "."
		}
		if c.DOI != "" {
			line += " DOI: " + c.DOI
		}
		// Append findings from extraction data if available
		if findings, ok := findingsMap[c.DOI]; ok {
			line += " | Findings: " + findings
		}
		if fields, ok := fieldsMap[c.DOI]; ok {
			line += " | Fields: " + fields
		}
		b.WriteString(line + "\n")
	}

	// Include papers from extraction that might not be in citation catalog yet
	citedDOIs := make(map[string]bool)
	for _, c := range citations {
		if c.DOI != "" {
			citedDOIs[c.DOI] = true
		}
	}
	for _, p := range papers {
		if p.DOI != "" && !citedDOIs[p.DOI] {
			key := generateCitationKey(p.Authors, p.Year)
			line := fmt.Sprintf("[%s]: %s (%s). %s.", key, p.Authors, p.Year, p.Title)
			if p.Journal != "" {
				line += " " + p.Journal + "."
			}
			if p.DOI != "" {
				line += " DOI: " + p.DOI
			}
			if p.KeyFindings != "" {
				line += " | Findings: " + p.KeyFindings
			}
			b.WriteString(line + "\n")
		}
	}

	b.WriteString("== END PAPER CATALOG ==\n")
	return b.String()
}

// fetchExtractionData queries the slr_extraction collection and returns per-paper
// extraction data for the given session. This provides granular paper-level data
// (not just aggregated summaries) for manuscript prompts.
func (m *M9Manuscript) fetchExtractionData(ctx context.Context, sessionID string) []ExtractionPaperData {
	coll := m.deps.MongoRepo.GetExtractionCollection()
	cursor, err := coll.Find(ctx, bson.M{"session_id": sessionID})
	if err != nil {
		return nil
	}
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil
	}

	var results []ExtractionPaperData
	for _, doc := range docs {
		extracted, _ := doc["extracted"].(bool)
		if !extracted {
			continue
		}

		epd := ExtractionPaperData{
			DOI:         getStr(doc, "doi", "DOI"),
			Title:       getStr(doc, "title", "Title"),
			Authors:     getStr(doc, "authors", "Authors"),
			Year:        getStr(doc, "year", "Year"),
			Journal:     getStr(doc, "journal", "Journal"),
			KeyFindings: getStr(doc, "key_findings"),
			QARedFlags:  getStr(doc, "qa_red_flags"),
		}

		// Parse fields array (may be primitive.A from bson decode)
		if fields, ok := doc["fields"]; ok {
			var arr []interface{}
			switch v := fields.(type) {
			case primitive.A:
				arr = []interface{}(v)
			case []interface{}:
				arr = v
			}
			for _, f := range arr {
				var fMap bson.M
				switch fm := f.(type) {
				case bson.M:
					fMap = fm
				}
				if fMap != nil {
					ef := ExtractedField{
						Key:      getStr(fMap, "key"),
						Value:    getStr(fMap, "value"),
						Evidence: getStr(fMap, "evidence"),
						Status:   getStr(fMap, "status"),
					}
					epd.Fields = append(epd.Fields, ef)
				}
			}
		}

		results = append(results, epd)
	}
	return results
}

// buildPaperCatalog collects all included papers from the screening collection and
// generates a citation key for each (format: firstAuthorSurnameYear, e.g., zhang2025).
// Duplicate keys are deduplicated with a/b/c suffixes.
// Falls back to extraction collection if screening filter is too restrictive.
func (m *M9Manuscript) buildPaperCatalog(ctx context.Context, session *model.SLRSession) []PaperCitation {
	papers, err := m.deps.MongoRepo.GetAllScreeningPapers(ctx, session.ID)
	if err != nil {
		return nil
	}

	var catalog []PaperCitation
	for _, p := range papers {
		// Try strict filter first: full_text_retrieved + INCLUDE
		retrieved, _ := p["full_text_retrieved"].(bool)
		incAbs := getStr(p, "Final_Decision") == "INCLUDE" ||
			(getStr(p, "Final_Decision") == "" && getStr(p, "Screener_1_Decision") == "INCLUDE")
		if !(retrieved && incAbs && finalFullDecision(p) == "INCLUDE") {
			continue
		}

		authors := getStr(p, "Authors", "authors")
		year := getStr(p, "Year", "year")

		catalog = append(catalog, PaperCitation{
			Key:     generateCitationKey(authors, year),
			Authors: authors,
			Title:   getStr(p, "Title", "title"),
			Year:    year,
			Journal: getStr(p, "Journal", "journal"),
			DOI:     getStr(p, "DOI", "doi"),
		})
	}

	// Fallback: if strict filter yields too few papers, also try extraction docs
	if len(catalog) < 5 {
		logger.Logf(session.ID, "      [Catalog] Strict filter: %d papers. Trying extraction fallback...", len(catalog))
		extPapers := m.fetchExtractionData(ctx, session.ID)
		existingDOIs := make(map[string]bool)
		for _, c := range catalog {
			if c.DOI != "" {
				existingDOIs[strings.ToLower(c.DOI)] = true
			}
		}
		for _, ep := range extPapers {
			if ep.DOI != "" && !existingDOIs[strings.ToLower(ep.DOI)] {
				catalog = append(catalog, PaperCitation{
					Key:     generateCitationKey(ep.Authors, ep.Year),
					Authors: ep.Authors,
					Title:   ep.Title,
					Year:    ep.Year,
					Journal: ep.Journal,
					DOI:     ep.DOI,
				})
				existingDOIs[strings.ToLower(ep.DOI)] = true
			}
		}
		logger.Logf(session.ID, "      [Catalog] After extraction fallback: %d papers total.", len(catalog))
	}

	// Deduplicate keys with a/b/c suffixes
	deduplicateCitationKeys(catalog)

	return catalog
}

// generateCitationKey creates a citation key from the first author surname and year.
// E.g., "Zhang, L.; Wang, H." + "2025" => "zhang2025".
func generateCitationKey(authors, year string) string {
	surname := extractFirstSurname(authors)
	if surname == "" {
		surname = "unknown"
	}
	if year == "" {
		year = "0000"
	}
	return surname + year
}

// extractFirstSurname extracts the first author's surname in lowercase.
// Handles many formats:
//   - "Zhang, L.; Wang, H."        → zhang
//   - "Y. Gao; X. Wu"              → gao
//   - "L. Zhang"                    → zhang
//   - "Y. Gao, X. Wu, J. Lu"       → gao
//   - "Gao Y., Wu X."              → gao
//   - "Gao, Y."                    → gao
func extractFirstSurname(authors string) string {
	if authors == "" {
		return ""
	}

	// Split by common multi-author separators to get FIRST author only
	first := authors
	for _, sep := range []string{";", " and ", " & ", "., "} {
		if idx := strings.Index(first, sep); idx > 0 {
			first = first[:idx]
		}
	}
	first = strings.TrimSpace(first)

	// If there's a comma, check if it's "Surname, Init" or "Init1, Init2, Surname"
	if idx := strings.Index(first, ","); idx > 0 {
		beforeComma := strings.TrimSpace(first[:idx])
		// If before comma is short (1-3 chars like "Y" or "Y."), it's probably initial
		// and surname is after comma — but this is rare. Usually "Zhang, L." format.
		if len(beforeComma) <= 3 {
			// Format: "Y., Zhang" or similar — take after comma
			afterComma := strings.TrimSpace(first[idx+1:])
			parts := strings.Fields(afterComma)
			if len(parts) > 0 {
				first = parts[len(parts)-1]
			}
		} else {
			// Format: "Zhang, L." — surname is before comma
			first = beforeComma
		}
	} else {
		// No comma: could be "Y. Gao" or "Gao Y." or just "Gao"
		parts := strings.Fields(first)
		if len(parts) > 1 {
			// Check if first part looks like initial (short, has dot)
			if len(parts[0]) <= 3 || strings.Contains(parts[0], ".") {
				// "Y. Gao" or "Y Gao" — surname is last
				first = parts[len(parts)-1]
			} else if len(parts[len(parts)-1]) <= 3 || strings.Contains(parts[len(parts)-1], ".") {
				// "Gao Y." — surname is first
				first = parts[0]
			} else {
				// Ambiguous, take last word as surname (Western convention)
				first = parts[len(parts)-1]
			}
		}
		// If single word, use it as-is
	}

	// Clean: lowercase, only letters
	first = strings.TrimRight(first, ".,;: ")
	var sb strings.Builder
	for _, r := range strings.ToLower(first) {
		if unicode.IsLetter(r) {
			sb.WriteRune(r)
		}
	}
	result := sb.String()
	if result == "" {
		return ""
	}
	return result
}

// deduplicateCitationKeys appends a/b/c/... suffixes to duplicate keys in-place.
func deduplicateCitationKeys(catalog []PaperCitation) {
	counts := make(map[string]int)
	for _, c := range catalog {
		counts[c.Key]++
	}

	// Only process keys that have duplicates
	assigned := make(map[string]int)
	for i := range catalog {
		key := catalog[i].Key
		if counts[key] > 1 {
			idx := assigned[key]
			suffix := string(rune('a' + idx))
			catalog[i].Key = key + suffix
			assigned[key] = idx + 1
		}
	}
}
