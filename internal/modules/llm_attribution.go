package modules

import (
	"context"
	"fmt"
	"strings"
)

// llm_attribution.go — xAI: seragamkan pesan error LLM agar SELALU menyebut role +
// provider + NAMA MODEL asli + langkah perbaikan. Tanpa ini, error seperti "stream
// kosong dari provider" tak memberi tahu user config mana (role apa) yang harus dibenahi.

// roleDisplay menerjemahkan kunci role internal (mis. "reviewer1_fallback") menjadi nama
// yang ramah untuk ditampilkan ke user.
func roleDisplay(role string) string {
	switch role {
	case "brain":
		return "Brain"
	case "reviewer1":
		return "Reviewer 1"
	case "reviewer1_fallback":
		return "Reviewer 1 (fallback)"
	case "reviewer2":
		return "Reviewer 2"
	case "reviewer2_fallback":
		return "Reviewer 2 (fallback)"
	case "supervisor":
		return "Supervisor"
	case "supervisor_fallback":
		return "Supervisor (fallback)"
	case "auditor":
		return "Auditor"
	case "":
		return "LLM"
	default:
		return role
	}
}

// providerLabel mengembalikan "Provider (model)" untuk satu providerID, atau "" bila
// providerID kosong. Sumber: konfigurasi LLM milik sesi/tenant (bukan hardcode).
func (d *ModuleDeps) providerLabel(ctx context.Context, providerID string) string {
	if providerID == "" {
		return ""
	}
	if cfg, _ := d.MongoRepo.GetLLMConfig(ctx, providerID); cfg != nil {
		lbl := cfg.ProviderName
		if lbl == "" {
			lbl = providerID
		}
		if cfg.DefaultModel != "" {
			lbl += " (" + cfg.DefaultModel + ")"
		}
		return lbl
	}
	return providerID
}

// roleLabel mengembalikan label "Provider (model)" untuk PRIMARY provider sebuah role,
// atau "belum dikonfigurasi" bila role tak punya provider.
func (d *ModuleDeps) roleLabel(ctx context.Context, role string) string {
	primary, _ := d.LLMFactory.RoleProviders(ctx, role)
	if lbl := d.providerLabel(ctx, primary); lbl != "" {
		return lbl
	}
	return "belum dikonfigurasi"
}

// isLLMConnectivityError menandai error yang berarti endpoint LLM TAK TERJANGKAU (server
// mati / base URL salah / DNS gagal / koneksi di-reset) — ini SISTEMIK, bukan kegagalan
// konten per-item. Dipakai untuk fail-fast: percuma meneruskan item lain yang pasti gagal
// identik (mis. seluruh batch ekstraksi saat server LLM mati).
func isLLMConnectivityError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, sig := range []string{
		"connection refused", "actively refused", "dial tcp", "no such host",
		"connection reset", "connectex", "network is unreachable", "no route to host",
	} {
		if strings.Contains(s, sig) {
			return true
		}
	}
	return false
}

// isContextOverflowError menandai kegagalan yang berbau prompt melebihi context window model
// / stream kosong (200 OK tanpa konten). Dipakai utk memberi saran "pakai model context besar"
// pada langkah yang mengirim prompt besar (mis. ekstraksi full-text di Reviewer 1) — kasus yang
// LOLOS smoke test (prompt mungil) tapi GAGAL saat prompt nyata yang besar.
func isContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, sig := range []string{
		"stream kosong", "context window", "context length", "maximum context",
		"context_length_exceeded", "too many tokens", "tokens exceed", "reduce the length",
		"prompt is too long", "input is too long",
	} {
		if strings.Contains(s, sig) {
			return true
		}
	}
	return false
}

// llmError membungkus error pemanggilan LLM dengan atribusi xAI yang konsisten:
// "<aksi> gagal via role <Role> (<provider (model)>): <err> — periksa provider <Role>
// di Pengaturan LLM (API key, nama model, kuota/limit)". `role` adalah kunci role internal
// (mis. "brain", "reviewer1"); `action` deskripsi singkat langkah (mis. "Rekomendasi framework").
func (d *ModuleDeps) llmError(ctx context.Context, role, action string, err error) error {
	disp := roleDisplay(role)
	primary, _ := d.LLMFactory.RoleProviders(ctx, role)
	hint := fmt.Sprintf("periksa provider %s di Pengaturan LLM (API key, nama model, kuota/limit)", disp)
	// Enrichment: provider rprompt = aplikasi LLMy lokal. Kalau connectivity error, user perlu
	// menjalankannya dulu — sebut dengan nama UI (LLMy) DAN nama internal (rprompt).
	if isLLMConnectivityError(err) && strings.HasPrefix(primary, "rprompt") {
		hint = fmt.Sprintf("pastikan aplikasi LLMy (%s) sudah dijalankan di PC Anda sebelum melanjutkan pipeline", primary)
	}
	return fmt.Errorf("%s gagal via role %s (%s): %w — %s",
		action, disp, d.roleLabel(ctx, role), err, hint)
}
