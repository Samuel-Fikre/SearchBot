package search

import (
	"SearchBot/internal/models"
	"fmt"
	"log"
	"time"

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

	// Delete the index if it exists
	if _, err := client.DeleteIndex(index); err != nil {
		log.Printf("Warning: Failed to delete index (this is normal for first run): %v", err)
	}

	// Create a fresh index with explicit settings
	_, err := client.CreateIndex(&meilisearch.IndexConfig{
		Uid:        index,
		PrimaryKey: "message_uid",
	})
	if err != nil {
		log.Printf("Warning: Index creation returned: %v", err)
	}

	// Get the index
	meiliIndex := client.Index(index)
	
	// Configure searchable attributes with weights
	task, err := meiliIndex.UpdateSearchableAttributes(&[]string{
		"text",
		"username",
	})
	if err != nil {
		log.Printf("Warning: Failed to update searchable attributes: %v", err)
	} else {
		meiliIndex.WaitForTask(task.TaskUID)
	}

	// Configure ranking rules
	task, err = meiliIndex.UpdateRankingRules(&[]string{
		"words",
		"typo",
		"proximity",
		"attribute",
		"sort",
		"exactness",
	})
	if err != nil {
		log.Printf("Warning: Failed to update ranking rules: %v", err)
	} else {
		meiliIndex.WaitForTask(task.TaskUID)
	}

	// Configure filterable attributes
	task, err = meiliIndex.UpdateFilterableAttributes(&[]string{
		"chat_id",
		"user_id",
		"message_id",
		"created_at",
	})
	if err != nil {
		log.Printf("Warning: Failed to update filterable attributes: %v", err)
	} else {
		meiliIndex.WaitForTask(task.TaskUID)
	}

	// Configure sortable attributes
	task, err = meiliIndex.UpdateSortableAttributes(&[]string{
		"created_at",
	})
	if err != nil {
		log.Printf("Warning: Failed to update sortable attributes: %v", err)
	} else {
		meiliIndex.WaitForTask(task.TaskUID)
	}

	// Configure typo tolerance
	settings := meilisearch.Settings{
		TypoTolerance: &meilisearch.TypoTolerance{
			Enabled: true,
			DisableOnWords: []string{},
			DisableOnAttributes: []string{},
		},
	}
	task, err = meiliIndex.UpdateSettings(&settings)
	if err != nil {
		log.Printf("Warning: Failed to update settings: %v", err)
	} else {
		meiliIndex.WaitForTask(task.TaskUID)
	}

	return &MeiliSearch{
		client: client,
		index:  index,
	}
}

func (m *MeiliSearch) IndexMessage(msg *models.Message) error {
	index := m.client.Index(m.index)
	
	// Create a document for indexing with a unique message_uid field
	document := map[string]interface{}{
		"message_uid": fmt.Sprintf("%d-%d", msg.ChatID, msg.MessageID), // Primary key
		"message_id": msg.MessageID,
		"chat_id":    msg.ChatID,
		"user_id":    msg.UserID,
		"username":   msg.Username,
		"text":       msg.Text,
		"created_at": msg.CreatedAt.Unix(), // Store as Unix timestamp for better sorting
	}

	// Debug log
	log.Printf("Indexing document: %+v", document)

	// Add the document
	task, err := index.AddDocuments([]map[string]interface{}{document})
	if err != nil {
		log.Printf("Error adding document: %v", err)
		return err
	}

	// Wait for the indexing task to complete
	taskInfo, err := index.WaitForTask(task.TaskUID)
	if err != nil {
		log.Printf("Error waiting for indexing task: %v", err)
		return err
	}

	// Check if the task was successful
	if taskInfo.Status != "succeeded" {
		log.Printf("Indexing task failed: %+v", taskInfo)
		return fmt.Errorf("indexing task failed: %s", taskInfo.Status)
	}

	log.Printf("Successfully indexed message with ID: %s", document["message_uid"])
	return nil
}

func (m *MeiliSearch) Search(query string) ([]models.Message, error) {
	index := m.client.Index(m.index)

	// Add more search options for better matching
	searchRes, err := index.Search(query, &meilisearch.SearchRequest{
		Limit: 20,
		Sort: []string{"created_at:desc"},
		MatchingStrategy: "all", // Match all words in the query
		AttributesToSearchOn: []string{"text"}, // Focus on message text
		AttributesToRetrieve: []string{"*"},    // Get all attributes
		ShowMatchesPosition: true,
		Filter: "created_at > 0",
	})
	if err != nil {
		log.Printf("Search error: %v", err)
		return nil, err
	}

	log.Printf("Search query: %s", query) // Debug log
	log.Printf("Number of hits: %d", len(searchRes.Hits)) // Debug log
	log.Printf("Search results: %+v", searchRes) // Debug log

	var messages []models.Message
	for _, hit := range searchRes.Hits {
		if doc, ok := hit.(map[string]interface{}); ok {
			message := models.Message{}
			
			// More defensive type conversion
			if msgID, ok := doc["message_id"].(float64); ok {
				message.MessageID = int64(msgID)
			}
			if chatID, ok := doc["chat_id"].(float64); ok {
				message.ChatID = int64(chatID)
			}
			if userID, ok := doc["user_id"].(float64); ok {
				message.UserID = int64(userID)
			}
			if username, ok := doc["username"].(string); ok {
				message.Username = username
			}
			if text, ok := doc["text"].(string); ok {
				message.Text = text
			}
			if timestamp, ok := doc["created_at"].(float64); ok {
				message.CreatedAt = time.Unix(int64(timestamp), 0)
			}
			
			messages = append(messages, message)
		}
	}

	// Debug log
	log.Printf("Converted %d messages from search results", len(messages))
	for _, msg := range messages {
		log.Printf("Found message: %s: %s", msg.Username, msg.Text)
	}

	return messages, nil
} 