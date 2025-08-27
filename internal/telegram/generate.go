package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/kapitanov/gptbot/internal/telegram/mdparser"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"gopkg.in/telebot.v4"

	"github.com/kapitanov/gptbot/internal/gpt"
	"github.com/kapitanov/gptbot/internal/storage"
	"github.com/kapitanov/gptbot/internal/telegram/texts"
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

	err := tg.storage.TX(msg.Sender.ID, func(chain *storage.MessageChain) error {
		return tg.generateE(msg, text, chain)
	})
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

func (tg *Telegram) generateE(msg *telebot.Message, request string, chain *storage.MessageChain) error {
	gptMessages, err := generateGPTMessages(msg, request, chain)
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

	reply, err = tg.reply(msg, reply, response)
	if err != nil {
		log.Error().Err(err).
			Str("username", msg.Sender.Username).
			Int("msg", msg.ID).
			Str("request", request).
			Str("response", response).
			Msg("failed to send reply")
		return err
	}

	var replyToID *int
	if msg.ReplyTo != nil {
		replyToID = &msg.ReplyTo.ID
	}
	err = chain.Store(msg.ID, replyToID, storage.User, request)
	if err != nil {
		return err
	}

	err = chain.Store(reply.ID, &msg.ID, storage.Bot, response)
	if err != nil {
		return err
	}

	log.Info().
		Str("username", msg.Sender.Username).
		Int("msg", msg.ID).
		Str("request", request).
		Str("response", response).
		Msg("generated a reply")

	return nil
}

func (tg *Telegram) reply(msg, reply *telebot.Message, response string) (*telebot.Message, error) {
	const maxTextLength = 4096 - 1

	response, entities := mdparser.Parse(response)

	_ = tg.bot.Delete(reply)

	if len(response) <= maxTextLength {
		var err error
		reply, err = tg.bot.Reply(msg, response, &telebot.SendOptions{Entities: entities})
		if err != nil {
			log.Error().Err(err).
				Str("username", msg.Sender.Username).
				Int("msg", msg.ID).
				Msg("failed to reply")
			return nil, err
		}
	} else {
		for len(response) > 0 {
			var text string
			if len(response) <= maxTextLength {
				text = response
				response = ""
			} else {
				text = response[:maxTextLength]
				response = response[maxTextLength:]
			}

			var err error
			reply, err = tg.bot.Reply(msg, text, telebot.Silent)
			if err != nil {
				log.Error().Err(err).
					Str("username", msg.Sender.Username).
					Int("msg", msg.ID).
					Msg("failed to reply")
				return nil, err
			}
		}
	}

	return reply, nil
}

func generateGPTMessages(msg *telebot.Message, text string, chain *storage.MessageChain) ([]gpt.Message, error) {
	text = normalizeText(text)
	if text == "" {
		return nil, errors.New("text is empty")
	}

	msgID := 0
	if msg.ReplyTo != nil {
		msgID = msg.ReplyTo.ID
	}
	storedMessages := chain.Read(msgID)

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
