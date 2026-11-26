package llmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rs/zerolog/log"
)

// openAIProvider is a LLM provider backed by the OpenAI API (Responses API).
type openAIProvider struct {
	client          openai.Client
	models          []string
	promptFilePath  string
	history         History
	enableTracing   bool
	reasoningEffort ReasoningEffort
}

// newOpenAIProvider creates a new OpenAI LLM provider client.
func newOpenAIProvider(parameters Parameters) (*openAIProvider, error) {
	var clientOptions []option.RequestOption
	logEvent := log.Info().Str("provider", string(ProviderOpenAI))

	if parameters.URL != "" {
		logEvent = logEvent.Str("url", parameters.URL)
		clientOptions = append(clientOptions, option.WithBaseURL(parameters.URL))
	}

	if parameters.Token != "" {
		logEvent = logEvent.Str("token", "****")
		clientOptions = append(clientOptions, option.WithAPIKey(parameters.Token))
	}

	client := openai.NewClient(clientOptions...)
	models := ParseModels(parameters.Model)

	logEvent.
		Strs("models", models).
		Str("prompt", parameters.PromptFilePath).
		Str("reasoning", string(parameters.ReasoningEffort)).
		Msg("llm client configuration")

	_, err := client.Models.List(context.Background())
	if err != nil {
		return nil, err
	}

	return &openAIProvider{
		client:          client,
		models:          models,
		promptFilePath:  parameters.PromptFilePath,
		history:         parameters.History,
		enableTracing:   parameters.EnableTracing,
		reasoningEffort: ReasoningEffort(parameters.ReasoningEffort),
	}, nil
}

// Models returns the list of available model names.
func (llm *openAIProvider) Models() []string {
	return slices.Clone(llm.models)
}

// DefaultModel returns the model name used when the request does not specify one.
func (llm *openAIProvider) DefaultModel() string {
	return llm.models[0]
}

// Generate generates a new message from the input request.
func (llm *openAIProvider) Generate(ctx context.Context, req Request, onDelta DeltaHandler) (Response, error) {
	request, err := llm.prepareRequest(req)
	if err != nil {
		return Response{}, err
	}

	stream := llm.client.Responses.NewStreaming(ctx, request)
	defer func() { _ = stream.Close() }()

	var response *responses.Response
	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "response.output_text.delta":
			if onDelta != nil && event.Delta != "" {
				onDelta(event.Delta)
			}
		case "response.completed", "response.failed", "response.incomplete":
			// These are terminal events carrying the final response object.
			// Incomplete and failed responses are returned as-is, matching
			// the non-streaming API behaviour.
			completed := event.Response
			response = &completed
		case "error":
			return Response{}, fmt.Errorf("llm stream error: %s", event.Message)
		}
	}

	if err = stream.Err(); err != nil {
		return Response{}, err
	}

	if response == nil {
		return Response{}, fmt.Errorf("llm stream ended without a completed response")
	}

	r := Response{
		ID:        response.ID,
		Text:      response.OutputText(),
		Reasoning: outputReasoning(response),
		Usage: Usage{
			PromptTokens:     response.Usage.InputTokens,
			CompletionTokens: response.Usage.OutputTokens,
			TotalTokens:      response.Usage.TotalTokens,
		},
		Model:           response.Model,
		TotalTokens:     response.Usage.TotalTokens,
		ReasoningEffort: llm.reasoningEffort,
		Duration:        time.Second * time.Duration(response.CompletedAt-response.CreatedAt),
	}

	if err = llm.updateHistory(req, request.Input.OfInputItemList, r); err != nil {
		return Response{}, err
	}

	llm.trace(request, response)

	log.Info().
		Str("id", r.ID).
		Str("model", r.Model).
		Int64("tokens", r.TotalTokens).
		Str("reasoning", string(r.ReasoningEffort)).
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

func (llm *openAIProvider) prepareRequest(request Request) (responses.ResponseNewParams, error) {
	itemsList, err := llm.prepareRequestMessages(request)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}

	model := request.Model
	if model == "" {
		model = llm.DefaultModel()
	}

	req := responses.ResponseNewParams{
		Model: shared.ResponsesModel(model),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: itemsList,
		},
		Reasoning: shared.ReasoningParam{
			Effort: shared.ReasoningEffort(llm.reasoningEffort),
		},
	}

	return req, nil
}

func (llm *openAIProvider) prepareRequestMessages(request Request) ([]responses.ResponseInputItemUnionParam, error) {
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

func (llm *openAIProvider) updateHistory(request Request, itemsList []responses.ResponseInputItemUnionParam, response Response) error {
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

func (llm *openAIProvider) trace(request responses.ResponseNewParams, response *responses.Response) {
	if !llm.enableTracing {
		return
	}

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
