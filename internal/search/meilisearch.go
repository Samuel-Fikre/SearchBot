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
	client *meilisearch.Client
	baseIndexName string
}

type SearchStrategy struct {
	KeyTerms          []string `json:"key_terms"`
	RelevanceCriteria string   `json:"relevance_criteria"`
	SearchQuery       string   `json:"search_query,omitempty"`
}

// NewMeiliSearch creates a new MeiliSearch instance
func NewMeiliSearch(host, apiKey, baseIndexName string) *MeiliSearch {
	client := meilisearch.NewClient(meilisearch.ClientConfig{
		Host:   host,
		APIKey: apiKey,
	})
	return &MeiliSearch{
		client: client,
		baseIndexName: baseIndexName,
	}
}

// getGroupIndex returns the index for a specific group
func (m *MeiliSearch) getGroupIndex(groupID int64) *meilisearch.Index {
	indexName := fmt.Sprintf("%s_group_%d", m.baseIndexName, groupID)
	index := m.client.Index(indexName)
	
	// Ensure index exists and has proper settings
	_, err := index.FetchInfo()
	if err != nil {
		// Index doesn't exist, create it with proper settings
		task, err := m.client.CreateIndex(&meilisearch.IndexConfig{
			Uid: indexName,
			PrimaryKey: "message_uid",
		})
		if err != nil {
			log.Printf("Error creating index: %v", err)
			return index
		}
		
		// Wait for index creation
		_, err = m.client.WaitForTask(task.TaskUID)
		if err != nil {
			log.Printf("Error waiting for index creation: %v", err)
			return index
		}

		// Configure index settings
		_, err = index.UpdateSearchableAttributes(&[]string{
			"text",
			"username",
		})
		if err != nil {
			log.Printf("Error updating searchable attributes: %v", err)
		}

		// Update sortable attributes
		_, err = index.UpdateSortableAttributes(&[]string{
			"created_at",
			"message_id",
		})
		if err != nil {
			log.Printf("Error updating sortable attributes: %v", err)
		}

		// Wait for settings to be applied
		tasks, err := index.GetTasks(&meilisearch.TasksQuery{
			Types: []meilisearch.TaskType{meilisearch.TaskTypeSettingsUpdate},
		})
		if err != nil {
			log.Printf("Error getting tasks: %v", err)
		} else {
			for _, task := range tasks.Results {
				_, err = m.client.WaitForTask(task.UID)
				if err != nil {
					log.Printf("Error waiting for task %d: %v", task.UID, err)
				}
			}
		}
	}
	
	return index
}

// IndexMessage indexes a message in the appropriate group index
func (m *MeiliSearch) IndexMessage(msg *models.Message) error {
	index := m.getGroupIndex(msg.ChatID)
	
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

	// Wait for indexing to complete
	_, err = m.client.WaitForTask(task.TaskUID)
	return err
}

// SearchMessages searches for messages in a specific group
func (m *MeiliSearch) SearchMessages(groupID int64, req *meilisearch.SearchRequest) ([]models.Message, error) {
	// Try to get messages from both possible indices (in case of group migration)
	var allMessages []models.Message
	
	// First try the current group ID
	currentIndex := m.getGroupIndex(groupID)
	result, err := currentIndex.Search(req.Query, req)
	if err != nil {
		log.Printf("Error searching current index: %v", err)
	} else {
		messages := m.convertHitsToMessages(result.Hits)
		allMessages = append(allMessages, messages...)
	}
	
	// If this is a supergroup (starts with -100), also try the old group ID
	if groupID < -1000000000000 {
		oldGroupID := -1 * (groupID + 1000000000000) // Convert back from supergroup ID to regular group ID
		oldIndex := m.getGroupIndex(oldGroupID)
		result, err := oldIndex.Search(req.Query, req)
		if err != nil {
			log.Printf("Error searching old index: %v", err)
		} else {
			messages := m.convertHitsToMessages(result.Hits)
			allMessages = append(allMessages, messages...)
		}
	}
	
	// Sort all messages by time
	sort.Slice(allMessages, func(i, j int) bool {
		return allMessages[i].CreatedAt.After(allMessages[j].CreatedAt)
	})
	
	return allMessages, nil
}

// convertHitsToMessages converts search hits to Message objects
func (m *MeiliSearch) convertHitsToMessages(hits []interface{}) []models.Message {
	var messages []models.Message
	for _, hit := range hits {
		if doc, ok := hit.(map[string]interface{}); ok {
			message := models.Message{}
			
			// More defensive type conversion with logging
			if msgID, ok := doc["message_id"].(float64); ok {
				message.MessageID = int64(msgID)
				log.Printf("Converted message_id: %v", message.MessageID)
			} else {
				log.Printf("Warning: Could not convert message_id: %v (%T)", doc["message_id"], doc["message_id"])
				continue
			}
			if chatID, ok := doc["chat_id"].(float64); ok {
				message.ChatID = int64(chatID)
				log.Printf("Converted chat_id: %v", message.ChatID)
			} else {
				log.Printf("Warning: Could not convert chat_id: %v (%T)", doc["chat_id"], doc["chat_id"])
				continue
			}
			if userID, ok := doc["user_id"].(float64); ok {
				message.UserID = int64(userID)
			}
			if username, ok := doc["username"].(string); ok {
				message.Username = username
				log.Printf("Found username: %v", message.Username)
			} else {
				log.Printf("Warning: Could not get username: %v (%T)", doc["username"], doc["username"])
				continue
			}
			if text, ok := doc["text"].(string); ok {
				message.Text = text
				log.Printf("Found text: %v", message.Text)
			} else {
				log.Printf("Warning: Could not get text: %v (%T)", doc["text"], doc["text"])
				continue
			}
			if timestamp, ok := doc["created_at"].(float64); ok {
				message.CreatedAt = time.Unix(int64(timestamp), 0)
			}
			
			messages = append(messages, message)
			log.Printf("Successfully converted and added message: %+v", message)
		} else {
			log.Printf("Warning: Could not convert hit to document: %+v (%T)", hit, hit)
		}
	}
	return messages
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
			Sort: []string{"created_at:asc"},
			Limit: 5, // Reduced from 10 to 5 for more focused context
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