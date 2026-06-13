package model

import "time"

// ScopusConfig menyimpan API key Scopus (Elsevier) yang bisa diubah RUNTIME via
// web. Disimpan di collection scopus_config (_id="default").
type ScopusConfig struct {
	ID        string    `bson:"_id,omitempty" json:"id,omitempty"`
	APIKey    string    `bson:"api_key" json:"api_key"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}
