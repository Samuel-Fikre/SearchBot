package main

import (
	"log"
	"os"
	"time"

	"SearchBot/internal/models"
	"SearchBot/internal/search"
	"SearchBot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

var (
	mongoStorage *storage.MongoStorage
	meiliSearch *search.MeiliSearch
)

func init() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found")
	}

	// Initialize MongoDB
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	var err error
	mongoStorage, err = storage.NewMongoStorage(mongoURI, "telegram_bot", "messages")
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}

	// Initialize Meilisearch
	meiliHost := os.Getenv("MEILI_HOST")
	if meiliHost == "" {
		meiliHost = "http://localhost:7700"
	}
	meiliKey := os.Getenv("MEILI_KEY")
	meiliSearch = search.NewMeiliSearch(meiliHost, meiliKey, "messages")
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

		// Store regular messages
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
			results, err := meiliSearch.Search(query)
			if err != nil {
				msg.Text = "Sorry, an error occurred while searching."
				log.Printf("Search error: %v", err)
			} else if len(results) == 0 {
				msg.Text = "No messages found matching your query."
			} else {
				msg.Text = "Found messages:\n\n"
				for _, result := range results {
					msg.Text += "From @" + result.Username + ":\n" + result.Text + "\n\n"
				}
			}
		}
	case "ask":
		question := message.CommandArguments()
		if question == "" {
			msg.Text = "Please provide a question. Example: /ask What was discussed about Docker?"
		} else {
			// TODO: Implement AI-powered question answering with Hugging Face
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
	msg := &models.Message{
		MessageID: int64(message.MessageID),
		ChatID:    message.Chat.ID,
		UserID:    message.From.ID,
		Username:  message.From.UserName,
		Text:      message.Text,
		CreatedAt: time.Now(),
	}

	// Store in MongoDB
	if err := mongoStorage.StoreMessage(msg); err != nil {
		log.Printf("Error storing message in MongoDB: %v", err)
		return
	}

	// Index in Meilisearch
	if err := meiliSearch.IndexMessage(msg); err != nil {
		log.Printf("Error indexing message in Meilisearch: %v", err)
	}
}
