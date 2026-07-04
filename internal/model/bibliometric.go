package model

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

// ===== Modul 8b: Bibliometric / SLNA (opsional) =====

// BibliometricData = output L1 (data prep + thesaurus).
type BibliometricData struct {
	RecordsAnalyzed   int    `bson:"records_analyzed" json:"records_analyzed"`
	ThesaurusKeywords string `bson:"thesaurus_keywords" json:"thesaurus_keywords"` // format VOSviewer
	ThesaurusAuthors  string `bson:"thesaurus_authors" json:"thesaurus_authors"`
	Approach          string `bson:"approach" json:"approach"`
	LogMarkdown       string `bson:"log_markdown" json:"log_markdown"`
	ModelUsed         string `bson:"model_used,omitempty" json:"model_used,omitempty"` // atribusi xAI
}

// VOSViewerParams = output L2 (9-parameter justification, siap-Methods).
type VOSViewerParams struct {
	TypeOfAnalysis string `bson:"type_of_analysis" json:"type_of_analysis"`
	UnitOfAnalysis string `bson:"unit_of_analysis" json:"unit_of_analysis"`
	TableMarkdown  string `bson:"table_markdown" json:"table_markdown"`
	ModelUsed      string `bson:"model_used,omitempty" json:"model_used,omitempty"` // atribusi xAI
}

// ClusterInterpretation = output L3 (tier 1-4 + bridge + structural holes).
type ClusterInterpretation struct {
	Markdown      string `bson:"markdown" json:"markdown"`
	TableMarkdown string `bson:"table_markdown" json:"table_markdown"`
	ModelUsed     string `bson:"model_used,omitempty" json:"model_used,omitempty"` // atribusi xAI
}

// SLNAIntegration = output L4 (validasi tema lintas-method + convergent gaps).
type SLNAIntegration struct {
	Markdown       string `bson:"markdown" json:"markdown"`
	ConvergentGaps string `bson:"convergent_gaps" json:"convergent_gaps"`
	ModelUsed      string `bson:"model_used,omitempty" json:"model_used,omitempty"` // atribusi xAI
}

// UnmarshalJSON toleran terhadap non-determinisme LLM: field yang SEHARUSNYA string
// kadang dikembalikan sebagai ARRAY atau OBJECT (mis. convergent_gaps: [...] → dulu
// crash "cannot unmarshal array into ... of type string", lapor balqis M8B_STEP4).
// Juga MENYELAMATKAN markdown kosong: sebagian model menaruh tabel di key lain
// (mis. tabel_validasi_tema) alih-alih di markdown. Hanya JSON (LLM parse) yang
// terpengaruh — round-trip bson/Mongo tetap pakai field string biasa.
func (s *SLNAIntegration) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	s.Markdown = flexJSONToString(raw["markdown"])
	s.ConvergentGaps = flexJSONToString(raw["convergent_gaps"])
	// Salvage: bila markdown kosong, ambil dari key alternatif yang sering dipakai model.
	if strings.TrimSpace(s.Markdown) == "" {
		for _, k := range []string{"tabel_validasi_tema", "table_markdown", "tabel", "validasi_tema", "table", "themes"} {
			if v, ok := raw[k]; ok {
				if salv := flexJSONToString(v); strings.TrimSpace(salv) != "" {
					s.Markdown = salv
					break
				}
			}
		}
	}
	return nil
}

// flexJSONToString mengubah nilai JSON APA PUN (string/array/object/angka/bool) menjadi
// string yang terbaca — jaring pengaman untuk output LLM yang tak konsisten tipenya.
func flexJSONToString(b json.RawMessage) string {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return ""
	}
	switch b[0] {
	case '"':
		var str string
		if json.Unmarshal(b, &str) == nil {
			return str
		}
		return string(b)
	case '[':
		var arr []json.RawMessage
		if json.Unmarshal(b, &arr) != nil {
			return string(b)
		}
		parts := make([]string, 0, len(arr))
		for _, el := range arr {
			if s := strings.TrimSpace(flexJSONToString(el)); s != "" {
				parts = append(parts, "- "+s)
			}
		}
		return strings.Join(parts, "\n")
	case '{':
		var obj map[string]json.RawMessage
		if json.Unmarshal(b, &obj) != nil {
			return string(b)
		}
		parts := make([]string, 0, len(obj))
		for k, v := range obj {
			parts = append(parts, k+": "+flexJSONToString(v))
		}
		sort.Strings(parts) // urutan stabil (map iteration acak)
		return strings.Join(parts, " | ")
	default:
		return strings.Trim(string(b), `"`)
	}
}

type ModulBibliometricSummary struct {
	Markdown string `bson:"markdown" json:"markdown"`
}

// UnmarshalJSON: toleran non-determinisme LLM (field string kadang dibalas array/objek) —
// kelas bug yang sama dgn SLNAIntegration. flexJSONToString meratakan apa pun jadi string.
func (v *VOSViewerParams) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	v.TypeOfAnalysis = flexJSONToString(raw["type_of_analysis"])
	v.UnitOfAnalysis = flexJSONToString(raw["unit_of_analysis"])
	v.TableMarkdown = flexJSONToString(raw["table_markdown"])
	return nil
}

func (c *ClusterInterpretation) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	c.Markdown = flexJSONToString(raw["markdown"])
	c.TableMarkdown = flexJSONToString(raw["table_markdown"])
	return nil
}
