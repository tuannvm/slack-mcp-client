# Slack Formatting Guide

This comprehensive guide explains how to format messages for Slack using both `mrkdwn` (Markdown) and Block Kit structures, and documents the fully implemented Slack formatting features in the Slack MCP Client.

## Overview

The Slack MCP Client includes a comprehensive message formatting system that supports two main approaches:

1. **mrkdwn**: Slack's version of Markdown for text formatting
2. **Block Kit**: Rich, interactive message layouts with JSON structure

The client automatically handles format conversion and detection, providing rich, interactive messages in Slack with full production-ready implementation.

## ✅ Implementation Status

The Slack formatting system is **fully implemented** and production-ready with the following features:

### Automatic Format Detection

The client automatically detects and handles multiple message formats:

- ✅ **Plain Text**: Simple text messages with proper escaping
- ✅ **Markdown Text**: Messages with Markdown formatting (bold, italic, code blocks, etc.)
- ✅ **JSON Block Kit**: Messages in Block Kit JSON format  
- ✅ **Structured Data**: Messages with key-value pairs automatically converted to Block Kit format

### Current Implementation Architecture

The formatter is implemented in `internal/slack/formatter/` with these components:

**Core Files:**
1. **`formatter.go`**: Main formatting logic and Block Kit generation
2. **`detector.go`**: Format detection and automatic conversion
3. **`formatter_test.go`**: Comprehensive test suite

**Key Functions:**
```go
// FormatMessage - Main entry point for message formatting
func FormatMessage(text string, options FormatOptions) []slack.MsgOption

// CreateBlockMessage - Generate Block Kit messages programmatically
func CreateBlockMessage(text string, blockOptions BlockOptions) string

// FormatMarkdown - Convert standard Markdown to Slack mrkdwn
func FormatMarkdown(text string) string

// ConvertQuotedStringsToCode - Auto-convert quoted strings
func ConvertQuotedStringsToCode(text string) string
```

## Markdown to Slack Mapping Reference

Below is a comprehensive table mapping common Markdown elements to their Slack equivalents:

| Feature               | Standard Markdown Syntax           | Slack Syntax / Notes                                                  | Support           |
|-----------------------|------------------------------------|------------------------------------------------------------------------|-------------------|
| Headings              | `# Heading 1` `## Heading 2`       | Not supported in messages (Block Kit headers exist but not via `#`)    | No   |
| Paragraphs            | Blank line separates paragraphs    | No explicit paragraph syntax – use a blank line or Shift+Enter          | No   |
| Line breaks           | Two spaces at end + `\n`           | Shift+Enter (literal `\n` not parsed in message UI)                    | No   |
| Bold                  | `**bold**`                         | `*bold*` (asterisks) - ✅ **Auto-converted**                           | Yes  |
| Italic                | `*italic*` or `_italic_`           | `_italic_` (underscores only) - ✅ **Auto-converted**                  | Partial |
| Strikethrough         | `~~strike~~`                       | `~strike~` - ✅ **Auto-converted**                                     | Partial |
| Blockquote            | `> quote`                          | `> quote`                                                               | Yes  |
| Ordered list          | `1. item`                          | `1. item`                                                               | Yes  |
| Unordered list        | `- item` or `* item`               | `• item` or `- item` (bullets rendered automatically)                  | Yes  |
| Inline code           | `` `code` ``                       | `` `code` ``                                                            | Yes  |
| Fenced code block     | <pre>```code```</pre>              | ```code```                                                               | Yes  |
| Horizontal rule       | `---`                              | Not supported                                                          | No   |
| Links                 | `[text](http://example.com)`       | `<http://example.com|text>` - ✅ **Auto-converted**                   | Partial |
| Images                | `![alt](http://img.png)`           | Not via Markdown – upload or drag-and-drop                              | No   |
| Tables                | `| col1 \| col2 |`                | Not supported                                                          | No   |
| Definition lists      | `Term : Definition`               | Not supported                                                          | No   |
| Footnotes             | `[^1]`                             | Not supported                                                          | No   |
| Task lists            | `- [ ] item`                       | Not supported                                                          | No   |
| HTML                  | `<br>`, `<em>`                     | Not supported                                                          | No   |
| Emoji                 | `:smile:` or Unicode 😄            | `:smile:` (auto-converted) or paste Unicode                             | Yes  |
| Automatic URL linking | `<http://example.com>`             | Paste URL – auto-linked                                                 | Yes  |
| Disable auto-link     | `<http://example.com|http://...>`  | Use `<` `|` `>` syntax to control display                              | Yes  |
| Quoted Strings        | `"quoted text"`                    | ✅ **Auto-converted to** `` `quoted text` ``                           | Yes  |

