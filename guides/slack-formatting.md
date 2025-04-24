# Slack-Formatted Output Guide

This guide explains how to use the new Slack-formatted output feature in the Slack MCP Client.

## Overview

The Slack MCP Client now supports rich message formatting using Slack's Block Kit and Markdown (mrkdwn) syntax. This allows for more visually appealing and interactive messages in Slack.

## Features

### 1. Automatic Format Detection

The client automatically detects the format of messages and applies the appropriate formatting:

- **Plain Text**: Simple text messages
- **Markdown Text**: Messages with Markdown formatting (bold, italic, code blocks, etc.)
- **JSON Block Kit**: Messages in Block Kit JSON format
- **Structured Data**: Messages with key-value pairs that are automatically converted to Block Kit format

### 2. Markdown Formatting

You can use Slack's Markdown syntax in your messages. The formatter automatically converts standard Markdown syntax to Slack's mrkdwn format. For a comprehensive mapping between standard Markdown and Slack's supported syntax, see the [Markdown-Slack Mapping Guide](./markdown-slack-mapping.md).

- **Bold**: 
  - Standard Markdown: `**bold text**` 
  - Slack Markdown: `*bold text*` (automatically converted)
- **Italic**: 
  - Standard Markdown: `*italic text*` or `_italic text_` 
  - Slack Markdown: `_italic text_` (automatically converted)
- **Strikethrough**: 
  - Standard Markdown: `~~struck text~~` 
  - Slack Markdown: `~struck text~` (automatically converted)
- **Code**: `` `code` `` (same in both)
- **Code Block**:
  ```
  ```language
  code block
  ```
  ```
- **Block Quote**:
  ```
  > This is a quote
  > Multi-line quote
  ```
- **Lists**:
  ```
  • Bullet item (or - Bullet item)
  • Another bullet item
  
  1. Numbered item
  2. Another numbered item
  ```
- **Links**:
  - Standard Markdown: `[link text](https://example.com)` 
  - Slack Markdown: `<https://example.com|link text>` (automatically converted)

#### Automatic Quoted String Conversion

The formatter automatically converts double-quoted strings to inline code blocks for better visualization. For example:

```
All of these were created on "2020-11-17T05:07:52Z" or "2020-11-17T05:07:54Z".
Among them, "kube-node-lease", "kube-public", and "kube-system" share the exact
same creation timestamp: "2020-11-17T05:07:52Z".
```

Will be displayed as:

```
All of these were created on `2020-11-17T05:07:52Z` or `2020-11-17T05:07:54Z`.
Among them, `kube-node-lease`, `kube-public`, and `kube-system` share the exact
same creation timestamp: `2020-11-17T05:07:52Z`.
```

This makes timestamps, IDs, and other quoted values stand out better in messages.

### 3. Block Kit JSON

For more complex layouts, you can return a Block Kit JSON structure:

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

### 4. Structured Data

The client can automatically convert structured data (key-value pairs) to Block Kit format:

```
Status: Success
Duration: 5m 32s
Result: Passed
```

This will be displayed as a nicely formatted Block Kit message with fields.

## Usage in Code

The formatter package provides several utility functions for formatting messages:

```go
import "github.com/tuannvm/slack-mcp-client/internal/slack/formatter"

// Format text as bold
boldText := formatter.BoldText("Important")

// Create a code block
codeBlock := formatter.CodeBlock("var x = 10;", "javascript")

// Create a link
link := formatter.Link("http://example.com", "Example")

// Create a Block Kit message
blockOptions := formatter.BlockOptions{
    HeaderText: "Status Report",
    Fields: []formatter.Field{
        {Title: "Status", Value: "Success"},
        {Title: "Duration", Value: "5m 32s"},
    },
    Actions: []formatter.Action{
        {Text: "View Details", URL: "http://example.com"},
    },
}
blockMessage := formatter.CreateBlockMessage("Operation completed successfully", blockOptions)
```

## Implementation Details

The Slack-formatted output feature is implemented in the `internal/slack/formatter` package, which provides:

1. **Message Type Detection**: Automatically detects the type of message (plain text, markdown, JSON Block Kit, structured data)
2. **Formatting Functions**: Utility functions for formatting text with Markdown
3. **Block Kit Generation**: Functions for creating Block Kit messages
4. **Structured Data Conversion**: Automatic conversion of structured data to Block Kit format

The `postMessage` method in the Slack client has been updated to use these formatting functions.

## Examples

### Example 1: Simple Markdown

```
*Build #123* _passed_ 🎉
• Duration: 4m 12s
• Triggered by: <@U23456789>
```

### Example 2: Block Kit JSON

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

```
Title: Build Status
Status: Success
Duration: 4m 12s
Triggered By: User123
```

This will be automatically converted to a Block Kit message with a header and fields.
