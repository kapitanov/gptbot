package mdparser

import (
	"regexp"
	"strings"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Parse parses markdown text and returns formatted text with entities
func Parse(text string) (string, []tgbotapi.MessageEntity) {
	parser := &telegramMarkdownParser{
		entities: make([]tgbotapi.MessageEntity, 0),
	}
	return parser.parse(text)
}

type telegramMarkdownParser struct {
	output   strings.Builder
	entities []tgbotapi.MessageEntity
	pos      int // Current position in UTF-16 units (for Telegram entity offsets)
}

func (p *telegramMarkdownParser) parse(text string) (string, []tgbotapi.MessageEntity) {
	p.output.Reset()
	p.entities = make([]tgbotapi.MessageEntity, 0)
	p.pos = 0

	// Trim whitespace
	text = strings.TrimSpace(text)
	if text == "" {
		return "", p.entities
	}

	// Process text line by line (code blocks are handled within line processing)
	p.processLines(text)

	result := p.output.String()
	// Remove trailing newlines
	result = strings.TrimRight(result, "\n")

	return result, p.entities
}

func (p *telegramMarkdownParser) handleCodeBlocksInText(text string) string {
	// Handle triple backtick code blocks
	codeBlockRegex := regexp.MustCompile("```(?:([a-zA-Z0-9_+-]+)\n)?((?:[^`]|`[^`]|``[^`])*?)```")

	result := codeBlockRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatches := codeBlockRegex.FindStringSubmatch(match)
		if len(submatches) < 3 {
			return match
		}

		language := submatches[1]
		content := strings.TrimSpace(submatches[2])

		start := p.pos
		p.output.WriteString(content)
		length := utf8.RuneCountInString(content)
		p.pos += length

		entity := tgbotapi.MessageEntity{
			Type:   "pre",
			Offset: start,
			Length: length,
		}
		if language != "" {
			entity.Language = language
		}
		p.entities = append(p.entities, entity)

		// Return a special marker to indicate this was processed
		return "\x00CODEBLOCK_PROCESSED\x00"
	})

	// Remove the processed markers to avoid further processing
	result = strings.ReplaceAll(result, "\x00CODEBLOCK_PROCESSED\x00", "")

	return result
}

func (p *telegramMarkdownParser) processLines(text string) {
	// Check if the entire text is a code block first
	codeBlockRegex := regexp.MustCompile("^```(?:([a-zA-Z0-9_+-]+)\n)?((?:[^`]|`[^`]|``[^`])*?)```$")
	if match := codeBlockRegex.FindStringSubmatch(text); match != nil {
		language := match[1]
		content := strings.TrimSpace(match[2])

		start := p.pos
		p.output.WriteString(content)
		length := utf8.RuneCountInString(content)
		p.pos += length

		entity := tgbotapi.MessageEntity{
			Type:   "pre",
			Offset: start,
			Length: length,
		}
		if language != "" {
			entity.Language = language
		}
		p.entities = append(p.entities, entity)
		return
	}

	// Handle code blocks that can span multiple lines in mixed content
	text = p.handleCodeBlocksInText(text)

	lines := strings.Split(text, "\n")
	listCounter := 1
	inOrderedList := false

	for i, line := range lines {
		// Skip empty lines that were markers for processed code blocks
		if strings.TrimSpace(line) == "" && strings.Contains(text, "\x00CODEBLOCK_PROCESSED\x00") {
			continue
		}

		// Handle headings
		if p.handleHeading(line, i, len(lines)) {
			continue
		}

		// Handle unordered lists
		if p.handleUnorderedList(line, i, len(lines), &inOrderedList) {
			continue
		}

		// Handle ordered lists
		if p.handleOrderedList(line, i, len(lines), &listCounter, &inOrderedList) {
			continue
		} else {
			inOrderedList = false
			listCounter = 1
		}

		// Handle blockquotes
		if p.handleBlockquote(line, i, len(lines)) {
			continue
		}

		// Regular line - process inline markdown
		p.handleRegularLine(line, i, len(lines))
	}
}

