package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"strings"

	_ "github.com/joho/godotenv/autoload"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/kapitanov/gptbot/internal/gpt"
	"github.com/kapitanov/gptbot/internal/storage"
	"github.com/kapitanov/gptbot/internal/telegram"
)

func main() {
	configureLogger()

	s, err := storage.New(os.Getenv("STORAGE_PATH"))
	if err != nil {
		panic(err)
	}

	g, err := gpt.New(os.Getenv("OPENAI_TOKEN"))
	if err != nil {
		panic(err)
	}

	accessProvider := NewAccessProvider(os.Getenv("TELEGRAM_BOT_ACCESS"))

	tg, err := telegram.New(telegram.Options{
		Token:         os.Getenv("TELEGRAM_BOT_TOKEN"),
		AccessChecker: accessProvider,
		GPT:           g,
		Storage:       s,
	})
	if err != nil {
		panic(err)
	}
	defer tg.Close()

	ctx, cancel := context.WithCancel(context.Background())
	interrupt := make(chan os.Signal)
	signal.Notify(interrupt, os.Interrupt)

	go func() {
		<-interrupt
		cancel()
	}()

	log.Info().Msg("press <ctrl+c> to exit")
	tg.Run(ctx)
	log.Info().Msg("good bye")
}

func configureLogger() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out: os.Stderr,
	})

	log.Logger = log.Logger.With().Timestamp().Logger()
}

// AccessProvider checks access to telegram chats.
type AccessProvider struct {
	ids       map[int64]struct{}
	usernames map[string]struct{}
}

// NewAccessProvider creates new access provider.
// Input string must be a list of telegram user ids and usernames separated by commas, spaces or semicolons.
func NewAccessProvider(s string) *AccessProvider {
	ap := &AccessProvider{
		ids:       make(map[int64]struct{}),
		usernames: make(map[string]struct{}),
	}

	fieldFunc := func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	}

	for _, username := range strings.FieldsFunc(s, fieldFunc) {
		username = strings.TrimSpace(username)

		id, err := strconv.ParseInt(username, 10, 64)
		if err == nil {
			ap.ids[id] = struct{}{}
		} else {
			username = strings.TrimPrefix(username, "@")
			ap.usernames[username] = struct{}{}
		}
	}

	return ap
}

// CheckAccess checks access to telegram chat and returns true if access is granted.
func (ap *AccessProvider) CheckAccess(id int64, username string) bool {
	if _, ok := ap.ids[id]; ok {
		return true
	}

	if _, ok := ap.usernames[username]; ok {
		return true
	}

	return false
}
