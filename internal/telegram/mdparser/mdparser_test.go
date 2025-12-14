package mdparser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/telebot.v4"
)

func TestParse(t *testing.T) {
	type TestCase struct {
		Name             string
		Input            string
		ExpectedText     string
		ExpectedEntities []telebot.MessageEntity
	}

	testCases := []TestCase{
		{
			Name:             "PlainText",
			Input:            "Hello world!",
			ExpectedText:     "Hello world!",
			ExpectedEntities: []telebot.MessageEntity(nil),
		},
		{
			Name:         "Bold",
			Input:        "Hello **world!**",
			ExpectedText: "Hello world!",
			ExpectedEntities: []telebot.MessageEntity{
				{Type: telebot.EntityBold, Offset: 6, Length: 6},
			},
		},
		{
			Name:         "Italic",
			Input:        "Hello *world!*",
			ExpectedText: "Hello world!",
			ExpectedEntities: []telebot.MessageEntity{
				{Type: telebot.EntityItalic, Offset: 6, Length: 6},
			},
		},
		{
			Name:         "ItalicAlt",
			Input:        "Hello _world!_",
			ExpectedText: "Hello world!",
			ExpectedEntities: []telebot.MessageEntity{
				{Type: telebot.EntityItalic, Offset: 6, Length: 6},
			},
		},
		{
			Name:         "Strikethrough",
			Input:        "Hello ~~world!~~",
			ExpectedText: "Hello world!",
			ExpectedEntities: []telebot.MessageEntity{
				{Type: telebot.EntityStrikethrough, Offset: 6, Length: 6},
			},
		},
		{
			Name:         "Code",
			Input:        "Hello `world!`",
			ExpectedText: "Hello world!",
			ExpectedEntities: []telebot.MessageEntity{
				{Type: telebot.EntityCode, Offset: 6, Length: 6},
			},
		},
		{
			Name:         "Hyperlink",
			Input:        "Hello [world](https://example.com)!",
			ExpectedText: "Hello world!",
			ExpectedEntities: []telebot.MessageEntity{
				{Type: telebot.EntityTextLink, Offset: 6, Length: 5, URL: "https://example.com"},
			},
		},
		{
			Name:         "CodeBlock",
			Input:        "Hello\n\n```\nSource Code\n```",
			ExpectedText: "Hello\nSource Code",
			ExpectedEntities: []telebot.MessageEntity{
				{Type: telebot.EntityCodeBlock, Offset: 6, Length: 12},
			},
		},
		{
			Name:         "CodeBlockWithLanguage",
			Input:        "Hello\n\n```bash\nSource Code\n```",
			ExpectedText: "Hello\nSource Code",
			ExpectedEntities: []telebot.MessageEntity{
				{Type: telebot.EntityCodeBlock, Offset: 6, Length: 12, Language: "bash"},
			},
		},
		{
			Name:         "Heading1",
			Input:        "# Hello World!",
			ExpectedText: "\n\nHello World!",
			ExpectedEntities: []telebot.MessageEntity{
				{Type: telebot.EntityUnderline, Offset: 2, Length: 12},
				{Type: telebot.EntityBold, Offset: 2, Length: 12},
			},
		},
		{
			Name:         "Heading2",
			Input:        "## Hello World!",
			ExpectedText: "\n\nHello World!",
			ExpectedEntities: []telebot.MessageEntity{
				{Type: telebot.EntityUnderline, Offset: 2, Length: 12},
			},
		},
		{
			Name:             "BulletList1",
			Input:            "- Item 1\n- Item 2\n- Item 3",
			ExpectedText:     "• Item 1\n• Item 2\n• Item 3",
			ExpectedEntities: []telebot.MessageEntity(nil),
		},
		{
			Name:             "BulletList2",
			Input:            "* Item 1\n* Item 2\n* Item 3",
			ExpectedText:     "• Item 1\n• Item 2\n• Item 3",
			ExpectedEntities: []telebot.MessageEntity(nil),
		},
		{
			Name:             "BulletList3",
			Input:            " - Item 1\n - Item 2\n - Item 3",
			ExpectedText:     "• Item 1\n• Item 2\n• Item 3",
			ExpectedEntities: []telebot.MessageEntity(nil),
		},
		{
			Name:             "OrderedList",
			Input:            "1. Item 1\n1. Item 2\n1. Item 3",
			ExpectedText:     "1. Item 1\n2. Item 2\n3. Item 3",
			ExpectedEntities: []telebot.MessageEntity(nil),
		},
		{
			Name:         "BlockQuote",
			Input:        "Hello\n> Quote",
			ExpectedText: "HelloQuote",
			ExpectedEntities: []telebot.MessageEntity{
				{Type: telebot.EntityBlockquote, Offset: 5, Length: 5},
			},
		},
		{
			Name:         "Table",
			Input:        "| Column 1 | Column 2 | Column 3 |\n|----------|----------|----------|\n| 5        | 1        | 0.5      |\n| 10       | 0.99     | 0.83     |\n| 15       | 0.98     | 1.16     |",
			ExpectedText: "| Column 1 | Column 2 | Column 3 |\n|----------|----------|----------|\n| 5        | 1        | 0.5      |\n| 10       | 0.99     | 0.83     |\n| 15       | 0.98     | 1.16     |",
			ExpectedEntities: []telebot.MessageEntity{
				{Type: telebot.EntityCodeBlock, Offset: 0, Length: 174},
			},
		},
		{
			Name:         "TableNonFormatted",
			Input:        "| Column 1 | Column 2 | Column 3 |\n|--|--|--|\n| 5 | 1 | 0.5 |\n| 10 | 0.99 | 0.83 |\n| 15 | 0.98 | 1.16 |",
			ExpectedText: "| Column 1 | Column 2 | Column 3 |\n|----------|----------|----------|\n| 5        | 1        | 0.5      |\n| 10       | 0.99     | 0.83     |\n| 15       | 0.98     | 1.16     |",
			ExpectedEntities: []telebot.MessageEntity{
				{Type: telebot.EntityCodeBlock, Offset: 0, Length: 174},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.Name, func(t *testing.T) {
			actualText, actualEntities := Parse(tc.Input)

			t.Logf("text:     %q", actualText)
			t.Logf("entities: %+v", actualEntities)

			assert.Equal(t, tc.ExpectedText, actualText)
			assert.Equal(t, tc.ExpectedEntities, actualEntities)
		})
	}
}
