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
		ToolDescription: "Create a new Slack canvas with markdown content",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title": map[string]interface{}{
					"type":        "string",
					"description": "Title of the canvas",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "Markdown content for the canvas",
				},
				"channel_id": map[string]interface{}{
					"type":        "string",
					"description": "Channel ID where the canvas will be created (optional)",
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
					"description": "Edit operation: 'replace' (replace entire content), 'insert_at_end' (append content), 'insert_at_start' (prepend content)",
					"enum":        []string{"replace", "insert_at_end", "insert_at_start"},
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "New markdown content",
				},
				"old_content": map[string]interface{}{
					"type":        "string",
					"description": "Old content to replace (only for 'replace' operation when replacing specific text)",
				},
			},
			"required": []string{"canvas_id", "operation", "content"},
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

	// If channel ID is provided, create a channel canvas
	if channelID != "" {
		// Create canvas directly in the channel
		canvasID, err = ct.client.CreateChannelCanvasContext(ctx, channelID, docContent)
		if err != nil {
			ct.logger.ErrorKV("Failed to create channel canvas", "error", err, "channel_id", channelID)
			return "", fmt.Errorf("failed to create channel canvas: %w", err)
		}
		ct.logger.InfoKV("Channel canvas created", "canvas_id", canvasID, "channel_id", channelID)
	} else {
		// Create standalone canvas
		canvasID, err = ct.client.CreateCanvasContext(ctx, title, docContent)
		if err != nil {
			ct.logger.ErrorKV("Failed to create canvas", "error", err)
			return "", fmt.Errorf("failed to create canvas: %w", err)
		}
		ct.logger.InfoKV("Standalone canvas created", "canvas_id", canvasID, "title", title)
	}

	result := map[string]interface{}{
		"canvas_id": canvasID,
		"status":    "created",
	}
	
	if title != "" {
		result["title"] = title
	}
	
	if channelID != "" {
		result["channel_id"] = channelID
		result["message"] = fmt.Sprintf("Canvas created successfully in channel %s! The canvas will appear in the channel.", channelID)
	} else {
		result["message"] = fmt.Sprintf("Canvas created with ID: %s. You can find it in your Slack workspace.", canvasID)
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

	// Build the changes array based on operation
	var changes []slack.CanvasChange
	formattedContent := ct.formatMarkdownForCanvas(content)

	// Create DocumentContent for the new content
	docContent := slack.DocumentContent{
		Type:     "markdown",
		Markdown: formattedContent,
	}

	switch operation {
	case "replace":
		// Replace entire canvas content
		// The Slack Canvas API typically uses operations at section level
		// For replacing entire content, we would need to know the section IDs
		// This is a simplified implementation
		changes = append(changes, slack.CanvasChange{
			Operation:       "replace",
			DocumentContent: docContent,
		})
	case "insert_at_end":
		changes = append(changes, slack.CanvasChange{
			Operation:       "insert_at_end",
			DocumentContent: docContent,
		})
	case "insert_at_start":
		changes = append(changes, slack.CanvasChange{
			Operation:       "insert_at_start",
			DocumentContent: docContent,
		})
	default:
		return "", fmt.Errorf("unsupported operation: %s", operation)
	}

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