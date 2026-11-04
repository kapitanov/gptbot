package llmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rs/zerolog/log"
)

const DefaultReasoningEffort = string(shared.ReasoningEffortMinimal)

// LLM is a LLM client.
type LLM struct {
	client          openai.Client
	model           string
	promptFilePath  string
	history         History
	enableTracing   bool
	reasoningEffort shared.ReasoningEffort
}

// MaxConversationDepth limits conversation depth.
const MaxConversationDepth = 5

// Parameters holds the configuration for the LLM client.
type Parameters struct {
	URL             string
	Token           string
	Model           string
	PromptFilePath  string
	History         History
	EnableTracing   bool
	ReasoningEffort string
}

// History represents the conversation history storage.
type History interface {
	// Retrieve fetches the conversation history for the given conversation ID.
	Retrieve(conversationID string) ([]Message, error)

	// Append adds messages to the conversation history for the given conversation ID.
	Append(conversationID string, messages ...Message) error
}

// New creates a new LLM client.
func New(parameters Parameters) (*LLM, error) {
	var clientOptions []option.RequestOption
	logEvent := log.Info()

	if parameters.URL != "" {
		logEvent = logEvent.Str("url", parameters.URL)
		clientOptions = append(clientOptions, option.WithBaseURL(parameters.URL))
	}

	if parameters.Token != "" {
		logEvent = logEvent.Str("token", "****")
		clientOptions = append(clientOptions, option.WithAPIKey(parameters.Token))
	}

	client := openai.NewClient(clientOptions...)

	logEvent.
		Str("model", parameters.Model).
		Str("prompt", parameters.PromptFilePath).
		Str("reasoning", parameters.ReasoningEffort).
		Msg("llm client configuration")
	_, err := client.Models.List(context.Background())
	if err != nil {
		return nil, err
	}

	return &LLM{
		client:          client,
		model:           parameters.Model,
		promptFilePath:  parameters.PromptFilePath,
		history:         parameters.History,
		enableTracing:   parameters.EnableTracing,
		reasoningEffort: shared.ReasoningEffort(parameters.ReasoningEffort),
	}, nil
}

// Message is a message in a conversation.
type Message struct {
	Participant Participant // Conversation participant.
	Text        string      // Message text.
}

// Participant is the side of conversation.
type Participant string

const (
	ParticipantAssistant Participant = "assistant" // Bot.
	ParticipantUser      Participant = "user"      // User.
)

// Request is a GPT request.
type Request struct {
	Message        string
	ConversationID string
}

// Response is a GPT response.
type Response struct {
	ID          string
	Text        string                  // Transformed text.
	Reasoning   string                  // Reasoning output.
	Usage       responses.ResponseUsage // Token usage.
	TotalTokens int64                   // Total number of tokens used.
	Model       string                  // Model name.
	Duration    time.Duration           // Duration of the response.
}

// Generate generates a new message from the input stream.
func (llm *LLM) Generate(ctx context.Context, req Request) (Response, error) {
	request, err := llm.prepareRequest(req)
	if err != nil {
		return Response{}, err
	}

	response, err := llm.client.Responses.New(ctx, request)
	if err != nil {
		return Response{}, err
	}

	r := Response{
		ID:          response.ID,
		Text:        response.OutputText(),
		Reasoning:   outputReasoning(response),
		Usage:       response.Usage,
		Model:       response.Model,
		TotalTokens: response.Usage.TotalTokens,
		Duration:    time.Second * time.Duration(response.CompletedAt-response.CreatedAt),
	}

	if err = llm.updateHistory(req, request.Input.OfInputItemList, r); err != nil {
		return Response{}, err
	}

	llm.trace(request, response)

	log.Info().
		Str("id", r.ID).
		Str("model", r.Model).
		Int64("tokens", response.Usage.TotalTokens).
		Str("reasoning", string(llm.reasoningEffort)).
		Dur("duration", r.Duration).
		Msg("llm stats")

	return r, nil
}

func outputReasoning(r *responses.Response) string {
	var outputText strings.Builder
	for _, item := range r.Output {
		for _, content := range item.Content {
			if content.Type == "reasoning_text" {
				outputText.WriteString(content.Text)
			}
		}
	}
	return outputText.String()
}

func (llm *LLM) prepareRequest(request Request) (responses.ResponseNewParams, error) {
	itemsList, err := llm.prepareRequestMessages(request)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}

	req := responses.ResponseNewParams{
		Model: shared.ResponsesModel(llm.model),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: itemsList,
		},
		Reasoning: shared.ReasoningParam{
			Effort: llm.reasoningEffort,
		},
	}

	return req, nil
}

func (llm *LLM) prepareRequestMessages(request Request) ([]responses.ResponseInputItemUnionParam, error) {
	promptRaw, err := os.ReadFile(llm.promptFilePath)
	if err != nil {
		log.Error().Err(err).Str("path", llm.promptFilePath).Msg("unable to load prompt")
		return nil, err
	}

	itemsList := []responses.ResponseInputItemUnionParam{
		{
			OfMessage: &responses.EasyInputMessageParam{
				Role: responses.EasyInputMessageRoleSystem,
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: param.Opt[string]{Value: string(promptRaw)},
				},
			},
		},
	}
	historyMessages, err := llm.history.Retrieve(request.ConversationID)
	if err != nil {
		return nil, err
	}

	for _, m := range historyMessages {
		role := responses.EasyInputMessageRoleUser
		if m.Participant == ParticipantAssistant {
			role = responses.EasyInputMessageRoleAssistant
		}
		itemsList = append(itemsList, responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Role: role,
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: param.Opt[string]{Value: m.Text},
				},
			},
		})
	}

	itemsList = append(itemsList, responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{
			Role: responses.EasyInputMessageRoleUser,
			Content: responses.EasyInputMessageContentUnionParam{
				OfString: param.Opt[string]{Value: request.Message},
			},
		},
	})

	return itemsList, nil
}

func (llm *LLM) updateHistory(request Request, itemsList []responses.ResponseInputItemUnionParam, response Response) error {
	return llm.history.Append(
		request.ConversationID,
		Message{
			Participant: ParticipantUser,
			Text:        itemsList[len(itemsList)-1].OfMessage.Content.OfString.Value,
		},
		Message{
			Participant: ParticipantAssistant,
			Text:        response.Text,
		},
	)
}

func (llm *LLM) trace(request responses.ResponseNewParams, response *responses.Response) {
	if !llm.enableTracing {
		return
	}

	var sb strings.Builder
	sb.WriteString("Request:\n")
	requestJSON := formatJSON(request)
	responseJSON := prettifyJSON([]byte(response.RawJSON()))

	_, _ = fmt.Fprintf(os.Stdout, "REQUEST:\n%s\nRESPONSE:\n%s\n\n", requestJSON, responseJSON)
}

func formatJSON[T json.Marshaler](v T) string {
	bs, _ := v.MarshalJSON()
	return prettifyJSON(bs)
}

func prettifyJSON(bs []byte) string {
	var t any
	_ = json.Unmarshal(bs, &t)
	formatted, _ := json.MarshalIndent(t, "", "  ")
	return string(formatted)
}
