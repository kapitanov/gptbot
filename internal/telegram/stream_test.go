package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/telebot.v4"

	"github.com/kapitanov/gptbot/internal/llmprovider"
	"github.com/kapitanov/gptbot/internal/storage"
)

// fakeTelegramAPI records rich message requests sent by the bot.
type fakeTelegramAPI struct {
	mu     sync.Mutex
	drafts []fakeDraftRequest
	finals []fakeRichRequest
}

type fakeDraftRequest struct {
	ChatID      string `json:"chat_id"`
	DraftID     string `json:"draft_id"`
	RichMessage string `json:"rich_message"`
}

type fakeRichRequest struct {
	ChatID      string `json:"chat_id"`
	RichMessage string `json:"rich_message"`
}

func newFakeTelegramAPI(t *testing.T) (*fakeTelegramAPI, *telebot.Bot) {
	t.Helper()

	api := &fakeTelegramAPI{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}

		switch {
		case strings.HasSuffix(r.URL.Path, "/sendRichMessageDraft"):
			var req fakeDraftRequest
			if err = json.Unmarshal(body, &req); err != nil {
				t.Errorf("failed to decode draft request: %v", err)
			}
			api.mu.Lock()
			api.drafts = append(api.drafts, req)
			api.mu.Unlock()
			_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
		case strings.HasSuffix(r.URL.Path, "/sendRichMessage"):
			var req fakeRichRequest
			if err = json.Unmarshal(body, &req); err != nil {
				t.Errorf("failed to decode rich message request: %v", err)
			}
			api.mu.Lock()
			api.finals = append(api.finals, req)
			api.mu.Unlock()
			_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"},"text":"ok"}}`)
		default:
			t.Errorf("unexpected API method %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	bot, err := telebot.NewBot(telebot.Settings{
		URL:     server.URL,
		Token:   "test",
		Client:  server.Client(),
		Offline: true,
	})
	if err != nil {
		t.Fatalf("failed to create bot: %v", err)
	}

	return api, bot
}

func (a *fakeTelegramAPI) draftSnapshot() []fakeDraftRequest {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]fakeDraftRequest(nil), a.drafts...)
}

func (a *fakeTelegramAPI) finalSnapshot() []fakeRichRequest {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]fakeRichRequest(nil), a.finals...)
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", what)
}

func testMessage() *telebot.Message {
	return &telebot.Message{
		ID:     42,
		Sender: &telebot.User{ID: 1, Username: "alice"},
		Chat:   &telebot.Chat{ID: 1, Type: telebot.ChatPrivate},
	}
}

func TestDraftStreamSendsThinkingThenPartialText(t *testing.T) {
	api, bot := newFakeTelegramAPI(t)
	msg := testMessage()

	draft := newDraftStream(bot, msg)
	waitFor(t, "thinking draft", func() bool {
		return len(api.draftSnapshot()) >= 1
	})

	first := api.draftSnapshot()[0]
	if first.DraftID != "42" {
		t.Fatalf("draft_id = %q, want %q", first.DraftID, "42")
	}

	var initial telebot.InputRichMessage
	if err := json.Unmarshal([]byte(first.RichMessage), &initial); err != nil {
		t.Fatalf("failed to decode initial draft: %v", err)
	}
	if !strings.Contains(initial.HTML, "<tg-thinking>") {
		t.Fatalf("initial draft HTML = %q, want a thinking block", initial.HTML)
	}

	draft.Append("Hello")
	waitFor(t, "partial text draft", func() bool {
		for _, draft := range api.draftSnapshot() {
			var rich telebot.InputRichMessage
			if json.Unmarshal([]byte(draft.RichMessage), &rich) == nil && rich.Markdown == "Hello" {
				return true
			}
		}
		return false
	})

	draft.Close()

	sent := len(api.draftSnapshot())
	time.Sleep(draftUpdateInterval + 200*time.Millisecond)
	if got := len(api.draftSnapshot()); got != sent {
		t.Fatalf("draft updates after Close: %d -> %d", sent, got)
	}
}

// fakeLLMProvider returns a fixed response and streams the configured deltas.
type fakeLLMProvider struct {
	response llmprovider.Response
	deltas   []string
}

func (p *fakeLLMProvider) Models() []string {
	return []string{"fake-model"}
}

func (p *fakeLLMProvider) DefaultModel() string {
	return "fake-model"
}

func (p *fakeLLMProvider) Generate(_ context.Context, _ llmprovider.Request, onDelta llmprovider.DeltaHandler) (llmprovider.Response, error) {
	for _, delta := range p.deltas {
		if onDelta != nil {
			onDelta(delta)
		}
	}
	return p.response, nil
}

func TestGenerateESendsOneFinalMessage(t *testing.T) {
	api, bot := newFakeTelegramAPI(t)
	msg := testMessage()

	st, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	tg := &Telegram{
		bot:     bot,
		storage: st,
		llm: &fakeLLMProvider{
			response: llmprovider.Response{
				Text:            "Hello",
				Model:           "fake-model",
				Usage:           llmprovider.Usage{TotalTokens: 10},
				ReasoningEffort: llmprovider.DefaultReasoningEffort,
				Duration:        time.Second,
			},
			deltas: []string{"Hel", "lo"},
		},
	}

	if err = tg.generateE(msg, "hi"); err != nil {
		t.Fatalf("generateE() error = %v", err)
	}

	finals := api.finalSnapshot()
	if len(finals) != 1 {
		t.Fatalf("final messages = %d, want exactly 1", len(finals))
	}

	var final telebot.InputRichMessage
	if err = json.Unmarshal([]byte(finals[0].RichMessage), &final); err != nil {
		t.Fatalf("failed to decode final message: %v", err)
	}
	if !strings.Contains(final.Markdown, "Hello") {
		t.Fatalf("final markdown = %q, want it to contain %q", final.Markdown, "Hello")
	}
	if !strings.Contains(final.Markdown, "model fake-model") {
		t.Fatalf("final markdown = %q, want it to contain the model stats", final.Markdown)
	}

	if len(api.draftSnapshot()) == 0 {
		t.Fatal("no streamed drafts were sent")
	}
}
