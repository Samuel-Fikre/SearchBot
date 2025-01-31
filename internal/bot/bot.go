package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"SearchBot/internal/ai"
	"SearchBot/internal/models"
	"SearchBot/internal/search"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/meilisearch/meilisearch-go"
)

// Bot handles Telegram bot functionality
type Bot struct {
	api    *tgbotapi.BotAPI
	ai     *ai.GeminiAI
	search *search.MeiliSearch
}

// NewBot creates a new Bot instance
func NewBot(api *tgbotapi.BotAPI, ai *ai.GeminiAI, search *search.MeiliSearch) *Bot {
	return &Bot{
		api:    api,
		ai:     ai,
		search: search,
	}
}

// sendMessage sends a message to a chat
func (b *Bot) sendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := b.api.Send(msg)
	return err
}

// HandleAskCommand handles the /ask command
func (b *Bot) HandleAskCommand(ctx context.Context, msg *tgbotapi.Message) error {
	// Extract the question from the message
	question := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/ask"))
	if question == "" {
		return b.sendMessage(msg.Chat.ID, "Please provide a question after /ask")
	}

	// First, fetch recent messages from the database
	searchReq := &meilisearch.SearchRequest{
		Query: "",  // Empty query to get all messages
		Limit: 100, // Get a good number of recent messages
		AttributesToSearchOn: []string{"text"},
		Sort: []string{"created_at:desc"}, // Most recent first
	}
	
	messages, err := b.search.SearchMessages(ctx, searchReq)
	if err != nil {
		return fmt.Errorf("failed to fetch messages: %v", err)
	}

	// Format all messages for AI analysis
	var messagesText strings.Builder
	for _, message := range messages {
		messagesText.WriteString(fmt.Sprintf("@%s: %s\n", message.Username, message.Text))
	}

	// Let AI analyze the messages and user's question
	analysisPrompt := fmt.Sprintf(`You are an intelligent search assistant for a coding group chat.
A user asked: '%s'

Here are ALL the recent messages from our chat:
%s

Your task is to find messages that would help answer their question, even if they use completely different terms.
Think about:
1. What the user is trying to find or learn about - consider synonyms, related concepts, and specific products/tools
2. Which messages discuss relevant tools/concepts, even if they use different names
3. Messages that mention alternatives or related approaches
4. The context and flow of conversations - look for related messages before and after key discussions

For example:
- If someone asks about "AI models" or "language models", find messages about specific AI models like ChatGPT, DeepSeek, Claude, etc.
- If they ask about "AWS testing tools" or "local cloud testing", find messages about LocalStack
- If they ask about "collecting website data" or "data extraction", find messages about web scraping

When you find relevant messages:
1. Explain WHY these messages are relevant to their question
2. Point out the semantic connections (e.g. "DeepSeek is an AI model that was discussed here")
3. Include enough context to understand the discussion

Your response must be a raw JSON object with NO FORMATTING AT ALL.
Example: {"relevant_messages":["@username: exact message text"],"explanation":"why these messages are helpful"}

Remember: Focus on finding messages that would actually help them, even if the messages use completely different terminology.`, 
		question, messagesText.String())

	analysis, err := b.ai.AnswerQuestion(ctx, analysisPrompt, nil)
	if err != nil {
		return fmt.Errorf("failed to analyze messages: %v", err)
	}

	// Clean and parse the AI response
	analysis = cleanJSONResponse(analysis)
	
	var result struct {
		RelevantMessages []string `json:"relevant_messages"`
		Explanation      string   `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(analysis), &result); err != nil {
		return fmt.Errorf("failed to parse analysis: %v", err)
	}

	// If no relevant messages found
	if len(result.RelevantMessages) == 0 {
		return b.sendMessage(msg.Chat.ID, "I couldn't find any relevant discussions about this topic in our chat history. You might be the first one to bring this up!")
	}

	// Format the response
	var response strings.Builder
	response.WriteString(result.Explanation)
	response.WriteString("\n\nHere are the relevant discussions:\n\n")
	
	// Create message entities for clickable links
	var entities []tgbotapi.MessageEntity

	baseOffset := len(result.Explanation) + len("\n\nHere are the relevant discussions:\n\n")
	
	for i, relevantMsg := range result.RelevantMessages {
		// Extract message details
		parts := strings.SplitN(relevantMsg, ": ", 2)
		if len(parts) != 2 {
			continue
		}
		username := strings.TrimPrefix(parts[0], "@")
		messageText := parts[1]

		// Find the corresponding message from our fetched messages
		var messageID int64
		for _, m := range messages {
			if m.Username == username && m.Text == messageText {
				messageID = m.MessageID
				break
			}
		}

		// Format the message with a number and full text
		messageNum := fmt.Sprintf("%d. ", i+1)
		fullMessage := fmt.Sprintf("%s@%s: %s\n", messageNum, username, messageText)
		response.WriteString(fullMessage)

		// Create a text_link entity for the entire message line if we have the message ID
		if messageID != 0 {
			chatIDStr := fmt.Sprintf("%d", msg.Chat.ID)
			if strings.HasPrefix(chatIDStr, "-100") {
				chatIDStr = chatIDStr[4:]
			}
			
			// Add mention entity for the username
			entities = append(entities, tgbotapi.MessageEntity{
				Type:   "mention",
				Offset: baseOffset + len(messageNum),
				Length: len("@" + username),
			})
			
			// Add text_link entity for the entire message line
			entities = append(entities, tgbotapi.MessageEntity{
				Type:   "text_link",
				Offset: baseOffset,
				Length: len(fullMessage) - 1, // -1 to exclude the newline
				URL:    fmt.Sprintf("https://t.me/c/%s/%d", chatIDStr, messageID),
			})
		}
		
		baseOffset += len(fullMessage)
	}
	
	response.WriteString("\nTip: Click on any message to jump to that part of the chat history.")

	// Send message with entities
	replyMsg := tgbotapi.NewMessage(msg.Chat.ID, response.String())
	replyMsg.Entities = entities
	replyMsg.ParseMode = "" // Ensure no parsing mode interferes with our entities
	_, err = b.api.Send(replyMsg)
	return err
}

// cleanJSONResponse cleans up the AI's response to extract valid JSON
func cleanJSONResponse(response string) string {
	response = strings.TrimSpace(response)
	response = strings.ReplaceAll(response, "```json", "")
	response = strings.ReplaceAll(response, "```", "")
	response = strings.ReplaceAll(response, "`", "")
	response = strings.ReplaceAll(response, "\n", "")
	response = strings.ReplaceAll(response, "\r", "")
	response = strings.ReplaceAll(response, "\t", "")
	
	// Extract JSON between first { and last }
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start != -1 && end != -1 && end > start {
		response = response[start:end+1]
	}
	
	return strings.TrimSpace(response)
}

// groupMessagesByContext groups messages based on their semantic context
func groupMessagesByContext(messages []models.Message, relevanceCriteria string) [][]models.Message {
	if len(messages) == 0 {
		return nil
	}

	// Sort messages by time
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})

	// Group messages that are semantically related and within time window
	const conversationTimeout = 2 * time.Minute
	var conversations [][]models.Message
	currentConvo := []models.Message{messages[0]}

	for i := 1; i < len(messages); i++ {
		timeDiff := messages[i].CreatedAt.Sub(messages[i-1].CreatedAt)
		
		// Check if messages are related by:
		// 1. Time proximity
		// 2. Direct replies
		// 3. Shared context (based on the AI's relevance criteria)
		isRelated := false
		
		// Time proximity check
		if timeDiff <= conversationTimeout {
			isRelated = true
		}
		
		// Direct reply check
		if isDirectReply(messages[i-1].Text, messages[i].Text) {
			isRelated = true
		}
		
		// Context similarity check (if messages share significant terms)
		prevWords := extractSignificantTerms(messages[i-1].Text)
		currWords := extractSignificantTerms(messages[i].Text)
		if hasCommonTerms(prevWords, currWords) {
			isRelated = true
		}
		
		if isRelated {
			currentConvo = append(currentConvo, messages[i])
		} else {
			if len(currentConvo) > 0 {
				conversations = append(conversations, currentConvo)
			}
			currentConvo = []models.Message{messages[i]}
		}
	}
	
	if len(currentConvo) > 0 {
		conversations = append(conversations, currentConvo)
	}

	return conversations
}

