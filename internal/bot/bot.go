package bot

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleAskCommand(ctx context.Context, msg *tgbotapi.Message) error {
	// Extract the question from the message
	question := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/ask"))
	if question == "" {
		return b.sendMessage(msg.Chat.ID, "Please provide a question after /ask")
	}

	// First, ask AI to understand the question and generate a search strategy
	searchStrategy, err := b.ai.AnswerQuestion(ctx, fmt.Sprintf(`Analyze this question and generate a search strategy in JSON format:
Question: "%s"
Generate a JSON object with:
1. key_terms: array of important terms to search for
2. relevance_criteria: what makes a message relevant to this question
3. search_query: the most effective search term to find relevant messages
Example: {"key_terms":["docker","container"],"relevance_criteria":"messages about Docker containers and their usage","search_query":"docker"}`, question), nil)
	if err != nil {
		return fmt.Errorf("failed to generate search strategy: %v", err)
	}

	// Search for relevant messages using the AI-generated strategy
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