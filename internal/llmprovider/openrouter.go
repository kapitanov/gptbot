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
func (p *openRouterProvider) Generate(ctx context.Context, req Request, onDelta DeltaHandler) (Response, error) {
	request, err := p.prepareRequest(req)
	if err != nil {
		return Response{}, err
	}

	started := time.Now()
	stream := p.client.Chat.Completions.NewStreaming(ctx, request, openRouterServerToolsOption())
	defer func() { _ = stream.Close() }()

	var (
		id              string
		model           string
		text            strings.Builder
		reasoning       strings.Builder
		usage           Usage
		totalPrice      float64
		serverToolUsage ServerToolUsage
		citations       []Citation
		citationKeys    = make(map[string]bool)
		usageReceived   bool
	)

	for stream.Next() {
		chunk := stream.Current()

		if chunk.ID != "" {
			id = chunk.ID
		}
		if chunk.Model != "" {
			model = chunk.Model
		}

		var extras streamChunkExtras
		if err := json.Unmarshal([]byte(chunk.RawJSON()), &extras); err == nil {
			if len(extras.Choices) > 0 {
				choice := extras.Choices[0]
				reasoning.WriteString(choice.Delta.Reasoning)

				for _, annotation := range append(choice.Delta.Annotations, choice.Annotations...) {
					citation, ok := annotation.citation()
					if !ok {
						continue
					}

					key := citation.URL + "\x00" + citation.Title
					if citationKeys[key] {
						continue
					}

					citationKeys[key] = true
					citations = append(citations, citation)
				}
			}

			if extras.Usage != nil {
				usageReceived = true
				usage = Usage{
					PromptTokens:     extras.Usage.PromptTokens,
					CompletionTokens: extras.Usage.CompletionTokens,
					TotalTokens:      extras.Usage.TotalTokens,
				}
				totalPrice = extras.Usage.Cost
				serverToolUsage = ServerToolUsage{
					ToolCallsRequested: extras.Usage.ServerToolUseDetails.ToolCallsRequested,
					ToolCallsExecuted:  extras.Usage.ServerToolUseDetails.ToolCallsExecuted,
					WebSearchRequests:  extras.Usage.ServerToolUseDetails.WebSearchRequests,
				}
			}
		}

		for _, choice := range chunk.Choices {
			if choice.Index != 0 || choice.Delta.Content == "" {
				continue
			}

			text.WriteString(choice.Delta.Content)
			if onDelta != nil {
				onDelta(choice.Delta.Content)
			}
		}
	}

	if err = stream.Err(); err != nil {
		return Response{}, err
	}

	if !usageReceived {
		return Response{}, fmt.Errorf("llm stream ended without the final usage chunk")
	}

	r := Response{
		ID:              id,
		Text:            text.String(),
		Reasoning:       reasoning.String(),
		Usage:           usage,
		Model:           model,
		TotalTokens:     usage.TotalTokens,
		ReasoningEffort: p.reasoningEffort,
		TotalPrice:      totalPrice,
		Duration:        time.Since(started),
		ServerToolUsage: serverToolUsage,
		Citations:       citations,
	}

	if err = p.updateHistory(req, r); err != nil {
		return Response{}, err
	}

	p.trace(request, r)

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

// streamChunkExtras holds OpenRouter specific fields of a streamed chunk.
// They are not part of the standard Chat Completions chunk schema.
type streamChunkExtras struct {
	Choices []struct {
		Annotations []chatCompletionAnnotation `json:"annotations"`
		Delta       struct {
			Reasoning   string                     `json:"reasoning"`
			Annotations []chatCompletionAnnotation `json:"annotations"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens         int64   `json:"prompt_tokens"`
		CompletionTokens     int64   `json:"completion_tokens"`
		TotalTokens          int64   `json:"total_tokens"`
		Cost                 float64 `json:"cost"`
		ServerToolUseDetails struct {
			ToolCallsRequested int `json:"tool_calls_requested"`
			ToolCallsExecuted  int `json:"tool_calls_executed"`
			WebSearchRequests  int `json:"web_search_requests"`
		} `json:"server_tool_use_details"`
	} `json:"usage"`
}

// chatCompletionAnnotation is a web citation produced by a server side tool.
// OpenRouter standardizes its web search/fetch results into "url_citation"
// annotations of the message.
type chatCompletionAnnotation struct {
	Type        string `json:"type"`
	URLCitation struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		Content string `json:"content"`
	} `json:"url_citation"`
}

// citation converts the annotation to a Citation.
func (a chatCompletionAnnotation) citation() (Citation, bool) {
	if a.Type != "url_citation" || a.URLCitation.URL == "" {
		return Citation{}, false
	}

	return Citation{
		URL:     a.URLCitation.URL,
		Title:   a.URLCitation.Title,
		Content: a.URLCitation.Content,
	}, true
}

func (p *openRouterProvider) trace(request openai.ChatCompletionNewParams, response Response) {
	if !p.enableTracing {
		return
	}

	requestJSON := formatJSON(request)
	responseJSON, _ := json.MarshalIndent(response, "", "  ")

	_, _ = fmt.Fprintf(os.Stdout, "REQUEST:\n%s\nRESPONSE:\n%s\n\n", requestJSON, responseJSON)
}
