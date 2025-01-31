package search

import (
	"SearchBot/internal/models"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/meilisearch/meilisearch-go"
	"context"
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
			MinWordSizeForTypos: meilisearch.MinWordSizeForTypos{
				OneTypo: 2,  // Allow typos for words longer than 2 characters
				TwoTypos: 4, // Allow two typos for words longer than 4 characters
			},
		},
		Pagination: &meilisearch.Pagination{
			MaxTotalHits: 100,
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

func (m *MeiliSearch) SearchMessages(ctx context.Context, strategyJSON string) ([]models.Message, error) {
	// Parse the search strategy
	var strategy SearchStrategy
	if err := json.Unmarshal([]byte(strategyJSON), &strategy); err != nil {
		return nil, fmt.Errorf("failed to parse search strategy: %v", err)
	}

	// Log the search strategy for debugging
	log.Printf("Search strategy: %+v", strategy)

	// Get index stats
	index := m.client.Index(m.index)
	stats, err := index.GetStats()
	if err != nil {
		log.Printf("Error getting index stats: %v", err)
	} else {
		log.Printf("Total documents in index: %d", stats.NumberOfDocuments)
	}

	// Build search query from key terms
	var searchTerms []string
	if len(strategy.KeyTerms) > 0 {
		searchTerms = strategy.KeyTerms
	} else if strategy.SearchQuery != "" {
		searchTerms = []string{strategy.SearchQuery}
	}

	// Join terms with OR for broader matches
	searchQuery := strings.Join(searchTerms, " OR ")
	log.Printf("Performing search with query: %s", searchQuery)

	// Add more search options for better matching
	searchReq := &meilisearch.SearchRequest{
		Query: searchQuery,
		Limit: 20,
		MatchingStrategy: "all", // Match all terms for better relevance
		AttributesToSearchOn: []string{"text"},
		Sort: []string{"created_at:desc"},
		ShowMatchesPosition: true,
	}

	result, err := index.Search(searchQuery, searchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to search messages: %v", err)
	}

	log.Printf("Search results - Hits: %d, Query: %s", len(result.Hits), result.Query)

	var messages []models.Message
	for _, hit := range result.Hits {
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
			log.Printf("Found message: %s: %s", message.Username, message.Text)
		} else {
			log.Printf("Warning: Could not convert hit to document: %+v", hit)
		}
	}

	return messages, nil
} 