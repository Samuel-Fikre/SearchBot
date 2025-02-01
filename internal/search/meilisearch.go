package search

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"SearchBot/internal/models"

	"github.com/meilisearch/meilisearch-go"
)

// MeiliSearch handles search functionality using Meilisearch
type MeiliSearch struct {
	client        *meilisearch.Client
	baseIndexName string
	maxRetries    int
	retryDelay    time.Duration
}

type SearchStrategy struct {
	KeyTerms          []string `json:"key_terms"`
	RelevanceCriteria string   `json:"relevance_criteria"`
	SearchQuery       string   `json:"search_query,omitempty"`
}

// NewMeiliSearch creates a new MeiliSearch instance
func NewMeiliSearch(host, apiKey, baseIndexName string) *MeiliSearch {
	client := meilisearch.NewClient(meilisearch.ClientConfig{
		Host:    host,
		APIKey:  apiKey,
		Timeout: 10 * time.Second, // Add timeout
	})

	return &MeiliSearch{
		client:        client,
		baseIndexName: baseIndexName,
		maxRetries:    3,               // Maximum number of retries
		retryDelay:    2 * time.Second, // Delay between retries
	}
}

// withRetry executes an operation with retry logic
func (m *MeiliSearch) withRetry(operation string, fn func() error) error {
	var lastErr error
	for i := 0; i <= m.maxRetries; i++ {
		if i > 0 {
			log.Printf("Retrying %s (attempt %d/%d) after error: %v", operation, i, m.maxRetries, lastErr)
			time.Sleep(m.retryDelay * time.Duration(i)) // Exponential backoff
		}

		if err := fn(); err != nil {
			lastErr = err
			continue
		}
		return nil // Success
	}
	return fmt.Errorf("failed after %d retries: %v", m.maxRetries, lastErr)
}

// getGroupIndex gets or creates an index for a specific group
func (m *MeiliSearch) getGroupIndex(chatID int64) string {
	return fmt.Sprintf("messages_group_%d", chatID)
}

// configureIndex configures the settings for an index
func (m *MeiliSearch) configureIndex(indexName string) error {
	index := m.client.Index(indexName)

	// Configure index settings
	settings := &meilisearch.Settings{
		SearchableAttributes: []string{
			"text",
			"username",
		},
		FilterableAttributes: []string{
			"chat_id",
			"message_id",
			"user_id",
			"username",
			"created_at",
		},
		SortableAttributes: []string{
			"created_at",
			"message_id",
		},
	}

	// Update index settings
	_, err := index.UpdateSettings(settings)
	if err != nil {
		return fmt.Errorf("failed to update index settings: %v", err)
	}

	return nil
}

// IndexMessage indexes a message in Meilisearch
func (m *MeiliSearch) IndexMessage(msg *models.Message) error {
	// Get the index for this group
	indexName := m.getGroupIndex(msg.ChatID)
	index := m.client.Index(indexName)

	// Configure index settings first
	if err := m.configureIndex(indexName); err != nil {
		return fmt.Errorf("failed to configure index: %v", err)
	}

	// Create a unique ID for the message that includes both chat ID and message ID
	messageUID := fmt.Sprintf("%d-%d", msg.ChatID, msg.MessageID)

	// Create document to index
	document := map[string]interface{}{
		"message_uid": messageUID,
		"message_id":  msg.MessageID,
		"chat_id":     msg.ChatID,
		"user_id":     msg.UserID,
		"username":    msg.Username,
		"text":        msg.Text,
		"created_at":  msg.CreatedAt.Unix(), // Store as Unix timestamp for sorting
	}

	// Add document to index
	_, err := index.AddDocuments([]map[string]interface{}{document})
	if err != nil {
		return fmt.Errorf("failed to add document: %v", err)
	}

	return nil
}

