package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/tuannvm/slack-mcp-client/internal/mcp"
)

// LLMMCPBridge provides a bridge between LLM responses and MCP tool calls.
// It detects when an LLM response should trigger a tool call and executes it.
type LLMMCPBridge struct {
	mcpClient *mcp.Client
	logger    *log.Logger
}

// NewLLMMCPBridge creates a new bridge between LLM and MCP.
func NewLLMMCPBridge(mcpClient *mcp.Client, logger *log.Logger) *LLMMCPBridge {
	return &LLMMCPBridge{
		mcpClient: mcpClient,
		logger:    logger,
	}
}

// ProcessLLMResponse processes an LLM response and detects if it should trigger an MCP tool call.
// If a tool call is detected, it executes the tool and returns the result.
// If no tool call is detected, it returns the original LLM response.
func (b *LLMMCPBridge) ProcessLLMResponse(ctx context.Context, llmResponse, userPrompt string) (string, error) {
	// Check for JSON tool call format
	if toolCall := b.detectJSONToolCall(llmResponse); toolCall != nil {
		b.logger.Printf("Detected JSON tool call: %v", toolCall)
		return b.executeToolCall(ctx, toolCall)
	}

	// Check for filesystem list requests
	if path := b.detectFilesystemListRequest(userPrompt, llmResponse); path != "" {
		b.logger.Printf("Detected filesystem list request for path: %s", path)
		return b.executeFilesystemList(ctx, path)
	}

	// Add more tool detection patterns as needed

	// No tool call detected, return original response
	return llmResponse, nil
}

// Tool represents a detected tool call
type Tool struct {
	Name   string                 `json:"tool"`
	Action string                 `json:"action"`
	Args   map[string]interface{} `json:"args"`
}

// detectJSONToolCall attempts to find and parse a JSON tool call in the LLM response
func (b *LLMMCPBridge) detectJSONToolCall(response string) *Tool {
	// Look for JSON patterns like {"tool": "...", "action": "...", "args": {...}}
	jsonRegex := regexp.MustCompile(`\{[\s\S]*"tool"[\s\S]*\}`)
	match := jsonRegex.FindString(response)
	
	if match != "" {
		var tool Tool
		if err := json.Unmarshal([]byte(match), &tool); err == nil && tool.Name != "" {
			return &tool
		}
	}
	return nil
}

// detectFilesystemListRequest looks for patterns indicating the user wants to list files
func (b *LLMMCPBridge) detectFilesystemListRequest(userPrompt, llmResponse string) string {
	// Common patterns for filesystem list requests
	patterns := []string{
		`(?i)what(?:'s| is) (?:in|available in|inside) ([/\w\s\.-]+)`,
		`(?i)list (?:the )?(?:files|contents) (?:in|of|from) ([/\w\s\.-]+)`,
		`(?i)show (?:me )?(?:the )?(?:files|contents) (?:in|of|from) ([/\w\s\.-]+)`,
	}

	// First check user prompt (higher priority)
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(userPrompt)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	// Then check LLM response
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(llmResponse)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	return ""
}

// executeToolCall executes a detected tool call
func (b *LLMMCPBridge) executeToolCall(ctx context.Context, tool *Tool) (string, error) {
	switch tool.Name {
	case "filesystem":
		switch tool.Action {
		case "list":
			path, ok := tool.Args["path"].(string)
			if !ok {
				return "", fmt.Errorf("invalid path argument for filesystem list action")
			}
			return b.executeFilesystemList(ctx, path)
		}
	}

	return "", fmt.Errorf("unsupported tool or action: %s.%s", tool.Name, tool.Action)
}

// executeFilesystemList executes a filesystem list tool call
func (b *LLMMCPBridge) executeFilesystemList(ctx context.Context, path string) (string, error) {
	if b.mcpClient == nil {
		return "", fmt.Errorf("MCP client is not available")
	}

	b.logger.Printf("Calling MCP filesystem tool for path: %s", path)
	
	// Call the filesystem tool with the CallTool method
	result, err := b.mcpClient.CallTool(ctx, "filesystem", map[string]interface{}{
		"action": "list",
		"path":   path,
	})
	if err != nil {
		b.logger.Printf("Error calling MCP filesystem tool: %v", err)
		return "", fmt.Errorf("failed to list files: %w", err)
	}

	b.logger.Printf("Successfully retrieved file listing from MCP server")
	return fmt.Sprintf("Files in %s:\n%s", path, result), nil
}