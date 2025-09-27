package telegram

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"

	"github.com/kapitanov/gptbot/internal/gpt"
	"github.com/kapitanov/gptbot/internal/storage"
	"github.com/kapitanov/gptbot/internal/telegram/mdparser"
	"github.com/kapitanov/gptbot/internal/telegram/texts"
)

const MaxTextLength = 4095 // Telegram limit minus 1

// AccessChecker interface for checking access
type AccessChecker interface {
	CheckAccess(id int64, username string) bool
}

// Config represents telegram bot configuration
type Config struct {
	Token         string
	AccessChecker AccessChecker
	GPT           *gpt.GPT
	Storage       *storage.Storage
	Logger        *logrus.Logger
}

// Telegram represents the telegram bot
type Telegram struct {
	bot           *tgbotapi.BotAPI
	storage       *storage.Storage
	gpt           *gpt.GPT
	accessChecker AccessChecker
	logger        *logrus.Logger
	botInfo       *tgbotapi.User
}

// New creates a new telegram bot instance
func New(config *Config) (*Telegram, error) {
	bot, err := tgbotapi.NewBotAPI(config.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API: %w", err)
	}

	return &Telegram{
		bot:           bot,
		storage:       config.Storage,
		gpt:           config.GPT,
		accessChecker: config.AccessChecker,
		logger:        config.Logger,
	}, nil
}

// Run starts the bot
func (t *Telegram) Run() error {
	// Get bot info
	botInfo, err := t.bot.GetMe()
	if err != nil {
		return fmt.Errorf("failed to get bot info: %w", err)
	}
	t.botInfo = &botInfo

	t.logger.WithFields(logrus.Fields{
		"id":       t.botInfo.ID,
		"username": t.botInfo.UserName,
	}).Info("Connected to Telegram")

	t.setupHandlers()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := t.bot.GetUpdatesChan(u)

	for update := range updates {
		go t.handleUpdate(update)
	}

	return nil
}

// Close stops the bot
func (t *Telegram) Close() {
	if t.bot != nil {
		t.bot.StopReceivingUpdates()
		t.logger.Info("Telegram bot stopped")
	}
}

func (t *Telegram) setupHandlers() {
	// Handlers are set up in handleUpdate method
}

func (t *Telegram) handleUpdate(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message

	// Handle /start command
	if msg.IsCommand() && msg.Command() == "start" {
		t.onStartCommand(msg)
		return
	}

	// Handle text messages
	if msg.Text != "" {
		t.onText(msg)
		return
	}

	// Handle media messages
	if msg.Photo != nil {
		t.onPhoto(msg)
		return
	}

	if msg.Video != nil {
		t.onVideo(msg)
		return
	}

	if msg.Audio != nil {
		t.onAudio(msg)
		return
	}

	if msg.Animation != nil {
		t.onAnimation(msg)
		return
	}

	if msg.Document != nil {
		t.onDocument(msg)
		return
	}

	if msg.Voice != nil {
		t.onVoice(msg)
		return
	}
}

func (t *Telegram) onStartCommand(msg *tgbotapi.Message) {
	if !t.hasAccess(msg) {
		return
	}

	response := tgbotapi.NewMessage(msg.Chat.ID, texts.Welcome)
	if _, err := t.bot.Send(response); err != nil {
		t.logger.WithError(err).WithFields(logrus.Fields{
			"username":   msg.From.UserName,
			"message_id": msg.MessageID,
		}).Error("Failed to send welcome message")
	}
}

func (t *Telegram) onText(msg *tgbotapi.Message) {
	t.generate(msg, msg.Text, "")
}

func (t *Telegram) onPhoto(msg *tgbotapi.Message) {
	caption := ""
	if msg.Caption != "" {
		caption = msg.Caption
	}
	t.generate(msg, caption, caption)
}

func (t *Telegram) onVideo(msg *tgbotapi.Message) {
	caption := ""
	if msg.Caption != "" {
		caption = msg.Caption
	}
	t.generate(msg, caption, caption)
}

