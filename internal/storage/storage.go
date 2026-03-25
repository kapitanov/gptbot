package storage

import (
	"os"
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

func (s *Storage) GetLastResponseID(userID int64) (string, error) {
	var lastResponseID string
	err := s.do(func(root *RootYAML, save func() error) error {
		conversation, exists := root.Conversations[userID]
		if !exists {
			return nil
		}

		lastResponseID = conversation.LastResponseID
		return nil
	})
	return lastResponseID, err
}

func (s *Storage) SetLastResponseID(userID int64, responseID string) error {
	return s.do(func(root *RootYAML, save func() error) error {
		conversation, exists := root.Conversations[userID]
		if !exists {
			conversation = &ConversationYAML{}
			root.Conversations[userID] = conversation
		}

		conversation.LastResponseID = responseID
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

// RootYAML is a YAML model for data root.
type RootYAML struct {
	Conversations map[int64]*ConversationYAML `yaml:"conversations"` // Conversations.
}

// ConversationYAML is a YAML model for conversation.
type ConversationYAML struct {
	LastResponseID string `yaml:"last_response_id"` // ID of the last response sent by the bot.
}
