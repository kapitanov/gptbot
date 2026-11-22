package storage

import (
	"testing"

	"github.com/kapitanov/gptbot/internal/llmprovider"
)

func TestModelSurvivesClear(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	const conversationID = "u1"

	if model, err := s.Model(conversationID); err != nil || model != "" {
		t.Fatalf("unexpected initial model %q (err: %v)", model, err)
	}

	if err = s.SetModel(conversationID, "model-b"); err != nil {
		t.Fatal(err)
	}

	if err = s.Append(conversationID, llmprovider.Message{Participant: llmprovider.ParticipantUser, Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	if err = s.Clear(conversationID); err != nil {
		t.Fatal(err)
	}

	model, err := s.Model(conversationID)
	if err != nil {
		t.Fatal(err)
	}
	if model != "model-b" {
		t.Fatalf("model after clear = %q, want %q", model, "model-b")
	}

	messages, err := s.Retrieve(conversationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages after clear = %d, want 0", len(messages))
	}
}
