package mdparser

import (
	"bytes"
	"fmt"
	"iter"
	"strings"
	"unicode/utf8"

	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
	"github.com/willabides/mdtable"
	"gopkg.in/telebot.v4"
)

func Parse(srcText string) (string, []telebot.MessageEntity) {
	md := parser.NewWithExtensions(parser.NoIntraEmphasis | parser.Tables | parser.FencedCode | parser.Autolink | parser.Strikethrough | parser.HardLineBreak | parser.OrderedListStart)
	doc := md.Parse([]byte(srcText))

	c := &converter{}
	c.Visit(doc)
	return c.Result()
}

type converter struct {
	out      bytes.Buffer
	entities []telebot.MessageEntity
	curRunes int
	curUnits int
}

func (c *converter) Visit(node ast.Node) {
	switch n := node.(type) {
	case *ast.Paragraph:
		c.visitParagraph(n)
	case *ast.Strong:
		c.visitStrong(n)
	case *ast.Emph:
		c.visitEmph(n)
	case *ast.Del:
		c.visitDel(n)
	case *ast.Link:
		c.visitLink(n)
	case *ast.Heading:
		c.visitHeading(n)
	case *ast.Code:
		c.visitCode(n)
	case *ast.CodeBlock:
		c.visitCodeBlock(n)
	case *ast.List:
		c.visitList(n)
	case *ast.BlockQuote:
		c.visitBlockQuote(n)
	case *ast.Table:
		c.visitTable(n)
	default:
		c.visitGeneric(node)
	}
}

func (c *converter) visitParagraph(node *ast.Paragraph) {
	siblings := node.Parent.GetChildren()
	var prevSibling ast.Node
	for i := range siblings {
		if siblings[i] == node {
			if i > 0 {
				prevSibling = siblings[i-1]
			}
			break
		}
	}

	if prevSibling != nil {
		c.ensureNewline()
	}

	c.visitGeneric(node)
}

func (c *converter) visitStrong(node *ast.Strong) {
	end := c.begin(telebot.MessageEntity{Type: telebot.EntityBold})
	c.visitGeneric(node)
	end()
}

func (c *converter) visitEmph(node *ast.Emph) {
	end := c.begin(telebot.MessageEntity{Type: telebot.EntityItalic})
	c.visitGeneric(node)
	end()
}

func (c *converter) visitDel(node *ast.Del) {
	end := c.begin(telebot.MessageEntity{Type: telebot.EntityStrikethrough})
	c.visitGeneric(node)
	end()
}

func (c *converter) visitLink(node *ast.Link) {
	href := string(node.Destination)
	end := c.begin(telebot.MessageEntity{Type: telebot.EntityTextLink, URL: href})
	c.visitGeneric(node)
	end()
}

func (c *converter) visitHeading(node *ast.Heading) {
	entities := []telebot.MessageEntity{
		{Type: telebot.EntityUnderline},
	}

	if node.Level == 1 {
		entities = append(entities, telebot.MessageEntity{Type: telebot.EntityBold})
	}

	c.writeBytes([]byte("\n\n"))
	end := c.begin(entities...)
	c.visitGeneric(node)
	end()
	c.writeBytes([]byte("\n"))
}

func (c *converter) visitCode(node *ast.Code) {
	end := c.begin(telebot.MessageEntity{Type: telebot.EntityCode})
	c.visitGeneric(node)
	end()
}

func (c *converter) visitCodeBlock(node *ast.CodeBlock) {
	c.ensureNewline()

	lang := string(node.Info)
	end := c.begin(telebot.MessageEntity{Type: telebot.EntityCodeBlock, Language: lang})
	c.visitGeneric(node)
	end()
}

func (c *converter) visitList(node *ast.List) {
	c.ensureNewline()
	if (node.ListFlags & ast.ListTypeOrdered) != 0 {
		c.visitOrderedList(node)
	} else {
		c.visitBulletList(node)
	}
}

func (c *converter) visitBulletList(node *ast.List) {
	for _, child := range node.Children {
		c.writeText("• ")
		c.Visit(child)
		c.writeText("\n")
	}
}

