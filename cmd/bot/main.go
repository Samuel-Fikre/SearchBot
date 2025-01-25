package main

import (
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

func init() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found")
	}
}

func main() {
	// Get bot token from environment variable
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is not set in .env file")
	}

	// Initialize bot
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatal(err)
	}

	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Set up updates configuration
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60

	// Get updates channel
	updates := bot.GetUpdatesChan(updateConfig)

	// Handle updates
	for update := range updates {
		if update.Message == nil {
			continue
		}

		// Log received message
		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

		// Handle commands
		if update.Message.IsCommand() {
			handleCommand(bot, update.Message)
			continue
		}

		// Store regular messages for indexing (to be implemented)
		storeMessage(update.Message)
	}
}

func handleCommand(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	msg := tgbotapi.NewMessage(message.Chat.ID, "")

	switch message.Command() {
	case "start":
		msg.Text = "Hello! I'm a search bot. I can help you find messages in this group. Use /help to see available commands."
	case "help":
		msg.Text = `Available commands:
/search <query> - Search for messages
/ask <question> - Ask a question about past messages
/help - Show this help message`
	case "search":
		query := message.CommandArguments()
		if query == "" {
			msg.Text = "Please provide a search query. Example: /search golang"
		} else {
			// TODO: Implement search functionality
			msg.Text = "Search functionality coming soon!"
		}
	case "ask":
		question := message.CommandArguments()
		if question == "" {
			msg.Text = "Please provide a question. Example: /ask What was discussed about Docker?"
		} else {
			// TODO: Implement AI-powered question answering
			msg.Text = "AI-powered search coming soon!"
		}
	default:
		msg.Text = "Unknown command. Use /help to see available commands."
	}

	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending message: %v", err)
	}
}

func storeMessage(message *tgbotapi.Message) {
	// TODO: Implement message storage in MongoDB and indexing in Meilisearch
	log.Printf("Message stored: %s", message.Text)
}
