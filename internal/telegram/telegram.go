package telegram

import (
	"context"
	"time"

	"github.com/alitto/pond"
	"github.com/rs/zerolog/log"
	"gopkg.in/tucnak/telebot.v2"

	"github.com/kapitanov/gptbot/internal/telegram/texts"
)

const (
	workerPoolCapacity       = 2
	workerPoolBufferCapacity = 10
)

// Telegram is a telegram bot.
type Telegram struct {
	bot           *telebot.Bot
	transformer   Transformer
	accessChecker AccessChecker
	workerPool    *pond.WorkerPool
}

// Options is a telegram bot options.
type Options struct {
	Token         string        // Telegram bot token.
	Transformer   Transformer   // Text transformer.
	AccessChecker AccessChecker // Access checker.
}

// Transformer transforms text.
type Transformer interface {
	// Transform transforms text.
	Transform(ctx context.Context, text string) (string, error)
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
		transformer:   options.Transformer,
		workerPool:    pond.New(workerPoolCapacity, workerPoolBufferCapacity),
	}

	bot.Handle("/start", tg.onStartCommand)
	bot.Handle(telebot.OnText, func(msg *telebot.Message) { tg.process(msg, msg.Text, "") })
	bot.Handle(telebot.OnPhoto, func(msg *telebot.Message) { tg.process(msg, msg.Photo.Caption, msg.Caption) })
	bot.Handle(telebot.OnVideo, func(msg *telebot.Message) { tg.process(msg, msg.Video.Caption, msg.Caption) })
	bot.Handle(telebot.OnAudio, func(msg *telebot.Message) { tg.process(msg, msg.Audio.Caption, msg.Caption) })
	bot.Handle(telebot.OnAnimation, func(msg *telebot.Message) { tg.process(msg, msg.Animation.Caption, msg.Caption) })
	bot.Handle(telebot.OnDocument, func(msg *telebot.Message) { tg.process(msg, msg.Document.Caption, msg.Caption) })
	bot.Handle(telebot.OnVoice, func(msg *telebot.Message) { tg.process(msg, msg.Voice.Caption, msg.Caption) })

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
	tg.workerPool.StopAndWait()

	_, err := tg.bot.Close()
	if err != nil {
		log.Error().Err(err).Msg("failed to close telegram bot")
	}
}

func (tg *Telegram) onStartCommand(msg *telebot.Message) {
	if !tg.hasAccess(msg) {
		return
	}

	_, err := tg.bot.Send(msg.Sender, texts.Welcome)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send welcome message")
	}
}

func (tg *Telegram) process(msg *telebot.Message, text, altText string) {
	if !tg.hasAccess(msg) {
		return
	}

	if text == "" {
		text = altText
	}
	if text == "" {
		log.Warn().Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("empty text")

		_, err := tg.bot.Reply(msg, texts.MissingText)
		if err != nil {
			log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send error message")
		}
		return
	}

	completed := tg.notifyProcessing(msg)
	log.Info().Str("username", msg.Sender.Username).Str("in", text).Msg("processing")

	tg.workerPool.Submit(func() {
		transformedText, err := tg.transformer.Transform(context.Background(), text)
		if err != nil {
			log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Str("text", text).Msg("failed to transform text")

			_, err = tg.bot.Reply(msg, texts.Failure)
			if err != nil {
				log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send error message")
			}
			return
		}

		log.Info().Str("username", msg.Sender.Username).Int("msg", msg.ID).Str("out", transformedText).Msg("processed")

		completed()
		_, err = tg.bot.Reply(msg, transformedText)
		if err != nil {
			log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send response message")
		}
	})
}

func (tg *Telegram) notifyProcessing(msg *telebot.Message) func() {
	reply, err := tg.bot.Reply(msg, texts.Thinking, telebot.Silent)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send thinking message")
		reply = nil
	}

	tg.bot.Notify(msg.Sender, telebot.Typing)

	if reply == nil {
		return func() {}
	}

	return func() {
		err = tg.bot.Delete(reply)
		if err != nil {
			log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to delete thinking message")
		}
	}
}

func (tg *Telegram) hasAccess(msg *telebot.Message) bool {
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