// extractSignificantTerms extracts meaningful terms from text
func extractSignificantTerms(text string) []string {
	text = strings.ToLower(text)
	words := strings.Fields(text)
	var terms []string
	
	for _, word := range words {
		// Clean the word
		word = strings.Trim(word, ".,!?()[]{}:;\"'")
		
		// Keep significant words
		if len(word) > 3 && !isCommonWord(word) {
			terms = append(terms, word)
		}
	}
	
	return terms
}

// hasCommonTerms checks if two sets of terms share any significant words
func hasCommonTerms(terms1, terms2 []string) bool {
	// Create map of first set of terms
	termMap := make(map[string]bool)
	for _, term := range terms1 {
		termMap[term] = true
	}
	
	// Check if any term from second set exists in map
	for _, term := range terms2 {
		if termMap[term] {
			return true
		}
		// Also check for substring matches
		for term1 := range termMap {
			if len(term1) > 3 && len(term) > 3 {
				if strings.Contains(term1, term) || strings.Contains(term, term1) {
					return true
				}
			}
		}
	}
	
	return false
}

// isTopicRelated checks if two topics are related
func isTopicRelated(topic1, topic2 string) bool {
	// If either topic is empty, they're not related
	if topic1 == "" || topic2 == "" {
		return false
	}
	
	// Topics are related if:
	// 1. They are exactly the same
	if topic1 == topic2 {
		return true
	}
	
	// 2. They are part of the same technical group
	technicalGroups := map[string][]string{
		"stack":   {"localstack", "aws", "cloud", "docker"},
		"scrape":  {"crawler", "crawling", "scraping", "extract"},
		"docker":  {"container", "localstack", "stack"},
		"aws":     {"localstack", "cloud", "stack"},
	}
	
	// Check if topics belong to the same group
	for _, group := range technicalGroups {
		inGroup1 := false
		inGroup2 := false
		for _, term := range group {
			if strings.Contains(topic1, term) {
				inGroup1 = true
			}
			if strings.Contains(topic2, term) {
				inGroup2 = true
			}
		}
		if inGroup1 && inGroup2 {
			return true
		}
	}
	
	// 3. One contains the other
	if strings.Contains(topic1, topic2) || strings.Contains(topic2, topic1) {
		return true
	}
	
	return false
}

