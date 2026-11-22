package telegram

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/telebot.v4"

	"github.com/kapitanov/gptbot/internal/llmprovider"
	"github.com/kapitanov/gptbot/internal/telegram/texts"
)

func (tg *Telegram) setupHandlers() {
	tg.bot.Handle("/start", tg.onStartCommand)
	tg.bot.Handle("/reset", tg.onResetCommand)
	tg.bot.Handle("/clear", tg.onResetCommand)
	tg.bot.Handle("/model", tg.onModelCommand)
	tg.bot.Handle(&telebot.Btn{Unique: modelCallbackUnique}, tg.onModelCallback)
	tg.bot.Handle(telebot.OnText, tg.onText)
	tg.bot.Handle(telebot.OnPhoto, tg.onPhoto)
	tg.bot.Handle(telebot.OnVideo, tg.onVideo)
	tg.bot.Handle(telebot.OnAudio, tg.onAudio)
	tg.bot.Handle(telebot.OnAnimation, tg.onAnimation)
	tg.bot.Handle(telebot.OnDocument, tg.onDocument)
	tg.bot.Handle(telebot.OnVoice, tg.onVoice)
}

func (tg *Telegram) onStartCommand(ctx telebot.Context) error {
	msg := ctx.Message()

	if !tg.hasAccess(msg) {
		return nil
	}

	_, err := tg.bot.Send(msg.Sender, texts.Welcome)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send welcome message")
		return err
	}

	return nil
}

func (tg *Telegram) onResetCommand(ctx telebot.Context) error {
	msg := ctx.Message()

	if !tg.hasAccess(msg) {
		return nil
	}

	conversationID := getConversationID(msg)

	err := tg.storage.Clear(conversationID)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Str("conversation", conversationID).Msg("failed to reset conversation")
		return err
	}

	log.Info().Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("conversation reset")

	_, err = tg.bot.Send(msg.Sender, texts.Reset)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send reset message")
		return err
	}

	return nil
}

func (tg *Telegram) onText(ctx telebot.Context) error {
	msg := ctx.Message()

	return tg.generate(msg, msg.Text, "")
}

func (tg *Telegram) onModelCommand(ctx telebot.Context) error {
	msg := ctx.Message()

	if !tg.hasAccess(msg) {
		return nil
	}

	name := strings.TrimSpace(msg.Payload)
	if name == "" {
		return tg.sendModelsList(ctx)
	}

	model, ok := llmprovider.FindModel(tg.llm.Models(), name)
	if !ok {
		_, err := tg.bot.Reply(msg, fmt.Sprintf(texts.UnknownModel, name))
		if err != nil {
			log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send unknown model message")
		}
		return err
	}

	return tg.selectModel(ctx, model)
}

func (tg *Telegram) onModelCallback(ctx telebot.Context) error {
	cb := ctx.Callback()
	if cb == nil || cb.Sender == nil || cb.Message == nil {
		return nil
	}

	// Present the callback message as a regular message so that access checks
	// and the conversation ID work the same way as for text messages.
	msg := cb.Message
	msg.Sender = cb.Sender

	if !tg.hasAccess(msg) {
		_ = ctx.Respond()
		return nil
	}

	model, ok := llmprovider.FindModel(tg.llm.Models(), cb.Data)
	if !ok {
		if err := ctx.RespondAlert(fmt.Sprintf(texts.UnknownModel, cb.Data)); err != nil {
			log.Error().Err(err).Str("username", cb.Sender.Username).Msg("failed to respond to callback")
			return err
		}
		return nil
	}

	return tg.selectModel(ctx, model)
}

// sendModelsList replies with the list of available models as an inline keyboard.
func (tg *Telegram) sendModelsList(ctx telebot.Context) error {
	msg := ctx.Message()
	conversationID := getConversationID(msg)

	current, err := tg.modelForConversation(conversationID)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Str("conversation", conversationID).Msg("failed to load conversation model")
		return err
	}

	_, err = tg.bot.Reply(msg, fmt.Sprintf(texts.ModelsTitle, current), tg.modelKeyboard(current))
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send models list")
	}
	return err
}

// selectModel pins the model to the conversation and reports the result.
func (tg *Telegram) selectModel(ctx telebot.Context, model string) error {
	msg := ctx.Message()
	conversationID := getConversationID(msg)

	previous, err := tg.modelForConversation(conversationID)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Str("conversation", conversationID).Msg("failed to load conversation model")
		return err
	}

	err = tg.storage.SetModel(conversationID, model)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Str("conversation", conversationID).Msg("failed to set conversation model")
		return err
	}

	log.Info().Str("username", msg.Sender.Username).Str("conversation", conversationID).Str("model", model).Msg("conversation model selected")

	selected := fmt.Sprintf(texts.ModelSelected, model)

	if ctx.Callback() == nil {
		_, err = tg.bot.Reply(msg, selected)
		if err != nil {
			log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send model selected message")
		}
		return err
	}

	// Answer the button press with a toast.
	if err = ctx.RespondText(selected); err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Msg("failed to respond to callback")
	}

	// Refresh the list in place, keeping the keyboard for further switching.
	// Skip the edit when nothing changed to avoid "message is not modified".
	if previous == model {
		return nil
	}

	err = ctx.Edit(fmt.Sprintf(texts.ModelsTitle, model), tg.modelKeyboard(model))
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Msg("failed to edit models list")
	}
	return err
}

func (tg *Telegram) onPhoto(ctx telebot.Context) error {
	msg := ctx.Message()

	return tg.generate(msg, msg.Photo.Caption, msg.Caption)
}

func (tg *Telegram) onVideo(ctx telebot.Context) error {
	msg := ctx.Message()

	return tg.generate(msg, msg.Video.Caption, msg.Caption)
}

func (tg *Telegram) onAudio(ctx telebot.Context) error {
	msg := ctx.Message()

	return tg.generate(msg, msg.Audio.Caption, msg.Caption)
}

func (tg *Telegram) onAnimation(ctx telebot.Context) error {
	msg := ctx.Message()

	return tg.generate(msg, msg.Animation.Caption, msg.Caption)
}

func (tg *Telegram) onDocument(ctx telebot.Context) error {
	msg := ctx.Message()

	return tg.generate(msg, msg.Document.Caption, msg.Caption)
}

func (tg *Telegram) onVoice(ctx telebot.Context) error {
	msg := ctx.Message()

	return tg.generate(msg, msg.Voice.Caption, msg.Caption)
}
