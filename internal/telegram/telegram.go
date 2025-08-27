package telegram

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"gopkg.in/telebot.v4"

	"github.com/kapitanov/gptbot/internal/gpt"
	"github.com/kapitanov/gptbot/internal/storage"
	"github.com/kapitanov/gptbot/internal/telegram/texts"
)

// Telegram is a telegram bot.
type Telegram struct {
	bot           *telebot.Bot
	storage       *storage.Storage
	gpt           *gpt.GPT
	accessChecker AccessChecker
}

// Options is a telegram bot options.
type Options struct {
	Token         string           // Telegram bot token.
	GPT           *gpt.GPT         // GPT text transformer.
	AccessChecker AccessChecker    // Access checker.
	Storage       *storage.Storage // Storage.
}

// AccessChecker checks access to telegram chats.
type AccessChecker interface {
	// CheckAccess checks access to telegram chat and returns true if access is granted.
	CheckAccess(id int64, username string) bool
}

// New creates a new telegram bot.
func New(options Options) (*Telegram, error) {
	bot, err := telebot.NewBot(telebot.Settings{
		Token:  options.Token,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		return nil, err
	}

	log.Info().Int64("id", bot.Me.ID).Str("username", bot.Me.Username).Msg("connected to telegram")

	tg := &Telegram{
		bot:           bot,
		accessChecker: options.AccessChecker,
		gpt:           options.GPT,
		storage:       options.Storage,
	}

	tg.setupHandlers()

	return tg, nil
}

// Run runs telegram bot in foreground until context is canceled.
func (tg *Telegram) Run(ctx context.Context) {
	go tg.bot.Start()
	defer tg.bot.Stop()

	<-ctx.Done()
}

// Close shuts telegram bot down.
func (tg *Telegram) Close() {
	_, err := tg.bot.Close()
	if err != nil {
		log.Error().Err(err).Msg("failed to close telegram bot")
	}
}

func (tg *Telegram) hasAccess(msg *telebot.Message) bool {
	if msg.Sender.ID == tg.bot.Me.ID {
		return true
	}

	if tg.accessChecker.CheckAccess(msg.Sender.ID, msg.Sender.Username) {
		return true
	}

	log.Error().Str("username", msg.Sender.Username).Msg("access denied")

	_, err := tg.bot.Reply(msg, texts.AccessDenied)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send access denied message")
	}
	return false
}