// SearchMessages searches for messages in a group's index
func (m *MeiliSearch) SearchMessages(chatID int64, searchReq *meilisearch.SearchRequest) ([]models.Message, error) {
	indexName := m.getGroupIndex(chatID)

	// Configure index settings
	if err := m.configureIndex(indexName); err != nil {
		return nil, fmt.Errorf("failed to configure index: %v", err)
	}

	// Perform search
	index := m.client.Index(indexName)
	searchRes, err := index.Search("", searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %v", err)
	}

	// Convert hits to messages
	var messages []models.Message
	for _, hit := range searchRes.Hits {
		msg := models.Message{}

		// Convert map[string]interface{} to Message struct
		if messageID, ok := hit.(map[string]interface{})["message_id"].(float64); ok {
			msg.MessageID = int64(messageID)
		}
		if chatID, ok := hit.(map[string]interface{})["chat_id"].(float64); ok {
			msg.ChatID = int64(chatID)
		}
		if userID, ok := hit.(map[string]interface{})["user_id"].(float64); ok {
			msg.UserID = int64(userID)
		}
		if username, ok := hit.(map[string]interface{})["username"].(string); ok {
			msg.Username = username
		}
		if text, ok := hit.(map[string]interface{})["text"].(string); ok {
			msg.Text = text
		}
		if timestamp, ok := hit.(map[string]interface{})["created_at"].(float64); ok {
			msg.CreatedAt = time.Unix(int64(timestamp), 0)
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

// fetchMessageContext fetches messages before and after each message to provide conversation context
func (m *MeiliSearch) fetchMessageContext(messages []models.Message) ([]models.Message, error) {
	const contextWindow = 30 * time.Second // Reduced from 2 minutes to 30 seconds for tighter context

	// Create a map to track unique messages
	uniqueMessages := make(map[string]models.Message)

	// Add original messages to the map
	for _, msg := range messages {
		uniqueMessages[msg.GetSearchID()] = msg
	}

	index := m.client.Index(m.baseIndexName)

	// For each message, fetch context
	for _, msg := range messages {
		// Create time range filter
		beforeTime := msg.CreatedAt.Add(-contextWindow)
		afterTime := msg.CreatedAt.Add(contextWindow)

		filter := fmt.Sprintf(
			"chat_id = %d AND created_at >= %d AND created_at <= %d",
			msg.ChatID,
			beforeTime.Unix(),
			afterTime.Unix(),
		)

		// Search for context messages
		contextReq := &meilisearch.SearchRequest{
			Filter: filter,
			Sort:   []string{"created_at:asc"},
			Limit:  5, // Reduced from 10 to 5 for more focused context
		}

		result, err := index.Search("", contextReq)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch context: %v", err)
		}

		// Add context messages to the map
		for _, hit := range result.Hits {
			if doc, ok := hit.(map[string]interface{}); ok {
				message := models.Message{}

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

				uniqueMessages[message.GetSearchID()] = message
			}
		}
	}

	// Convert map back to slice
	var result []models.Message
	for _, msg := range uniqueMessages {
		result = append(result, msg)
	}

	// Sort by time
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result, nil
}

// isCommonWord returns true if the word is too common to be useful for search
func isCommonWord(word string) bool {
	word = strings.ToLower(word)
	commonWords := map[string]bool{
		"the": true, "be": true, "to": true, "of": true, "and": true,
		"a": true, "in": true, "that": true, "have": true, "i": true,
		"it": true, "for": true, "not": true, "on": true, "with": true,
		"he": true, "as": true, "you": true, "do": true, "at": true,
		"this": true, "but": true, "his": true, "by": true, "from": true,
		"they": true, "we": true, "say": true, "her": true, "she": true,
		"or": true, "an": true, "will": true, "my": true, "one": true,
		"all": true, "would": true, "there": true, "their": true, "what": true,
		"was": true, "were": true, "been": true, "being": true, "into": true,
		"who": true, "whom": true, "whose": true, "which": true, "where": true,
		"when": true, "why": true, "how": true, "any": true, "some": true,
		"can": true, "could": true, "may": true, "might": true, "must": true,
		"shall": true, "should": true, "about": true, "many": true, "most": true,
		"other": true, "such": true, "than": true, "then": true, "these": true,
		"those": true, "only": true, "very": true, "also": true, "just": true,
	}
	return commonWords[word]
}