## Detailed mrkdwn Rules

### 1. General Markdown (`mrkdwn`) Rules  
- Escape literal `&`, `<`, and `>` as `&amp;`, `&lt;`, `&gt;`.  
- Italic: `_italic text_`  
- Bold: `*bold text*`  
- Strikethrough: `~struck text~`  
- Block quote (one or more lines):  
  ```
  > This is a quote.
  > Still quoted.
  ```  
- Inline code: `` `code snippet` ``  
- Code block:  
  ````
  ```  
  multiple lines of code  
  ```  
  ````  
- Bulleted list (use actual bullet character):  
  ```
  • Item one  
  • Item two  
  ```  
- Numbered list (manual numbering):  
  ```
  1. First  
  2. Second  
  ```  
- Line breaks: insert `\n` where you want a new line.

### 2. Links, Mentions & Emoji  
- Automatic URL links: paste `http://example.com`.  
- Manual links: `<http://example.com|Link Text>`  
- User mention: `<@U12345678>`  
- Channel mention: `<#C12345678|general>`  
- Email link: `<mailto:alice@example.com|Email Alice>`  
- Emoji: include Unicode emoji (e.g. 😄) or colon syntax `:smile:`.

### 3. Special Parsing  
- Date formatting:  
  ```
  <!date^1622559600^{date_short} at {time}|Jun 1 2021 at 12:00 PM UTC>
  ```  
- Special mentions: `<!here>`, `<!channel>`, `<!everyone>`.

### 4. ✅ Automatic Quoted String Conversion

The formatter automatically converts double-quoted strings to inline code blocks for better visualization:

**Input:**
```
All of these were created on "2020-11-17T05:07:52Z" or "2020-11-17T05:07:54Z".
Among them, "kube-node-lease", "kube-public", and "kube-system" share the exact
same creation timestamp: "2020-11-17T05:07:52Z".
```

**Output:**
```
All of these were created on `2020-11-17T05:07:52Z` or `2020-11-17T05:07:54Z`.
Among them, `kube-node-lease`, `kube-public`, and `kube-system` share the exact
same creation timestamp: `2020-11-17T05:07:52Z`.
```

## Block Kit Layouts

### When to Use Block Kit

Use Block Kit layouts for complex responses that need:
- Rich visual structure with headers, sections, and fields
- Interactive elements like buttons
- Organized data presentation
- Multiple content types in one message

### Block Kit Structure

Return a JSON payload with both a fallback `text` and a `blocks` array:

```json
{
  "text": "Summary: Job completed",
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text",
        "text": "Job Status"
      }
    },
    {
      "type": "section",
      "fields": [
        {
          "type": "mrkdwn",
          "text": "*Result:*\nSuccess"
        },
        {
          "type": "mrkdwn",
          "text": "*Duration:*\n5m 32s"
        }
      ]
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "View logs: <http://logs.example.com|Open Logs>"
      }
    }
  ]
}
```

### Common Block Types

- **Header**: `"type": "header"` - Large, prominent titles
- **Section**: `"type": "section"` - Main content with text and fields
- **Divider**: `"type": "divider"` - Visual separator
- **Actions**: `"type": "actions"` - Interactive buttons and elements
- **Context**: `"type": "context"` - Subtle contextual information

### ✅ Structured Data Auto-Conversion

The client automatically converts structured data to Block Kit format:

**Input:**
```
Status: Success
Duration: 5m 32s
Result: Passed
```

**Output:** Automatically formatted as a Block Kit message with fields.

## Practical Examples

### Example 1: Simple mrkdwn Response
_User asks:_ "What's the server status?"  
_Your response:_

```
*Server Status* ✅

• CPU: 45% usage
• Memory: 60% usage  
• Disk: 30% usage
• Last restart: <!date^1622559600^{date_short} at {time}|Jun 1 2021 at 12:00 PM UTC>

All systems operational!
```

### Example 2: Block Kit Response
_User asks:_ "Show me the latest build results."  
_Your response:_

