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
	"SearchBot/internal/search"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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

	// First, let AI understand the question and formulate a search strategy
	searchContext := fmt.Sprintf(`You are a search assistant helping find relevant conversations in a coding group chat.
A user asked: '%s'

Your task is to identify key concepts and terms that would help find similar discussions.
DO NOT try to answer the question. Instead, think about what words and phrases people might have used when discussing this topic.

CRITICAL: Your response must be a raw JSON object with NO FORMATTING AT ALL.
DO NOT use markdown, code blocks, backticks, newlines, or indentation.
DO NOT add any text before or after the JSON.

Your response should look EXACTLY like this:
{"key_terms":["term1","term2"],"relevance_criteria":"what makes a message part of a relevant discussion"}

REMEMBER: Focus on finding CONVERSATIONS where this was discussed, not answering the question.`, question)
	
	log.Printf("Sending prompt to AI:\n%s", searchContext)
	
	searchStrategy, err := b.ai.AnswerQuestion(ctx, searchContext, nil)
	if err != nil {
		msg.Text = "Sorry, I'm having trouble understanding what you're looking for. Could you rephrase it?"
		log.Printf("AI error in search strategy: %v", err)
		return b.sendMessage(msg.Chat.ID, msg.Text)
	}
	
	log.Printf("Raw AI response:\n%s", searchStrategy)
	
	// Clean up the response to extract just the JSON
	searchStrategy = strings.TrimSpace(searchStrategy)
	searchStrategy = strings.ReplaceAll(searchStrategy, "```json", "")
	searchStrategy = strings.ReplaceAll(searchStrategy, "```", "")
	searchStrategy = strings.ReplaceAll(searchStrategy, "`", "")
	searchStrategy = strings.ReplaceAll(searchStrategy, "\n", "")
	searchStrategy = strings.ReplaceAll(searchStrategy, "\r", "")
	searchStrategy = strings.ReplaceAll(searchStrategy, "\t", "")
	searchStrategy = strings.ReplaceAll(searchStrategy, "  ", " ")
	
	// Remove any text before the first { and after the last }
	if start := strings.Index(searchStrategy, "{"); start != -1 {
		if end := strings.LastIndex(searchStrategy, "}"); end != -1 && end > start {
			searchStrategy = searchStrategy[start : end+1]
		}
	}
	
	searchStrategy = strings.TrimSpace(searchStrategy)
	
	// Final validation
	if !strings.HasPrefix(searchStrategy, "{") || !strings.HasSuffix(searchStrategy, "}") {
		msg.Text = "Sorry, I'm having trouble processing your search. Could you try asking in a different way?"
		log.Printf("Invalid JSON format - missing braces: %s", searchStrategy)
		return b.sendMessage(msg.Chat.ID, msg.Text)
	}
	
	// Validate JSON structure
	var jsonCheck map[string]interface{}
	if err := json.Unmarshal([]byte(searchStrategy), &jsonCheck); err != nil {
		msg.Text = "Sorry, I'm having trouble processing your search. Could you try asking in a different way?"
		log.Printf("JSON validation error: %v\nInvalid JSON: %s", err, searchStrategy)
		return b.sendMessage(msg.Chat.ID, msg.Text)
	}
	
	log.Printf("Valid JSON object: %+v", jsonCheck)
	
	// Now search for relevant messages using the AI's strategy
	messages, err := b.search.SearchMessages(ctx, searchStrategy)
	if err != nil {
		return fmt.Errorf("failed to search messages: %v", err)
	}

	// If no messages found, inform the user
	if len(messages) == 0 {
		return b.sendMessage(msg.Chat.ID, "I couldn't find any past discussions about this topic in the chat history. You might be the first one to bring this up!")
	}

	// Group messages by conversation
	conversations := groupMessagesByConversation(messages)
	
	// Format the response to show conversation snippets
	var response strings.Builder
	response.WriteString("I found some relevant discussions:\n\n")
	
	for i, convo := range conversations {
		response.WriteString(fmt.Sprintf("Conversation %d:\n", i+1))
		for _, msg := range convo {
			response.WriteString(fmt.Sprintf("@%s: %s\n", msg.Username, msg.Text))
		}
		response.WriteString("\n")
	}
	
	response.WriteString("\nTip: You can click on any message to jump to that part of the chat history.")

	// Send the answer
	return b.sendMessage(msg.Chat.ID, response.String())
}

// groupMessagesByConversation groups messages that are part of the same conversation
// based on time proximity and context
func groupMessagesByConversation(messages []models.Message) [][]models.Message {
	if len(messages) == 0 {
		return nil
	}

	// Sort messages by time
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})

	// Group messages that are within 5 minutes of each other
	const conversationTimeout = 5 * time.Minute
	var conversations [][]models.Message
	currentConvo := []models.Message{messages[0]}

	for i := 1; i < len(messages); i++ {
		timeDiff := messages[i].CreatedAt.Sub(messages[i-1].CreatedAt)
		if timeDiff <= conversationTimeout {
			// Same conversation
			currentConvo = append(currentConvo, messages[i])
		} else {
			// New conversation
			conversations = append(conversations, currentConvo)
			currentConvo = []models.Message{messages[i]}
		}
	}
	
	// Add the last conversation
	if len(currentConvo) > 0 {
		conversations = append(conversations, currentConvo)
	}

	return conversations
} 