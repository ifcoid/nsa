package model

import "time"

// ZenodoConfig menyimpan personal access token Zenodo (scope deposit:write) yang bisa
// diubah RUNTIME via web. Per-user (backend bisa lokal). Disimpan di collection
// zenodo_config (_id="default"). Token TIDAK pernah dikembalikan ke klien (redact).
type ZenodoConfig struct {
	ID        string    `bson:"_id,omitempty" json:"id,omitempty"`
	Token     string    `bson:"token" json:"token"`
	Sandbox   bool      `bson:"sandbox" json:"sandbox"` // true → sandbox.zenodo.org (uji tanpa DOI asli)
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}
