package modules

import "testing"

// TestDeriveSearchURL mengunci logika resolusi endpoint /search: SEARCH_ENDPOINT
// eksplisit menang; jika kosong, diturunkan dari EMBED_ENDPOINT (".../v1" ->
// ".../search") sehingga user cukup mengelola satu URL tunnel.
func TestDeriveSearchURL(t *testing.T) {
	cases := []struct {
		name, search, embed, want string
	}{
		{"explicit search menang", "https://x.io/search", "https://y.io/v1", "https://x.io/search"},
		{"turunkan dari embed /v1", "", "https://abc.trycloudflare.com/v1", "https://abc.trycloudflare.com/search"},
		{"turunkan + trim trailing slash", "", "https://abc.trycloudflare.com/v1/", "https://abc.trycloudflare.com/search"},
		{"embed tanpa /v1", "", "https://abc.trycloudflare.com", "https://abc.trycloudflare.com/search"},
		{"embed sudah /search (user salah tempel)", "", "https://abc.trycloudflare.com/search", "https://abc.trycloudflare.com/search"},
		{"keduanya kosong", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("SEARCH_ENDPOINT", c.search)
			t.Setenv("EMBED_ENDPOINT", c.embed)
			if got := deriveSearchURL(); got != c.want {
				t.Fatalf("deriveSearchURL() = %q, want %q", got, c.want)
			}
		})
	}
}

// TestNormalizeEmbedBase mengunci toleransi field EMBED_ENDPOINT: apa pun bentuk URL
// tunnel yang ditempel user (base, /v1, atau /search), selalu dipetakan ke base /v1
// agar EmbedWith menembak /v1/embeddings dengan benar.
func TestNormalizeEmbedBase(t *testing.T) {
	cases := map[string]string{
		"https://abc.trycloudflare.com/v1":      "https://abc.trycloudflare.com/v1",
		"https://abc.trycloudflare.com/v1/":     "https://abc.trycloudflare.com/v1",
		"https://abc.trycloudflare.com":         "https://abc.trycloudflare.com/v1",
		"https://abc.trycloudflare.com/search":  "https://abc.trycloudflare.com/v1",
		"https://abc.trycloudflare.com/search/": "https://abc.trycloudflare.com/v1",
		"  ":                                    "",
		"":                                      "",
	}
	for in, want := range cases {
		if got := normalizeEmbedBase(in); got != want {
			t.Errorf("normalizeEmbedBase(%q) = %q, want %q", in, got, want)
		}
	}
}
