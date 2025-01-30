package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"SearchBot/internal/ai"
	"SearchBot/internal/models"
	"SearchBot/internal/search"
	"SearchBot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

var (
	mongoStorage *storage.MongoStorage
	meiliSearch *search.MeiliSearch
	geminiAI    *ai.GeminiAI
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

	// Initialize Gemini AI
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		log.Fatal("GEMINI_API_KEY is not set in .env file")
	}

	geminiAI, err = ai.NewGeminiAI(geminiKey)
	if err != nil {
		log.Fatal("Failed to initialize Gemini AI:", err)
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
	// Add allowed update types
	updateConfig.AllowedUpdates = []string{"message", "channel_post", "my_chat_member"}

	// Get updates channel
	updates := bot.GetUpdatesChan(updateConfig)

	// Handle updates
	for update := range updates {
		// Handle bot being added to a group
		if update.MyChatMember != nil {
			handleChatMemberUpdate(bot, update.MyChatMember)
			continue
		}

		// Handle messages
		if update.Message != nil {
			// Log received message
			log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

			// Handle commands
			if update.Message.IsCommand() {
				handleCommand(bot, update.Message)
				continue
			}

			// Store regular messages
			if update.Message.Chat.IsGroup() || update.Message.Chat.IsSuperGroup() {
				storeMessage(update.Message)
			}
		}
	}
}

func handleChatMemberUpdate(bot *tgbotapi.BotAPI, update *tgbotapi.ChatMemberUpdated) {
	// Check if the bot was added to a group
	if update.NewChatMember.User.ID == bot.Self.ID {
		switch update.NewChatMember.Status {
		case "member", "administrator":
			// Bot was added to group or made admin
			msg := tgbotapi.NewMessage(update.Chat.ID, 
				"Thanks for adding me! Please make me an administrator with access to messages to enable full functionality.\n\n"+
				"Required permissions:\n"+
				"- Read Messages\n"+
				"- Send Messages\n\n"+
				"Use /help to see available commands.")
			if _, err := bot.Send(msg); err != nil {
				log.Printf("Error sending welcome message: %v", err)
			}
		}
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
/status - Check bot permissions and status
/help - Show this help message`
	case "status":
		if message.Chat.IsGroup() || message.Chat.IsSuperGroup() {
			// Get bot's member info in the group
			chatMemberConfig := tgbotapi.GetChatMemberConfig{
				ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
					ChatID: message.Chat.ID,
					UserID: bot.Self.ID,
				},
			}
			
			member, err := bot.GetChatMember(chatMemberConfig)
			if err != nil {
				msg.Text = "Error checking bot status."
				log.Printf("Error getting bot member info: %v", err)
			} else {
				msg.Text = fmt.Sprintf("Bot Status in this group:\n"+
					"Role: %s\n", member.Status)
				
				if member.Status == "administrator" {
					msg.Text += "✅ Bot is properly configured with admin access.\n"
				} else {
					msg.Text += "❌ Bot needs to be an administrator to access messages.\n" +
						"Please make me an administrator with these permissions:\n" +
						"- Read Messages\n" +
						"- Send Messages"
				}
			}
		} else {
			msg.Text = "This command only works in groups."
		}
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
			// First, search for relevant messages
			results, err := meiliSearch.Search(question)
			if err != nil {
				msg.Text = "Sorry, an error occurred while searching."
				log.Printf("Search error: %v", err)
			} else if len(results) == 0 {
				msg.Text = "I couldn't find any messages related to your question."
			} else {
				// Use Gemini to generate an answer based on the relevant messages
				answer, err := geminiAI.AnswerQuestion(context.Background(), question, results)
				if err != nil {
					msg.Text = "Sorry, I couldn't generate an answer at this time."
					log.Printf("Gemini error: %v", err)
				} else {
					msg.Text = answer
				}
			}
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