// isDirectReply checks if a message is a direct reply to the previous message
func isDirectReply(prevText, currText string) bool {
	// Convert to lowercase for consistent matching
	prevText = strings.ToLower(prevText)
	currText = strings.ToLower(currText)
	
	// Check if it's a short response (less than 5 words) to a question
	if strings.HasSuffix(prevText, "?") {
		words := strings.Fields(currText)
		if len(words) < 5 {
			return true
		}
	}
	
	// Check if the current message references words from the previous message
	prevWords := strings.Fields(prevText)
	currWords := strings.Fields(currText)
	
	// Get significant words from previous message
	var significantPrevWords []string
	for _, word := range prevWords {
		if len(word) > 3 && !isCommonWord(word) {
			significantPrevWords = append(significantPrevWords, word)
		}
	}
	
	// Check if current message contains any significant words from previous message
	for _, currWord := range currWords {
		for _, prevWord := range significantPrevWords {
			if strings.Contains(strings.ToLower(currWord), strings.ToLower(prevWord)) {
				return true
			}
		}
	}
	
	return false
}

// isCommonWord returns true if the word is too common to be useful for topic detection
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
		"know": true, "like": true, "time": true, "make": true, "see": true,
		"find": true, "want": true, "does": true, "need": true, "going": true,
		"after": true, "again": true, "our": true, "well": true, "way": true,
		"even": true, "new": true, "because": true, "give": true, "day": true,
		"anyone": true, "anybody": true, "anything": true, "everyone": true,
		"everybody": true, "everything": true, "someone": true, "somebody": true,
		"something": true, "nothing": true, "nobody": true, "none": true,
	}
	return commonWords[word]
} 