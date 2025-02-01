package storage

import (
	"SearchBot/internal/models"
	"time"
)

// MessageStorage defines the interface for message storage
type MessageStorage interface {
	StoreMessage(msg *models.Message) error
	GetMessagesByChat(chatID int64) ([]models.Message, error)
	GetMessage(chatID int64, messageID int64) (*models.Message, error)
	GetRecentMessages(chatID int64, limit int64) ([]models.Message, error)
	GetMessagesByTimeRange(chatID int64, start, end time.Time) ([]models.Message, error)
}
