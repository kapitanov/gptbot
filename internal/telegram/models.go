package telegram

import (
	"gopkg.in/telebot.v4"

	"github.com/kapitanov/gptbot/internal/llmprovider"
)

// modelCallbackUnique is the callback endpoint unique name of model selection buttons.
const modelCallbackUnique = "gptbot-model"

// modelForConversation returns the model pinned to the conversation,
// or the provider's default model when nothing is pinned or the pinned model
// is no longer in the configured list.
func (tg *Telegram) modelForConversation(conversationID string) (string, error) {
	model, err := tg.storage.Model(conversationID)
	if err != nil {
		return "", err
	}

	if model == "" {
		return tg.llm.DefaultModel(), nil
	}

	if resolved, ok := llmprovider.FindModel(tg.llm.Models(), model); ok {
		return resolved, nil
	}

	return tg.llm.DefaultModel(), nil
}

// modelKeyboard builds an inline keyboard with a row of buttons per available model.
// The current model is marked with a checkmark.
func (tg *Telegram) modelKeyboard(current string) *telebot.ReplyMarkup {
	var rows []telebot.Row
	for _, model := range tg.llm.Models() {
		text := model
		if model == current {
			text = "✅ " + model
		}
		rows = append(rows, telebot.Row{{Unique: modelCallbackUnique, Text: text, Data: model}})
	}

	markup := &telebot.ReplyMarkup{}
	markup.Inline(rows...)

	return markup
}
