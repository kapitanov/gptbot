package telegram

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"gopkg.in/tucnak/telebot.v2"

	"github.com/kapitanov/gptbot/internal/gpt"
	"github.com/kapitanov/gptbot/internal/storage"
	"github.com/kapitanov/gptbot/internal/telegram/texts"
)

func (tg *Telegram) generate(msg *telebot.Message, text, altText string) {
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
		return
	}

	err := tg.storage.TX(msg.Sender.ID, func(chain *storage.MessageChain) error {
		return tg.generateE(msg, text, chain)
	})
	if err != nil {
		log.Error().Err(err).
			Str("username", msg.Sender.Username).
			Int("msg", msg.ID).
			Str("text", text).
			Msg("failed to process")

		_, err := tg.bot.Reply(msg, texts.Failure)
		if err != nil {
			log.Error().Err(err).
				Str("username", msg.Sender.Username).
				Int("msg", msg.ID).
				Msg("failed to send error message")
		}
	}
}

func (tg *Telegram) generateE(msg *telebot.Message, request string, chain *storage.MessageChain) error {
	gptMessages, err := generateGPTMessages(request, chain)
	if err != nil {
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

	response, err := tg.gpt.Generate(context.Background(), gptMessages)
	if err != nil {
		return err
	}

	err = tg.reply(msg, reply, response)
	if err != nil {
		log.Error().Err(err).
			Str("username", msg.Sender.Username).
			Int("msg", msg.ID).
			Str("request", request).
			Str("response", response).
			Msg("failed to send reply")
		return err
	}

	err = chain.Store(storage.User, request)
	if err != nil {
		return err
	}

	err = chain.Store(storage.Bot, response)
	if err != nil {
		return err
	}

	return tg.reply(msg, reply, response)
}

func (tg *Telegram) reply(msg, reply *telebot.Message, response string) error {
	const maxTextLength = 4096 - 1

	for len(response) > 0 {
		var text string
		if len(response) <= maxTextLength {
			text = response
			response = ""
		} else {
			text = response[:maxTextLength]
			response = response[maxTextLength:]
		}

		if reply != nil {
			_, err := tg.bot.Edit(reply, text)
			if err != nil {
				log.Error().Err(err).
					Str("username", msg.Sender.Username).
					Int("msg", msg.ID).
					Msg("failed to send reply")
				return err
			}

			reply = nil
		} else {
			_, err := tg.bot.Reply(msg, text, telebot.Silent)
			if err != nil {
				log.Error().Err(err).
					Str("username", msg.Sender.Username).
					Int("msg", msg.ID).
					Msg("failed to reply")
				return err
			}
		}
	}

	return nil
}

func generateGPTMessages(text string, chain *storage.MessageChain) ([]gpt.Message, error) {
	text = normalizeText(text)
	if text == "" {
		return nil, errors.New("text is empty")
	}

	storedMessages := chain.Read()

	gptMessages := make([]gpt.Message, 0, len(storedMessages))
	for _, storedMessage := range storedMessages {
		gptMessage := gpt.Message{
			Text:        storedMessage.Text,
			Participant: gpt.ParticipantBot,
		}
		if storedMessage.Side == storage.User {
			gptMessage.Participant = gpt.ParticipantUser
		}

		gptMessages = append(gptMessages, gptMessage)
	}

	gptMessages = append(gptMessages, gpt.Message{
		Text:        text,
		Participant: gpt.ParticipantUser,
	})
	return gptMessages, nil
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
