package gpt

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
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
	cfg := loadGTPConfig()

	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return "", nil
	}

	if !strings.HasSuffix(text, ".") {
		text = text + "."
	}

	response, err := g.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: cfg.Prompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: text,
			},
		},
	})
	if err != nil {
		return "", err
	}

	log.Info().Int("tokens", response.Usage.TotalTokens).Msg("openai usage")

	transformedText := response.Choices[0].Message.Content
	return transformedText, nil
}
