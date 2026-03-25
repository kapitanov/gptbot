package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/kapitanov/gptbot/internal/gpt"
	"github.com/kapitanov/gptbot/internal/telegram/mdparser"
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

	lastResponseID, err := tg.storage.GetLastResponseID(msg.Sender.ID)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to get last response id")
		return err
	}

	reply, err := tg.bot.Reply(msg, texts.Thinking, telebot.Silent)
	if err != nil {
		log.Error().Err(err).
			Str("username", msg.Sender.Username).
			Int("msg", msg.ID).
			Msg("failed to reply")
		return err
	}

	err = tg.bot.Notify(msg.Sender, telebot.Typing)
	if err != nil {
		log.Error().Err(err).
			Str("username", msg.Sender.Username).
			Int("msg", msg.ID).
			Msg("failed to send typing notification")
	}

	response, err := tg.gpt.Generate(context.Background(), gpt.Request{Message: request, PrevResponseID: lastResponseID})
	if err != nil {
		return err
	}

	reply, err = tg.reply(msg, reply, response)
	if err != nil {
		log.Error().Err(err).
			Str("username", msg.Sender.Username).
			Int("msg", msg.ID).
			Str("request", request).
			Str("response", response.Text).
			Msg("failed to send reply")
		return err
	}

	err = tg.storage.SetLastResponseID(msg.Sender.ID, response.ID)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to get last response id")
		return err
	}

	log.Info().
		Str("username", msg.Sender.Username).
		Int("msg", msg.ID).
		Str("request", request).
		Str("response", response.Text).
		Msg("generated a reply")

	return nil
}

func (tg *Telegram) reply(msg, reply *telebot.Message, response gpt.Response) (*telebot.Message, error) {
	const maxTextLength = 4096 - 1

	transformResult := mdparser.Transform(mdparser.TransformRequest{Text: response.Text, MaxLength: maxTextLength})

	_ = tg.bot.Delete(reply)

	for _, chunk := range transformResult.Chunks {
		var err error
		reply, err = tg.bot.Reply(msg, chunk.Text, telebot.Silent, telebot.ModeMarkdownV2)
		if err != nil {
			log.Error().Err(err).
				Str("username", msg.Sender.Username).
				Int("msg", msg.ID).
				Msg("failed to reply")
			return nil, err
		}
	}

	return reply, nil
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
