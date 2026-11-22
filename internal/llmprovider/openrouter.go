package llmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rs/zerolog/log"
)

// openRouterDefaultURL is the default OpenRouter API URL.
const openRouterDefaultURL = "https://openrouter.ai/api/v1"

// openRouterServerTools is the list of OpenRouter built-in server side tools
// (executed by OpenRouter, no client-side implementation needed).
var openRouterServerTools = []string{
	"openrouter:web_search", // Search the web for current information.
	"openrouter:web_fetch",  // Fetch and extract content from URLs.
	"openrouter:datetime",   // Get the current date and time.
}

// openRouterServerToolsOption returns a request option that enables
// the OpenRouter built-in server side tools on a request.
func openRouterServerToolsOption() option.RequestOption {
	tools := make([]map[string]string, 0, len(openRouterServerTools))
	for _, t := range openRouterServerTools {
		tools = append(tools, map[string]string{"type": t})
	}
	return option.WithJSONSet("tools", tools)
}

// openRouterProvider is a LLM provider backed by the OpenRouter API (Chat Completions API).
type openRouterProvider struct {
	client          openai.Client
	models          []string
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
	models := ParseModels(parameters.Model)

	log.Info().
		Str("provider", string(ProviderOpenRouter)).
		Str("url", url).
		Strs("models", models).
		Str("prompt", parameters.PromptFilePath).
		Str("reasoning", string(parameters.ReasoningEffort)).
		Msg("llm client configuration")

	_, err := client.Models.List(context.Background())
	if err != nil {
		return nil, err
	}

	return &openRouterProvider{
		client:          client,
		models:          models,
		promptFilePath:  parameters.PromptFilePath,
		history:         parameters.History,
		enableTracing:   parameters.EnableTracing,
		reasoningEffort: parameters.ReasoningEffort,
	}, nil
}

// Models returns the list of available model names.
func (p *openRouterProvider) Models() []string {
	return slices.Clone(p.models)
}

// DefaultModel returns the model name used when the request does not specify one.
func (p *openRouterProvider) DefaultModel() string {
	return p.models[0]
}

// Generate generates a new message from the input request.
func (p *openRouterProvider) Generate(ctx context.Context, req Request) (Response, error) {
	request, err := p.prepareRequest(req)
	if err != nil {
		return Response{}, err
	}

	started := time.Now()
	response, err := p.client.Chat.Completions.New(ctx, request, openRouterServerToolsOption())
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
		ServerToolUsage: completionServerToolUsage(response),
		Citations:       completionCitations(response),
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

	// Server tools run inside OpenRouter's agent loop, so search queries are not
	// reported back. Infer what was searched from the returned web citations.
	if len(r.Citations) > 0 || r.ServerToolUsage.WebSearchRequests > 0 {
		citations := make([]string, 0, len(r.Citations))
		for _, c := range r.Citations {
			citations = append(citations, c.URL)
		}

		log.Info().
			Int("web_searches", r.ServerToolUsage.WebSearchRequests).
			Int("tool_calls", r.ServerToolUsage.ToolCallsExecuted).
			Strs("citations", citations).
			Msg("server tool usage")
	}

	return r, nil
}

func (p *openRouterProvider) prepareRequest(request Request) (openai.ChatCompletionNewParams, error) {
	messages, err := p.prepareMessages(request)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}

	model := request.Model
	if model == "" {
		model = p.DefaultModel()
	}

	return openai.ChatCompletionNewParams{
		Model:           shared.ChatModel(model),
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

// completionServerToolUsage extracts server side tool usage counters from the OpenRouter response.
// OpenRouter returns them in a non-standard "server_tool_use_details" field of the usage object.
func completionServerToolUsage(response *openai.ChatCompletion) ServerToolUsage {
	var raw struct {
		Usage struct {
			Details struct {
				ToolCallsRequested int `json:"tool_calls_requested"`
				ToolCallsExecuted  int `json:"tool_calls_executed"`
				WebSearchRequests  int `json:"web_search_requests"`
			} `json:"server_tool_use_details"`
		} `json:"usage"`
	}

	if err := json.Unmarshal([]byte(response.RawJSON()), &raw); err != nil {
		return ServerToolUsage{}
	}

	return ServerToolUsage{
		ToolCallsRequested: raw.Usage.Details.ToolCallsRequested,
		ToolCallsExecuted:  raw.Usage.Details.ToolCallsExecuted,
		WebSearchRequests:  raw.Usage.Details.WebSearchRequests,
	}
}

// completionCitations extracts web citations from the OpenRouter response.
// OpenRouter standardizes results of its server side web search/fetch tools into
// "url_citation" annotations of the message.
func completionCitations(response *openai.ChatCompletion) []Citation {
	var raw struct {
		Choices []struct {
			Message struct {
				Annotations []struct {
					Type        string `json:"type"`
					URLCitation struct {
						URL     string `json:"url"`
						Title   string `json:"title"`
						Content string `json:"content"`
					} `json:"url_citation"`
				} `json:"annotations"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal([]byte(response.RawJSON()), &raw); err != nil {
		return nil
	}
	if len(raw.Choices) == 0 {
		return nil
	}

	var citations []Citation
	for _, a := range raw.Choices[0].Message.Annotations {
		if a.Type != "url_citation" || a.URLCitation.URL == "" {
			continue
		}
		citations = append(citations, Citation{
			URL:     a.URLCitation.URL,
			Title:   a.URLCitation.Title,
			Content: a.URLCitation.Content,
		})
	}

	return citations
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
