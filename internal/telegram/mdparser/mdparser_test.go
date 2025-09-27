package mdparser

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestMarkdownParser(t *testing.T) {
	testCases := []struct {
		name             string
		input            string
		expectedText     string
		expectedEntities []tgbotapi.MessageEntity
	}{
		{
			name:             "PlainText",
			input:            "Hello world!",
			expectedText:     "Hello world!",
			expectedEntities: []tgbotapi.MessageEntity{},
		},
		{
			name:         "Bold",
			input:        "Hello **world!**",
			expectedText: "Hello world!",
			expectedEntities: []tgbotapi.MessageEntity{
				{Type: "bold", Offset: 6, Length: 6},
			},
		},
		{
			name:         "Italic",
			input:        "Hello *world!*",
			expectedText: "Hello world!",
			expectedEntities: []tgbotapi.MessageEntity{
				{Type: "italic", Offset: 6, Length: 6},
			},
		},
		{
			name:         "ItalicAlt",
			input:        "Hello _world!_",
			expectedText: "Hello world!",
			expectedEntities: []tgbotapi.MessageEntity{
				{Type: "italic", Offset: 6, Length: 6},
			},
		},
		{
			name:         "Code",
			input:        "Hello `world!`",
			expectedText: "Hello world!",
			expectedEntities: []tgbotapi.MessageEntity{
				{Type: "code", Offset: 6, Length: 6},
			},
		},
		{
			name:         "Strikethrough",
			input:        "Hello ~~world!~~",
			expectedText: "Hello world!",
			expectedEntities: []tgbotapi.MessageEntity{
				{Type: "strikethrough", Offset: 6, Length: 6},
			},
		},
		{
			name:         "Link",
			input:        "Hello [world](https://example.com)!",
			expectedText: "Hello world!",
			expectedEntities: []tgbotapi.MessageEntity{
				{Type: "text_link", Offset: 6, Length: 5, URL: "https://example.com"},
			},
		},
		{
			name:         "CodeBlock",
			input:        "```\nSource Code\n```",
			expectedText: "Source Code",
			expectedEntities: []tgbotapi.MessageEntity{
				{Type: "pre", Offset: 0, Length: 11},
			},
		},
		{
			name:         "CodeBlockWithLanguage",
			input:        "```go\npackage main\n```",
			expectedText: "package main",
			expectedEntities: []tgbotapi.MessageEntity{
				{Type: "pre", Offset: 0, Length: 12, Language: "go"},
			},
		},
		{
			name:         "Heading1",
			input:        "# Hello World!",
			expectedText: "Hello World!",
			expectedEntities: []tgbotapi.MessageEntity{
				{Type: "bold", Offset: 0, Length: 12},
				{Type: "underline", Offset: 0, Length: 12},
			},
		},
		{
			name:         "Heading2",
			input:        "## Hello World!",
			expectedText: "Hello World!",
			expectedEntities: []tgbotapi.MessageEntity{
				{Type: "underline", Offset: 0, Length: 12},
			},
		},
		{
			name:             "BulletList1",
			input:            "- Item 1\n- Item 2\n- Item 3",
			expectedText:     "• Item 1\n• Item 2\n• Item 3",
			expectedEntities: []tgbotapi.MessageEntity{},
		},
		{
			name:             "BulletList2",
			input:            "* Item 1\n* Item 2\n* Item 3",
			expectedText:     "• Item 1\n• Item 2\n• Item 3",
			expectedEntities: []tgbotapi.MessageEntity{},
		},
		{
			name:             "OrderedList",
			input:            "1. Item 1\n2. Item 2\n3. Item 3",
			expectedText:     "• 1. Item 1\n• 2. Item 2\n• 3. Item 3",
			expectedEntities: []tgbotapi.MessageEntity{},
		},
		{
			name:         "BlockQuote",
			input:        "Hello\n> Quote",
			expectedText: "Hello\nQuote",
			expectedEntities: []tgbotapi.MessageEntity{
				{Type: "blockquote", Offset: 6, Length: 5},
			},
		},
		{
			name:         "Mixed",
			input:        "**Bold** and *italic* and `code`",
			expectedText: "Bold and italic and code",
			expectedEntities: []tgbotapi.MessageEntity{
				{Type: "bold", Offset: 0, Length: 4},
				{Type: "italic", Offset: 9, Length: 6},
				{Type: "code", Offset: 20, Length: 4},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualText, actualEntities := Parse(tc.input)

			if actualText != tc.expectedText {
				t.Errorf("Text mismatch:\nExpected: %q\nActual:   %q", tc.expectedText, actualText)
			}

			if len(actualEntities) != len(tc.expectedEntities) {
				t.Errorf("Entity count mismatch:\nExpected: %d\nActual:   %d", len(tc.expectedEntities), len(actualEntities))
				t.Logf("Actual entities: %+v", actualEntities)
				return
			}

			for i, expected := range tc.expectedEntities {
				if i >= len(actualEntities) {
					t.Errorf("Missing entity at index %d", i)
					continue
				}

				actual := actualEntities[i]

				if actual.Type != expected.Type {
					t.Errorf("Entity type mismatch at %d:\nExpected: %s\nActual:   %s", i, expected.Type, actual.Type)
				}

				if actual.Offset != expected.Offset {
					t.Errorf("Entity offset mismatch at %d:\nExpected: %d\nActual:   %d", i, expected.Offset, actual.Offset)
				}

				if actual.Length != expected.Length {
					t.Errorf("Entity length mismatch at %d:\nExpected: %d\nActual:   %d", i, expected.Length, actual.Length)
				}

				if expected.URL != "" && actual.URL != expected.URL {
					t.Errorf("Entity URL mismatch at %d:\nExpected: %s\nActual:   %s", i, expected.URL, actual.URL)
				}

				if expected.Language != "" && actual.Language != expected.Language {
					t.Errorf("Entity language mismatch at %d:\nExpected: %s\nActual:   %s", i, expected.Language, actual.Language)
				}
			}
		})
	}
}

func TestMarkdownParserEdgeCases(t *testing.T) {
	t.Run("EmptyString", func(t *testing.T) {
		text, entities := Parse("")
		if text != "" {
			t.Errorf("Expected empty string, got %q", text)
		}
		if len(entities) != 0 {
			t.Errorf("Expected no entities, got %d", len(entities))
		}
	})

	t.Run("OnlyWhitespace", func(t *testing.T) {
		text, entities := Parse("   \n\t  ")
		if text != "" {
			t.Errorf("Expected empty string after trimming, got %q", text)
		}
		if len(entities) != 0 {
			t.Errorf("Expected no entities, got %d", len(entities))
		}
	})

	t.Run("UnclosedMarkdown", func(t *testing.T) {
		text, _ := Parse("Hello **world")
		// Should not process unclosed markdown
		if text != "Hello **world" {
			t.Errorf("Expected unclosed markdown to remain unchanged, got %q", text)
		}
	})

	t.Run("NestedMarkdown", func(t *testing.T) {
		text, entities := Parse("**Bold with *italic* inside**")
		// For simplicity, nested markdown might not be fully supported
		// The important thing is that some formatting is applied
		if len(entities) == 0 {
			t.Error("Expected at least one entity for nested markdown")
		}
		// Text should have some processing applied
		if text == "**Bold with *italic* inside**" {
			t.Error("Expected some markdown processing to occur")
		}
	})
}
