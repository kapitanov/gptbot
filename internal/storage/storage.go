package storage

import (
	"os"
	"slices"
	"sync"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Storage stores conversation data.
type Storage struct {
	filename string
	mutex    sync.Mutex
}

// New creates new storage.
func New(filename string) (*Storage, error) {
	s := &Storage{
		filename: filename,
	}

	err := s.do(func(_ *RootYAML, _ func() error) error {
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s, nil
}

// TX runs a function within a transaction.
func (s *Storage) TX(userID int64, fn func(chain *MessageChain) error) error {
	return s.do(func(root *RootYAML, save func() error) error {
		conversation, exists := root.Conversations[userID]
		if !exists {
			conversation = &ConversationYAML{}
			root.Conversations[userID] = conversation
		}

		if conversation.Messages == nil {
			conversation.Messages = make(map[int]*MessageYAML)
		}

		return fn(&MessageChain{
			conversation: conversation,
			save:         save,
		})
	})
}

func (s *Storage) do(fn func(root *RootYAML, save func() error) error) error {
	// A global lock is a terrible idea, but for this pet project it should be OK.00
	s.mutex.Lock()
	defer s.mutex.Unlock()

	root, err := s.load()
	if err != nil {
		return err
	}

	return fn(root, func() error {
		return s.store(root)
	})
}

func (s *Storage) load() (*RootYAML, error) {
	data, err := os.ReadFile(s.filename)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}

		f, err := os.Create(s.filename)
		if err != nil {
			return nil, err
		}

		err = f.Close()
		if err != nil {
			return nil, err
		}

		data = []byte("")
	}

	var root RootYAML
	err = yaml.Unmarshal(data, &root)
	if err != nil {
		return nil, err
	}

	if root.Conversations == nil {
		root.Conversations = make(map[int64]*ConversationYAML)
	}

	return &root, nil
}

func (s *Storage) store(root *RootYAML) error {
	data, err := yaml.Marshal(root)
	if err != nil {
		return err
	}

	err = os.WriteFile(s.filename, data, 0x777)
	if err != nil {
		return err
	}

	return nil
}

// MessageChain is a single conversation.
type MessageChain struct {
	conversation *ConversationYAML
	save         func() error
}

// Store writes new message into the conversation.
func (c *MessageChain) Store(msgID int, replyToID *int, side MessageSide, text string) error {
	msg := MessageYAML{
		ReplyTo: replyToID,
		Side:    side,
		Text:    text,
	}
	c.conversation.Messages[msgID] = &msg
	return c.save()
}

// Store reads all messages from the conversation.
func (c *MessageChain) Read(messageID int) []Message {
	var messages []Message

	for {
		msg, ok := c.conversation.Messages[messageID]
		if !ok {
			break
		}
		messages = append(messages, Message{
			Side: msg.Side,
			Text: msg.Text,
		})
		if msg.ReplyTo == nil {
			break
		}

		messageID = *msg.ReplyTo
	}

	slices.Reverse(messages)
	return messages
}

// Message is a message in a conversation.
type Message struct {
	Side MessageSide // Conversation side.
	Text string      // Message text.
}

// MessageSide is a side of conversation.
type MessageSide string

const (
	Bot  MessageSide = "bot"  // Bot.
	User MessageSide = "user" // User.
)

// RootYAML is a YAML model for data root.
type RootYAML struct {
	Conversations map[int64]*ConversationYAML `yaml:"conversations"` // Conversations.
}

// ConversationYAML is a YAML model for conversation.
type ConversationYAML struct {
	Messages map[int]*MessageYAML `yaml:"messages"` // Messages.
}

// MessageYAML is a YAML model for a message.
type MessageYAML struct {
	ReplyTo *int        `yaml:"reply_to,omitempty"` // ID of message (if this one is a reply).
	Side    MessageSide `yaml:"side"`               // Conversation side.
	Text    string      `yaml:"text"`               // Message text.
}
