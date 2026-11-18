package llmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rs/zerolog/log"
)

// openRouterDefaultURL is the default OpenRouter API URL.
const openRouterDefaultURL = "https://openrouter.ai/api/v1"

// openRouterProvider is a LLM provider backed by the OpenRouter API (Chat Completions API).
type openRouterProvider struct {
	client          openai.Client
	model           string
	promptFilePath  string
	history         History
	enableTracing   bool
	reasoningEffort ReasoningEffort
}

// newOpenRouterProvider creates a new OpenRouter LLM provider client.
func newOpenRouterProvider(parameters Parameters) (*openRouterProvider, error) {
	url := parameters.URL
	if url == "" {
		url = openRouterDefaultURL
	}

	var clientOptions []option.RequestOption
	clientOptions = append(clientOptions, option.WithBaseURL(url))

	if parameters.Token != "" {
		clientOptions = append(clientOptions, option.WithAPIKey(parameters.Token))
	}

	client := openai.NewClient(clientOptions...)

	log.Info().
		Str("provider", string(ProviderOpenRouter)).
		Str("url", url).
		Str("model", parameters.Model).
		Str("prompt", parameters.PromptFilePath).
		Str("reasoning", string(parameters.ReasoningEffort)).
		Msg("llm client configuration")

	_, err := client.Models.List(context.Background())
	if err != nil {
		return nil, err
	}

	return &openRouterProvider{
		client:          client,
		model:           parameters.Model,
		promptFilePath:  parameters.PromptFilePath,
		history:         parameters.History,
		enableTracing:   parameters.EnableTracing,
		reasoningEffort: parameters.ReasoningEffort,
	}, nil
}

// Generate generates a new message from the input request.
func (p *openRouterProvider) Generate(ctx context.Context, req Request) (Response, error) {
	request, err := p.prepareRequest(req)
	if err != nil {
		return Response{}, err
	}

	started := time.Now()
	response, err := p.client.Chat.Completions.New(ctx, request)
	if err != nil {
		return Response{}, err
	}

	r := Response{
		ID:        response.ID,
		Text:      completionText(response),
		Reasoning: completionReasoning(response),
		Usage: Usage{
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
		},
		Model:           response.Model,
		TotalTokens:     response.Usage.TotalTokens,
		ReasoningEffort: p.reasoningEffort,
		TotalPrice:      completionCost(response),
		Duration:        time.Since(started),
	}

	if err = p.updateHistory(req, r); err != nil {
		return Response{}, err
	}

	p.trace(request, response)

	log.Info().
		Str("id", r.ID).
		Str("model", r.Model).
		Int64("tokens", r.TotalTokens).
		Str("reasoning", string(r.ReasoningEffort)).
		Float64("price", r.TotalPrice).
		Dur("duration", r.Duration).
		Msg("llm stats")

	return r, nil
}

func (p *openRouterProvider) prepareRequest(request Request) (openai.ChatCompletionNewParams, error) {
	messages, err := p.prepareMessages(request)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}

	return openai.ChatCompletionNewParams{
		Model:           shared.ChatModel(p.model),
		Messages:        messages,
		ReasoningEffort: shared.ReasoningEffort(p.reasoningEffort),
	}, nil
}

func (p *openRouterProvider) prepareMessages(request Request) ([]openai.ChatCompletionMessageParamUnion, error) {
	promptRaw, err := os.ReadFile(p.promptFilePath)
	if err != nil {
		log.Error().Err(err).Str("path", p.promptFilePath).Msg("unable to load prompt")
		return nil, err
	}

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(string(promptRaw)),
	}

	historyMessages, err := p.history.Retrieve(request.ConversationID)
	if err != nil {
		return nil, err
	}

	for _, m := range historyMessages {
		if m.Participant == ParticipantAssistant {
			messages = append(messages, openai.AssistantMessage(m.Text))
		} else {
			messages = append(messages, openai.UserMessage(m.Text))
		}
	}

	messages = append(messages, openai.UserMessage(request.Message))

	return messages, nil
}

func (p *openRouterProvider) updateHistory(request Request, response Response) error {
	return p.history.Append(
		request.ConversationID,
		Message{
			Participant: ParticipantUser,
			Text:        request.Message,
		},
		Message{
			Participant: ParticipantAssistant,
			Text:        response.Text,
		},
	)
}

// completionReasoning extracts the reasoning text from the OpenRouter response.
// OpenRouter returns reasoning in a non-standard "reasoning" field of the message.
func completionReasoning(response *openai.ChatCompletion) string {
	var raw struct {
		Choices []struct {
			Message struct {
				Reasoning string `json:"reasoning"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal([]byte(response.RawJSON()), &raw); err != nil {
		return ""
	}

	if len(raw.Choices) == 0 {
		return ""
	}

	return raw.Choices[0].Message.Reasoning
}

// completionCost extracts the total cost in USD from the OpenRouter response.
// OpenRouter returns it in a non-standard "cost" field of the usage object.
func completionCost(response *openai.ChatCompletion) float64 {
	var raw struct {
		Usage struct {
			Cost float64 `json:"cost"`
		} `json:"usage"`
	}

	if err := json.Unmarshal([]byte(response.RawJSON()), &raw); err != nil {
		return 0
	}

	return raw.Usage.Cost
}

func completionText(response *openai.ChatCompletion) string {
	if len(response.Choices) == 0 {
		return ""
	}

	return response.Choices[0].Message.Content
}

func (p *openRouterProvider) trace(request openai.ChatCompletionNewParams, response *openai.ChatCompletion) {
	if !p.enableTracing {
		return
	}

	requestJSON := formatJSON(request)
	responseJSON := prettifyJSON([]byte(response.RawJSON()))

	_, _ = fmt.Fprintf(os.Stdout, "REQUEST:\n%s\nRESPONSE:\n%s\n\n", requestJSON, responseJSON)
}
