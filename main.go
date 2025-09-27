package main

import (
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"

	"github.com/kapitanov/gptbot/internal/gpt"
	"github.com/kapitanov/gptbot/internal/storage"
	"github.com/kapitanov/gptbot/internal/telegram"
)

// AccessProvider checks access to telegram chats
type AccessProvider struct {
	ids       map[int64]bool
	usernames map[string]bool
}

// NewAccessProvider creates a new access provider from access string
func NewAccessProvider(accessString string) *AccessProvider {
	ap := &AccessProvider{
		ids:       make(map[int64]bool),
		usernames: make(map[string]bool),
	}

	if accessString == "" {
		return ap
	}

	// Split by commas, semicolons, or spaces
	entries := strings.FieldsFunc(accessString, func(c rune) bool {
		return c == ',' || c == ';' || c == ' '
	})

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Try to parse as number (user ID)
		if id, err := strconv.ParseInt(entry, 10, 64); err == nil {
			ap.ids[id] = true
		} else {
			// Remove @ prefix if present and add as username
			username := strings.TrimPrefix(entry, "@")
			ap.usernames[username] = true
		}
	}

	return ap
}

// CheckAccess checks access to telegram chat and returns true if access is granted
func (ap *AccessProvider) CheckAccess(id int64, username string) bool {
	if ap.ids[id] {
		return true
	}

	if ap.usernames[username] {
		return true
	}

	return false
}

func main() {
	// Load .env file if present
	godotenv.Load()

	// Configure logger
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Initialize storage
	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./var/data.yaml"
	}

	store, err := storage.New(storagePath)
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize storage")
	}

	if err := store.Initialize(); err != nil {
		logger.WithError(err).Fatal("Failed to initialize storage")
	}

	// Initialize GPT
	openaiToken := os.Getenv("OPENAI_TOKEN")
	if openaiToken == "" {
		logger.Fatal("OPENAI_TOKEN environment variable is required")
	}

	gptClient, err := gpt.New(openaiToken)
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize GPT client")
	}

	if err := gptClient.Initialize(); err != nil {
		logger.WithError(err).Fatal("Failed to initialize GPT client")
	}

	// Create access provider
	accessString := os.Getenv("TELEGRAM_BOT_ACCESS")
	accessProvider := NewAccessProvider(accessString)

	// Initialize Telegram bot
	telegramToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if telegramToken == "" {
		logger.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	bot, err := telegram.New(&telegram.Config{
		Token:         telegramToken,
		AccessChecker: accessProvider,
		GPT:           gptClient,
		Storage:       store,
		Logger:        logger,
	})
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize Telegram bot")
	}

	// Handle graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		logger.Info("Shutting down...")
		bot.Close()
		logger.Info("Good bye")
		os.Exit(0)
	}()

	// Start the bot
	logger.Info("Press Ctrl+C to exit")
	if err := bot.Run(); err != nil {
		logger.WithError(err).Fatal("Failed to start bot")
	}
}
