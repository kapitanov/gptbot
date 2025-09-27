package gpt

import (
	"context"
	"fmt"
	"os"

	"github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v3"
)

// MaxConversationDepth limits conversation depth
const MaxConversationDepth = 5

// Participant is the side of conversation
type Participant string

const (
	ParticipantBot  Participant = "bot"
	ParticipantUser Participant = "user"
)

// Message represents a conversation message
type Message struct {
	Participant Participant `json:"participant"`
	Text        string      `json:"text"`
}

// Config represents GPT configuration
type Config struct {
	Model       string  `yaml:"model"`
	Temperature float32 `yaml:"temperature"`
	Prompt      string  `yaml:"prompt"`
}

// DefaultConfig returns default GPT configuration
func DefaultConfig() *Config {
	return &Config{
		Model:       "gpt-3.5-turbo",
		Temperature: 0.9,
		Prompt: `    Как бы малообразованный человек в состоянии алкогольного опьянения, ` +
			` разговаривающий матом и эмоционально, пересказал бы следующую новость?` +
			`Комментарий должен быть эмоциональным, просторечным и матерным.`,
	}
}

// LoadConfig loads GPT configuration from file
func LoadConfig() (*Config, error) {
	defaultSourcePath := "./conf/gpt.yaml"
	sourcePath := os.Getenv("CONFIG_PATH")
	if sourcePath == "" {
		sourcePath = defaultSourcePath
	}

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return DefaultConfig(), nil // Return default config if file doesn't exist
	}

	config := DefaultConfig()
	if err := yaml.Unmarshal(data, config); err != nil {
		return DefaultConfig(), nil // Return default config if parsing fails
	}

	return config, nil
}

// GPT is a GPT text transformer
type GPT struct {
	client *openai.Client
}

// New creates a new GPT instance
func New(token string) (*GPT, error) {
	if token == "" {
		return nil, fmt.Errorf("OpenAI token is required")
	}

	client := openai.NewClient(token)
	return &GPT{client: client}, nil
}

// Initialize initializes the GPT client
func (g *GPT) Initialize() error {
	// Test the connection by listing models
	_, err := g.client.ListModels(context.Background())
	if err != nil {
		return fmt.Errorf("failed to initialize OpenAI client: %w", err)
	}
	return nil
}

// Generate generates a new message from the input stream
func (g *GPT) Generate(messages []Message) (string, error) {
	request, err := g.createChatCompletionRequest(messages)
	if err != nil {
		return "", err
	}

	response, err := g.client.CreateChatCompletion(context.Background(), *request)
	if err != nil {
		return "", fmt.Errorf("OpenAI API error: %w", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}

	return response.Choices[0].Message.Content, nil
}

func (g *GPT) createChatCompletionRequest(messages []Message) (*openai.ChatCompletionRequest, error) {
	config, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	request := &openai.ChatCompletionRequest{
		Model:       config.Model,
		Temperature: config.Temperature,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: config.Prompt,
			},
		},
	}

	// Limit conversation depth
	limitedMessages := messages
	if len(messages) > MaxConversationDepth {
		limitedMessages = messages[len(messages)-MaxConversationDepth:]
	}

	// Convert messages to OpenAI format
	for _, message := range limitedMessages {
		var role string
		if message.Participant == ParticipantBot {
			role = openai.ChatMessageRoleAssistant
		} else {
			role = openai.ChatMessageRoleUser
		}

		request.Messages = append(request.Messages, openai.ChatCompletionMessage{
			Role:    role,
			Content: message.Text,
		})
	}

	return request, nil
}
