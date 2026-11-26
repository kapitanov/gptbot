package telegram

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/kapitanov/gptbot/internal/llmprovider"
	"github.com/kapitanov/gptbot/internal/telegram/texts"
	"github.com/rs/zerolog/log"
	"gopkg.in/telebot.v4"
)

func (tg *Telegram) generate(msg *telebot.Message, text, altText string) error {
	if !tg.hasAccess(msg) {
		return nil
	}

	if text == "" {
		text = altText
	}

	if text == "" {
		if msg.AlbumID != "" {
			return nil
		}

		log.Warn().
			Str("username", msg.Sender.Username).
			Int("msg", msg.ID).
			Msg("empty text")

		_, err := tg.bot.Reply(msg, texts.MissingText)
		if err != nil {
			log.Error().Err(err).
				Str("username", msg.Sender.Username).
				Int("msg", msg.ID).
				Msg("failed to send error message")
		}
		return err
	}

	err := tg.generateE(msg, text)
	if err != nil {
		log.Error().Err(err).
			Str("username", msg.Sender.Username).
			Int("msg", msg.ID).
			Str("text", text).
			Msg("failed to process")

		_, err := tg.bot.Reply(msg, fmt.Sprintf("%s\n%s", texts.Failure, err.Error()))
		if err != nil {
			log.Error().Err(err).
				Str("username", msg.Sender.Username).
				Int("msg", msg.ID).
				Msg("failed to send error message")
			return err
		}
	}
	return nil
}

func (tg *Telegram) generateE(msg *telebot.Message, request string) error {
	request = normalizeText(request)
	if request == "" {
		return nil
	}

	conversationID := getConversationID(msg)

	model, err := tg.modelForConversation(conversationID)
	if err != nil {
		return err
	}

	draft := newDraftStream(tg.bot, msg)
	response, err := tg.llm.Generate(
		context.Background(),
		llmprovider.Request{Message: request, ConversationID: conversationID, Model: model},
		draft.Append,
	)
	draft.Close()

	if err != nil {
		return err
	}

	markdown := response.Text

	if tg.enableReasoningOutput && response.Reasoning != "" {
		markdown = fmt.Sprintf("<details><summary>Reasoning</summary>\n\n%s\n\n<hr/>End of reasoning\n\n</details>\n\n%s", html.EscapeString(response.Reasoning), markdown)
	}

	markdown = fmt.Sprintf(
		"%s\n\n<sub>model %s, reasoning %s, %s tokens%s%s, generated in %.2fs, %.1f tokens/sec</sub>",
		markdown,
		response.Model,
		string(response.ReasoningEffort),
		prettyTokens(response.Usage.TotalTokens),
		prettyPrice(response.TotalPrice),
		prettyWebSearches(response.ServerToolUsage.WebSearchRequests),
		response.Duration.Seconds(),
		float64(response.Usage.TotalTokens)/response.Duration.Seconds(),
	)

	_, err = tg.bot.Send(msg.Sender, &telebot.InputRichMessage{
		Markdown: markdown,
	})
	if err != nil {
		log.Error().Err(err).
			Str("username", msg.Sender.Username).
			Int("msg", msg.ID).
			Msg("failed to reply")
		return err
	}

	log.Info().
		Str("username", msg.Sender.Username).
		Int("msg", msg.ID).
		Str("conversation", conversationID).
		Int64("tokens", response.Usage.TotalTokens).
		Str("model", response.Model).
		Str("request", request).
		Str("response", response.Text).
		Msg("generated a reply")

	return nil
}

func normalizeText(text string) string {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return ""
	}

	if !strings.HasSuffix(text, ".") {
		text = text + "."
	}

	return text
}

func prettyTokens(tokens int64) string {
	if tokens < 1000 {
		return fmt.Sprintf("%d", tokens)
	}
	return fmt.Sprintf("%.1fk", float64(tokens)/1000)
}

func prettyPrice(price float64) string {
	if price <= 0 {
		return ""
	}
	return fmt.Sprintf(", $%.4f", price)
}

func prettyWebSearches(searches int) string {
	if searches <= 0 {
		return ""
	}
	return fmt.Sprintf(", %d web search%s", searches, plural(searches))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}
