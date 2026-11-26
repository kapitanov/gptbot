package llmprovider

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// recordingHistory is a History implementation that records appended messages.
type recordingHistory struct {
	messages []Message
}

func (h *recordingHistory) Retrieve(string) ([]Message, error) {
	return nil, nil
}

func (h *recordingHistory) Append(_ string, messages ...Message) error {
	h.messages = append(h.messages, messages...)
	return nil
}

func writePrompt(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(path, []byte("system prompt"), 0o600); err != nil {
		t.Fatalf("failed to write prompt: %v", err)
	}
	return path
}

func sseResponse(events ...string) string {
	var b strings.Builder
	for _, event := range events {
		b.WriteString("data: ")
		b.WriteString(event)
		b.WriteString("\n\n")
	}
	return b.String()
}

func TestOpenAIProviderStreamsText(t *testing.T) {
	const completedResponse = `{"id":"resp_1","object":"response","created_at":100,"completed_at":102,"model":"gpt-test","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello"}]}],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if body["stream"] != true {
			t.Errorf("stream = %v, want true", body["stream"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sseResponse(
			`{"type":"response.output_text.delta","sequence_number":1,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"Hel"}`,
			`{"type":"response.output_text.delta","sequence_number":2,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"lo"}`,
			`{"type":"response.completed","sequence_number":3,"response":`+completedResponse+`}`,
		))
	}))
	defer server.Close()

	history := &recordingHistory{}
	llm := &openAIProvider{
		client:          openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test")),
		models:          []string{"gpt-test"},
		promptFilePath:  writePrompt(t),
		history:         history,
		reasoningEffort: DefaultReasoningEffort,
	}

	var deltas []string
	response, err := llm.Generate(
		context.Background(),
		Request{Message: "hi", ConversationID: "conversation"},
		func(delta string) { deltas = append(deltas, delta) },
	)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if want := []string{"Hel", "lo"}; !reflect.DeepEqual(deltas, want) {
		t.Fatalf("deltas = %#v, want %#v", deltas, want)
	}
	if response.Text != "Hello" {
		t.Fatalf("Text = %q, want %q", response.Text, "Hello")
	}
	if response.ID != "resp_1" {
		t.Fatalf("ID = %q, want %q", response.ID, "resp_1")
	}
	if response.Model != "gpt-test" {
		t.Fatalf("Model = %q, want %q", response.Model, "gpt-test")
	}
	if want := (Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}); response.Usage != want {
		t.Fatalf("Usage = %#v, want %#v", response.Usage, want)
	}
	if response.Duration != 2*time.Second {
		t.Fatalf("Duration = %s, want 2s", response.Duration)
	}

	wantHistory := []Message{
		{Participant: ParticipantUser, Text: "hi"},
		{Participant: ParticipantAssistant, Text: "Hello"},
	}
	if !reflect.DeepEqual(history.messages, wantHistory) {
		t.Fatalf("history = %#v, want %#v", history.messages, wantHistory)
	}
}

func TestOpenAIProviderRequiresCompletedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sseResponse(
			`{"type":"response.output_text.delta","sequence_number":1,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"Hel"}`,
		))
	}))
	defer server.Close()

	llm := &openAIProvider{
		client:          openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test")),
		models:          []string{"gpt-test"},
		promptFilePath:  writePrompt(t),
		history:         &recordingHistory{},
		reasoningEffort: DefaultReasoningEffort,
	}

	_, err := llm.Generate(context.Background(), Request{Message: "hi", ConversationID: "conversation"}, nil)
	if err == nil {
		t.Fatal("Generate() error = nil, want an error")
	}
}

func TestOpenAIProviderFailsOnFailedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sseResponse(
			`{"type":"response.output_text.delta","sequence_number":1,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"Par"}`,
			`{"type":"response.failed","sequence_number":2,"response":{"id":"resp_failed","object":"response","created_at":100,"model":"gpt-test","status":"failed","output":[],"usage":null,"error":{"code":"server_error","message":"upstream exploded"}}}`,
		))
	}))
	defer server.Close()

	history := &recordingHistory{}
	llm := &openAIProvider{
		client:          openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test")),
		models:          []string{"gpt-test"},
		promptFilePath:  writePrompt(t),
		history:         history,
		reasoningEffort: DefaultReasoningEffort,
	}

	_, err := llm.Generate(context.Background(), Request{Message: "hi", ConversationID: "conversation"}, nil)
	if err == nil {
		t.Fatal("Generate() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "upstream exploded") {
		t.Fatalf("Generate() error = %q, want it to contain %q", err, "upstream exploded")
	}
	if len(history.messages) != 0 {
		t.Fatalf("history = %#v, want no messages on failure", history.messages)
	}
}

func TestOpenRouterProviderFailsOnIncompleteStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sseResponse(
			`{"id":"gen-1","object":"chat.completion.chunk","created":100,"model":"test/model","choices":[{"index":0,"delta":{"role":"assistant","content":"Par"}}]}`,
		))
	}))
	defer server.Close()

	history := &recordingHistory{}
	llm := &openRouterProvider{
		client:          openai.NewClient(option.WithBaseURL(server.URL)),
		models:          []string{"test/model"},
		promptFilePath:  writePrompt(t),
		history:         history,
		reasoningEffort: "low",
	}

	_, err := llm.Generate(context.Background(), Request{Message: "hi", ConversationID: "conversation"}, nil)
	if err == nil {
		t.Fatal("Generate() error = nil, want an error")
	}
	if len(history.messages) != 0 {
		t.Fatalf("history = %#v, want no messages on failure", history.messages)
	}
}

func TestOpenRouterProviderStreamsTextAndStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if body["stream"] != true {
			t.Errorf("stream = %v, want true", body["stream"])
		}
		if _, ok := body["tools"]; !ok {
			t.Error("request does not enable server side tools")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sseResponse(
			`{"id":"gen-1","object":"chat.completion.chunk","created":100,"model":"test/model","choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"}}]}`,
			`{"id":"gen-1","object":"chat.completion.chunk","created":100,"model":"test/model","choices":[{"index":0,"delta":{"content":"lo"}}]}`,
			`{"id":"gen-1","object":"chat.completion.chunk","created":100,"model":"test/model","choices":[{"index":0,"delta":{"reasoning":" because","annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com/one","title":"One","content":"first"}}]}}]}`,
			`{"id":"gen-1","object":"chat.completion.chunk","created":100,"model":"test/model","choices":[{"index":0,"delta":{},"annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com/two","title":"Two","content":"second"}}]}]}`,
			`{"id":"gen-1","object":"chat.completion.chunk","created":100,"model":"test/model","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14,"cost":0.0012,"server_tool_use_details":{"tool_calls_requested":1,"tool_calls_executed":1,"web_search_requests":1}}}`,
			`[DONE]`,
		))
	}))
	defer server.Close()

	history := &recordingHistory{}
	llm := &openRouterProvider{
		client:          openai.NewClient(option.WithBaseURL(server.URL)),
		models:          []string{"test/model"},
		promptFilePath:  writePrompt(t),
		history:         history,
		reasoningEffort: "low",
	}

	var deltas []string
	response, err := llm.Generate(
		context.Background(),
		Request{Message: "hi", ConversationID: "conversation"},
		func(delta string) { deltas = append(deltas, delta) },
	)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if want := []string{"Hel", "lo"}; !reflect.DeepEqual(deltas, want) {
		t.Fatalf("deltas = %#v, want %#v", deltas, want)
	}
	if response.Text != "Hello" {
		t.Fatalf("Text = %q, want %q", response.Text, "Hello")
	}
	if response.Reasoning != " because" {
		t.Fatalf("Reasoning = %q, want %q", response.Reasoning, " because")
	}
	if response.ID != "gen-1" {
		t.Fatalf("ID = %q, want %q", response.ID, "gen-1")
	}
	if response.Model != "test/model" {
		t.Fatalf("Model = %q, want %q", response.Model, "test/model")
	}
	if want := (Usage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14}); response.Usage != want {
		t.Fatalf("Usage = %#v, want %#v", response.Usage, want)
	}
	if response.TotalTokens != 14 {
		t.Fatalf("TotalTokens = %d, want 14", response.TotalTokens)
	}
	if math.Abs(response.TotalPrice-0.0012) > 1e-12 {
		t.Fatalf("TotalPrice = %v, want 0.0012", response.TotalPrice)
	}
	if want := (ServerToolUsage{ToolCallsRequested: 1, ToolCallsExecuted: 1, WebSearchRequests: 1}); response.ServerToolUsage != want {
		t.Fatalf("ServerToolUsage = %#v, want %#v", response.ServerToolUsage, want)
	}
	if len(response.Citations) != 2 {
		t.Fatalf("Citations = %#v, want 2 citations", response.Citations)
	}
	if response.Citations[0].URL != "https://example.com/one" || response.Citations[1].URL != "https://example.com/two" {
		t.Fatalf("Citations = %#v", response.Citations)
	}
	if response.Duration <= 0 {
		t.Fatalf("Duration = %s, want > 0", response.Duration)
	}

	wantHistory := []Message{
		{Participant: ParticipantUser, Text: "hi"},
		{Participant: ParticipantAssistant, Text: "Hello"},
	}
	if !reflect.DeepEqual(history.messages, wantHistory) {
		t.Fatalf("history = %#v, want %#v", history.messages, wantHistory)
	}
}
