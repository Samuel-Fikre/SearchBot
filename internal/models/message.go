package models

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Message represents a Telegram message stored in the database
type Message struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"-"`              // Hide from JSON
	MessageID int64             `bson:"message_id" json:"message_id"`
	ChatID    int64             `bson:"chat_id" json:"chat_id"`
	UserID    int64             `bson:"user_id" json:"user_id"`
	Username  string            `bson:"username" json:"username"`
	Text      string            `bson:"text" json:"text"`
	CreatedAt time.Time         `bson:"created_at" json:"created_at"`
}

// GetSearchID returns a unique ID for Meilisearch indexing
func (m *Message) GetSearchID() string {
	return fmt.Sprintf("%d-%d", m.ChatID, m.MessageID)
} 