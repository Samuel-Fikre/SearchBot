package search

import (
	"SearchBot/internal/models"
	"log"

	"github.com/meilisearch/meilisearch-go"
)

type MeiliSearch struct {
	client *meilisearch.Client
	index  string
}

func NewMeiliSearch(host, apiKey, index string) *MeiliSearch {
	config := meilisearch.ClientConfig{
		Host:   host,
		APIKey: apiKey,
	}
	client := meilisearch.NewClient(config)

	return &MeiliSearch{
		client: client,
		index:  index,
	}
}

func (m *MeiliSearch) IndexMessage(msg *models.Message) error {
	index := m.client.Index(m.index)
	
	// Create a document for indexing
	document := map[string]interface{}{
		"id":         msg.ID.Hex(),
		"message_id": msg.MessageID,
		"chat_id":    msg.ChatID,
		"user_id":    msg.UserID,
		"username":   msg.Username,
		"text":       msg.Text,
		"created_at": msg.CreatedAt,
	}

	_, err := index.AddDocuments([]map[string]interface{}{document})
	if err != nil {
		log.Printf("Error indexing message: %v", err)
		return err
	}

	return nil
}

func (m *MeiliSearch) Search(query string) ([]models.Message, error) {
	index := m.client.Index(m.index)

	searchRes, err := index.Search(query, &meilisearch.SearchRequest{
		Limit: 20,
	})
	if err != nil {
		return nil, err
	}

	var messages []models.Message
	for _, hit := range searchRes.Hits {
		if doc, ok := hit.(map[string]interface{}); ok {
			// Convert the document back to a Message struct
			message := models.Message{
				MessageID: int64(doc["message_id"].(float64)),
				ChatID:    int64(doc["chat_id"].(float64)),
				UserID:    int64(doc["user_id"].(float64)),
				Username:  doc["username"].(string),
				Text:      doc["text"].(string),
			}
			messages = append(messages, message)
		}
	}

	return messages, nil
} 