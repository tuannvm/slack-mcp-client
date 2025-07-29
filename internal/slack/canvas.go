package slackbot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/slack-go/slack"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
)

// CanvasTool implements the canvas creation and editing functionality for Slack
type CanvasTool struct {
	client *slack.Client
	logger *logging.Logger
}

// NewCanvasTool creates a new canvas tool instance
func NewCanvasTool(client *slack.Client, logger *logging.Logger) *CanvasTool {
	return &CanvasTool{
		client: client,
		logger: logger,
	}
}

// CreateCanvasToolInfo returns the tool info for canvas creation
func (ct *CanvasTool) CreateCanvasToolInfo() mcp.ToolInfo {
	return mcp.ToolInfo{
		ServerName:      "slack-native",
		ToolName:        "canvas_create",
		ToolDescription: "Create a new Slack canvas with markdown content. Returns canvas_id, url, and metadata. In channels, creates a channel canvas. In DMs, creates a standalone canvas that can be shared later.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title": map[string]interface{}{
					"type":        "string",
					"description": "Title of the canvas (used for standalone canvases)",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "Markdown content for the canvas",
				},
				"channel_id": map[string]interface{}{
					"type":        "string",
					"description": "Channel ID where the canvas will be created (automatically provided via SLACK_CHANNEL_ID context)",
				},
			},
			"required": []string{"content"},
		},
		Client: ct,
	}
}

// EditCanvasToolInfo returns the tool info for canvas editing
func (ct *CanvasTool) EditCanvasToolInfo() mcp.ToolInfo {
	return mcp.ToolInfo{
		ServerName:      "slack-native",
		ToolName:        "canvas_edit",
		ToolDescription: "Edit an existing Slack canvas with new content or changes",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"canvas_id": map[string]interface{}{
					"type":        "string",
					"description": "ID of the canvas to edit",
				},
				"operation": map[string]interface{}{
					"type":        "string",
					"description": "Edit operation: 'replace' (replace entire content or section), 'insert_at_end' (append content), 'insert_at_start' (prepend content), 'insert_after' (insert after section), 'insert_before' (insert before section)",
					"enum":        []string{"replace", "insert_at_end", "insert_at_start", "insert_after", "insert_before"},
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "New markdown content",
				},
				"section_id": map[string]interface{}{
					"type":        "string",
					"description": "Section ID for targeted operations (required for insert_after, insert_before, optional for replace)",
				},
			},
			"required": []string{"canvas_id", "operation", "content"},
		},
		Client: ct,
	}
}

// SectionsLookupToolInfo returns the tool info for canvas sections lookup
func (ct *CanvasTool) SectionsLookupToolInfo() mcp.ToolInfo {
	return mcp.ToolInfo{
		ServerName:      "slack-native",
		ToolName:        "canvas_sections_lookup",
		ToolDescription: "Find sections in a canvas matching specified criteria (headers, text content)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"canvas_id": map[string]interface{}{
					"type":        "string",
					"description": "ID of the canvas to search",
				},
				"section_types": map[string]interface{}{
					"type":        "array",
					"description": "Types of sections to find: 'h1', 'h2', 'h3', 'any_header'",
					"items": map[string]interface{}{
						"type": "string",
						"enum": []string{"h1", "h2", "h3", "any_header"},
					},
				},
				"contains_text": map[string]interface{}{
					"type":        "string",
					"description": "Find sections containing this text",
				},
			},
			"required": []string{"canvas_id"},
		},
		Client: ct,
	}
}

