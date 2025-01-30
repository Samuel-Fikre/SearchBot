package ai

import (
	"context"
	"fmt"

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

func (g *GeminiAI) AnswerQuestion(ctx context.Context, question string, relevantMessages []models.Message) (string, error) {
	// Create context from relevant messages
	context := "Based on the following messages:\n\n"
	for _, msg := range relevantMessages {
		context += fmt.Sprintf("From @%s: %s\n", msg.Username, msg.Text)
	}
	context += "\n\nQuestion: " + question + "\n\nPlease provide a concise and relevant answer based on the messages above."

	// Generate response
	resp, err := g.model.GenerateContent(ctx, genai.Text(context))
	if err != nil {
		return "", fmt.Errorf("failed to generate response: %v", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response generated")
	}

	answer := resp.Candidates[0].Content.Parts[0].(genai.Text)
	return string(answer), nil
} 