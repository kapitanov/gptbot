package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"github.com/kapitanov/gptbot/internal/access"
	"github.com/kapitanov/gptbot/internal/llmprovider"
	"github.com/kapitanov/gptbot/internal/storage"
	"github.com/kapitanov/gptbot/internal/telegram"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "gptbot",
		Version: "latest",
	}

	verbose := rootCmd.PersistentFlags().BoolP("verbose", "v", false, "enable verbose logs")
	quiet := rootCmd.PersistentFlags().BoolP("quiet", "q", false, "suppress non-error logs")

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		if *verbose {
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
		}
		if *quiet {
			zerolog.SetGlobalLevel(zerolog.ErrorLevel)
		}

		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
		})
		log.Logger = log.Logger.With().Timestamp().Logger()
	}

	rootCmd.AddCommand(runCommand())
	rootCmd.AddCommand(chatCommand())

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.Error().Msg(err.Error())
		os.Exit(1)
	}
}

func withLLMProvider(cmd *cobra.Command) func(history llmprovider.History) (*llmprovider.LLM, error) {
	var llmURL, llmToken, llmModel, llmPromptFilePath, llmReasoningEffort string
	cmd.Flags().StringVar(&llmURL, "llm-url", "", "LLM provider URL (default from $GPTBOT_LLM_URL, or \"https://api.openai.com/v1/\")")
	cmd.Flags().StringVar(&llmToken, "llm-token", "", "LLM provider token (default from $GPTBOT_LLM_TOKEN)")
	cmd.Flags().StringVar(&llmModel, "llm-model", "", "LLM model (default from $GPTBOT_LLM_MODEL)")
	cmd.Flags().StringVar(&llmPromptFilePath, "llm-prompt", "", "LLM prompt file path (default from $GPTBOT_LLM_PROMPT)")
	cmd.Flags().StringVar(&llmReasoningEffort, "llm-reasoning", "", "LLM reasoning effort (default from $GPTBOT_LLM_REASONING)")

	return func(history llmprovider.History) (*llmprovider.LLM, error) {
		if llmURL == "" {
			llmURL = os.Getenv("GPTBOT_LLM_URL")
			if llmURL == "" {
				return nil, errors.New("llm url is required")
			}
		}
		if llmToken == "" {
			llmToken = os.Getenv("GPTBOT_LLM_TOKEN")
		}
		if llmModel == "" {
			llmModel = os.Getenv("GPTBOT_LLM_MODEL")
			if llmModel == "" {
				return nil, errors.New("llm model is required")
			}
		}
		if llmPromptFilePath == "" {
			llmPromptFilePath = os.Getenv("GPTBOT_LLM_PROMPT")

			if llmPromptFilePath == "" {
				return nil, errors.New("llm prompt file path is required")
			}
		}
		if llmReasoningEffort == "" {
			llmReasoningEffort = os.Getenv("GPTBOT_LLM_REASONING")

			if llmReasoningEffort == "" {
				llmReasoningEffort = llmprovider.DefaultReasoningEffort
			}
		}

		llm, err := llmprovider.New(llmprovider.Parameters{
			URL:             llmURL,
			Token:           llmToken,
			Model:           llmModel,
			PromptFilePath:  llmPromptFilePath,
			History:         history,
			EnableTracing:   os.Getenv("GPTBOT_LLM_TRACE") != "",
			ReasoningEffort: llmReasoningEffort,
		})
		if err != nil {
			return nil, err
		}

		return llm, nil
	}
}

func withStorage(cmd *cobra.Command) func() (*storage.Storage, error) {
	var storageDir string
	cmd.Flags().StringVar(&storageDir, "storage-dir", "", "Storage dir path (default from $GPTBOT_STORAGE_DIR)")

	return func() (*storage.Storage, error) {
		if storageDir == "" {
			storageDir = os.Getenv("GPTBOT_STORAGE_DIR")
		}

		if storageDir == "" {
			return nil, errors.New("storage dir path is required")
		}

		return storage.New(storageDir)
	}
}

func withAccessProvider(cmd *cobra.Command) func() (*access.Provider, error) {
	var accessClauses []string
	cmd.Flags().StringSliceVar(&accessClauses, "tg-access", nil, "Telegram access clauses (default from $TELEGRAM_BOT_ACCESS)")

	return func() (*access.Provider, error) {
		if len(accessClauses) == 0 {
			accessClauses = strings.Split(os.Getenv("TELEGRAM_BOT_ACCESS"), ",")
		}

		return access.NewProvider(strings.Join(accessClauses, ",")), nil
	}
}

func withTelegramBot(cmd *cobra.Command) func(s *storage.Storage, llm *llmprovider.LLM, p *access.Provider) (*telegram.Telegram, error) {
	var tgToken string
	cmd.Flags().StringVar(&tgToken, "tg-token", "", "Telegram bot token (default from $TELEGRAM_BOT_TOKEN)")

	return func(s *storage.Storage, llm *llmprovider.LLM, p *access.Provider) (*telegram.Telegram, error) {
		if tgToken == "" {
			tgToken = os.Getenv("TELEGRAM_BOT_TOKEN")
			if tgToken == "" {
				return nil, errors.New("telegram bot token is required")
			}
		}

		tg, err := telegram.New(telegram.Options{
			Token:                 tgToken,
			AccessChecker:         p,
			LLM:                   llm,
			Storage:               s,
			EnableReasoningOutput: os.Getenv("GPTBOT_LLM_REASONING_OUTPUT") != "",
		})
		if err != nil {
			return nil, err
		}
		return tg, nil
	}
}
