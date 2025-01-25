# Telegram Search Bot

An AI-powered Telegram bot that provides advanced search capabilities for group chats using Go, MongoDB, Meilisearch, and Hugging Face.

## Features

- Store and index messages from Telegram groups
- Fast and typo-tolerant search using Meilisearch
- AI-powered contextual search using Hugging Face
- Efficient message storage with MongoDB

## Prerequisites

- Go 1.19 or later
- MongoDB
- Meilisearch
- Telegram Bot Token
- Hugging Face API Key

## Setup

1. Clone the repository
2. Copy `.env.example` to `.env` and fill in your configuration values
3. Install dependencies:
   ```bash
   go mod download
   ```
4. Run the bot:
   ```bash
   go run cmd/bot/main.go
   ```

## Configuration

Set the following environment variables in your `.env` file:

- `TELEGRAM_BOT_TOKEN`: Your Telegram bot token from BotFather
- `MONGODB_URI`: MongoDB connection string
- `MONGODB_DATABASE`: MongoDB database name
- `MEILISEARCH_HOST`: Meilisearch server URL
- `MEILISEARCH_API_KEY`: Meilisearch API key
- `HUGGINGFACE_API_KEY`: Hugging Face API key

## Usage

1. Add the bot to your Telegram group
2. The bot will automatically index all messages
3. Use `/search <query>` to search for messages
4. Use `/ask <question>` for AI-powered contextual search

## License

MIT # SearchBot
# SearchBot