func (c *converter) visitOrderedList(node *ast.List) {
	start := node.Start
	if start == 0 {
		start = 1
	}

	for i, child := range node.Children {
		c.writeText(fmt.Sprintf("%d. ", i+start))
		c.Visit(child)
		c.writeText("\n")
	}
}

func (c *converter) visitTable(node *ast.Table) {
	var tableContent [][]string

	for row := range traverseTableRows(node) {
		var tableRow []string
		for _, c := range row.GetChildren() {
			if cell, ok := c.(*ast.TableCell); ok {
				cellText := renderTableCellText(cell)
				tableRow = append(tableRow, cellText)
			}
		}

		tableContent = append(tableContent, tableRow)
	}

	tableText := mdtable.Generate(tableContent)

	c.ensureNewline()
	end := c.begin(telebot.MessageEntity{Type: telebot.EntityCodeBlock})
	c.writeBytes(tableText)
	end()
}

func traverseTableRows(table *ast.Table) iter.Seq[*ast.TableRow] {
	var header *ast.TableHeader
	var body *ast.TableBody
	var footer *ast.TableFooter

	for _, c := range table.GetChildren() {
		switch child := c.(type) {
		case *ast.TableHeader:
			header = child
		case *ast.TableBody:
			body = child
		case *ast.TableFooter:
			footer = child
		}
	}

	var containers []ast.Node
	if header != nil {
		containers = append(containers, header)
	}
	if body != nil {
		containers = append(containers, body)
	}
	if footer != nil {
		containers = append(containers, footer)
	}

	return func(yield func(*ast.TableRow) bool) {
		for _, container := range containers {
			for _, child := range container.GetChildren() {
				row, ok := child.(*ast.TableRow)
				if !ok {
					continue
				}

				if !yield(row) {
					return
				}
			}
		}
	}
}

func renderTableCellText(node *ast.TableCell) string {
	var sb strings.Builder
	renderNodeText(node, &sb)
	return sb.String()
}

func renderNodeText(node ast.Node, sb *strings.Builder) {
	container := node.AsContainer()
	if container != nil {
		for _, child := range container.GetChildren() {
			renderNodeText(child, sb)
		}
	}

	leaf := node.AsLeaf()
	if leaf != nil {
		_, _ = sb.Write(leaf.Literal)
	}
}

func (c *converter) visitBlockQuote(node *ast.BlockQuote) {
	end := c.begin(telebot.MessageEntity{Type: telebot.EntityBlockquote})
	c.visitGeneric(node)
	end()
}

func (c *converter) visitGeneric(node ast.Node) {
	container := node.AsContainer()
	if container != nil {
		for _, child := range container.GetChildren() {
			c.Visit(child)
		}
	}

	leaf := node.AsLeaf()
	if leaf != nil {
		c.writeBytes(leaf.Literal)
	}
}

func (c *converter) writeBytes(s []byte) {
	c.writeText(string(s))
}

func (c *converter) writeText(str string) {
	str = strings.ReplaceAll(str, "\r\n", "\n") // normalize newlines
	c.out.WriteString(str)
	c.curRunes += utf8.RuneCountInString(str)
	c.curUnits += utf16Len(str)
}

func (c *converter) ensureNewline() {
	if c.out.Len() == 0 {
		return
	}

	if c.out.Bytes()[c.out.Len()-1] != '\n' {
		c.writeText("\n")
	}
}

func (c *converter) addEntity(t telebot.EntityType, url, lang string) func() {
	return c.begin(telebot.MessageEntity{Type: t, URL: url, Language: lang})
}

func (c *converter) begin(entities ...telebot.MessageEntity) func() {
	start := c.curUnits
	return func() {
		length := c.curUnits - start
		for _, entity := range entities {
			entity.Offset = start
			entity.Length = length
			c.entities = append(c.entities, entity)
		}
	}
}

func (c *converter) Result() (string, []telebot.MessageEntity) {
	plain := strings.TrimRight(c.out.String(), "\n")
	return plain, c.entities
}

func utf16Len(s string) int {
	// Count UTF-16 code units (Telegram uses UTF-16 for offsets)
	n := 0
	for _, r := range s {
		if r <= 0xFFFF {
			n++
		} else {
			n += 2 // surrogate pair
		}
	}
	return n
}
