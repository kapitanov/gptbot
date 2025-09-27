package storage

import (
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// MessageSide is a side of conversation
type MessageSide string

const (
	MessageSideBot  MessageSide = "bot"
	MessageSideUser MessageSide = "user"
)

// StoredMessage represents a stored message
type StoredMessage struct {
	Side    MessageSide `yaml:"side"`
	Text    string      `yaml:"text"`
	ReplyTo *int        `yaml:"reply_to,omitempty"`
}

// Conversation represents a conversation
type Conversation struct {
	Messages map[int]*StoredMessage `yaml:"messages"`
}

// Root represents the root storage structure
type Root struct {
	Conversations map[int64]*Conversation `yaml:"conversations"`
}

// Storage stores conversation data
type Storage struct {
	filename string
	mutex    sync.Mutex
}

// New creates a new storage instance
func New(filename string) (*Storage, error) {
	if filename == "" {
		filename = "./var/data.yaml"
	}

	return &Storage{
		filename: filename,
	}, nil
}

// Initialize initializes the storage
func (s *Storage) Initialize() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, err := s.load()
	return err
}

// TX runs a function within a transaction
func (s *Storage) TX(userID int64, fn func(*MessageChain) error) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	root, err := s.load()
	if err != nil {
		return err
	}

	conversation := root.Conversations[userID]
	if conversation == nil {
		conversation = &Conversation{
			Messages: make(map[int]*StoredMessage),
		}
		root.Conversations[userID] = conversation
	}

	if conversation.Messages == nil {
		conversation.Messages = make(map[int]*StoredMessage)
	}

	chain := &MessageChain{
		conversation: conversation,
		save: func() error {
			return s.store(root)
		},
	}

	return fn(chain)
}

func (s *Storage) load() (*Root, error) {
	data, err := os.ReadFile(s.filename)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, create it
			root := &Root{
				Conversations: make(map[int64]*Conversation),
			}
			if err := s.store(root); err != nil {
				return nil, err
			}
			return root, nil
		}
		return nil, err
	}

	root := &Root{}
	if err := yaml.Unmarshal(data, root); err != nil {
		return nil, err
	}

	if root.Conversations == nil {
		root.Conversations = make(map[int64]*Conversation)
	}

	return root, nil
}

func (s *Storage) store(root *Root) error {
	data, err := yaml.Marshal(root)
	if err != nil {
		return err
	}

	return os.WriteFile(s.filename, data, 0644)
}

// MessageChain is a single conversation
type MessageChain struct {
	conversation *Conversation
	save         func() error
}

// Store writes new message into the conversation
func (mc *MessageChain) Store(msgID int, replyToID *int, side MessageSide, text string) error {
	msg := &StoredMessage{
		Side: side,
		Text: text,
	}

	if replyToID != nil {
		msg.ReplyTo = replyToID
	}

	mc.conversation.Messages[msgID] = msg
	return mc.save()
}

// Message represents a message from the conversation
type Message struct {
	Side MessageSide
	Text string
}

// Read reads all messages from the conversation
func (mc *MessageChain) Read(messageID int) []Message {
	var messages []Message
	currentID := messageID

	for {
		msg := mc.conversation.Messages[currentID]
		if msg == nil {
			break
		}

		messages = append(messages, Message{
			Side: msg.Side,
			Text: msg.Text,
		})

		if msg.ReplyTo == nil {
			break
		}

		currentID = *msg.ReplyTo
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages
}
