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

	// Check if it's valid JSON
	err := json.Unmarshal([]byte(content), &blockMessage)
	if err != nil || len(blockMessage.Blocks) == 0 {
		return false
	}

	// Additional validation for block structure
	for _, block := range blockMessage.Blocks {
		blockJSON, err := json.Marshal(block)
		if err != nil {
			return false
		}

		var blockMap map[string]interface{}
		if err := json.Unmarshal(blockJSON, &blockMap); err != nil {
			return false
		}

		// Check if block has a type
		blockType, ok := blockMap["type"].(string)
		if !ok || blockType == "" {
			return false
		}

		// Validate specific block types
		switch blockType {
		case "section":
			// Section must have text or fields
			_, hasText := blockMap["text"]
			fields, hasFields := blockMap["fields"]
			if !hasText && !hasFields {
				return false
			}
			// If it has fields, it must be an array
			if hasFields {
				fieldsArray, ok := fields.([]interface{})
				if !ok || len(fieldsArray) == 0 || len(fieldsArray) > 10 {
					return false
				}
			}
		case "header":
			// Header must have text
			text, hasText := blockMap["text"]
			if !hasText {
				return false
			}
			// Text must be a map with type and text
			textMap, ok := text.(map[string]interface{})
			if !ok {
				return false
			}
			// Type must be plain_text
			textType, hasType := textMap["type"]
			if !hasType || textType != "plain_text" {
				return false
			}
		case "actions":
			// Actions must have elements
			elements, hasElements := blockMap["elements"]
			if !hasElements {
				return false
			}
			// Elements must be an array
			elementsArray, ok := elements.([]interface{})
			if !ok || len(elementsArray) == 0 || len(elementsArray) > 5 {
				return false
			}
		}
	}

	return true
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
// If the structured data can't be properly formatted as blocks, it falls back to text
func FormatStructuredData(content string) string {
	data := ExtractStructuredData(content)
	if len(data) == 0 {
		return content // Return original content if no structured data found
	}

	// If we have too many fields, it might cause issues with Slack's limits
	if len(data) > 10 {
		// Just apply markdown formatting instead of trying to use blocks
		return FormatMarkdown(content)
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
		// Truncate very long values to avoid Slack API limits
		if len(value) > 2000 {
			value = value[:1997] + "..."
		}

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

	// Try to create a Block Kit message
	blockMessage := CreateBlockMessage(content, blockOptions)

	// Validate the Block Kit message
	if !isValidBlockKit(blockMessage) {
		// If validation fails, fall back to simple markdown formatting
		return FormatMarkdown(content)
	}

	return blockMessage
}

// ImageInfo represents information about a markdown image
type ImageInfo struct {
	AltText string
	URL     string
}

// ExtractMarkdownImages extracts markdown image links from text
func ExtractMarkdownImages(text string) []ImageInfo {
	// Pattern to match markdown images: ![alt text](url)
	// This pattern handles URLs that might contain parentheses
	imagePattern := regexp.MustCompile(`!\[([^\]]*)\]\((https?://[^\s]+)\)`)
	matches := imagePattern.FindAllStringSubmatch(text, -1)
	
	var images []ImageInfo
	for _, match := range matches {
		if len(match) == 3 {
			url := match[2]
			// Clean up URL if it appears to be truncated or has extra characters
			url = strings.TrimSpace(url)
			
			// If URL ends with a parenthesis that's likely part of the markdown, remove it
			if strings.HasSuffix(url, ")") && strings.Count(url, "(") < strings.Count(url, ")") {
				url = url[:len(url)-1]
			}
			
			images = append(images, ImageInfo{
				AltText: match[1],
				URL:     url,
			})
		}
	}
	return images
}

// HasMarkdownImages checks if text contains markdown image links
func HasMarkdownImages(text string) bool {
	imagePattern := regexp.MustCompile(`!\[([^\]]*)\]\((https?://[^\s]+)\)`)
	return imagePattern.MatchString(text)
}

// ConvertMarkdownWithImages converts text with markdown images to Block Kit format
func ConvertMarkdownWithImages(text string) string {
	images := ExtractMarkdownImages(text)
	if len(images) == 0 {
		return text
	}

	// Remove image markdown from text
	imagePattern := regexp.MustCompile(`!\[([^\]]*)\]\((https?://[^\s]+)\)`)
	textWithoutImages := imagePattern.ReplaceAllString(text, "")
	
	// Apply markdown formatting to the remaining text
	formattedText := FormatMarkdown(textWithoutImages)
	
	// Create blocks
	blocks := []map[string]interface{}{}
	
	// Add text content if not empty
	trimmedText := strings.TrimSpace(formattedText)
	if trimmedText != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": trimmedText,
			},
		})
	}
	
	// Add image blocks
	for _, img := range images {
		// Validate URL starts with http:// or https://
		if !strings.HasPrefix(img.URL, "http://") && !strings.HasPrefix(img.URL, "https://") {
			// Skip invalid URLs
			continue
		}
		
		imageBlock := map[string]interface{}{
			"type":      "image",
			"image_url": img.URL,
			"alt_text":  img.AltText,
		}
		
		// Add title if alt text is provided and not empty
		if img.AltText != "" {
			imageBlock["title"] = map[string]interface{}{
				"type": "plain_text",
				"text": img.AltText,
			}
		}
		
		blocks = append(blocks, imageBlock)
	}
	
	// If we have no valid image blocks after validation, return original text
	if len(blocks) == 0 || (len(blocks) == 1 && blocks[0]["type"] == "section") {
		return text
	}
	
	// Create the final message
	message := map[string]interface{}{
		"text":   text, // Fallback text
		"blocks": blocks,
	}
	
	// Convert to JSON
	jsonBytes, err := json.Marshal(message)
	if err != nil {
		return text // Fallback to original text if JSON marshaling fails
	}
	
	return string(jsonBytes)
}
