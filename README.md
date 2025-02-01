# SearchBot - Intelligent Telegram Group Chat Search Assistant

SearchBot is a powerful Telegram bot that helps you search through your group chat history using natural language queries and AI-powered semantic search. It automatically indexes messages and provides intelligent search capabilities to find relevant discussions, even when the exact terms don't match.

## Features

- 🔍 **Semantic Search**: Find relevant messages even when using different terminology
- 🤖 **AI-Powered**: Uses Google's Gemini AI to understand questions and find relevant context
- 📝 **Automatic Indexing**: Indexes new messages automatically when added to a group
- 🔗 **Clickable Results**: Direct links to jump to specific messages in chat history
- 📊 **Context Preservation**: Shows full conversation threads for better understanding
- ⚡ **Fast Search**: Uses Meilisearch for lightning-fast text search
- 🔐 **Permission Aware**: Respects Telegram's permission system

## Installation

### Prerequisites

1. Go 1.19 or later
2. MongoDB
3. Meilisearch
4. Telegram Bot Token
5. Google Gemini API Key

### Setup Instructions

1. Clone the repository:
   ```bash
   git clone https://github.com/Samuel-Fikre/SearchBot.git
   cd SearchBot
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Set up environment variables:
   ```bash
   export TELEGRAM_BOT_TOKEN="your_bot_token"
   export MONGODB_URI="your_mongodb_uri"
   export MEILISEARCH_HOST="your_meilisearch_host"
   export MEILISEARCH_KEY="your_meilisearch_key"
   export GEMINI_API_KEY="your_gemini_api_key"
   ```

4. Run the bot:
   ```bash
   go run cmd/bot/main.go
   ```

### Docker Installation

1. Build the Docker image:
   ```bash
   docker build -t searchbot .
   ```

2. Run with Docker:
   ```bash
   docker run -d \
     -e TELEGRAM_BOT_TOKEN="your_bot_token" \
     -e MONGODB_URI="your_mongodb_uri" \
     -e MEILISEARCH_HOST="your_meilisearch_host" \
     -e MEILISEARCH_KEY="your_meilisearch_key" \
     -e GEMINI_API_KEY="your_gemini_api_key" \
     searchbot
   ```

## Deployment

### Using Meilisearch Cloud

1. **Set up Meilisearch Cloud**
   - Create an account at [Meilisearch Cloud](https://cloud.meilisearch.com)
   - Create a new project and select your preferred region
   - Generate a secure Master Key:
     - At least 16 characters long
     - Mix of uppercase and lowercase letters
     - Include numbers and special characters
     - Avoid dictionary words or patterns
   - Save your Project URL and Master Key securely

2. **Configure Railway Deployment**
   - Fork this repository to your GitHub account
   - Create a new project on [Railway](https://railway.app)
   - Connect your GitHub repository
   - Add the following environment variables:
     ```
     TELEGRAM_BOT_TOKEN=your_bot_token
     MONGODB_URI=your_mongodb_uri
     MEILISEARCH_HOST=your_meilisearch_cloud_url
     MEILISEARCH_KEY=your_secure_master_key  # Example format: xM3i#K9$pL2*vN8@qR5
     GEMINI_API_KEY=your_gemini_api_key
     ```
   - Deploy your project

### Security Notes
- Never share or commit your master key
- Rotate the key periodically
- Use different keys for development and production
- Store keys in secure environment variables, never in code

### Storage and Scaling
- Meilisearch Cloud handles storage and scaling automatically
- Choose a plan based on your expected message volume
- Automatic backups are included
- Monitoring and analytics available through Meilisearch Cloud dashboard

## Usage

### Adding the Bot to Your Group

1. Add @RedatSearchBot to your Telegram group
2. Make the bot an administrator with these permissions:
   - Read Messages
   - Send Messages
3. The bot will automatically start indexing new messages

### Available Commands

- `/ask <question>` - Ask a question about past discussions
- `/search <query>` - Search for specific messages
- `/help` - Show available commands
- `/status` - Check bot permissions and status

### Use Cases

1. **Finding Previous Discussions**
   ```
   /ask Has anyone discussed using LocalStack for AWS testing?
   ```
   The bot will find relevant messages about LocalStack or AWS testing, even if they use different terminology.

2. **Technical Support**
   ```
   /ask What solutions were suggested for Docker permission issues?
   ```
   The bot will find and show previous discussions about Docker permission problems and their solutions.

3. **Code Examples**
   ```
   /ask Can someone show me how to use the Meilisearch Go client?
   ```
   The bot will find messages containing code examples or discussions about Meilisearch implementation.

4. **Project References**
   ```
   /ask What tools were recommended for web scraping?
   ```
   The bot will find messages mentioning web scraping tools, libraries, or related discussions.

### Search Tips

1. **Be Specific**: Include relevant technical terms in your questions
2. **Context Matters**: The bot understands technical relationships (e.g., AWS ↔ LocalStack)
3. **Natural Language**: Ask questions naturally, no need for special syntax
4. **Click Links**: Click on message links to jump to the original context

## Architecture

- **Frontend**: Telegram Bot API
- **Backend**: Go
- **Search Engine**: Meilisearch
- **Database**: MongoDB
- **AI**: Google Gemini
- **Message Processing**:
  1. Messages are stored in MongoDB
  2. Indexed in Meilisearch for fast text search
  3. Processed by Gemini AI for semantic understanding
  4. Results are grouped by conversation context

## Contributing

1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.
