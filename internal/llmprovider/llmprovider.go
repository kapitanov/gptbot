// Package llmprovider provides LLM clients with a provider-agnostic API.
//
// Two implementations are available, selected via Parameters.Provider
// (or the LLM_PROVIDER / GPTBOT_LLM_PROVIDER environment variable):
//
//   - "openai":    OpenAI API (Responses API)
//   - "openrouter": OpenRouter API (Chat Completions API)
package llmprovider

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ProviderType is a LLM provider type name.
type ProviderType string

const (
	// ProviderOpenAI is the OpenAI provider name.
	ProviderOpenAI ProviderType = "openai"

	// ProviderOpenRouter is the OpenRouter provider name.
	ProviderOpenRouter ProviderType = "openrouter"
)

// DefaultProvider is the default LLM provider name.
const DefaultProvider = ProviderOpenAI

// ReasoningEffort is a reasoning effort level.
type ReasoningEffort string

// DefaultReasoningEffort is the default reasoning effort.
const DefaultReasoningEffort ReasoningEffort = "minimal"

// MaxConversationDepth limits conversation depth.
const MaxConversationDepth = 5

// Provider is a provider-agnostic LLM client.
type Provider interface {
	// Models returns the list of available model names.
	Models() []string

	// DefaultModel returns the model name used when the request does not specify one.
	DefaultModel() string

	// Generate generates a new message from the input request.
	Generate(ctx context.Context, req Request) (Response, error)
}

// ParseModels parses a comma-separated list of model names.
// Empty and duplicate entries are skipped.
func ParseModels(s string) []string {
	var models []string
	seen := make(map[string]bool)

	for _, m := range strings.Split(s, ",") {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		models = append(models, m)
	}

	return models
}

// FindModel returns the model from the list matching the given name
// (case-insensitively) and true, or an empty string and false.
func FindModel(models []string, name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}

	for _, model := range models {
		if strings.EqualFold(model, name) {
			return model, true
		}
	}

	return "", false
}

// Parameters holds the configuration for the LLM client.
type Parameters struct {
	Provider        ProviderType    // Provider type ("openai" or "openrouter").
	URL             string          // Provider API URL.
	Token           string          // Provider API token.
	Model           string          // Comma-separated list of model names.
	PromptFilePath  string          // System prompt file path.
	History         History         // Conversation history storage.
	EnableTracing   bool            // Enable request/response tracing.
	ReasoningEffort ReasoningEffort // Reasoning effort.
}

// History represents the conversation history storage.
type History interface {
	// Retrieve fetches the conversation history for the given conversation ID.
	Retrieve(conversationID string) ([]Message, error)

	// Append adds messages to the conversation history for the given conversation ID.
	Append(conversationID string, messages ...Message) error
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
	Message        string // Request text.
	ConversationID string // Conversation ID.
	Model          string // Model name; when empty, the provider's default model is used.
}

// Usage is token usage of a request.
type Usage struct {
	PromptTokens     int64 // Number of tokens in the prompt.
	CompletionTokens int64 // Number of tokens in the completion.
	TotalTokens      int64 // Total number of tokens used.
}

// Response is a GPT response.
type Response struct {
	ID              string          // Response ID.
	Text            string          // Transformed text.
	Reasoning       string          // Reasoning output.
	Usage           Usage           // Token usage.
	TotalTokens     int64           // Total number of tokens used.
	Model           string          // Model name actually used for the response.
	ReasoningEffort ReasoningEffort // Reasoning effort used.
	TotalPrice      float64         // Total price in USD, when reported by the provider.
	Duration        time.Duration   // Duration of the response.

	// ServerToolUsage holds server side tool usage counters, when reported by the provider.
	ServerToolUsage ServerToolUsage

	// Citations holds web citations produced by server side web search/fetch tools,
	// when reported by the provider.
	Citations []Citation
}

// ServerToolUsage holds server side tool usage counters.
type ServerToolUsage struct {
	ToolCallsRequested int // Total server tool calls requested by the model.
	ToolCallsExecuted  int // Server tool calls executed.
	WebSearchRequests  int // Web search queries performed.
}

// Citation is a web citation produced by a server side web search/fetch tool.
type Citation struct {
	URL     string // Cited URL.
	Title   string // Title of the cited page.
	Content string // Content excerpt of the cited page.
}

// New creates a new LLM provider client.
func New(parameters Parameters) (Provider, error) {
	name := parameters.Provider
	if name == "" {
		name = DefaultProvider
	}

	if len(ParseModels(parameters.Model)) == 0 {
		return nil, fmt.Errorf("llm model is required")
	}

	switch name {
	case ProviderOpenAI:
		return newOpenAIProvider(parameters)
	case ProviderOpenRouter:
		return newOpenRouterProvider(parameters)
	default:
		return nil, fmt.Errorf("unknown llm provider %q (supported providers: %q, %q)",
			name, ProviderOpenAI, ProviderOpenRouter)
	}
}
