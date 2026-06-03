package model

import "time"

// GitHubConfig = konfigurasi publikasi figur ke repo GitHub + GitHub Pages.
// Disimpan sebagai satu dokumen (_id="default") di koleksi github_config.
type GitHubConfig struct {
	ID        string    `bson:"_id,omitempty" json:"id,omitempty"`
	Enabled   bool      `bson:"enabled" json:"enabled"`
	Token     string    `bson:"token" json:"token"`         // PAT (redacted saat GET)
	Owner     string    `bson:"owner" json:"owner"`         // user/org
	Repo      string    `bson:"repo" json:"repo"`           // nama repo
	Branch    string    `bson:"branch" json:"branch"`       // default "main"
	BasePath  string    `bson:"base_path" json:"base_path"` // mis. "figures" atau "docs/figures"
	PagesURL  string    `bson:"pages_url" json:"pages_url"` // mis. https://user.github.io/repo
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

func (g *GitHubConfig) Defaults() {
	if g.Branch == "" {
		g.Branch = "main"
	}
	if g.BasePath == "" {
		g.BasePath = "figures"
	}
}

// IsReady = siap dipakai untuk publish.
func (g *GitHubConfig) IsReady() bool {
	return g != nil && g.Enabled && g.Token != "" && g.Owner != "" && g.Repo != ""
}
