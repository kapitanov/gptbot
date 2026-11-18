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
	"time"
)

// Provider names.
const (
	// ProviderOpenAI is the OpenAI provider name.
	ProviderOpenAI = "openai"

	// ProviderOpenRouter is the OpenRouter provider name.
	ProviderOpenRouter = "openrouter"
)

// DefaultProvider is the default LLM provider name.
const DefaultProvider = ProviderOpenAI

// DefaultReasoningEffort is the default reasoning effort.
const DefaultReasoningEffort = "minimal"

// MaxConversationDepth limits conversation depth.
const MaxConversationDepth = 5

// Provider is a provider-agnostic LLM client.
type Provider interface {
	// Generate generates a new message from the input request.
	Generate(ctx context.Context, req Request) (Response, error)
}

// Parameters holds the configuration for the LLM client.
type Parameters struct {
	Provider        string  // Provider name ("openai" or "openrouter").
	URL             string  // Provider API URL.
	Token           string  // Provider API token.
	Model           string  // Model name.
	PromptFilePath  string  // System prompt file path.
	History         History // Conversation history storage.
	EnableTracing   bool    // Enable request/response tracing.
	ReasoningEffort string  // Reasoning effort.
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
	Message        string
	ConversationID string
}

// Usage is token usage of a request.
type Usage struct {
	PromptTokens     int64 // Number of tokens in the prompt.
	CompletionTokens int64 // Number of tokens in the completion.
	TotalTokens      int64 // Total number of tokens used.
}

// Response is a GPT response.
type Response struct {
	ID              string        // Response ID.
	Text            string        // Transformed text.
	Reasoning       string        // Reasoning output.
	Usage           Usage         // Token usage.
	TotalTokens     int64         // Total number of tokens used.
	Model           string        // Model name.
	ReasoningEffort string        // Reasoning effort used.
	Duration        time.Duration // Duration of the response.
}

// New creates a new LLM provider client.
func New(parameters Parameters) (Provider, error) {
	name := parameters.Provider
	if name == "" {
		name = DefaultProvider
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
