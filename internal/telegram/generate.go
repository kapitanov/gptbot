package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/kapitanov/gptbot/internal/gpt"
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

	stopThinking := tg.withThinkingMsg(msg)
	response, err := tg.gpt.Generate(context.Background(), gpt.Request{Message: request, PrevResponseID: lastResponseID})
	stopThinking()

	if err != nil {
		return err
	}

	_, err = sendRichMessage(tg.bot, msg.Sender, inputRichMessage{
		Markdown: response.Text,
	})
	if err != nil {
		log.Error().Err(err).
			Str("username", msg.Sender.Username).
			Int("msg", msg.ID).
			Msg("failed to reply")
		return err
	}

	err = tg.storage.SetLastResponseID(msg.Sender.ID, response.ID)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to set last response id")
		return err
	}

	log.Info().
		Str("username", msg.Sender.Username).
		Int("msg", msg.ID).
		Str("last", lastResponseID).
		Str("request", request).
		Str("response", response.Text).
		Int64("tokens", response.Usage.TotalTokens).
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

func (tg *Telegram) withThinkingMsg(msg *telebot.Message) func() {
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer close(done)

		const draftID = 1
		thinking := inputRichMessage{
			HTML: fmt.Sprintf(`<tg-thinking>%s</tg-thinking>`, html.EscapeString(texts.Thinking)),
		}
		err := sendRichMessageDraft(tg.bot, msg.Sender, draftID, thinking)
		if err != nil {
			log.Error().Err(err).
				Str("username", msg.Sender.Username).
				Int("msg", msg.ID).
				Msg("failed to send draft")
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
				err := sendRichMessageDraft(tg.bot, msg.Sender, draftID, thinking)
				if err != nil {
					log.Error().Err(err).
						Str("username", msg.Sender.Username).
						Int("msg", msg.ID).
						Msg("failed to send draft")
				}
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

type inputRichMessage struct {
	HTML                string `json:"html,omitempty"`
	Markdown            string `json:"markdown,omitempty"`
	IsRTL               bool   `json:"is_rtl,omitempty"`
	SkipEntityDetection bool   `json:"skip_entity_detection,omitempty"`
}

// sendRichMessage implements https://core.telegram.org/bots/api#sendrichmessage.
func sendRichMessage(b *telebot.Bot, to telebot.Recipient, msg inputRichMessage) (*telebot.Message, error) {
	// see https://core.telegram.org/bots/api#sendrichmessage
	params := map[string]interface{}{
		"chat_id":      to.Recipient(),
		"rich_message": msg,
	}

	data, err := b.Raw("sendRichMessage", params)
	if err != nil {
		return nil, err
	}

	return extractMessage(data)
}

// sendRichMessageDraft implements https://core.telegram.org/bots/api#sendrichmessagedraft.
func sendRichMessageDraft(b *telebot.Bot, to telebot.Recipient, draftID int, msg inputRichMessage) error {
	if to == nil {
		return telebot.ErrBadRecipient
	}

	params := map[string]interface{}{
		"chat_id":      to.Recipient(),
		"draft_id":     draftID,
		"rich_message": msg,
	}

	_, err := b.Raw("sendRichMessageDraft", params)
	return err
}

// extractMessage extracts common Message result from given data.
// Should be called after extractOk or b.Raw() to handle possible errors.
func extractMessage(data []byte) (*telebot.Message, error) {
	var resp struct {
		Result *telebot.Message
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		var resp struct {
			Result bool
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, err
		}
		if resp.Result {
			return nil, telebot.ErrTrueResult
		}
		return nil, err
	}
	return resp.Result, nil
}
