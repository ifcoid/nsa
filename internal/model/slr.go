package model

import "time"

type SLRSession struct {
	ID                string            `bson:"_id,omitempty"`
	Topic             string            `bson:"topic"`
	PICO              map[string]string `bson:"pico"`
	InclusionCriteria []string          `bson:"inclusion_criteria"`
	ExclusionCriteria []string          `bson:"exclusion_criteria"`
	Status            string            `bson:"status"`   // "INIT", "WAITING_APPROVAL", "APPROVED", "NEEDS_REVISION", "REJECTED"
	Feedback          string            `bson:"feedback"` // Catatan dari manusia jika butuh revisi
	UpdatedAt         time.Time         `bson:"updated_at"`
}

type Paper struct {
	ID        string `bson:"_id,omitempty"`
	SessionID string `bson:"session_id"`
	Title     string `bson:"title"`
	Abstract  string `bson:"abstract"`
	Status    string `bson:"status"` // "PENDING", "ACCEPT", "REJECT"
	Reason    string `bson:"reason"`
}
