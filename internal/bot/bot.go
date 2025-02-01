package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	// First check if bot has necessary permissions
	chatMember, err := b.api.GetChatMember(tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: msg.Chat.ID,
			UserID: b.api.Self.ID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to check bot permissions: %v", err)
	}

	// Check if bot is admin and has message access
	if chatMember.Status != "administrator" {
		return b.sendMessage(msg.Chat.ID, 
			"⚠️ I need to be an administrator to access message history.\n"+
			"Please make me an administrator with these permissions:\n"+
			"- Read Messages\n"+
			"- Send Messages")
	}

	// Extract the question from the message
	question := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/ask"))
	if question == "" {
		return b.sendMessage(msg.Chat.ID, "Please provide a question after /ask")
	}

	log.Printf("Processing question: %s", question)

	// First, fetch recent messages from the database
	searchReq := &meilisearch.SearchRequest{
		Query: "",  // Empty query to get all messages
		Limit: 100, // Get a good number of recent messages
		AttributesToSearchOn: []string{"text"},
		Sort: []string{"created_at:desc"}, // Most recent first
	}
	
	messages, err := b.search.SearchMessages(msg.Chat.ID, searchReq)
	if err != nil {
		return fmt.Errorf("failed to fetch messages: %v", err)
	}

	log.Printf("Found %d messages in database", len(messages))

	if len(messages) == 0 {
		return b.sendMessage(msg.Chat.ID, 
			"I don't have any messages in my database yet. "+
			"This could be because:\n"+
			"1. I was just added to the group\n"+
			"2. I don't have access to read messages\n"+
			"Please make sure I'm an administrator with message access and wait for new messages to be indexed.")
	}

	// Format all messages for AI analysis
	var messagesText strings.Builder
	for _, message := range messages {
		messagesText.WriteString(fmt.Sprintf("@%s: %s\n", message.Username, message.Text))
	}

	log.Printf("Sending %d messages to AI for analysis", len(messages))

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
4. IMPORTANT: You MUST include the EXACT messages in your response, including username and text

Your response must be a raw JSON object with NO FORMATTING AT ALL.
Example: {"relevant_messages":["@username: exact message text"],"explanation":"why these messages are helpful"}

