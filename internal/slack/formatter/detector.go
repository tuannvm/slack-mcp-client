// Package formatter provides utilities for formatting messages for Slack
package formatter

import (
	"encoding/json"
	"regexp"
	"strings"
)

// MessageType represents the detected type of a message
type MessageType int

const (
	// PlainText is a simple text message
	PlainText MessageType = iota
	// MarkdownText is a message with markdown formatting
	MarkdownText
	// JSONBlock is a message with Block Kit JSON structure
	JSONBlock
	// StructuredData is a message with structured data that should be formatted as blocks
	StructuredData
)

// DetectMessageType analyzes the content and determines the appropriate message type
func DetectMessageType(content string) MessageType {
	// Check if it's valid JSON Block Kit format
	if isValidBlockKit(content) {
		return JSONBlock
	}

	// Check if it contains structured data patterns
	if containsStructuredData(content) {
		return StructuredData
	}

	// Check if it contains markdown formatting
	if containsMarkdown(content) {
		return MarkdownText
	}

	// Default to plain text
	return PlainText
}

// isValidBlockKit checks if the content is a valid Block Kit JSON
func isValidBlockKit(content string) bool {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "{") || !strings.HasSuffix(content, "}") {
		return false
	}

	var blockMessage struct {
		Blocks []interface{} `json:"blocks"`
	}

	err := json.Unmarshal([]byte(content), &blockMessage)
	return err == nil && len(blockMessage.Blocks) > 0
}

// containsStructuredData checks if the content contains patterns that suggest structured data
func containsStructuredData(content string) bool {
	// Look for patterns like "Status: Success", "Result: Passed", etc.
	statusPattern := regexp.MustCompile(`(?i)(status|result|state|build|job)[\s]*:[\s]*\w+`)
	if statusPattern.MatchString(content) {
		return true
	}

	// Look for bullet points or numbered lists
	listPattern := regexp.MustCompile(`(?m)^[\s]*[•\-\*\d][\s\.\)][\s]+\w+`)
	if listPattern.MatchString(content) {
		return true
	}

	// Look for key-value pairs
	kvPattern := regexp.MustCompile(`(?m)^[\s]*[\w\s]+:[\s]*[\w\s]+$`)
	matches := kvPattern.FindAllString(content, -1)
	if len(matches) >= 3 { // If we have at least 3 key-value pairs, consider it structured
		return true
	}

	return false
}

// containsMarkdown checks if the content contains markdown formatting
func containsMarkdown(content string) bool {
	// Check for bold, italic, code blocks, etc.
	markdownPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\*[^*\n]+\*`),                 // Bold
		regexp.MustCompile(`_[^_\n]+_`),                   // Italic
		regexp.MustCompile("```[\\s\\S]*?```"),            // Code block
		regexp.MustCompile("`[^`\n]+`"),                   // Inline code
		regexp.MustCompile(`(?m)^>[\s].+$`),               // Block quote
		regexp.MustCompile(`(?m)^[\s]*[•\-\*][\s]+\w+`),   // Bullet list
		regexp.MustCompile(`(?m)^[\s]*\d+\.[\s]+\w+`),     // Numbered list
		regexp.MustCompile(`<https?://[^|>]+(\|[^>]+)?>`), // Links
	}

	for _, pattern := range markdownPatterns {
		if pattern.MatchString(content) {
			return true
		}
	}

	return false
}

// ExtractStructuredData extracts key-value pairs from structured content
func ExtractStructuredData(content string) map[string]string {
	result := make(map[string]string)

	// Extract key-value pairs using regex
	kvPattern := regexp.MustCompile(`(?m)^[\s]*([^:]+):[\s]*(.+)$`)
	matches := kvPattern.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			key := strings.TrimSpace(match[1])
			value := strings.TrimSpace(match[2])
			result[key] = value
		}
	}

	return result
}

// FormatStructuredData formats structured data as a Block Kit message
func FormatStructuredData(content string) string {
	data := ExtractStructuredData(content)
	if len(data) == 0 {
		return content // Return original content if no structured data found
	}

	// Create fields for Block Kit
	var fields []Field
	var headerText string

	// Check for a title/header in the data
	titleKeys := []string{"Title", "Header", "Subject", "Name"}
	for _, key := range titleKeys {
		if value, exists := data[key]; exists {
			headerText = value
			delete(data, key) // Remove from data to avoid duplication
			break
		}
	}

	// Convert remaining data to fields
	for key, value := range data {
		fields = append(fields, Field{
			Title: key,
			Value: value,
		})
	}

	// Create Block Kit message
	blockOptions := BlockOptions{
		HeaderText: headerText,
		Fields:     fields,
	}

	return CreateBlockMessage(content, blockOptions)
}
