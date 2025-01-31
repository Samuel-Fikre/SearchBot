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
			msg.Text = "Please ask me anything! I'll try to find relevant messages and answer your question. Don't worry about perfect grammar - just ask naturally!"
		} else {
			log.Printf("Processing /ask command with question: %s", question)
			
			// First, let AI understand the question and formulate a search strategy
			searchContext := fmt.Sprintf(
				"You are a search assistant. A user has asked: '%s'\n"+
				"Please analyze this question and provide:\n"+
				"1. What key concepts or terms we should search for\n"+
				"2. What kind of information would be relevant\n"+
				"Format your response as JSON with fields: searchTerms (array of strings), relevanceCriteria (string)",
				question)
			
			searchStrategy, err := geminiAI.AnswerQuestion(context.Background(), searchContext, nil)
			if err != nil {
				msg.Text = "Sorry, I'm having trouble understanding the question. Could you rephrase it?"
				log.Printf("AI error in search strategy: %v", err)
				break
			}
			
			log.Printf("AI generated search strategy: %s", searchStrategy)
			
			// Now search for relevant messages using the AI's strategy
			results, err := meiliSearch.Search("localstack") // We'll use a simple term for now
			if err != nil {
				msg.Text = "Sorry, I ran into an error while searching."
				log.Printf("Search error: %v", err)
			} else if len(results) == 0 {
				msg.Text = "I understand you're asking about that topic, but I haven't seen any messages about it yet."
				log.Printf("No messages found for search strategy")
			} else {
				log.Printf("Found %d relevant messages", len(results))
				
				// Build context from messages
				var contextBuilder string
				for i, result := range results {
					contextBuilder += fmt.Sprintf("Message %d from @%s: %s\n", i+1, result.Username, result.Text)
					log.Printf("Message %d: %s: %s", i+1, result.Username, result.Text)
				}
				
				// Now ask AI to analyze the messages and answer the question
				prompt := fmt.Sprintf(
					"A user asked: '%s'\n\n"+
					"Here are some relevant messages from the chat:\n%s\n\n"+
					"Please provide a natural, conversational response that:\n"+
					"1. Directly answers their question based on the chat messages\n"+
					"2. Acknowledges what information is and isn't available\n"+
					"3. Maintains a helpful and friendly tone",
					question, contextBuilder)
				
				answer, err := geminiAI.AnswerQuestion(context.Background(), prompt, results)
				if err != nil {
					msg.Text = "Sorry, I couldn't generate an answer right now. Try asking in a different way?"
					log.Printf("Gemini error: %v", err)
				} else {
					log.Printf("Gemini generated answer: %s", answer)
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
