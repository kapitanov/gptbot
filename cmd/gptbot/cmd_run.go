package main

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func runCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the GPT bot",
		Args:  cobra.NoArgs,
	}

	getLLMProvider := withLLMProvider(cmd)
	getStorage := withStorage(cmd)
	getAccessProvider := withAccessProvider(cmd)
	getTelegramBot := withTelegramBot(cmd)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		llm, err := getLLMProvider(s)
		if err != nil {
			return err
		}
		accessProvider, err := getAccessProvider()
		if err != nil {
			return err
		}

		tg, err := getTelegramBot(s, llm, accessProvider)
		if err != nil {
			return err
		}
		defer tg.Close()

		log.Info().Msg("press <ctrl+c> to exit")
		tg.Run(cmd.Context())
		log.Info().Msg("good bye")
		return nil
	}

	return cmd
}
