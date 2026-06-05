package model

// LLMRoles memetakan PERAN agen ke provider (+fallback) sehingga tidak hardcode di kode.
// Disimpan sebagai satu dokumen (_id="default") di koleksi llm_roles.
type LLMRoles struct {
	ID                 string `bson:"_id,omitempty" json:"id,omitempty"`
	Reviewer1          string `bson:"reviewer1" json:"reviewer1"`
	Reviewer1Fallback  string `bson:"reviewer1_fallback" json:"reviewer1_fallback"`
	Reviewer2          string `bson:"reviewer2" json:"reviewer2"`
	Reviewer2Fallback  string `bson:"reviewer2_fallback" json:"reviewer2_fallback"`
	Supervisor         string `bson:"supervisor" json:"supervisor"`
	SupervisorFallback string `bson:"supervisor_fallback" json:"supervisor_fallback"`
	Brain              string `bson:"brain" json:"brain"`
	BrainFallback      string `bson:"brain_fallback" json:"brain_fallback"`
}

// DefaultLLMRoles = pemetaan default (sesuai perilaku kode saat ini).
func DefaultLLMRoles() LLMRoles {
	return LLMRoles{
		Reviewer1:          "zhipu",
		Reviewer1Fallback:  "rprompt1",
		Reviewer2:          "groq",
		Reviewer2Fallback:  "xiaomi",
		Supervisor:         "xiaomi",
		SupervisorFallback: "openrouter",
		Brain:              "gemini",
		BrainFallback:      "rprompt1",
	}
}

// FillDefaults mengisi field kosong dengan nilai default (merge).
func (r *LLMRoles) FillDefaults() {
	d := DefaultLLMRoles()
	if r.Reviewer1 == "" {
		r.Reviewer1 = d.Reviewer1
	}
	if r.Reviewer1Fallback == "" {
		r.Reviewer1Fallback = d.Reviewer1Fallback
	}
	if r.Reviewer2 == "" {
		r.Reviewer2 = d.Reviewer2
	}
	if r.Reviewer2Fallback == "" {
		r.Reviewer2Fallback = d.Reviewer2Fallback
	}
	if r.Supervisor == "" {
		r.Supervisor = d.Supervisor
	}
	if r.SupervisorFallback == "" {
		r.SupervisorFallback = d.SupervisorFallback
	}
	if r.Brain == "" {
		r.Brain = d.Brain
	}
	if r.BrainFallback == "" {
		r.BrainFallback = d.BrainFallback
	}
}
