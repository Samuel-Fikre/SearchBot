# Telegram Search Bot

A Telegram bot that provides advanced search capabilities for group messages using MongoDB for storage, Meilisearch for fast searching, and Hugging Face for AI-powered question answering (coming soon).

## Features

- Store and index all messages from Telegram groups
- Fast and typo-tolerant search using Meilisearch
- MongoDB for reliable message storage
- AI-powered question answering (coming soon)

## Prerequisites

- Go 1.21 or later
- MongoDB
- Meilisearch
- A Telegram Bot Token (from @BotFather)

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/yourusername/SearchBot.git
   cd SearchBot
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Copy the sample .env file and fill in your credentials:
   ```bash
   cp .env.example .env
   ```

4. Edit the `.env` file with your configuration:
   - Add your Telegram Bot Token
   - Configure MongoDB connection (if not using default)
   - Configure Meilisearch connection (if not using default)

## Running the Bot

1. Start MongoDB:
   ```bash
   mongod
   ```

2. Start Meilisearch:
   ```bash
   meilisearch
   ```

3. Run the bot:
   ```bash
   go run cmd/bot/main.go
   ```

## Usage

Add the bot to your Telegram group and grant it admin privileges to read messages. The bot will automatically index all messages.

Available commands:
- `/start` - Start the bot
- `/help` - Show available commands
- `/search <query>` - Search for messages
- `/ask <question>` - Ask a question about past messages (coming soon)

## Development

The project structure:
```
.
├── cmd/
│   └── bot/
│       └── main.go
├── internal/
│   ├── models/
│   │   └── message.go
│   ├── search/
│   │   └── meilisearch.go
│   └── storage/
│       └── mongodb.go
├── .env
├── go.mod
└── README.md
```

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.