// CallTool implements the MCPClientInterface for canvas operations
func (ct *CanvasTool) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	switch toolName {
	case "canvas_create":
		return ct.createCanvas(ctx, args)
	case "canvas_edit":
		return ct.editCanvas(ctx, args)
	case "canvas_sections_lookup":
		return ct.lookupSections(ctx, args)
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

// createCanvas handles canvas creation
func (ct *CanvasTool) createCanvas(ctx context.Context, args map[string]interface{}) (string, error) {
	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("content is required and must be a string")
	}

	title, _ := args["title"].(string)
	channelID, _ := args["channel_id"].(string)

	// Create DocumentContent with markdown
	docContent := slack.DocumentContent{
		Type:     "markdown",
		Markdown: ct.formatMarkdownForCanvas(content),
	}

	var canvasID string
	var err error
	var isDM bool

	// Check if this is a DM channel (starts with D)
	if channelID != "" && strings.HasPrefix(channelID, "D") {
		isDM = true
		ct.logger.InfoKV("Detected DM channel, will create standalone canvas", "channel_id", channelID)
	}

	// If channel ID is provided and it's not a DM, create a channel canvas
	if channelID != "" && !isDM {
		// Create canvas directly in the channel
		canvasID, err = ct.client.CreateChannelCanvasContext(ctx, channelID, docContent)
		if err != nil {
			ct.logger.ErrorKV("Failed to create channel canvas", "error", err, "channel_id", channelID)
			return "", fmt.Errorf("failed to create channel canvas: %w", err)
		}
		ct.logger.InfoKV("Channel canvas created", "canvas_id", canvasID, "channel_id", channelID)
	} else {
		// Create standalone canvas (for DMs or when no channel specified)
		canvasID, err = ct.client.CreateCanvasContext(ctx, title, docContent)
		if err != nil {
			ct.logger.ErrorKV("Failed to create canvas", "error", err)
			return "", fmt.Errorf("failed to create canvas: %w", err)
		}
		ct.logger.InfoKV("Standalone canvas created", "canvas_id", canvasID, "title", title)
	}

	// Try to get file info to retrieve permalink
	var canvasURL string
	file, _, _, err := ct.client.GetFileInfoContext(ctx, canvasID, 0, 0)
	if err != nil {
		ct.logger.ErrorKV("Failed to get canvas file info", "error", err, "canvas_id", canvasID)
		// Construct URL manually if we can't get file info
		// Format: https://[workspace].slack.com/docs/[team_id]/[file_id]
		// We'll need to provide instructions since we can't get the exact URL
	} else if file != nil {
		if file.Permalink != "" {
			canvasURL = file.Permalink
		} else if file.URLPrivate != "" {
			canvasURL = file.URLPrivate
		}
		ct.logger.InfoKV("Got canvas info", "permalink", file.Permalink, "url_private", file.URLPrivate)
	}

	result := map[string]interface{}{
		"canvas_id": canvasID,
		"status":    "created",
	}
	
	if title != "" {
		result["title"] = title
	}
	
	if canvasURL != "" {
		result["url"] = canvasURL
	}
	
	if channelID != "" {
		result["channel_id"] = channelID
	}
	
	// Add metadata about canvas type for LLM to understand
	if isDM {
		result["canvas_type"] = "standalone"
		result["created_in_dm"] = true
	} else if channelID != "" {
		result["canvas_type"] = "channel"
	} else {
		result["canvas_type"] = "standalone"
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

// editCanvas handles canvas editing
func (ct *CanvasTool) editCanvas(ctx context.Context, args map[string]interface{}) (string, error) {
	canvasID, ok := args["canvas_id"].(string)
	if !ok {
		return "", fmt.Errorf("canvas_id is required and must be a string")
	}

	operation, ok := args["operation"].(string)
	if !ok {
		return "", fmt.Errorf("operation is required and must be a string")
	}

	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("content is required and must be a string")
	}

	// Get optional section_id
	sectionID, _ := args["section_id"].(string)

	// Build the changes array based on operation
	var changes []slack.CanvasChange
	formattedContent := ct.formatMarkdownForCanvas(content)

	// Create DocumentContent for the new content
	docContent := slack.DocumentContent{
		Type:     "markdown",
		Markdown: formattedContent,
	}

	// Create the change object
	change := slack.CanvasChange{
		Operation:       operation,
		DocumentContent: docContent,
	}

	// Add section_id if provided and required
	switch operation {
	case "replace":
		// Section ID is optional for replace - if provided, replaces that section only
		if sectionID != "" {
			change.SectionID = sectionID
		}
	case "insert_at_end", "insert_at_start":
		// These operations don't need section_id
	case "insert_after", "insert_before":
		// These operations require section_id
		if sectionID == "" {
			return "", fmt.Errorf("%s operation requires section_id parameter", operation)
		}
		change.SectionID = sectionID
	default:
		return "", fmt.Errorf("unsupported operation: %s", operation)
	}

	changes = append(changes, change)

	// Edit canvas using Slack API
	params := slack.EditCanvasParams{
		CanvasID: canvasID,
		Changes:  changes,
	}

	err := ct.client.EditCanvasContext(ctx, params)
	if err != nil {
		ct.logger.ErrorKV("Failed to edit canvas", "error", err, "canvas_id", canvasID)
		return "", fmt.Errorf("failed to edit canvas: %w", err)
	}

	result := map[string]string{
		"canvas_id": canvasID,
		"status":    "edited",
		"operation": operation,
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

// lookupSections handles canvas sections lookup
func (ct *CanvasTool) lookupSections(ctx context.Context, args map[string]interface{}) (string, error) {
	canvasID, ok := args["canvas_id"].(string)
	if !ok {
		return "", fmt.Errorf("canvas_id is required and must be a string")
	}

	// Build criteria for sections lookup
	criteria := make(map[string]interface{})

	// Handle section_types array
	if sectionTypes, ok := args["section_types"].([]interface{}); ok {
		types := make([]string, len(sectionTypes))
		for i, t := range sectionTypes {
			if str, ok := t.(string); ok {
				types[i] = str
			}
		}
		if len(types) > 0 {
			criteria["section_types"] = types
		}
	}

	// Handle contains_text
	if containsText, ok := args["contains_text"].(string); ok && containsText != "" {
		criteria["contains_text"] = containsText
	}

	// If no criteria specified, return error
	if len(criteria) == 0 {
		return "", fmt.Errorf("at least one search criteria (section_types or contains_text) must be provided")
	}

	// Build LookupCanvasSectionsParams
	lookupCriteria := slack.LookupCanvasSectionsCriteria{}
	
	if sectionTypes, ok := criteria["section_types"].([]string); ok {
		lookupCriteria.SectionTypes = sectionTypes
	}
	
	if containsText, ok := criteria["contains_text"].(string); ok {
		lookupCriteria.ContainsText = containsText
	}
	
	params := slack.LookupCanvasSectionsParams{
		CanvasID: canvasID,
		Criteria: lookupCriteria,
	}
	
	// Call Slack API to lookup sections
	sections, err := ct.client.LookupCanvasSectionsContext(ctx, params)
	if err != nil {
		ct.logger.ErrorKV("Failed to lookup canvas sections", "error", err, "canvas_id", canvasID)
		return "", fmt.Errorf("failed to lookup canvas sections: %w", err)
	}

	// Build result
	result := map[string]interface{}{
		"canvas_id": canvasID,
		"sections":  sections,
		"count":     len(sections),
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

// formatMarkdownForCanvas ensures the content is properly formatted for Slack canvas
func (ct *CanvasTool) formatMarkdownForCanvas(content string) string {
	// Slack canvases support standard markdown
	// Ensure proper line endings
	return strings.ReplaceAll(content, "\r\n", "\n")
}

// Additional MCPClientInterface methods (not used for native tools)
func (ct *CanvasTool) Initialize(ctx context.Context) error {
	return nil
}

func (ct *CanvasTool) Cleanup() error {
	return nil
}