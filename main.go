package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/chzyer/readline"
	_ "github.com/joho/godotenv/autoload"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/kapitanov/gptbot/internal/gpt"
	"github.com/kapitanov/gptbot/internal/storage"
	"github.com/kapitanov/gptbot/internal/telegram"
)

func main() {
	rootCmd := &cobra.Command{
		Use: "gptbot",
	}

	verbose := rootCmd.PersistentFlags().BoolP("verbose", "v", false, "enable verbose logs")
	quiet := rootCmd.PersistentFlags().BoolP("quiet", "q", false, "suppress non-error logs")

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		if *verbose {
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
		}
		if *quiet {
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
		}

		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
		})
		log.Logger = log.Logger.With().Timestamp().Logger()
	}

	rootCmd.AddCommand(runCommand())
	rootCmd.AddCommand(chatCommand())

	if err := rootCmd.Execute(); err != nil {
		log.Error().Msg(err.Error())
		os.Exit(1)
	}
}

func runCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the GPT bot",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := storage.New(os.Getenv("STORAGE_PATH"))
			if err != nil {
				return err
			}

			g, err := gpt.New(os.Getenv("OPENAI_TOKEN"))
			if err != nil {
				return err
			}

			accessProvider := NewAccessProvider(os.Getenv("TELEGRAM_BOT_ACCESS"))

			tg, err := telegram.New(telegram.Options{
				Token:         os.Getenv("TELEGRAM_BOT_TOKEN"),
				AccessChecker: accessProvider,
				GPT:           g,
				Storage:       s,
			})
			if err != nil {
				return err
			}
			defer tg.Close()

			ctx, cancel := context.WithCancel(context.Background())
			interrupt := make(chan os.Signal, 1)
			signal.Notify(interrupt, os.Interrupt)

			go func() {
				<-interrupt
				cancel()
			}()

			log.Info().Msg("press <ctrl+c> to exit")
			tg.Run(ctx)
			log.Info().Msg("good bye")
			return nil
		},
	}
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

func chatCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "chat",
		Short: "Run the GPT bot in terminal chat mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := gpt.New(os.Getenv("OPENAI_TOKEN"))
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(context.Background())
			interrupt := make(chan os.Signal, 1)
			signal.Notify(interrupt, os.Interrupt)

			go func() {
				<-interrupt
				cancel()
			}()

			var messages []gpt.Message

			_, _ = fmt.Fprintf(os.Stderr, "(type \"/q\" to quit)\n")

			for {
				line, err := readLine()
				if err != nil {
					if errors.Is(err, readline.ErrInterrupt) || errors.Is(err, io.EOF) {
						return nil
					}
					break
				}

				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if line == "/exit" || line == "/quit" || line == "/q" {
					return nil
				}

				if ctx.Err() != nil {
					return nil
				}

				messages = append(messages, gpt.Message{
					Participant: gpt.ParticipantUser,
					Text:        line,
				})

				_, _ = fmt.Fprintf(os.Stderr, "... ")
				response, err := g.Generate(ctx, messages)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return nil
					}
					log.Error().Err(err).Msg("failed to generate response")
					continue
				}

				_, _ = fmt.Fprintf(os.Stderr, "\r< %s\n\n", response.Text)
				_, _ = fmt.Fprintf(os.Stderr, "# %d tokens\n\n", response.Usage.TotalTokens)

				messages = append(messages, gpt.Message{
					Participant: gpt.ParticipantBot,
					Text:        response.Text,
				})
			}
			return nil
		},
	}
}

func readLine() (string, error) {
	_, _ = fmt.Fprintf(os.Stderr, "> ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(line), nil
}
