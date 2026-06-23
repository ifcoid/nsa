package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"nsa/internal/llm"
	"nsa/internal/model"
)

// BibliometricAgent menangani Modul 8b (SLNA): thesaurus, parameter, interpretasi cluster, integrasi.
type BibliometricAgent struct {
	client llm.LLMClient
}

func NewBibliometricAgent(client llm.LLMClient) *BibliometricAgent {
	return &BibliometricAgent{client: client}
}

type ThesaurusResult struct {
	Keywords string `json:"thesaurus_keywords"`
	Authors  string `json:"thesaurus_authors"`
	Approach string `json:"approach"`
}

func (a *BibliometricAgent) BuildThesaurus(ctx context.Context, keywordSample string) (*ThesaurusResult, error) {
	systemPrompt := `Anda analis bibliometrik (SLNA). Bangun THESAURUS terminologi untuk VOSviewer dari daftar keyword mentah.
Aturan: lowercase; samakan "-" dan spasi (konsisten); merge sinonim (mis. "ai" & "artificial intelligence"),
plural/singular; buang stop words ("study","analysis","research"); domain-specific synonyms.

Format output thesaurus: satu mapping per baris, format "term_variant[TAB]canonical_label" (dipisah karakter TAB).
JANGAN sertakan baris header. Setiap baris memetakan SATU varian ke SATU label kanonik.
Contoh isi thesaurus_keywords:
brain-computer interfaces	brain-computer interface
brain computer interface	brain-computer interface
artificial intelligence	ai
deep learning models	deep learning
eeg signals	eeg
electroencephalography	eeg
state space models	state space model

Keluarkan HANYA JSON MURNI tanpa markdown:
{
  "thesaurus_keywords": "brain-computer interfaces\tbrain-computer interface\nbrain computer interface\tbrain-computer interface\nartificial intelligence\tai\n...",
  "thesaurus_authors": "(opsional, kosong jika tak ada data author)",
  "approach": "VOSviewer direct / bibliometrix R + alasan singkat"
}`
	raw, err := a.client.Generate(ctx, systemPrompt, fmt.Sprintf("=== KEYWORD MENTAH (sampel) ===\n%s", keywordSample))
	if err != nil {
		return nil, fmt.Errorf("BuildThesaurus LLM: %w", err)
	}
	var res ThesaurusResult
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &res); err != nil {
		return nil, fmt.Errorf("parse ThesaurusResult (%w). Raw: %s", err, ClipRaw(raw))
	}
	return &res, nil
}

func (a *BibliometricAgent) GenerateVOSParams(ctx context.Context, rqsJSON string, records int) (*model.VOSViewerParams, error) {
	systemPrompt := `Anda metodolog SLNA. Tetapkan 9 PARAMETER VOSviewer + justifikasi (siap-Methods).
Parameter: (1) Type of analysis (co-occurrence/co-authorship/citation/bibliographic coupling),
(2) Unit (author/index/all keywords / authors / documents / sources), (3) Counting (full/fractional),
(4) Min occurrences threshold, (5) Min cluster size, (6) Resolution (0.5-1.5),
(7) Normalization (association strength default), (8) Clustering algorithm (LinLog/modularity),
(9) Visualization (network/overlay/density — generate ketiganya). Setiap parameter WAJIB ada justifikasi.

Keluarkan HANYA JSON MURNI tanpa markdown:
{
  "type_of_analysis": "Co-occurrence",
  "unit_of_analysis": "Author keywords",
  "table_markdown": "| # | Parameter | Setting | Justifikasi |\n|...| (9 baris)"
}`
	userPrompt := fmt.Sprintf("Jumlah records: %d\n\n=== RESEARCH QUESTIONS ===\n%s", records, rqsJSON)
	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("GenerateVOSParams LLM: %w", err)
	}
	var res model.VOSViewerParams
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &res); err != nil {
		return nil, fmt.Errorf("parse VOSViewerParams (%w). Raw: %s", err, ClipRaw(raw))
	}
	return &res, nil
}

func (a *BibliometricAgent) InterpretClusters(ctx context.Context, vosInput string) (*model.ClusterInterpretation, error) {
	systemPrompt := `Anda analis SLNA. Berdasarkan output VOSviewer (di-paste pengguna: nodes/edges/clusters/top clusters/bridge/temporal),
klasifikasikan cluster dengan kriteria KUANTITATIF:
- TIER 1 CORE: size >=15% total nodes + TLS top-quartile.
- TIER 2 EMERGING: mayoritas keyword 5 tahun terakhir + tren naik (overlay).
- TIER 3 ESTABLISHED: keyword 5-10+ tahun + TLS tinggi (matur).
- TIER 4 PERIPHERAL: size <5% + TLS rendah (niche/potential gap).
Identifikasi BRIDGE keywords (muncul di >=2 cluster) + STRUCTURAL HOLES (2 cluster yang seharusnya terhubung tapi tidak).

Keluarkan HANYA JSON MURNI tanpa markdown:
{
  "markdown": "interpretasi naratif per tier + bridge + structural holes",
  "table_markdown": "| Tier | Cluster Label | Size (%) | TLS | Top-5 Keywords | Interpretation |\n|...|"
}`
	raw, err := a.client.Generate(ctx, systemPrompt, fmt.Sprintf("=== OUTPUT VOSVIEWER (paste user) ===\n%s", vosInput))
	if err != nil {
		return nil, fmt.Errorf("InterpretClusters LLM: %w", err)
	}
	var res model.ClusterInterpretation
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &res); err != nil {
		return nil, fmt.Errorf("parse ClusterInterpretation (%w). Raw: %s", err, ClipRaw(raw))
	}
	return &res, nil
}

func (a *BibliometricAgent) IntegrateSLNA(ctx context.Context, clusterMarkdown, slrSummary string) (*model.SLNAIntegration, error) {
	systemPrompt := `Anda integrator SLNA (bibliometric + SLR). Lakukan validasi tema lintas-method.
Per tema/finding SLR: cocokkan ke cluster bibliometric -> status CONVERGENT (sejalan, kuat) /
SLR-ONLY (tema di SLR tak prominent di network) / BIB-ONLY (cluster tak terangkat di SLR).
Tentukan research landscape positioning + CONVERGENT GAPS (gap yang muncul DI KEDUA: SLR synthesis + structural holes bibliometric — paling kuat untuk Future Research).

Keluarkan HANYA JSON MURNI tanpa markdown:
{
  "markdown": "tabel validasi tema (CONVERGENT/SLR-ONLY/BIB-ONLY) + positioning",
  "convergent_gaps": "3 convergent gaps + trace evidence kedua method"
}`
	userPrompt := fmt.Sprintf("=== CLUSTER INTERPRETATION (bibliometric) ===\n%s\n\n=== RINGKASAN SINTESIS SLR (M8) ===\n%s", clusterMarkdown, slrSummary)
	raw, err := a.client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("IntegrateSLNA LLM: %w", err)
	}
	var res model.SLNAIntegration
	if err := json.Unmarshal([]byte(CleanJSONResponse(raw)), &res); err != nil {
		return nil, fmt.Errorf("parse SLNAIntegration (%w). Raw: %s", err, ClipRaw(raw))
	}
	return &res, nil
}