func (t *Telegram) onAudio(msg *tgbotapi.Message) {
	caption := ""
	if msg.Caption != "" {
		caption = msg.Caption
	}
	t.generate(msg, caption, caption)
}

func (t *Telegram) onAnimation(msg *tgbotapi.Message) {
	caption := ""
	if msg.Caption != "" {
		caption = msg.Caption
	}
	t.generate(msg, caption, caption)
}

func (t *Telegram) onDocument(msg *tgbotapi.Message) {
	caption := ""
	if msg.Caption != "" {
		caption = msg.Caption
	}
	t.generate(msg, caption, caption)
}

func (t *Telegram) onVoice(msg *tgbotapi.Message) {
	caption := ""
	if msg.Caption != "" {
		caption = msg.Caption
	}
	t.generate(msg, caption, caption)
}

func (t *Telegram) hasAccess(msg *tgbotapi.Message) bool {
	if msg.From.ID == int64(t.botInfo.ID) {
		return true
	}

	if t.accessChecker.CheckAccess(msg.From.ID, msg.From.UserName) {
		return true
	}

	t.logger.WithFields(logrus.Fields{
		"username": msg.From.UserName,
		"user_id":  msg.From.ID,
	}).Error("Access denied")

	response := tgbotapi.NewMessage(msg.Chat.ID, texts.AccessDenied)
	response.ReplyToMessageID = msg.MessageID
	if _, err := t.bot.Send(response); err != nil {
		t.logger.WithError(err).WithFields(logrus.Fields{
			"username":   msg.From.UserName,
			"message_id": msg.MessageID,
		}).Error("Failed to send access denied message")
	}

	return false
}

func (t *Telegram) generate(msg *tgbotapi.Message, text, altText string) {
	if !t.hasAccess(msg) {
		return
	}

	if text == "" {
		text = altText
	}

	if text == "" {
		if msg.MediaGroupID != "" {
			return // Skip album messages without text
		}

		t.logger.WithFields(logrus.Fields{
			"username":   msg.From.UserName,
			"message_id": msg.MessageID,
		}).Warn("Empty text")

		response := tgbotapi.NewMessage(msg.Chat.ID, texts.MissingText)
		response.ReplyToMessageID = msg.MessageID
		if _, err := t.bot.Send(response); err != nil {
			t.logger.WithError(err).WithFields(logrus.Fields{
				"username":   msg.From.UserName,
				"message_id": msg.MessageID,
			}).Error("Failed to send missing text message")
		}
		return
	}

	err := t.storage.TX(msg.From.ID, func(chain *storage.MessageChain) error {
		return t.generateE(msg, text, chain)
	})

	if err != nil {
		t.logger.WithError(err).WithFields(logrus.Fields{
			"username":   msg.From.UserName,
			"message_id": msg.MessageID,
			"text":       text,
		}).Error("Failed to process message")

		response := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("%s\n%s", texts.Failure, err.Error()))
		response.ReplyToMessageID = msg.MessageID
		if _, err := t.bot.Send(response); err != nil {
			t.logger.WithError(err).WithFields(logrus.Fields{
				"username":   msg.From.UserName,
				"message_id": msg.MessageID,
			}).Error("Failed to send error message")
		}
	}
}

