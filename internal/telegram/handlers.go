package telegram

import (
	"github.com/rs/zerolog/log"
	"gopkg.in/tucnak/telebot.v2"

	"github.com/kapitanov/gptbot/internal/telegram/texts"
)

func (tg *Telegram) setupHandlers() {
	tg.bot.Handle("/start", tg.onStartCommand)
	tg.bot.Handle(telebot.OnText, tg.onText)
	tg.bot.Handle(telebot.OnPhoto, tg.onPhoto)
	tg.bot.Handle(telebot.OnVideo, tg.onVideo)
	tg.bot.Handle(telebot.OnAudio, tg.onAudio)
	tg.bot.Handle(telebot.OnAnimation, tg.onAnimation)
	tg.bot.Handle(telebot.OnDocument, tg.onDocument)
	tg.bot.Handle(telebot.OnVoice, tg.onVoice)
}

func (tg *Telegram) onStartCommand(msg *telebot.Message) {
	if !tg.hasAccess(msg) {
		return
	}

	_, err := tg.bot.Send(msg.Sender, texts.Welcome)
	if err != nil {
		log.Error().Err(err).Str("username", msg.Sender.Username).Int("msg", msg.ID).Msg("failed to send welcome message")
	}
}

func (tg *Telegram) onText(msg *telebot.Message) {
	tg.generate(msg, msg.Text, "")
}

func (tg *Telegram) onPhoto(msg *telebot.Message) {
	tg.generate(msg, msg.Photo.Caption, msg.Caption)
}

func (tg *Telegram) onVideo(msg *telebot.Message) {
	tg.generate(msg, msg.Video.Caption, msg.Caption)
}

func (tg *Telegram) onAudio(msg *telebot.Message) {
	tg.generate(msg, msg.Audio.Caption, msg.Caption)
}

func (tg *Telegram) onAnimation(msg *telebot.Message) {
	tg.generate(msg, msg.Animation.Caption, msg.Caption)
}

func (tg *Telegram) onDocument(msg *telebot.Message) {
	tg.generate(msg, msg.Document.Caption, msg.Caption)
}

func (tg *Telegram) onVoice(msg *telebot.Message) {
	tg.generate(msg, msg.Voice.Caption, msg.Caption)
}