Remember: 
1. Focus on finding messages that would actually help them, even if the messages use completely different terminology
2. You MUST include the EXACT messages in your response, do not paraphrase or summarize them
3. Include ALL relevant messages, even if they seem similar`, 
		question, messagesText.String())

	analysis, err := b.ai.AnswerQuestion(ctx, analysisPrompt, nil)
	if err != nil {
		return fmt.Errorf("failed to analyze messages: %v", err)
	}

	log.Printf("Received AI analysis response: %s", analysis)

	// Clean and parse the AI response
	analysis = cleanJSONResponse(analysis)
	
	var result struct {
		RelevantMessages []string `json:"relevant_messages"`
		Explanation      string   `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(analysis), &result); err != nil {
		log.Printf("Failed to parse AI response: %v", err)
		log.Printf("Raw response: %s", analysis)
		// Try to recover by searching for messages ourselves
		keywords := extractSignificantTerms(question)
		for _, message := range messages {
			messageTerms := extractSignificantTerms(message.Text)
			if hasCommonTerms(keywords, messageTerms) {
				result.RelevantMessages = append(result.RelevantMessages, 
					fmt.Sprintf("@%s: %s", message.Username, message.Text))
			}
		}
		if len(result.RelevantMessages) > 0 {
			result.Explanation = "Found some messages that might be relevant to your question."
		}
	}

	log.Printf("Found %d relevant messages", len(result.RelevantMessages))

	// If no relevant messages found in AI response, try to find them ourselves
	if len(result.RelevantMessages) == 0 {
		// Search for messages containing keywords from the question
		keywords := extractSignificantTerms(question)
		for _, message := range messages {
			messageTerms := extractSignificantTerms(message.Text)
			if hasCommonTerms(keywords, messageTerms) {
				result.RelevantMessages = append(result.RelevantMessages, 
					fmt.Sprintf("@%s: %s", message.Username, message.Text))
			}
		}
	}

	// If still no relevant messages found
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
	
	// Group messages by conversation
	var currentUsername string
	var currentConversation []models.Message
	var allConversations [][]models.Message

	// First, convert relevant messages to actual Message objects
	var relevantMessages []models.Message
	for _, relevantMsg := range result.RelevantMessages {
		parts := strings.SplitN(relevantMsg, ": ", 2)
		if len(parts) != 2 {
			log.Printf("Skipping malformed message: %s", relevantMsg)
			continue
		}
		username := strings.TrimPrefix(parts[0], "@")
		messageText := parts[1]

		// Find the corresponding message
		found := false
		for _, m := range messages {
			if m.Username == username && m.Text == messageText {
				relevantMessages = append(relevantMessages, m)
				found = true
				break
			}
		}
		if !found {
			log.Printf("Could not find original message for: @%s: %s", username, messageText)
		}
	}

	log.Printf("Successfully mapped %d relevant messages to original messages", len(relevantMessages))

	// Group messages by username
	for _, msg := range relevantMessages {
		if currentUsername == "" {
			currentUsername = msg.Username
		}
		
		if msg.Username != currentUsername {
			if len(currentConversation) > 0 {
				allConversations = append(allConversations, currentConversation)
				currentConversation = nil
			}
			currentUsername = msg.Username
		}
		currentConversation = append(currentConversation, msg)
	}
	if len(currentConversation) > 0 {
		allConversations = append(allConversations, currentConversation)
	}

	log.Printf("Grouped messages into %d conversations", len(allConversations))

	// Format conversations with numbers
	for i, conversation := range allConversations {
		// Format the conversation
		for j, message := range conversation {
			// Format the message
			var fullMessage string
			if j == 0 {
				fullMessage = fmt.Sprintf("%d. @%s: %s\n", i+1, message.Username, message.Text)
			} else {
				fullMessage = fmt.Sprintf("@%s: %s\n", message.Username, message.Text)
			}
			response.WriteString(fullMessage)

			// Create a text_link entity for the entire message line
			chatIDStr := fmt.Sprintf("%d", message.ChatID)
			// For supergroups, remove the -100 prefix and any remaining minus sign
			log.Printf("Original chatID: %s", chatIDStr)
			if strings.HasPrefix(chatIDStr, "-100") {
				chatIDStr = chatIDStr[4:] // Remove -100 prefix
			} else if strings.HasPrefix(chatIDStr, "-") {
				chatIDStr = chatIDStr[1:] // Remove single - prefix
			}
			log.Printf("Final chatID: %s, messageID: %d", chatIDStr, message.MessageID)
			
			// Add text_link entity for the entire message line
			messageURL := fmt.Sprintf("https://t.me/c/%s/%d", chatIDStr, message.MessageID)
			log.Printf("Generated URL: %s", messageURL)
			entities = append(entities, tgbotapi.MessageEntity{
				Type:   "text_link",
				Offset: baseOffset,
				Length: len(fullMessage) - 1, // -1 to exclude the newline
				URL:    messageURL,
			})
			
			baseOffset += len(fullMessage)
		}
		
		// Add a newline between conversations
		if i < len(allConversations)-1 {
			response.WriteString("\n")
			baseOffset += 1
		}
	}
	
	response.WriteString("\nTip: Click on any message to jump to that part of the chat history. "+
		"(Make sure I'm an administrator to access message history)")

	// Send message with entities
	replyMsg := tgbotapi.NewMessage(msg.Chat.ID, response.String())
	replyMsg.Entities = entities
	replyMsg.ParseMode = "" // Ensure no parsing mode interferes with our entities
	_, err = b.api.Send(replyMsg)
	if err != nil {
		log.Printf("Failed to send response: %v", err)
		// Try sending without entities as fallback
		return b.sendMessage(msg.Chat.ID, response.String())
	}
	return nil
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

// GetChatHistory fetches recent messages from a chat
func (b *Bot) GetChatHistory(chatID int64, limit int) ([]tgbotapi.Message, error) {
	var allMessages []tgbotapi.Message
	
	// Get chat member info to verify permissions
	chatMember, err := b.api.GetChatMember(tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: b.api.Self.ID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to check bot permissions: %v", err)
	}

	// Check if bot has necessary permissions
	if chatMember.Status != "administrator" {
		return nil, fmt.Errorf("bot needs to be an administrator to access message history")
	}

	// Send a temporary message to get current message ID
	msg := tgbotapi.NewMessage(chatID, "🔍 Fetching message history...")
	sentMsg, err := b.api.Send(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %v", err)
	}

	// Delete the temporary message
	deleteMsg := tgbotapi.NewDeleteMessage(chatID, sentMsg.MessageID)
	b.api.Request(deleteMsg)

	// Iterate through recent message IDs
	for i := 1; i <= limit; i++ {
		msgID := sentMsg.MessageID - i
		if msgID <= 0 {
			break
		}

		// Try to get the message by replying to it
		getMsg := tgbotapi.NewMessage(chatID, ".")
		getMsg.ReplyToMessageID = msgID
		
		msg, err := b.api.Send(getMsg)
		if err != nil {
			// Skip messages we can't access
			continue
		}

		// If we got a reply, that means we can access the original message
		if msg.ReplyToMessage != nil && msg.ReplyToMessage.Text != "" {
			allMessages = append(allMessages, *msg.ReplyToMessage)
		}
		
		// Delete our message
		deleteMsg = tgbotapi.NewDeleteMessage(chatID, msg.MessageID)
		b.api.Request(deleteMsg)

		// Rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	return allMessages, nil
} 