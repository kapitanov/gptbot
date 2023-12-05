package gpt

import (
	"context"

	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

// GPT is a GPT-3 text transformer.
type GPT struct {
	client *openai.Client
}

// MaxConversationDepth limits conversation depth.
const MaxConversationDepth = 5

// New creates a new GPT-3 text transformer.
func New(token string) (*GPT, error) {
	client := openai.NewClient(token)

	_, err := client.ListModels(context.Background())
	if err != nil {
		return nil, err
	}

	return &GPT{client: client}, nil
}

// Message is a message in a conversation.
type Message struct {
	Participant Participant // Conversation participant.
	Text        string      // Message text.
}

// Participant is the side of conversation.
type Participant int

const (
	ParticipantBot  Participant = iota // Bot.
	ParticipantUser                    // User.
)

// Generate generates a new message from the input stream.
func (g *GPT) Generate(ctx context.Context, messages []Message) (string, error) {
	request := g.createChatCompletionRequest(messages)

	for _, m := range request.Messages {
		log.Debug().Str("role", m.Role).Str("content", m.Content).Str("dir", "out").Msg("gpt request")
	}

	response, err := g.client.CreateChatCompletion(ctx, request)
	if err != nil {
		return "", err
	}

	for _, m := range response.Choices {
		log.Debug().Str("role", m.Message.Role).
			Str("content", m.Message.Content).
			Str("finish", string(m.FinishReason)).
			Msg("gpt response")
	}

	log.Debug().
		Str("object", response.Object).
		Str("model", response.Model).
		Int("tokens", response.Usage.TotalTokens).
		Int("prompt", response.Usage.PromptTokens).
		Int("response", response.Usage.CompletionTokens).
		Msg("gpt stats")

	transformedText := response.Choices[0].Message.Content
	return transformedText, nil
}

func (g *GPT) createChatCompletionRequest(messages []Message) openai.ChatCompletionRequest {
	cfg := loadGTPConfig()
	req := openai.ChatCompletionRequest{
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: cfg.Prompt,
			},
		},
	}

	if len(messages) > MaxConversationDepth {
		messages = messages[len(messages)-MaxConversationDepth:]
	}

	for _, message := range messages {
		role := openai.ChatMessageRoleUser
		if message.Participant == ParticipantBot {
			role = openai.ChatMessageRoleAssistant
		}

		req.Messages = append(req.Messages, openai.ChatCompletionMessage{
			Role:    role,
			Content: message.Text,
		})
	}

	return req
}
