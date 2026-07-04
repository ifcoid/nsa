package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"nsa/internal/latex"
	"nsa/internal/model"
)

// Zenodo draft-deposit: unggah kit reproducibility ke Zenodo sebagai DRAFT + prefill
// metadata, lalu BERHENTI. PUBLISH tetap manual oleh peneliti (DOI permanen = aksi
// tak-bisa-dibatalkan → wajib review metadata manusia; invariant HITL). Token per-user,
// disimpan terenkripsi-di-DB, TIDAK pernah dikembalikan ke klien.

func zenodoBase(sandbox bool) string {
	if sandbox {
		return "https://sandbox.zenodo.org/api"
	}
	return "https://zenodo.org/api"
}

// GetZenodoConfig — token disembunyikan (hanya key_set + sandbox).
func (h *SessionHandler) GetZenodoConfig(w http.ResponseWriter, req *http.Request) {
	cfg := h.mongoRepo.GetZenodoConfig(context.Background())
	set := cfg.Token != ""
	cfg.Token = ""
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{"config": cfg, "token_set": set})
}

// UpdateZenodoConfig — token kosong → pertahankan lama (jangan menghapus tak sengaja).
func (h *SessionHandler) UpdateZenodoConfig(w http.ResponseWriter, req *http.Request) {
	var cfg model.ZenodoConfig
	if err := json.NewDecoder(req.Body).Decode(&cfg); err != nil {
		sendJSONError(w, http.StatusBadRequest, "JSON tak valid")
		return
	}
	if strings.TrimSpace(cfg.Token) == "" {
		cfg.Token = h.mongoRepo.GetZenodoConfig(context.Background()).Token // preserve
	}
	if err := h.mongoRepo.UpdateZenodoConfig(context.Background(), &cfg); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal simpan konfig Zenodo: "+err.Error())
		return
	}
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{"message": "Konfig Zenodo tersimpan"})
}

type kitFile struct {
	name string
	data []byte
}

// gatherKit merakit artefak reproducibility untuk diunggah (credential-safe).
func (h *SessionHandler) gatherKit(ctx context.Context, s *model.SLRSession) []kitFile {
	var files []kitFile
	add := func(name, content string) {
		if strings.TrimSpace(content) != "" {
			files = append(files, kitFile{name, []byte(content)})
		}
	}
	if s.Manuscript != nil {
		add("manuscript.tex", s.Manuscript.Latex)
		add("references.bib", s.Manuscript.Bibtex)
		add("manuscript.md", s.Manuscript.Final)
	}
	// Laporan .md + .tex (konsisten LaTeX+BibTeX, katalog referensi sama).
	md := h.buildReportMarkdown(ctx, s)
	add("laporan_slr.md", md)
	if s.Manuscript != nil && strings.TrimSpace(s.Manuscript.Bibtex) != "" {
		tex := latex.MarkdownToLatex("Laporan SLR — "+strSafe(s.Topic, ""), md)
		tex = strings.Replace(tex, "\\end{document}",
			"\n\\clearpage\n\\nocite{*}\n\\bibliographystyle{unsrt}\n\\bibliography{references}\n\\end{document}", 1)
		add("laporan_slr.tex", tex)
	}
	if s.AuditReport != nil {
		add("protokol.md", s.AuditReport.ProtocolMarkdown)
		add("reproducibility.md", s.AuditReport.ReproPackageMarkdown)
	}
	// Figur bibliometrik (dari slr_figures).
	cur, err := h.mongoRepo.GetFiguresCollection().Find(ctx, bson.M{"session_id": s.ID})
	if err == nil {
		var docs []bson.M
		_ = cur.All(ctx, &docs)
		for _, d := range docs {
			name, _ := d["filename"].(string)
			b64, _ := d["data_b64"].(string)
			if name == "" || b64 == "" {
				continue
			}
			if raw, e := base64.StdEncoding.DecodeString(b64); e == nil {
				files = append(files, kitFile{"figures/" + name, raw})
			}
		}
	}
	return files
}

