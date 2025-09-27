package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kapitanov/gptbot/internal/storage"
)

func TestAccessProvider(t *testing.T) {
	t.Run("should allow access by ID", func(t *testing.T) {
		provider := NewAccessProvider("123456,@testuser,789")

		if !provider.CheckAccess(123456, "anyuser") {
			t.Error("Should allow access by ID")
		}
	})

	t.Run("should allow access by username", func(t *testing.T) {
		provider := NewAccessProvider("123456,@testuser,789")

		if !provider.CheckAccess(999, "testuser") {
			t.Error("Should allow access by username")
		}
	})

	t.Run("should deny access for wrong user", func(t *testing.T) {
		provider := NewAccessProvider("123456,@testuser,789")

		if provider.CheckAccess(999, "wronguser") {
			t.Error("Should deny access for wrong user")
		}
	})

	t.Run("should handle empty access string", func(t *testing.T) {
		provider := NewAccessProvider("")

		if provider.CheckAccess(123, "test") {
			t.Error("Should deny access when no access configured")
		}
	})
}

func TestStorage(t *testing.T) {
	testFile := filepath.Join(os.TempDir(), "test-storage-"+strconv.FormatInt(time.Now().UnixNano(), 10)+".yaml")
	defer os.Remove(testFile)

	t.Run("should create and initialize storage", func(t *testing.T) {
		storage, err := storage.New(testFile)
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}

		err = storage.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize storage: %v", err)
		}

		// File should be created
		if _, err := os.Stat(testFile); os.IsNotExist(err) {
			t.Error("Storage file should be created")
		}
	})

	t.Run("should store and retrieve messages", func(t *testing.T) {
		store, err := storage.New(testFile)
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}

		err = store.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize storage: %v", err)
		}

		userID := int64(12345)
		msgID := 1
		text := "Hello, world!"

		err = store.TX(userID, func(chain *storage.MessageChain) error {
			return chain.Store(msgID, nil, storage.MessageSideUser, text)
		})
		if err != nil {
			t.Fatalf("Failed to store message: %v", err)
		}

		err = store.TX(userID, func(chain *storage.MessageChain) error {
			messages := chain.Read(msgID)
			if len(messages) != 1 {
				t.Errorf("Expected 1 message, got %d", len(messages))
				return nil
			}
			if messages[0].Text != text {
				t.Errorf("Expected text %q, got %q", text, messages[0].Text)
			}
			if messages[0].Side != storage.MessageSideUser {
				t.Errorf("Expected side %v, got %v", storage.MessageSideUser, messages[0].Side)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("Failed to read messages: %v", err)
		}
	})

	t.Run("should handle conversation chains", func(t *testing.T) {
		store, err := storage.New(testFile)
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}

		err = store.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize storage: %v", err)
		}

		userID := int64(12345)
		msg1ID := 10
		msg2ID := 20
		msg3ID := 30

		// Store a chain of messages
		err = store.TX(userID, func(chain *storage.MessageChain) error {
			if err := chain.Store(msg1ID, nil, storage.MessageSideUser, "First message"); err != nil {
				return err
			}
			if err := chain.Store(msg2ID, &msg1ID, storage.MessageSideBot, "Bot response"); err != nil {
				return err
			}
			if err := chain.Store(msg3ID, &msg2ID, storage.MessageSideUser, "User reply"); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatalf("Failed to store conversation chain: %v", err)
		}

		// Read the chain from the latest message
		err = store.TX(userID, func(chain *storage.MessageChain) error {
			messages := chain.Read(msg3ID)
			if len(messages) != 3 {
				t.Errorf("Expected 3 messages in chain, got %d", len(messages))
				return nil
			}

			// Should be in chronological order
			if messages[0].Text != "First message" {
				t.Errorf("Expected first message to be 'First message', got %q", messages[0].Text)
			}
			if messages[1].Text != "Bot response" {
				t.Errorf("Expected second message to be 'Bot response', got %q", messages[1].Text)
			}
			if messages[2].Text != "User reply" {
				t.Errorf("Expected third message to be 'User reply', got %q", messages[2].Text)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("Failed to read conversation chain: %v", err)
		}
	})
}
