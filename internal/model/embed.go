package model

import "time"

// EmbedConfig menyimpan endpoint embedding (BGE-M3) yang bisa diubah RUNTIME via
// web (mis. saat tunnel Colab di-restart user). Disimpan di collection embed_config
// (_id="default"), jadi tak perlu redeploy untuk ganti URL.
type EmbedConfig struct {
	ID        string    `bson:"_id,omitempty" json:"id,omitempty"`
	Endpoint  string    `bson:"endpoint" json:"endpoint"` // base OpenAI-compatible, mis. https://xxx.trycloudflare.com/v1
	APIKey    string    `bson:"api_key" json:"api_key"`
	Model     string    `bson:"model" json:"model"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

func (c *EmbedConfig) Defaults() {
	if c.Model == "" {
		c.Model = "BAAI/bge-m3"
	}
}