func zenodoMetadata(s *model.SLRSession) map[string]interface{} {
	title := strSafe(s.Topic, "Systematic Literature Review")
	desc := "Reproducibility package (protocol, data, figures, manuscript) for a systematic literature review conducted with an AI-assisted, human-in-the-loop pipeline (PRISMA 2020). Generated automatically; please review and complete author metadata before publishing."
	if s.Manuscript != nil && strings.TrimSpace(s.Manuscript.Abstract) != "" {
		desc = strings.TrimSpace(s.Manuscript.Abstract)
	}
	return map[string]interface{}{
		"upload_type":  "dataset",
		"title":        "Reproducibility package: " + title,
		"description":  desc,
		"access_right": "open",
		"license":      "cc-by-4.0",
		// Creators WAJIB diedit peneliti sebelum publish (placeholder eksplisit).
		"creators": []map[string]string{{"name": "[EDIT: Nama Peneliti, Afiliasi]"}},
		"keywords": []string{"systematic literature review", "PRISMA", "reproducibility"},
	}
}

// ZenodoDeposit membuat DRAFT deposition + unggah kit + prefill metadata. TIDAK publish.
func (h *SessionHandler) ZenodoDeposit(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID required")
		return
	}
	ctx := context.Background()
	s, err := h.mongoRepo.GetSession(ctx, id)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}
	cfg := h.mongoRepo.GetZenodoConfig(ctx)
	if strings.TrimSpace(cfg.Token) == "" {
		sendJSONError(w, http.StatusBadRequest, "Token Zenodo belum diisi. Buka Pengaturan → Zenodo, tempel personal access token (scope deposit:write).")
		return
	}
	base := zenodoBase(cfg.Sandbox)
	client := &http.Client{Timeout: 120 * time.Second}

	doReq := func(method, url string, body []byte, ctype string) ([]byte, int, error) {
		r, _ := http.NewRequest(method, url, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+cfg.Token)
		if ctype != "" {
			r.Header.Set("Content-Type", ctype)
		}
		resp, e := client.Do(r)
		if e != nil {
			return nil, 0, e
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return b, resp.StatusCode, nil
	}

	// 1) Buat draft deposition.
	body, code, err := doReq("POST", base+"/deposit/depositions", []byte("{}"), "application/json")
	if err != nil {
		sendJSONError(w, http.StatusBadGateway, "Gagal menghubungi Zenodo: "+err.Error())
		return
	}
	if code == 401 {
		sendJSONError(w, http.StatusBadRequest, "Token Zenodo ditolak (401). Pastikan token valid + scope deposit:write"+sandboxHint(cfg.Sandbox))
		return
	}
	if code < 200 || code >= 300 {
		sendJSONError(w, http.StatusBadGateway, fmt.Sprintf("Zenodo menolak pembuatan draft (HTTP %d): %s", code, snippet(body)))
		return
	}
	var dep struct {
		ID    int `json:"id"`
		Links struct {
			Bucket string `json:"bucket"`
			HTML   string `json:"html"`
		} `json:"links"`
	}
	if json.Unmarshal(body, &dep) != nil || dep.Links.Bucket == "" {
		sendJSONError(w, http.StatusBadGateway, "Respons Zenodo tak terbaca (tak ada bucket/link).")
		return
	}

	// 2) Unggah kit ke bucket.
	files := h.gatherKit(ctx, s)
	uploaded, failed := 0, 0
	for _, f := range files {
		_, uc, ue := doReq("PUT", dep.Links.Bucket+"/"+f.name, f.data, "application/octet-stream")
		if ue != nil || uc < 200 || uc >= 300 {
			failed++
			continue
		}
		uploaded++
	}

	// 3) Prefill metadata (best-effort; jangan gagalkan seluruh deposit bila metadata gagal).
	metaBody, _ := json.Marshal(map[string]interface{}{"metadata": zenodoMetadata(s)})
	_, _, _ = doReq("PUT", fmt.Sprintf("%s/deposit/depositions/%d", base, dep.ID), metaBody, "application/json")

	// 4) Kembalikan link draft. TIDAK publish — peneliti review metadata + Publish sendiri.
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"draft_url":      dep.Links.HTML,
		"deposition_id":  dep.ID,
		"uploaded":       uploaded,
		"failed":         failed,
		"total":          len(files),
		"sandbox":        cfg.Sandbox,
		"published":      false,
		"next":           "Buka draft_url → lengkapi metadata (WAJIB: nama penulis/ORCID) → tekan Publish untuk mint DOI. DOI lalu disitasi di Data Availability manuskrip.",
	})
}

func sandboxHint(sandbox bool) string {
	if sandbox {
		return " (mode sandbox: token harus dari sandbox.zenodo.org, BUKAN zenodo.org)."
	}
	return " (mode produksi: token dari zenodo.org)."
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
