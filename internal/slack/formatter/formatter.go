// Package formatter provides utilities for formatting messages for Slack
// It supports both mrkdwn (Markdown) and Block Kit structures
package formatter

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/slack-go/slack"
)

// MessageFormat represents the format of a message to be sent to Slack
type MessageFormat int

const (
	// TextFormat represents a simple text message with mrkdwn formatting
	TextFormat MessageFormat = iota
	// BlockFormat represents a message with Block Kit structures
	BlockFormat
)

// FormatOptions contains options for formatting a message
type FormatOptions struct {
	Format     MessageFormat
	ThreadTS   string
	EscapeText bool
}

// DefaultOptions returns the default formatting options
func DefaultOptions() FormatOptions {
	return FormatOptions{
		Format:     TextFormat,
		ThreadTS:   "",
		EscapeText: true,
	}
}

// BlockOptions contains options for Block Kit messages
type BlockOptions struct {
	HeaderText string
	Fields     []Field
	Actions    []Action
}

// Field represents a field in a section block
type Field struct {
	Title string
	Value string
}

// Action represents an action button
type Action struct {
	Text string
	URL  string
}

// FormatMessage formats a message for Slack based on the provided options
func FormatMessage(text string, options FormatOptions) []slack.MsgOption {
	var msgOptions []slack.MsgOption

	if options.ThreadTS != "" {
		msgOptions = append(msgOptions, slack.MsgOptionTS(options.ThreadTS))
	}

	switch options.Format {
	case BlockFormat:
		// Parse the text as JSON Block Kit format
		var blockMessage struct {
			Text   string        `json:"text"`
			Blocks []interface{} `json:"blocks"`
		}

		if err := json.Unmarshal([]byte(text), &blockMessage); err == nil {
			// Successfully parsed as Block Kit JSON
			var blocks slack.Blocks
			// Convert the generic blocks to slack.Block objects
			for _, block := range blockMessage.Blocks {
				blockJSON, err := json.Marshal(block)
				if err != nil {
					continue
				}

				// Parse the block based on its type
				var blockMap map[string]interface{}
				if err := json.Unmarshal(blockJSON, &blockMap); err != nil {
					continue
				}

				blockType, ok := blockMap["type"].(string)
				if !ok {
					continue
				}

				var slackBlock slack.Block
				switch blockType {
				case "section":
					var section slack.SectionBlock
					if err := json.Unmarshal(blockJSON, &section); err == nil {
						slackBlock = section
					}
				case "header":
					var header slack.HeaderBlock
					if err := json.Unmarshal(blockJSON, &header); err == nil {
						slackBlock = header
					}
				case "actions":
					var actions slack.ActionBlock
					if err := json.Unmarshal(blockJSON, &actions); err == nil {
						slackBlock = actions
					}
				case "divider":
					slackBlock = slack.NewDividerBlock()
				case "context":
					var context slack.ContextBlock
					if err := json.Unmarshal(blockJSON, &context); err == nil {
						slackBlock = context
					}
				case "image":
					var image slack.ImageBlock
					if err := json.Unmarshal(blockJSON, &image); err == nil {
						slackBlock = image
					}
					// Add more block types as needed
				}

				if slackBlock != nil {
					blocks.BlockSet = append(blocks.BlockSet, slackBlock)
				}
			}

			if len(blocks.BlockSet) > 0 {
				// Create fallback text in case blocks fail
				fallbackText := blockMessage.Text
				if fallbackText == "" {
					// If no fallback text provided, use the original text
					fallbackText = text
				}

				// Add the blocks first, then the fallback text
				msgOptions = append(msgOptions, slack.MsgOptionBlocks(blocks.BlockSet...))
				msgOptions = append(msgOptions, slack.MsgOptionText(fallbackText, false))
			} else {
				// Failed to parse blocks, fall back to text
				msgOptions = append(msgOptions, slack.MsgOptionText(text, options.EscapeText))
			}
		} else {
			// Not valid JSON, treat as text
			msgOptions = append(msgOptions, slack.MsgOptionText(text, options.EscapeText))
		}
	case TextFormat:
		// Simple text message with mrkdwn
		msgOptions = append(msgOptions, slack.MsgOptionText(text, options.EscapeText))
	}

	return msgOptions
}

