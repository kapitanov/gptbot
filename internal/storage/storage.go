package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/kapitanov/gptbot/internal/llmprovider"
)

// Storage stores conversation data.
type Storage struct {
	dir   string
	mutex sync.Mutex
}

// New creates new storage.
func New(dir string) (*Storage, error) {
	s := &Storage{
		dir: dir,
	}
	return s, nil
}

// Retrieve fetches the conversation history for the given conversation ID.
func (s *Storage) Retrieve(conversationID string) ([]llmprovider.Message, error) {
	var messages []llmprovider.Message
	err := s.do(conversationID, func(conversation *conversationJSON, save func() error) error {
		if conversation == nil {
			return nil
		}

		messages = make([]llmprovider.Message, 0, len(conversation.Messages))
		for _, m := range conversation.Messages {
			messages = append(messages, m.toLLMMessage())
		}
		return nil
	})
	return messages, err
}

// Append adds messages to the conversation history for the given conversation ID.
func (s *Storage) Append(conversationID string, messages ...llmprovider.Message) error {
	return s.do(conversationID, func(conversation *conversationJSON, save func() error) error {
		for _, m := range messages {
			conversation.Messages = append(conversation.Messages, newMessageJSON(m))
		}

		return save()
	})
}

// Clear removes the conversation history for the given conversation ID.
func (s *Storage) Clear(conversationID string) error {
	return s.do(conversationID, func(conversation *conversationJSON, save func() error) error {
		conversation.Messages = nil
		return save()
	})
}

func (s *Storage) do(conversationID string, fn func(conversation *conversationJSON, save func() error) error) error {
	filePath := filepath.Join(s.dir, conversationID+".json")

	// A global lock is a terrible idea, but for this pet project it should be OK.
	s.mutex.Lock()
	defer s.mutex.Unlock()

	conversation, err := s.load(filePath)
	if err != nil {
		return err
	}

	return fn(conversation, func() error {
		return s.store(filePath, conversation)
	})
}

func (s *Storage) load(filePath string) (*conversationJSON, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		data = []byte("{}")
	}

	var root conversationJSON
	err = json.Unmarshal(data, &root)
	if err != nil {
		return nil, err
	}

	return &root, nil
}

func (s *Storage) store(filePath string, conversation *conversationJSON) error {
	data, err := json.MarshalIndent(conversation, "", "  ")
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(filePath), 0o777)
	if err != nil {
		return err
	}

	err = os.WriteFile(filePath, data, 0o777)
	if err != nil {
		return err
	}

	return nil
}

// conversationJSON is a JSON model for conversation.
type conversationJSON struct {
	Messages []messageJSON `json:"messages"` // Messages in the conversation.
}

// messageJSON is a JSON model for a message.
type messageJSON struct {
	User      *string `json:"user,omitempty"`
	Assistant *string `json:"assistant,omitempty"`
}

func newMessageJSON(msg llmprovider.Message) messageJSON {
	switch msg.Participant {
	case llmprovider.ParticipantUser:
		return messageJSON{User: &msg.Text}
	case llmprovider.ParticipantAssistant:
		return messageJSON{Assistant: &msg.Text}
	}
	return messageJSON{}
}

func (m messageJSON) toLLMMessage() llmprovider.Message {
	if m.User != nil {
		return llmprovider.Message{
			Participant: llmprovider.ParticipantUser,
			Text:        *m.User,
		}
	}
	if m.Assistant != nil {
		return llmprovider.Message{
			Participant: llmprovider.ParticipantAssistant,
			Text:        *m.Assistant,
		}
	}
	return llmprovider.Message{}
}
