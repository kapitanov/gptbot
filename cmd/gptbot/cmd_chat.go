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
	"github.com/kapitanov/gptbot/internal/beep"
	"github.com/kapitanov/gptbot/internal/llmprovider"
	"github.com/kapitanov/gptbot/internal/storage"
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

		model := resolveChatModel(s, conversationID, g)

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

			if modelName, isModelCommand := parseModelCommand(line); isModelCommand {
				model = selectChatModel(s, conversationID, g, model, modelName)
				continue
			}

			if ctx.Err() != nil {
				return nil
			}

			printUserChatMessage(line)

			var response llmprovider.Response
			runSpinner(ctx, func() {
				response, err = g.Generate(ctx, llmprovider.Request{Message: line, ConversationID: conversationID, Model: model}, nil)
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

// parseModelCommand returns the model name argument of a "/model" command
// and true, or an empty string and false when the line is not a "/model" command.
func parseModelCommand(line string) (string, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 || fields[0] != "/model" {
		return "", false
	}

	return strings.Join(fields[1:], " "), true
}

// resolveChatModel returns the model pinned to the chat conversation,
// falling back to the provider's default model when nothing is pinned or the
// pinned model is no longer configured.
func resolveChatModel(s *storage.Storage, conversationID string, g llmprovider.Provider) string {
	pinned, err := s.Model(conversationID)
	if err != nil {
		log.Error().Err(err).Msg("failed to load pinned model")
		return g.DefaultModel()
	}

	if pinned == "" {
		return g.DefaultModel()
	}

	if model, ok := llmprovider.FindModel(g.Models(), pinned); ok {
		return model
	}

	log.Warn().Str("model", pinned).Msg("pinned model is not configured anymore, using the default one")
	return g.DefaultModel()
}

// selectChatModel handles the "/model" command: it prints the list of models
// or pins the requested model to the conversation.
func selectChatModel(s *storage.Storage, conversationID string, g llmprovider.Provider, current, name string) string {
	if name == "" {
		_, _ = lipgloss.Println(faintStyle.Render("Current model:"), current)
		for _, model := range g.Models() {
			if model == current {
				_, _ = lipgloss.Println(userMessageStyle.Render("  * " + model))
			} else {
				_, _ = lipgloss.Println("    " + model)
			}
		}
		return current
	}

	model, ok := llmprovider.FindModel(g.Models(), name)
	if !ok {
		_, _ = lipgloss.Println(faintStyle.Render(fmt.Sprintf("Unknown model: %s", name)))
		return current
	}

	if err := s.SetModel(conversationID, model); err != nil {
		log.Error().Err(err).Msg("failed to set model")
		return current
	}

	_, _ = lipgloss.Println(faintStyle.Render("Model:"), model)
	return model
}

func readUserChatMessage(ctx context.Context) (string, bool) {
	var line string
	input := huh.NewInput().Title("").Placeholder("(type \"/model\" to pick a model, \"/q\" to quit)").Value(&line)

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
	text := ""
	if response.Reasoning != "" {
		for line := range strings.Lines(response.Reasoning) {
			text += "> " + line
		}
		text += "\n"
	}
	text += response.Text

	width, _, _ := term.GetSize(os.Stdout.Fd())
	r, _ := glamour.NewTermRenderer(
		glamour.WithStylePath(styles.DraculaStyle),
		glamour.WithWordWrap(width),
	)
	out, _ := r.RenderBytes([]byte(text))

	_, _ = lipgloss.Println(string(out))
	stats := fmt.Sprintf("%d tokens ⋅ model %s ⋅ duration %s",
		response.Usage.TotalTokens, response.Model, response.Duration)
	if response.TotalPrice > 0 {
		stats += fmt.Sprintf(" ⋅ price $%.4f", response.TotalPrice)
	}
	_, _ = lipgloss.Println(faintStyle.Render(stats))
	_, _ = lipgloss.Println()

	beep.Beep()
}
