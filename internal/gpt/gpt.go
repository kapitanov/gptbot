package gpt

import (
	"context"
	"encoding/json"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v3"
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

	type jsonOutput struct {
		OutputMarkdown string `json:"output_markdown"`
	}

	var transformedOutput jsonOutput
	if err = json.Unmarshal([]byte(transformedText), &transformedOutput); err == nil {
		transformedText = transformedOutput.OutputMarkdown
	}

	return transformedText, nil
}

func (g *GPT) createChatCompletionRequest(messages []Message) openai.ChatCompletionRequest {
	cfg := loadGTPConfig()
	req := openai.ChatCompletionRequest{
		Model:               cfg.Model.Name,
		MaxCompletionTokens: cfg.Model.MaxCompletionTokens,
		Temperature:         cfg.Model.Temperature,
		TopP:                cfg.Model.TopP,
		N:                   cfg.Model.N,
		PresencePenalty:     cfg.Model.PresencePenalty,
		Seed:                cfg.Model.Seed,
		FrequencyPenalty:    cfg.Model.FrequencyPenalty,
		ServiceTier:         cfg.Model.ServiceTier,
		Verbosity:           cfg.Model.Verbosity,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
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

	req.Messages = append(req.Messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: "Output format: JSON object with one string field: 'output_markdown'. 'output_markdown' is the response text in Markdown format.",
	})

	return req
}

type gptConfig struct {
	Model  gptModelConfig `yaml:"model"`
	Prompt string         `yaml:"prompt"`
}

type gptModelConfig struct {
	Name                string             `yaml:"name"`
	MaxCompletionTokens int                `yaml:"max_completion_tokens,omitempty"`
	Temperature         float32            `yaml:"temperature,omitempty"`
	TopP                float32            `yaml:"top_p,omitempty"`
	N                   int                `yaml:"n,omitempty"`
	PresencePenalty     float32            `yaml:"presence_penalty,omitempty"`
	Seed                *int               `yaml:"seed,omitempty"`
	FrequencyPenalty    float32            `yaml:"frequency_penalty,omitempty"`
	ServiceTier         openai.ServiceTier `yaml:"service_tier,omitempty"`
	Verbosity           string             `yaml:"verbosity,omitempty"`
}

func loadGTPConfig() *gptConfig {
	const defaultSourcePath = "./conf/gpt.yaml"
	sourcePath := os.Getenv("CONFIG_PATH")
	if sourcePath == "" {
		sourcePath = defaultSourcePath
	}

	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		log.Error().Err(err).Str("path", sourcePath).Msg("unable to load gpt config")
		return &defaultGTPConfig
	}

	var cfg gptConfig
	err = yaml.Unmarshal(raw, &cfg)
	if err != nil {
		log.Error().Err(err).Str("path", sourcePath).Msg("unable to parse gpt config")
		return &defaultGTPConfig
	}

	return &cfg
}

var defaultGTPConfig = gptConfig{
	Model:  gptModelConfig{Name: "gpt-3.5-turbo", Temperature: 0.9},
	Prompt: "Summarize the following text.",
}
