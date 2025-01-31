package ai

import (
	"context"
	"fmt"
	"strings"

	"SearchBot/internal/models"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type GeminiAI struct {
	client *genai.Client
	model  *genai.GenerativeModel
}

func NewGeminiAI(apiKey string) (*GeminiAI, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %v", err)
	}

	// Get the generative model
	model := client.GenerativeModel("gemini-pro")

	return &GeminiAI{
		client: client,
		model:  model,
	}, nil
}

func (g *GeminiAI) Close() {
	g.client.Close()
}

func (g *GeminiAI) AnswerQuestion(ctx context.Context, question string, messages []models.Message) (string, error) {
	// If no messages provided, this is a direct question to AI (e.g., for search strategy)
	if messages == nil {
		return g.generateResponse(ctx, question)
	}

	// If messages provided, we're generating an answer based on chat context
	var prompt strings.Builder
	prompt.WriteString(question)
	
	if len(messages) > 0 {
		prompt.WriteString("\n\nContext from chat messages:\n")
		for i, msg := range messages {
			prompt.WriteString(fmt.Sprintf("%d. @%s: %s\n", i+1, msg.Username, msg.Text))
		}
	}

	return g.generateResponse(ctx, prompt.String())
}

func (g *GeminiAI) generateResponse(ctx context.Context, prompt string) (string, error) {
	// Generate content directly using the model
	resp, err := g.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %v", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response generated")
	}

	// Get the response text
	response := resp.Candidates[0].Content.Parts[0].(genai.Text)
	return string(response), nil
} 