// CreateBlockMessage creates a Block Kit message with the given options
func CreateBlockMessage(text string, blockOptions BlockOptions) string {
	blocks := []map[string]interface{}{}

	// Add header if provided
	if blockOptions.HeaderText != "" {
		// Truncate header text if too long (Slack has a 150 char limit for plain_text)
		headerText := blockOptions.HeaderText
		if len(headerText) > 150 {
			headerText = headerText[:147] + "..."
		}

		blocks = append(blocks, map[string]interface{}{
			"type": "header",
			"text": map[string]interface{}{
				"type": "plain_text",
				"text": headerText,
			},
		})
	}

	// Add fields if provided
	if len(blockOptions.Fields) > 0 {
		// Slack has a limit of 10 fields per section
		// Split fields into multiple sections if needed
		for i := 0; i < len(blockOptions.Fields); i += 10 {
			end := i + 10
			if end > len(blockOptions.Fields) {
				end = len(blockOptions.Fields)
			}

			fields := []map[string]interface{}{}
			for _, field := range blockOptions.Fields[i:end] {
				// Truncate field text if too long (Slack has a 2000 char limit for text fields)
				fieldValue := field.Value
				if len(fieldValue) > 2000 {
					fieldValue = fieldValue[:1997] + "..."
				}

				fields = append(fields, map[string]interface{}{
					"type": "mrkdwn",
					"text": fmt.Sprintf("*%s*\n%s", field.Title, fieldValue),
				})
			}

			blocks = append(blocks, map[string]interface{}{
				"type":   "section",
				"fields": fields,
			})
		}
	}

	// Add text section if provided
	if text != "" {
		// Truncate text if too long (Slack has a 3000 char limit for text blocks)
		sectionText := text
		if len(sectionText) > 3000 {
			sectionText = sectionText[:2997] + "..."
		}

		blocks = append(blocks, map[string]interface{}{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": sectionText,
			},
		})
	}

	// Add actions if provided
	if len(blockOptions.Actions) > 0 {
		// Slack has a limit of 5 elements in an actions block
		actionCount := len(blockOptions.Actions)
		if actionCount > 5 {
			actionCount = 5
		}

		elements := []map[string]interface{}{}
		for i := 0; i < actionCount; i++ {
			action := blockOptions.Actions[i]

			// Truncate button text if too long (Slack has a 75 char limit for button text)
			buttonText := action.Text
			if len(buttonText) > 75 {
				buttonText = buttonText[:72] + "..."
			}

			elements = append(elements, map[string]interface{}{
				"type": "button",
				"text": map[string]interface{}{
					"type": "plain_text",
					"text": buttonText,
				},
				"url": action.URL,
			})
		}

		blocks = append(blocks, map[string]interface{}{
			"type":     "actions",
			"elements": elements,
		})
	}

	// Create the final message
	message := map[string]interface{}{
		"text":   text, // Fallback text
		"blocks": blocks,
	}

	// Convert to JSON
	jsonBytes, err := json.Marshal(message)
	if err != nil {
		return text // Fallback to plain text if JSON marshaling fails
	}

	return string(jsonBytes)
}

// FormatMarkdown formats text using Slack's mrkdwn syntax
func FormatMarkdown(text string) string {
	// Convert quoted strings to code blocks for better visualization
	text = ConvertQuotedStringsToCode(text)

	// Replace standard Markdown bold (**text**) with Slack bold (*text*)
	boldPattern := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	text = boldPattern.ReplaceAllString(text, "*$1*")

	// Replace standard Markdown block quotes (>) with Slack block quotes (>)
	quotePattern := regexp.MustCompile(`(?m)^\s*>\s+(.*)$`)
	text = quotePattern.ReplaceAllString(text, "> $1")

	return text
}

// ConvertQuotedStringsToCode converts double-quoted strings to inline code blocks
// for better visualization in Slack
func ConvertQuotedStringsToCode(text string) string {
	// Regex to find double-quoted strings
	// This pattern looks for "..." but avoids matching escaped quotes \"...\"
	pattern := regexp.MustCompile(`"([^"\\]*(\\.[^"\\]*)*)"`)

	// Replace each match with a code block
	text = pattern.ReplaceAllString(text, "`$1`")

	// Also handle specific patterns like "yyyy-MM-ddTHH:mm:ssZ" timestamps
	// which are common in Kubernetes and other outputs
	timestampPattern := regexp.MustCompile(`"(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)"`)
	text = timestampPattern.ReplaceAllString(text, "`$1`")

	// Handle quoted namespace names and other identifiers
	identifierPattern := regexp.MustCompile(`"([\w-]+)"`)
	text = identifierPattern.ReplaceAllString(text, "`$1`")

	return text
}

// EscapeMarkdown escapes special characters in Markdown
func EscapeMarkdown(text string) string {
	// Escape &, <, and >
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

// BoldText formats text as bold
func BoldText(text string) string {
	return fmt.Sprintf("*%s*", text)
}

// ItalicText formats text as italic
func ItalicText(text string) string {
	return fmt.Sprintf("_%s_", text)
}

// StrikethroughText formats text with strikethrough
func StrikethroughText(text string) string {
	return fmt.Sprintf("~%s~", text)
}

// CodeText formats text as inline code
func CodeText(text string) string {
	return fmt.Sprintf("`%s`", text)
}

// CodeBlock formats text as a code block
func CodeBlock(text, language string) string {
	if language != "" {
		return fmt.Sprintf("```%s\n%s\n```", language, text)
	}
	return fmt.Sprintf("```\n%s\n```", text)
}

// QuoteText formats text as a quote
func QuoteText(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n")
}

// BulletList creates a bulleted list from items
func BulletList(items []string) string {
	var result strings.Builder
	for _, item := range items {
		result.WriteString("• " + item + "\n")
	}
	return result.String()
}

// NumberedList creates a numbered list from items
func NumberedList(items []string) string {
	var result strings.Builder
	for i, item := range items {
		result.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
	}
	return result.String()
}

// Link creates a Slack link
func Link(url, text string) string {
	if text == "" {
		return url
	}
	return fmt.Sprintf("<%s|%s>", url, text)
}

// UserMention creates a user mention
func UserMention(userID string) string {
	return fmt.Sprintf("<@%s>", userID)
}

// ChannelMention creates a channel mention
func ChannelMention(channelID, channelName string) string {
	if channelName == "" {
		return fmt.Sprintf("<#%s>", channelID)
	}
	return fmt.Sprintf("<#%s|%s>", channelID, channelName)
}

// EmailLink creates an email link
func EmailLink(email, text string) string {
	if text == "" {
		return fmt.Sprintf("<mailto:%s>", email)
	}
	return fmt.Sprintf("<mailto:%s|%s>", email, text)
}
