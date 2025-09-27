// Custom Markdown parser for Telegram that matches the Go implementation behavior

// Telegram entity types
const EntityType = {
  Bold: 'bold',
  Italic: 'italic', 
  Underline: 'underline',
  Strikethrough: 'strikethrough',
  Code: 'code',
  CodeBlock: 'pre',
  TextLink: 'text_link',
  Blockquote: 'blockquote'
};

class TelegramMarkdownParser {
  constructor() {
    this.reset();
  }

  reset() {
    this.output = '';
    this.entities = [];
    this.pos = 0; // Current position in UTF-16 units
  }

  parse(text) {
    this.reset();
    this.processText(text);
    return [this.output.replace(/\n+$/, ''), this.entities];
  }

  processText(text) {
    // Handle code blocks first as they can contain other markdown syntax
    text = this.handleCodeBlocks(text);
    
    // Split into lines for processing but handle special concatenation cases
    const lines = text.split('\n');
    let result = '';
    let listCounter = 1;
    let inOrderedList = false;

    for (let i = 0; i < lines.length; i++) {
      let line = lines[i];
      
      // Handle headings
      const headingMatch = line.match(/^(#{1,6})\s+(.+)$/);
      if (headingMatch) {
        if (result.length > 0 && !result.endsWith('\n')) {
          result += '\n';
        }
        
        const level = headingMatch[1].length;
        const text = headingMatch[2];
        const startPos = this.calculatePosition(result);
        
        result += text;
        
        // Add entities for heading
        this.entities.push({
          type: EntityType.Underline,
          offset: startPos,
          length: this.utf16Len(text)
        });
        
        if (level === 1) {
          this.entities.push({
            type: EntityType.Bold,
            offset: startPos,
            length: this.utf16Len(text)
          });
        }
        continue;
      }

      // Handle block quotes - they concatenate without newlines
      const blockquoteMatch = line.match(/^>\s*(.+)$/);
      if (blockquoteMatch) {
        const quotedText = blockquoteMatch[1];
        const startPos = this.calculatePosition(result);
        
        result += quotedText;
        
        this.entities.push({
          type: EntityType.Blockquote,
          offset: startPos,
          length: this.utf16Len(quotedText)
        });
        continue;
      }

      // Handle bullet lists
      const bulletMatch = line.match(/^(\s*)[-*]\s+(.+)$/);
      if (bulletMatch) {
        if (result.length > 0 && !result.endsWith('\n')) {
          result += '\n';
        }
        result += '• ' + bulletMatch[2];
        inOrderedList = false;
        continue;
      }

      // Handle ordered lists
      const orderedMatch = line.match(/^(\s*)\d+\.\s+(.+)$/);
      if (orderedMatch) {
        if (!inOrderedList) {
          inOrderedList = true;
          listCounter = 1;
        }
        if (result.length > 0 && !result.endsWith('\n')) {
          result += '\n';
        }
        result += '• ' + listCounter + '. ' + orderedMatch[2];
        listCounter++;
        continue;
      } else if (inOrderedList && !line.match(/^\s*$/)) {
        inOrderedList = false;
        listCounter = 1;
      }

      // Regular line
      if (result.length > 0 && !result.endsWith('\n')) {
        result += '\n';
      }
      result += line;
    }

    this.output = result;
    
    // Now handle inline elements
    this.handleInlineElements();
  }

  handleCodeBlocks(text) {
    const codeBlockRegex = /```(\w*)\n?([\s\S]*?)\n?```/g;
    let result = '';
    let lastIndex = 0;
    let match;

    while ((match = codeBlockRegex.exec(text)) !== null) {
      // Add text before code block
      const beforeText = text.substring(lastIndex, match.index);
      result += beforeText;
      
      // Add newline before code block if needed, but handle the special case
      // where we have "Hello\n\n```" - we should only output "Hello\n" + code
      if (result.length > 0) {
        // Remove any trailing empty lines before code block
        result = result.replace(/\n+$/, '\n');
      }
      
      const language = match[1];
      const code = match[2].trim();
      const startPos = this.calculatePosition(result);
      
      result += code;
      
      this.entities.push({
        type: EntityType.CodeBlock,
        offset: startPos,
        length: this.utf16Len(code),
        language: language || undefined
      });

      lastIndex = match.index + match[0].length;
    }

    // Add remaining text
    result += text.substring(lastIndex);
    return result;
  }

  handleInlineElements() {
    // Process inline elements in order of precedence
    this.processInlinePattern(/\*\*([^*]+)\*\*/g, EntityType.Bold);
    this.processInlinePattern(/\*([^*]+)\*/g, EntityType.Italic);
    this.processInlinePattern(/_([^_]+)_/g, EntityType.Italic);
    this.processInlinePattern(/~~([^~]+)~~/g, EntityType.Strikethrough);
    this.processInlinePattern(/`([^`]+)`/g, EntityType.Code);
    this.processLinkPattern();
  }

  processInlinePattern(regex, entityType) {
    let match;
    const newOutput = this.output;
    let offset = 0;
    const newEntities = [];

    // Create a new output string without the markdown syntax
    let result = '';
    let lastIndex = 0;

    const globalRegex = new RegExp(regex.source, 'g');
    while ((match = globalRegex.exec(this.output)) !== null) {
      // Add text before match
      const beforeText = this.output.substring(lastIndex, match.index);
      result += beforeText;
      
      // Calculate the position in the new string
      const newPos = this.calculatePosition(result);
      const content = match[1];
      
      // Add the content without markdown syntax
      result += content;
      
      // Add entity
      newEntities.push({
        type: entityType,
        offset: newPos,
        length: this.utf16Len(content)
      });

      lastIndex = match.index + match[0].length;
    }

    // Add remaining text
    result += this.output.substring(lastIndex);
    
    if (newEntities.length > 0) {
      this.output = result;
      this.entities.push(...newEntities);
    }
  }

  processLinkPattern() {
    const linkRegex = /\[([^\]]+)\]\(([^)]+)\)/g;
    let match;
    const newEntities = [];
    let result = '';
    let lastIndex = 0;

    while ((match = linkRegex.exec(this.output)) !== null) {
      // Add text before match
      const beforeText = this.output.substring(lastIndex, match.index);
      result += beforeText;
      
      // Calculate position for the link text
      const newPos = this.calculatePosition(result);
      const linkText = match[1];
      const url = match[2];
      
      // Add just the link text
      result += linkText;
      
      // Add entity
      newEntities.push({
        type: EntityType.TextLink,
        offset: newPos,
        length: this.utf16Len(linkText),
        url: url
      });

      lastIndex = match.index + match[0].length;
    }

    // Add remaining text
    result += this.output.substring(lastIndex);
    
    if (newEntities.length > 0) {
      this.output = result;
      this.entities.push(...newEntities);
    }
  }

  calculatePosition(text) {
    return this.utf16Len(text);
  }

  utf16Len(str) {
    // Count UTF-16 code units (Telegram uses UTF-16 for offsets)
    let count = 0;
    for (const char of str) {
      const code = char.codePointAt(0);
      if (code <= 0xFFFF) {
        count++;
      } else {
        count += 2; // Surrogate pair
      }
    }
    return count;
  }
}

function parse(srcText) {
  try {
    const parser = new TelegramMarkdownParser();
    return parser.parse(srcText);
  } catch (error) {
    console.error('Markdown parsing error:', error);
    return [srcText, []];
  }
}

export { parse };