func (p *telegramMarkdownParser) handleHeading(line string, lineIndex, totalLines int) bool {
	headingRegex := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	match := headingRegex.FindStringSubmatch(line)
	if match == nil {
		return false
	}

	p.addNewlineIfNeeded()

	headingText := match[2]
	level := len(match[1])

	start := p.pos
	p.output.WriteString(headingText)
	length := utf8.RuneCountInString(headingText)
	p.pos += length

	// H1 gets both bold and underline, H2+ gets just underline
	if level == 1 {
		p.entities = append(p.entities, tgbotapi.MessageEntity{
			Type:   "bold",
			Offset: start,
			Length: length,
		})
	}
	p.entities = append(p.entities, tgbotapi.MessageEntity{
		Type:   "underline",
		Offset: start,
		Length: length,
	})

	p.addNewlineIfNotLast(lineIndex, totalLines)
	return true
}

func (p *telegramMarkdownParser) handleUnorderedList(line string, lineIndex, totalLines int, inOrderedList *bool) bool {
	listRegex := regexp.MustCompile(`^[\s]*[-*+]\s+(.+)$`)
	match := listRegex.FindStringSubmatch(line)
	if match == nil {
		return false
	}

	*inOrderedList = false
	p.addNewlineIfNeeded()

	p.output.WriteString("• ")
	p.pos += 2
	p.processInlineMarkdown(match[1])

	p.addNewlineIfNotLast(lineIndex, totalLines)
	return true
}

func (p *telegramMarkdownParser) handleOrderedList(line string, lineIndex, totalLines int, listCounter *int, inOrderedList *bool) bool {
	listRegex := regexp.MustCompile(`^[\s]*\d+\.\s+(.+)$`)
	match := listRegex.FindStringSubmatch(line)
	if match == nil {
		return false
	}

	if !*inOrderedList {
		*listCounter = 1
		*inOrderedList = true
	}

	p.addNewlineIfNeeded()

	p.output.WriteString("• ")
	p.pos += 2

	// Add the counter
	counterStr := strings.Join([]string{string(rune('0' + *listCounter)), ". "}, "")
	p.output.WriteString(counterStr)
	p.pos += utf8.RuneCountInString(counterStr)

	p.processInlineMarkdown(match[1])
	*listCounter++

	p.addNewlineIfNotLast(lineIndex, totalLines)
	return true
}

func (p *telegramMarkdownParser) handleBlockquote(line string, lineIndex, totalLines int) bool {
	quoteRegex := regexp.MustCompile(`^>\s*(.*)$`)
	match := quoteRegex.FindStringSubmatch(line)
	if match == nil {
		return false
	}

	p.addNewlineIfNeeded()

	start := p.pos
	quoteText := match[1]
	p.processInlineMarkdown(quoteText)
	length := utf8.RuneCountInString(quoteText)

	p.entities = append(p.entities, tgbotapi.MessageEntity{
		Type:   "blockquote",
		Offset: start,
		Length: length,
	})

	p.addNewlineIfNotLast(lineIndex, totalLines)
	return true
}

func (p *telegramMarkdownParser) handleRegularLine(line string, lineIndex, totalLines int) {
	if p.output.Len() > 0 && !strings.HasSuffix(p.output.String(), "\n") && line != "" {
		p.output.WriteString("\n")
		p.pos++
	}

	if line != "" {
		p.processInlineMarkdown(line)
	}

	if lineIndex < totalLines-1 && line != "" {
		p.output.WriteString("\n")
		p.pos++
	}
}