func (t *Telegram) generateE(msg *tgbotapi.Message, request string, chain *storage.MessageChain) error {
	gptMessages, err := t.generateGPTMessages(msg, request, chain)
	if err != nil {
		return err
	}

	// Send "thinking" message
	thinkingMsg := tgbotapi.NewMessage(msg.Chat.ID, texts.Thinking)
	thinkingMsg.ReplyToMessageID = msg.MessageID
	thinkingResponse, err := t.bot.Send(thinkingMsg)
	if err != nil {
		return fmt.Errorf("failed to send thinking message: %w", err)
	}

	// Send typing indicator
	typingAction := tgbotapi.NewChatAction(msg.Chat.ID, tgbotapi.ChatTyping)
	t.bot.Send(typingAction) // Ignore errors for typing indicator

	// Generate response
	response, err := t.gpt.Generate(gptMessages)
	if err != nil {
		return fmt.Errorf("failed to generate response: %w", err)
	}

	// Send the actual reply
	replyMsg, err := t.reply(msg, &thinkingResponse, response)
	if err != nil {
		return fmt.Errorf("failed to send reply: %w", err)
	}

	// Store messages in conversation chain
	var replyToID *int
	if msg.ReplyToMessage != nil {
		id := msg.ReplyToMessage.MessageID
		replyToID = &id
	}

	if err := chain.Store(msg.MessageID, replyToID, storage.MessageSideUser, request); err != nil {
		return fmt.Errorf("failed to store user message: %w", err)
	}

	if err := chain.Store(replyMsg.MessageID, &msg.MessageID, storage.MessageSideBot, response); err != nil {
		return fmt.Errorf("failed to store bot message: %w", err)
	}

	t.logger.WithFields(logrus.Fields{
		"username":   msg.From.UserName,
		"message_id": msg.MessageID,
		"request":    request,
		"response":   response,
	}).Info("Generated reply")

	return nil
}

func (t *Telegram) reply(msg *tgbotapi.Message, thinkingMsg *tgbotapi.Message, response string) (*tgbotapi.Message, error) {
	parsedResponse, entities := mdparser.Parse(response)

	// Delete the "thinking" message
	deleteMsg := tgbotapi.NewDeleteMessage(msg.Chat.ID, thinkingMsg.MessageID)
	t.bot.Send(deleteMsg) // Ignore errors when deleting thinking message

	if len([]rune(parsedResponse)) <= MaxTextLength {
		reply := tgbotapi.NewMessage(msg.Chat.ID, parsedResponse)
		reply.ReplyToMessageID = msg.MessageID
		reply.Entities = entities

		sentMsg, err := t.bot.Send(reply)
		if err != nil {
			return nil, err
		}
		return &sentMsg, nil
	} else {
		// Split long messages
		remainingText := []rune(parsedResponse)
		var lastReply *tgbotapi.Message

		for len(remainingText) > 0 {
			var text string
			if len(remainingText) <= MaxTextLength {
				text = string(remainingText)
				remainingText = nil
			} else {
				text = string(remainingText[:MaxTextLength])
				remainingText = remainingText[MaxTextLength:]
			}

			reply := tgbotapi.NewMessage(msg.Chat.ID, text)
			reply.ReplyToMessageID = msg.MessageID

			sentMsg, err := t.bot.Send(reply)
			if err != nil {
				return nil, err
			}
			lastReply = &sentMsg
		}

		return lastReply, nil
	}
}

func (t *Telegram) generateGPTMessages(msg *tgbotapi.Message, text string, chain *storage.MessageChain) ([]gpt.Message, error) {
	text = t.normalizeText(text)
	if text == "" {
		return nil, fmt.Errorf("text is empty")
	}

	msgID := 0
	if msg.ReplyToMessage != nil {
		msgID = msg.ReplyToMessage.MessageID
	}

	storedMessages := chain.Read(msgID)

	var gptMessages []gpt.Message
	for _, storedMessage := range storedMessages {
		participant := gpt.ParticipantUser
		if storedMessage.Side == storage.MessageSideBot {
			participant = gpt.ParticipantBot
		}

		gptMessages = append(gptMessages, gpt.Message{
			Participant: participant,
			Text:        storedMessage.Text,
		})
	}

	gptMessages = append(gptMessages, gpt.Message{
		Participant: gpt.ParticipantUser,
		Text:        text,
	})

	return gptMessages, nil
}

func (t *Telegram) normalizeText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	if !strings.HasSuffix(text, ".") {
		text = text + "."
	}

	return text
}
