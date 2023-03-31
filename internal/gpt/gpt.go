package gpt

import (
	"context"
	"fmt"

	"github.com/kapitanov/gptbot/internal/texts"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

const (
	modelName   = "gpt-3.5-turbo"
	temperature = 0.9
)

// GPT is a GPT-3 text transformer.
type GPT struct {
	client *openai.Client
}

// New creates a new GPT-3 text transformer.
func New(token string) (*GPT, error) {
	client := openai.NewClient(token)

	_, err := client.ListModels(context.Background())
	if err != nil {
		return nil, err
	}

	return &GPT{client: client}, nil
}

// Transform transforms text.
func (g *GPT) Transform(ctx context.Context, text string) (string, error) {
	requestText := fmt.Sprintf("%s\n\n\n%s", texts.Prompt, text)

	response, err := g.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: modelName,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: requestText,
			},
		},
		Temperature: temperature,
	})
	if err != nil {
		return "", err
	}

	log.Info().Int("tokens", response.Usage.TotalTokens).Msg("openai usage")

	transformedText := response.Choices[0].Message.Content
	return transformedText, nil
}
