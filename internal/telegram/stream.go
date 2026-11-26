package telegram

import (
	"html"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"gopkg.in/telebot.v4"

	"github.com/kapitanov/gptbot/internal/telegram/texts"
)

const (
	// draftUpdateInterval is the minimum interval between streamed draft updates.
	draftUpdateInterval = time.Second

	// draftHeartbeatInterval keeps the draft alive while the LLM is not producing
	// new text (Telegram drafts live for 30 seconds).
	draftHeartbeatInterval = 10 * time.Second
)

// draftStream renders a response while it is being generated as an ephemeral
// Telegram rich message draft. Drafts are never persisted: once generation
// completes, the caller sends the final formatted message separately.
type draftStream struct {
	bot      *telebot.Bot
	to       telebot.Recipient
	draftID  int
	username string

	mu   sync.Mutex
	text strings.Builder

	sent   string
	sentAt time.Time

	stop    chan struct{}
	stopped chan struct{}
}

// newDraftStream starts a draft stream for the given message.
func newDraftStream(bot *telebot.Bot, msg *telebot.Message) *draftStream {
	s := &draftStream{
		bot:      bot,
		to:       msg.Sender,
		draftID:  msg.ID,
		username: msg.Sender.Username,
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}

	go s.run()

	return s
}

// Append adds a text fragment to the draft.
func (s *draftStream) Append(delta string) {
	if delta == "" {
		return
	}

	s.mu.Lock()
	s.text.WriteString(delta)
	s.mu.Unlock()
}

// Close stops streamed updates and waits for the background goroutine to exit.
func (s *draftStream) Close() {
	close(s.stop)
	<-s.stopped
}

func (s *draftStream) run() {
	defer close(s.stopped)

	s.send("")

	ticker := time.NewTicker(draftUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.flush()
		}
	}
}

func (s *draftStream) flush() {
	s.mu.Lock()
	text := s.text.String()
	s.mu.Unlock()

	// Send when the text has changed, and repeat the last draft periodically
	// to keep it from expiring while the LLM is silent.
	if text == s.sent && time.Since(s.sentAt) < draftHeartbeatInterval {
		return
	}

	s.send(text)
}

func (s *draftStream) send(markdown string) {
	draft := &telebot.InputRichMessage{
		HTML: "<tg-thinking>" + html.EscapeString(texts.Thinking) + "</tg-thinking>",
	}
	if markdown != "" {
		draft = &telebot.InputRichMessage{Markdown: markdown}
	}

	err := s.bot.SendRichDraft(s.to, s.draftID, draft)
	if err != nil {
		log.Error().Err(err).
			Str("username", s.username).
			Int("msg", s.draftID).
			Msg("failed to send draft")
	}

	s.sent = markdown
	s.sentAt = time.Now()
}
