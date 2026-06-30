package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// BugTicket represents a bug report submitted via the @BugLaporBot Telegram bot.
// Stored in the "bug_tickets" MongoDB collection.
type BugTicket struct {
	ID               primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TelegramUpdateID int                `bson:"telegram_update_id" json:"telegram_update_id"`
	ChatID           int64              `bson:"chat_id" json:"chat_id"`
	ReporterName     string             `bson:"reporter_name" json:"reporter_name"`
	ReporterUsername string             `bson:"reporter_username" json:"reporter_username"`
	MessageText      string             `bson:"message_text" json:"message_text"`
	Caption          string             `bson:"caption" json:"caption"`
	FileName         string             `bson:"file_name" json:"file_name"`
	FileID           string             `bson:"file_id" json:"file_id"`
	Status           string             `bson:"status" json:"status"` // "open", "in_progress", "resolved", "closed"
	Replies          []BugTicketReply   `bson:"replies" json:"replies"`
	CreatedAt        time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt        time.Time          `bson:"updated_at" json:"updated_at"`
	ResolvedAt       *time.Time         `bson:"resolved_at,omitempty" json:"resolved_at,omitempty"`
	RelatedCommit    string             `bson:"related_commit,omitempty" json:"related_commit,omitempty"`
}

// BugTicketReply represents an outbound reply sent to the bug reporter.
type BugTicketReply struct {
	Text      string    `bson:"text" json:"text"`
	SentAt    time.Time `bson:"sent_at" json:"sent_at"`
	Direction string    `bson:"direction" json:"direction"` // "outbound"
}

// BugBotPollState stores the Telegram getUpdates offset to avoid re-processing messages.
// Stored in the "bug_bot_state" MongoDB collection as a single document with _id = "poll_state".
type BugBotPollState struct {
	ID     string `bson:"_id"`
	Offset int64  `bson:"offset"`
}