func (p *telegramMarkdownParser) processInlineMarkdown(text string) {
	remaining := text

	for len(remaining) > 0 {
		// Find the next markdown pattern - using separate patterns to avoid conflicts
		nextBoldStar := p.findNextPattern(remaining, `\*\*([^*]+?)\*\*`)
		nextBoldUnderscore := p.findNextPattern(remaining, `__([^_]+?)__`)
		nextItalicStar := p.findNextPattern(remaining, `\*([^*]+?)\*`)
		nextItalicUnderscore := p.findNextPattern(remaining, `_([^_]+?)_`)
		nextCode := p.findNextPattern(remaining, "`([^`]+)`")
		nextLink := p.findNextPattern(remaining, `\[([^\]]+)\]\(([^)]+)\)`)
		nextStrike := p.findNextPattern(remaining, `~~([^~]+?)~~`)

		// Find the earliest pattern
		earliest := p.findEarliest(nextBoldStar, nextBoldUnderscore, nextItalicStar, nextItalicUnderscore, nextCode, nextLink, nextStrike)

		if earliest == nil {
			// No more patterns, add remaining text as-is
			p.output.WriteString(remaining)
			p.pos += utf8.RuneCountInString(remaining)
			break
		}

		// Add text before the pattern
		if earliest.start > 0 {
			beforeText := remaining[:earliest.start]
			p.output.WriteString(beforeText)
			p.pos += utf8.RuneCountInString(beforeText)
		}

		// Process the pattern
		p.processPattern(earliest)

		// Continue with remaining text
		remaining = remaining[earliest.end:]
	}
}

type patternMatch struct {
	start   int
	end     int
	content string
	patType string
	url     string
}

func (p *telegramMarkdownParser) findNextPattern(text, pattern string) *patternMatch {
	regex := regexp.MustCompile(pattern)
	match := regex.FindStringSubmatchIndex(text)
	if match == nil {
		return nil
	}

	var content, patType, url string

	switch pattern {
	case `\*\*([^*]+?)\*\*`:
		patType = "bold"
		content = text[match[2]:match[3]]
	case `__([^_]+?)__`:
		patType = "bold"
		content = text[match[2]:match[3]]
	case `\*([^*]+?)\*`:
		patType = "italic"
		content = text[match[2]:match[3]]
	case `_([^_]+?)_`:
		patType = "italic"
		content = text[match[2]:match[3]]
	case "`([^`]+)`":
		patType = "code"
		content = text[match[2]:match[3]]
	case `\[([^\]]+)\]\(([^)]+)\)`:
		patType = "text_link"
		content = text[match[2]:match[3]]
		url = text[match[4]:match[5]]
	case `~~([^~]+?)~~`:
		patType = "strikethrough"
		content = text[match[2]:match[3]]
	}

	return &patternMatch{
		start:   match[0],
		end:     match[1],
		content: content,
		patType: patType,
		url:     url,
	}
}

func (p *telegramMarkdownParser) findEarliest(patterns ...*patternMatch) *patternMatch {
	var earliest *patternMatch
	for _, pattern := range patterns {
		if pattern != nil && (earliest == nil || pattern.start < earliest.start) {
			earliest = pattern
		}
	}
	return earliest
}

func (p *telegramMarkdownParser) processPattern(pattern *patternMatch) {
	start := p.pos
	p.output.WriteString(pattern.content)
	length := utf8.RuneCountInString(pattern.content)
	p.pos += length

	entity := tgbotapi.MessageEntity{
		Type:   pattern.patType,
		Offset: start,
		Length: length,
	}

	if pattern.url != "" {
		entity.URL = pattern.url
	}

	p.entities = append(p.entities, entity)
}

func (p *telegramMarkdownParser) addNewlineIfNeeded() {
	if p.output.Len() > 0 && !strings.HasSuffix(p.output.String(), "\n") {
		p.output.WriteString("\n")
		p.pos++
	}
}

func (p *telegramMarkdownParser) addNewlineIfNotLast(lineIndex, totalLines int) {
	if lineIndex < totalLines-1 {
		p.output.WriteString("\n")
		p.pos++
	}
}
