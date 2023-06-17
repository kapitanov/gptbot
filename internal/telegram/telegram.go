package telegram

import (
	"context"
	"sync"
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
		if msg.AlbumID != "" {
			return
		}

		log.Warn().Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("empty text")

		_, err := tg.bot.Reply(msg, texts.MissingText)
		if err != nil {
			log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send error message")
		}
		return
	}

	tg.processAsync(msg, func() (string, error) {
		transformedText, err := tg.transformer.Transform(context.Background(), text)
		if err != nil {
			log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Str("text", text).Msg("failed to transform text")
			return "", err
		}

		log.Info().Str("username", msg.Sender.Username).Int("msg", msg.ID).Str("out", transformedText).Msg("processed")
		return transformedText, nil
	})
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

var typingTicker = time.Tick(time.Second)

func (tg *Telegram) processAsync(msg *telebot.Message, fn func() (string, error)) {
	reply, err := tg.bot.Reply(msg, texts.Thinking, telebot.Silent)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to reply")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case <-typingTicker:
				err := tg.bot.Notify(msg.Sender, telebot.Typing)
				if err != nil {
					log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send typing notification")
					return
				}
			}
		}
	}()

	replyCh := make(chan struct {
		replyText string
		err       error
	})
	tg.workerPool.Submit(func() {
		replyText, err := fn()
		replyCh <- struct {
			replyText string
			err       error
		}{
			replyText: replyText,
			err:       err,
		}
	})

	r := <-replyCh

	cancel()
	wg.Wait()

	if r.err != nil {
		log.Error().Err(r.err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to process")

		_, err = tg.bot.Edit(reply, texts.Failure)
		if err != nil {
			log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send error message")
		}
		return
	}

	_, err = tg.bot.Edit(reply, r.replyText)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send reply")
	}
}
