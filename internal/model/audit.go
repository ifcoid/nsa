package model

// ===== Modul 10: Audit & Defensibility Gate (pra-submisi Q1) =====
//
// M10 adalah GERBANG konsolidasi/verifikasi SEBELUM pipeline ditutup (COMPLETED).
// Filosofi: NEURO-SYMBOLIC + HITL + xAI. Cek inti bersifat SIMBOLIK/DETERMINISTIK
// (dihitung dari state DB, bukan LLM yang bisa halu "PASS"), lalu manusia meng-ATESTASI
// bahwa manuskrip layak submit. Ini menutup celah Q1: rekonsiliasi angka lintas-modul,
// kelengkapan coverage (tiap studi included → diekstrak → di-appraisal), ambang κ,
// cakupan GRADE, RQ yatim, kelengkapan manuskrip/PRISMA, dan AKURASI disclosure AI.

// AuditCheck = satu butir pemeriksaan simbolik (deterministik).
type AuditCheck struct {
	ID       string `bson:"id" json:"id"`
	Category string `bson:"category" json:"category"` // PRISMA / Reliabilitas / Kelengkapan / Integritas / Pelaporan
	Name     string `bson:"name" json:"name"`
	Status   string `bson:"status" json:"status"` // PASS / WARN / FAIL
	Detail   string `bson:"detail" json:"detail"`
	Fix      string `bson:"fix,omitempty" json:"fix,omitempty"` // saran perbaikan bila WARN/FAIL
}

// AuditReport = hasil audit M10 + jejak atestasi manusia (HITL).
type AuditReport struct {
	Checks      []AuditCheck `bson:"checks" json:"checks"`
	Verdict     string       `bson:"verdict" json:"verdict"` // READY / READY_WITH_WARNINGS / NOT_READY
	Summary     string       `bson:"summary" json:"summary"`
	PassCount   int          `bson:"pass_count" json:"pass_count"`
	WarnCount   int          `bson:"warn_count" json:"warn_count"`
	FailCount   int          `bson:"fail_count" json:"fail_count"`
	GeneratedAt string       `bson:"generated_at" json:"generated_at"` // RFC3339
	// Atestasi HITL: diisi saat peneliti menyetujui gerbang (Approve M10).
	AttestedBy string `bson:"attested_by,omitempty" json:"attested_by,omitempty"`
	AttestedAt string `bson:"attested_at,omitempty" json:"attested_at,omitempty"`
}
