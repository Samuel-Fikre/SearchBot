package search

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"SearchBot/internal/models"

	"github.com/meilisearch/meilisearch-go"
)

type MeiliSearch struct {
	client *meilisearch.Client
	index  string
}

type SearchStrategy struct {
	KeyTerms          []string `json:"key_terms"`
	RelevanceCriteria string   `json:"relevance_criteria"`
	SearchQuery       string   `json:"search_query,omitempty"`
}

func NewMeiliSearch(host, apiKey, index string) *MeiliSearch {
	config := meilisearch.ClientConfig{
		Host:   host,
		APIKey: apiKey,
	}
	client := meilisearch.NewClient(config)

	// Try to get the index first
	meiliIndex := client.Index(index)
	
	// Only create if it doesn't exist
	_, err := meiliIndex.GetStats()
	if err != nil {
		log.Printf("Index doesn't exist, creating new one")
		_, err := client.CreateIndex(&meilisearch.IndexConfig{
			Uid:        index,
			PrimaryKey: "message_uid",
		})
		if err != nil {
			log.Printf("Warning: Index creation returned: %v", err)
		}
	}
	
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
		"username",
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

	// Configure typo tolerance and other settings
	settings := meilisearch.Settings{
		TypoTolerance: &meilisearch.TypoTolerance{
			Enabled: true,
			MinWordSizeForTypos: meilisearch.MinWordSizeForTypos{
				OneTypo: 2,  // Allow typos for words longer than 2 characters
				TwoTypos: 3, // Allow two typos for words longer than 3 characters
			},
			DisableOnWords: []string{}, // Don't disable typos for any words
			DisableOnAttributes: []string{}, // Allow typos on all attributes
		},
		Pagination: &meilisearch.Pagination{
			MaxTotalHits: 100,
		},
	}

	// Update settings
	task, err = meiliIndex.UpdateSettings(&settings)
	if err != nil {
		log.Printf("Warning: Failed to update settings: %v", err)
	} else {
		meiliIndex.WaitForTask(task.TaskUID)
	}

	// Configure searchable attributes with weights
	task, err = meiliIndex.UpdateSearchableAttributes(&[]string{
		"text",
		"username",
	})
	if err != nil {
		log.Printf("Warning: Failed to update searchable attributes: %v", err)
	} else {
		meiliIndex.WaitForTask(task.TaskUID)
	}

	// Configure case sensitivity
	task, err = meiliIndex.UpdateSearchableAttributes(&[]string{
		"text",
		"username",
	})
	if err != nil {
		log.Printf("Warning: Failed to update searchable attributes: %v", err)
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

	// First, try to get the document to see if it exists
	var result map[string]interface{}
	err := index.GetDocument(document["message_uid"].(string), &meilisearch.DocumentQuery{}, &result)
	if err == nil {
		log.Printf("Document already exists, updating: %s", document["message_uid"])
	}

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

	// Verify the document was indexed
	var verifyResult map[string]interface{}
	err = index.GetDocument(document["message_uid"].(string), &meilisearch.DocumentQuery{}, &verifyResult)
	if err != nil {
		log.Printf("Warning: Document not found after indexing: %v", err)
	} else {
		log.Printf("Successfully verified document exists: %s", document["message_uid"])
	}

	log.Printf("Successfully indexed message with ID: %s", document["message_uid"])
	return nil
}

func (m *MeiliSearch) SearchMessages(ctx context.Context, searchReq *meilisearch.SearchRequest) ([]models.Message, error) {
	// Get index stats
	index := m.client.Index(m.index)
	stats, err := index.GetStats()
	if err != nil {
		log.Printf("Error getting index stats: %v", err)
	} else {
		log.Printf("Total documents in index: %d", stats.NumberOfDocuments)
	}

	// If no search request provided, use default settings
	if searchReq == nil {
		searchReq = &meilisearch.SearchRequest{
			Query: "",
			Limit: 50,
			AttributesToSearchOn: []string{"text"},
			Sort: []string{"created_at:desc"},
		}
	}

	// Perform the search
	result, err := index.Search("", searchReq)
	if err != nil {
		log.Printf("Search error: %v", err)
		return nil, fmt.Errorf("failed to search messages: %v", err)
	}

	log.Printf("Search results - Hits: %d", len(result.Hits))
	log.Printf("Raw search results: %+v", result.Hits)

	var messages []models.Message
	for _, hit := range result.Hits {
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
	
	index := m.client.Index(m.index)
	
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