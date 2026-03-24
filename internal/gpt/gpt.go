package gpt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	// "github.com/sashabaranov/go-openai"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"gopkg.in/yaml.v3"
)

// GPT is a GPT-3 text transformer.
type GPT struct {
	client openai.Client
}

// MaxConversationDepth limits conversation depth.
const MaxConversationDepth = 5

// New creates a new GPT-3 text transformer.
func New(token string) (*GPT, error) {
	client := openai.NewClient(option.WithAPIKey(token))

	_, err := client.Models.List(context.Background())
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

// Request is a GPT request.
type Request struct {
	Messages []Message // Conversation messages.
}

// Response is a GPT response.
type Response struct {
	Text  string                  // Transformed text.
	Usage responses.ResponseUsage // Token usage.
}

// Generate generates a new message from the input stream.
func (g *GPT) Generate(ctx context.Context, messages []Message) (Response, error) {
	request, err := g.createChatCompletionRequest(messages)
	if err != nil {
		return Response{}, err
	}

	response, err := g.client.Responses.New(ctx, request)
	if err != nil {
		return Response{}, err
	}

	log.Debug().Str("model", response.Model).Int64("tokens", response.Usage.TotalTokens).Msg("gpt stats")

	transformedText := response.OutputText()

	type jsonOutput struct {
		OutputMarkdown string `json:"output_markdown"`
	}

	var transformedOutput jsonOutput
	if err = json.Unmarshal([]byte(transformedText), &transformedOutput); err == nil {
		transformedText = transformedOutput.OutputMarkdown
	}

	return Response{
		Text:  transformedText,
		Usage: response.Usage,
	}, nil
}

func (g *GPT) createChatCompletionRequest(messages []Message) (responses.ResponseNewParams, error) {
	cfg, err := loadGTPConfig()
	if err != nil {
		return responses.ResponseNewParams{}, err
	}

	itemsList := []responses.ResponseInputItemUnionParam{
		{
			OfMessage: &responses.EasyInputMessageParam{
				Role: responses.EasyInputMessageRoleSystem,
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: param.Opt[string]{Value: cfg.Prompt},
				},
			},
		},
	}

	if len(messages) > MaxConversationDepth {
		messages = messages[len(messages)-MaxConversationDepth:]
	}

	for _, message := range messages {
		role := responses.EasyInputMessageRoleUser
		if message.Participant == ParticipantBot {
			role = responses.EasyInputMessageRoleAssistant
		}

		itemsList = append(itemsList, responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Role: role,
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: param.Opt[string]{Value: message.Text},
				},
			},
		})
	}

	req := responses.ResponseNewParams{
		Model: shared.ResponsesModel(cfg.Model.Name),
		Text: responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
					Name: "output",
					Schema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"output_markdown": map[string]any{
								"type":        "string",
								"description": "The response text in Markdown format.",
								"example":     "Hello, *world*!",
							},
						},
						"required":             []string{"output_markdown"},
						"additionalProperties": false,
					},
					Strict: param.Opt[bool]{Value: true},
				},
			},
		},
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: itemsList,
		},
	}

	return req, nil
}

type gptConfig struct {
	Model  gptModelConfig `yaml:"model"`
	Prompt string         `yaml:"prompt"`
}

type gptModelConfig struct {
	Name string `yaml:"name"`
}

func loadGTPConfig() (*gptConfig, error) {
	const defaultSourcePath = "./conf/gpt.yaml"
	sourcePath := os.Getenv("CONFIG_PATH")
	if sourcePath == "" {
		sourcePath = defaultSourcePath
	}

	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		log.Error().Err(err).Str("path", sourcePath).Msg("unable to load gpt config")
		return nil, err
	}

	var cfg gptConfig
	err = yaml.Unmarshal(raw, &cfg)
	if err != nil {
		log.Error().Err(err).Str("path", sourcePath).Msg("unable to parse gpt config")
		return nil, err
	}

	promptPath := filepath.Join(filepath.Dir(sourcePath), "PROMPT.md")
	promptRaw, err := os.ReadFile(promptPath)
	if err != nil {
		log.Error().Err(err).Str("path", promptPath).Msg("unable to load prompt")
		return nil, err
	}

	cfg.Prompt = string(promptRaw)
	return &cfg, nil
}
