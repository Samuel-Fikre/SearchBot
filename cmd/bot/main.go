package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"SearchBot/internal/ai"
	"SearchBot/internal/bot"
	"SearchBot/internal/models"
	"SearchBot/internal/search"
	"SearchBot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"github.com/meilisearch/meilisearch-go"
)

var (
	mongoStorage storage.MessageStorage
	meiliSearch  *search.MeiliSearch
	geminiAI     *ai.GeminiAI
	searchBot    *bot.Bot
)

func init() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found")
	}

	// Initialize MongoDB
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Fatal("MONGODB_URI is not set in .env file")
	}

	log.Printf("Connecting to MongoDB...")

	// Initialize MongoDB storage with longer timeout
	mongoStore, err := storage.NewMongoDB(mongoURI, "telegram_bot", "messages")
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}
	mongoStorage = mongoStore
	log.Printf("Successfully connected to MongoDB")

	// Initialize Meilisearch
	meiliHost := os.Getenv("MEILI_HOST")
	log.Printf("Meilisearch Host from env: %s", meiliHost)
	if meiliHost == "" {
		meiliHost = "http://localhost:7700"
		log.Printf("Warning: MEILI_HOST not set, using default: %s", meiliHost)
	}
	meiliKey := os.Getenv("MEILI_KEY")
	log.Printf("Meilisearch Key length: %d", len(meiliKey))
	if meiliKey == "" {
		log.Printf("Warning: MEILI_KEY not set")
	}
	meiliSearch = search.NewMeiliSearch(meiliHost, meiliKey, "messages")
	log.Printf("Initialized Meilisearch with host: %s", meiliHost)

	// Initialize Gemini AI
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		log.Fatal("GEMINI_API_KEY is not set in .env file")
	}

	// Initialize Gemini AI client
	geminiClient, err := ai.NewGeminiAI(geminiKey)
	if err != nil {
		log.Fatal("Failed to initialize Gemini AI:", err)
	}
	geminiAI = geminiClient
}

func main() {
	// Get bot token from environment variable
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is not set in .env file")
	}

	// Initialize bot API
	api, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatal(err)
	}

	api.Debug = true
	log.Printf("Authorized on account %s", api.Self.UserName)

	// Create bot instance
	searchBot = bot.NewBot(api, geminiAI, meiliSearch, mongoStorage)

	// Set up updates configuration
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60
	updateConfig.AllowedUpdates = []string{"message", "channel_post", "my_chat_member"}

	// Get updates channel
	updates := api.GetUpdatesChan(updateConfig)

	// Handle updates
	for update := range updates {
		// Handle bot being added to a group
		if update.MyChatMember != nil {
			handleChatMemberUpdate(api, update.MyChatMember)
			continue
		}

		// Handle messages
		if update.Message != nil {
			// Log received message
			log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

			// Handle commands
			if update.Message.IsCommand() {
				handleCommand(api, update.Message)
				continue
			}

			// Store regular messages
			if update.Message.Chat.IsGroup() || update.Message.Chat.IsSuperGroup() {
				storeMessage(update.Message)
			}
		}
	}
}

func handleChatMemberUpdate(api *tgbotapi.BotAPI, update *tgbotapi.ChatMemberUpdated) {
	// Check if the bot was added to a group
	if update.NewChatMember.User.ID == api.Self.ID {
		switch update.NewChatMember.Status {
		case "member", "administrator":
			// Bot was added to group or made admin
			msg := tgbotapi.NewMessage(update.Chat.ID,
				"Thanks for adding me! I'll start indexing recent messages.\n\n"+
					"Required permissions:\n"+
					"- Read Messages\n"+
					"- Send Messages\n\n"+
					"Use /help to see available commands.")
			if _, err := api.Send(msg); err != nil {
				log.Printf("Error sending welcome message: %v", err)
			}

			// Fetch recent messages if bot is admin
			if update.NewChatMember.Status == "administrator" {
				go func() {
					// Get chat history using our helper
					messages, err := searchBot.GetChatHistory(update.Chat.ID, 100)
					if err != nil {
						log.Printf("Error fetching chat history: %v", err)
						errorMsg := tgbotapi.NewMessage(update.Chat.ID,
							"❌ Failed to fetch chat history. Please make sure I have the correct permissions.")
						if _, err := api.Send(errorMsg); err != nil {
							log.Printf("Error sending error message: %v", err)
						}
						return
					}

					// Store messages in reverse order (oldest first)
					storedCount := 0
					for i := len(messages) - 1; i >= 0; i-- {
						msg := messages[i]
						if msg.Text != "" { // Only store text messages
							storeMessage(&msg)
							storedCount++
						}
					}

					// Send confirmation
					confirmMsg := tgbotapi.NewMessage(update.Chat.ID,
						fmt.Sprintf("✅ Successfully indexed %d text messages from the chat history.", storedCount))
					if _, err := api.Send(confirmMsg); err != nil {
						log.Printf("Error sending confirmation message: %v", err)
					}
				}()
			}
		}
	}
}

func handleCommand(api *tgbotapi.BotAPI, message *tgbotapi.Message) {
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
					UserID: api.Self.ID,
				},
			}

			member, err := api.GetChatMember(chatMemberConfig)
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
			// Create search request
			searchReq := &meilisearch.SearchRequest{
				Query:                query,
				Limit:                50,
				AttributesToSearchOn: []string{"text"},
				Sort:                 []string{"created_at:desc"},
			}

			results, err := meiliSearch.SearchMessages(message.Chat.ID, searchReq)
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
		if err := searchBot.HandleAskCommand(context.Background(), message); err != nil {
			log.Printf("Error handling ask command: %v", err)
			msg.Text = "Sorry, an error occurred while processing your question."
		}
		return
	default:
		msg.Text = "Unknown command. Use /help to see available commands."
	}

	if _, err := api.Send(msg); err != nil {
		log.Printf("Error sending message: %v", err)
	}
}

func storeMessage(message *tgbotapi.Message) error {
	// Create message model
	msg := &models.Message{
		MessageID:    int64(message.MessageID),
		ChatID:       message.Chat.ID,
		ChatUsername: message.Chat.UserName,
		UserID:       message.From.ID,
		Username:     message.From.UserName,
		Text:         message.Text,
		CreatedAt:    time.Now(),
	}

	// Store in MongoDB
	if err := mongoStorage.StoreMessage(msg); err != nil {
		log.Printf("Failed to store message: %v", err)
		return err
	}

	// Index in Meilisearch
	if err := meiliSearch.IndexMessage(msg); err != nil {
		log.Printf("Failed to index message: %v", err)
		return err
	}

	log.Printf("Successfully processed message: %d", msg.MessageID)
	return nil
}
