package mdparser

import (
	"bytes"

	tgmd "github.com/Mad-Pixels/goldmark-tgmd"
)

type TransformRequest struct {
	Text      string
	MaxLength int
}

type TransformResult struct {
	Chunks []Chunk
}

type Chunk struct {
	Text string
}

func Transform(req TransformRequest) TransformResult {
	text := renderTgMarkdown(req.Text)
	chunks := splitIntoChunks(text, req.MaxLength)
	return TransformResult{Chunks: chunks}
}

func renderTgMarkdown(text string) string {
	md := tgmd.TGMD()

	var buf bytes.Buffer
	_ = md.Convert([]byte(text), &buf)

	return buf.String()
}

func splitIntoChunks(text string, maxLength int) []Chunk {
	if maxLength <= 0 {
		return []Chunk{{Text: text}}
	}

	var chunks []Chunk
	runes := []rune(text)

	for i := 0; i < len(runes); i += maxLength {
		end := i + maxLength
		if end > len(runes) {
			end = len(runes)
		}

		chunkText := string(runes[i:end])
		chunks = append(chunks, Chunk{Text: chunkText})
	}

	return chunks
}
