package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/x/term"
	"github.com/kapitanov/gptbot/internal/llmprovider"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func chatCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Run the GPT bot in terminal chat mode",
		Args:  cobra.NoArgs,
	}

	getLLMProvider := withLLMProvider(cmd)
	getStorage := withStorage(cmd)

	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}

		g, err := getLLMProvider(s)
		if err != nil {
			return err
		}

		ctx := cmd.Context()

		conversationID := "local"

		for {
			line, exit := readUserChatMessage(ctx)
			if exit {
				break
			}
			if line == "" {
				continue
			}
			if line == "/exit" || line == "/quit" || line == "/q" {
				return nil
			}

			if ctx.Err() != nil {
				return nil
			}

			printUserChatMessage(line)

			var response llmprovider.Response
			runSpinner(ctx, func() {
				response, err = g.Generate(ctx, llmprovider.Request{Message: line, ConversationID: conversationID})
			})

			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				log.Error().Err(err).Msg("failed to generate response")
				continue
			}

			printBotChatMessage(response)
		}
		return nil
	}
	return cmd
}

var (
	faintStyle       = lipgloss.NewStyle().Faint(true)
	userMessageStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.BrightCyan)
)

func readUserChatMessage(ctx context.Context) (string, bool) {
	var line string
	input := huh.NewInput().Title("").Placeholder("(type \"/q\" to quit)").Value(&line)

	group := huh.NewGroup(input)
	form := huh.NewForm(group).WithShowHelp(false)

	err := form.RunWithContext(ctx)
	if err != nil {
		return "", true
	}

	line = strings.TrimSpace(line)
	return line, false
}

func printUserChatMessage(message string) {
	_, _ = lipgloss.Println(">", userMessageStyle.Render(message))
}

func runSpinner(ctx context.Context, action func()) {
	_ = spinner.New().Context(ctx).Title(faintStyle.Render("Thinking....")).Action(action).Run()
}

func printBotChatMessage(response llmprovider.Response) {
	width, _, _ := term.GetSize(os.Stdout.Fd())
	r, _ := glamour.NewTermRenderer(
		glamour.WithStylePath(styles.DraculaStyle),
		glamour.WithWordWrap(width),
	)
	out, _ := r.RenderBytes([]byte(response.Text))

	_, _ = lipgloss.Println(string(out))
	_, _ = lipgloss.Println(faintStyle.Render(
		fmt.Sprintf("%d tokens ⋅ model %s ⋅ duration %s", response.Usage.TotalTokens, response.Model, response.Duration),
	))
	_, _ = lipgloss.Println()
}