```json
{
  "text": "Latest build results: Passed",
  "blocks": [
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*Build #123* _passed_ 🎉\n• Duration: 4m 12s\n• Triggered by: <@U23456789>"
      }
    },
    {
      "type": "actions",
      "elements": [
        {
          "type": "button",
          "text": {
            "type": "plain_text",
            "text": "View Details"
          },
          "url": "http://ci.example.com/build/123"
        }
      ]
    }
  ]
}
```

### Example 3: Structured Data
_User asks:_ "List the database connections."  
_Your response:_

```json
{
  "text": "Database Connections",
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text",
        "text": "Database Connections"
      }
    },
    {
      "type": "section",
      "fields": [
        {
          "type": "mrkdwn",
          "text": "*Primary DB:*\n✅ Connected"
        },
        {
          "type": "mrkdwn",
          "text": "*Replica DB:*\n✅ Connected"
        },
        {
          "type": "mrkdwn",
          "text": "*Cache DB:*\n⚠️ Degraded"
        },
        {
          "type": "mrkdwn",
          "text": "*Analytics DB:*\n❌ Disconnected"
        }
      ]
    },
    {
      "type": "context",
      "elements": [
        {
          "type": "mrkdwn",
          "text": "Last checked: <!date^1622559600^{time}|12:00 PM>"
        }
      ]
    }
  ]
}
```

## 🔧 Configuration

The formatter supports various configuration options:

```go
type FormatOptions struct {
    Format     MessageFormat  // TextFormat or BlockFormat
    ThreadTS   string        // For threading messages
    EscapeText bool          // Whether to escape special characters
}

type BlockOptions struct {
    HeaderText string     // Header text for Block Kit messages
    Fields     []Field    // Key-value fields
    Actions    []Action   // Action buttons
}
```

### Usage in Slack Client

The Slack client automatically uses the formatter:

```go
// Example from internal/slack/client.go
msgOptions := formatter.FormatMessage(response, formatter.FormatOptions{
    Format:     formatter.TextFormat,
    ThreadTS:   threadTS,
    EscapeText: false,
})
```

## 📊 Features Supported

| Feature | Status | Description |
|---------|--------|-------------|
| Text Formatting | ✅ | Bold, italic, strikethrough, code |
| Code Blocks | ✅ | Syntax highlighting support |
| Lists | ✅ | Bullet and numbered lists |
| Links | ✅ | Automatic URL detection and formatting |
| Block Kit | ✅ | Headers, sections, fields, actions |
| Auto-Detection | ✅ | Automatic format detection |
| Quoted Strings | ✅ | Auto-conversion to code blocks |
| Structured Data | ✅ | Auto-conversion to Block Kit |
| Interactive Elements | ✅ | Buttons and interactive components |
| Field Truncation | ✅ | Automatic handling of Slack limits |

## Best Practices

1. **Always provide fallback text** for Block Kit messages
2. **Use mrkdwn for simple responses**, Block Kit for complex ones
3. **Keep field counts under 10** per section for optimal display
4. **Use emojis strategically** to convey status and improve readability  
5. **Test interactive elements** like buttons and links
6. **Escape special characters** properly in mrkdwn
7. **Use headers** to organize complex information
8. **Provide clear visual hierarchy** with appropriate block types
9. **Use structured data** for tabular information that auto-converts to Block Kit
10. **Include fallback text** for accessibility and notification compatibility

## Troubleshooting

### Common Issues

1. **Text not formatting**: Check for proper escape sequences and syntax
2. **Block Kit validation errors**: Ensure JSON structure is correct and within Slack limits
3. **Links not working**: Verify URL format and accessibility
4. **Interactive elements failing**: Check button configurations and URLs
5. **Markdown not rendering**: Verify proper escape sequences and check for conflicting formatting

### Debug Mode

Enable debug logging to see formatting decisions:

```bash
LOG_LEVEL=debug ./slack-mcp-client
```

### Testing

Test your formatting by:
- Sending test messages to a development Slack workspace
- Using Slack's Block Kit Builder for complex layouts
- Validating JSON structure before sending
- Checking message rendering on different devices

## 📚 Reference

- **Implementation**: `internal/slack/formatter/`
- **Tests**: `internal/slack/formatter/formatter_test.go`
- **Slack Block Kit**: [Official Block Kit Documentation](https://api.slack.com/block-kit)
- **Slack mrkdwn**: [Slack Formatting Reference](https://api.slack.com/reference/surfaces/formatting)

The Slack formatting system is production-ready and handles all common use cases for rich message formatting in Slack, whether using simple mrkdwn or rich Block Kit layouts.
