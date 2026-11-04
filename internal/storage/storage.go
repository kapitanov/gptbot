package storage

import (
	"os"
	"sync"

	"github.com/kapitanov/gptbot/internal/llmprovider"
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

// Retrieve fetches the conversation history for the given conversation ID.
func (s *Storage) Retrieve(conversationID string) ([]llmprovider.Message, error) {
	var messages []llmprovider.Message
	err := s.do(func(root *RootYAML, save func() error) error {
		conversation, exists := root.Conversations[conversationID]
		if !exists {
			return nil
		}

		messages = make([]llmprovider.Message, 0, len(conversation.Messages))
		for _, m := range conversation.Messages {
			messages = append(messages, llmprovider.Message{Text: m.Text, Participant: m.Participant})
		}
		return nil
	})
	return messages, err
}

// Append adds messages to the conversation history for the given conversation ID.
func (s *Storage) Append(conversationID string, messages ...llmprovider.Message) error {
	return s.do(func(root *RootYAML, save func() error) error {
		conversation, exists := root.Conversations[conversationID]
		if !exists {
			conversation = &ConversationYAML{}
			root.Conversations[conversationID] = conversation
		}

		for _, m := range messages {
			conversation.Messages = append(conversation.Messages, MessageYAML{Text: m.Text, Participant: m.Participant})
		}

		return save()
	})
}

// Clear removes the conversation history for the given conversation ID.
func (s *Storage) Clear(conversationID string) error {
	return s.do(func(root *RootYAML, save func() error) error {
		delete(root.Conversations, conversationID)
		return save()
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
		root.Conversations = make(map[string]*ConversationYAML)
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

// RootYAML is a YAML model for data root.
type RootYAML struct {
	Conversations map[string]*ConversationYAML `yaml:"conversations"` // Conversations.
}

// ConversationYAML is a YAML model for conversation.
type ConversationYAML struct {
	Messages []MessageYAML `yaml:"messages"` // Messages in the conversation.
}

// MessageYAML is a YAML model for message.
type MessageYAML struct {
	Participant llmprovider.Participant `yaml:"participant"` // Conversation participant.
	Text        string                  `yaml:"text"`        // Message text.
}
