package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

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
	searchContext := fmt.Sprintf(`You are a search assistant. A user asked: '%s'

CRITICAL: Your response must be a raw JSON object with NO FORMATTING AT ALL.
DO NOT use markdown.
DO NOT use code blocks.
DO NOT use backticks.
DO NOT use newlines.
DO NOT use indentation.
DO NOT add any text before or after the JSON.

Your response should look EXACTLY like this:
{"key_terms":["term1"],"relevance_criteria":"criteria","search_query":"query"}

REMEMBER: Return ONLY the JSON object, nothing else.`, question)
	
	log.Printf("Sending prompt to AI:\n%s", searchContext)
	
	searchStrategy, err := b.ai.AnswerQuestion(ctx, searchContext, nil)
	if err != nil {
		msg.Text = "Sorry, I'm having trouble understanding the question. Could you rephrase it?"
		log.Printf("AI error in search strategy: %v", err)
		return b.sendMessage(msg.Chat.ID, msg.Text)
	}
	
	log.Printf("Raw AI response:\n%s", searchStrategy)
	
	// Clean up the response to extract just the JSON
	searchStrategy = strings.TrimSpace(searchStrategy)
	
	// Remove any potential markdown or formatting
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
	
	log.Printf("Cleaned search strategy:\n%s", searchStrategy)
	
	// Final validation
	if !strings.HasPrefix(searchStrategy, "{") || !strings.HasSuffix(searchStrategy, "}") {
		msg.Text = "Sorry, I'm having trouble processing the search strategy. Could you try asking in a different way?"
		log.Printf("Invalid JSON format - missing braces: %s", searchStrategy)
		return b.sendMessage(msg.Chat.ID, msg.Text)
	}
	
	// Validate JSON structure
	var jsonCheck map[string]interface{}
	if err := json.Unmarshal([]byte(searchStrategy), &jsonCheck); err != nil {
		msg.Text = "Sorry, I'm having trouble processing the search strategy. Could you try asking in a different way?"
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
		return b.sendMessage(msg.Chat.ID, "I couldn't find any relevant messages in the chat history. Try asking a different question or wait for more messages to be added.")
	}

	// Generate answer based on found messages and original question
	answer, err := b.ai.AnswerQuestion(ctx, question, messages)
	if err != nil {
		return fmt.Errorf("failed to generate answer: %v", err)
	}

	// Send the answer
	return b.sendMessage(msg.Chat.ID, answer)
} 