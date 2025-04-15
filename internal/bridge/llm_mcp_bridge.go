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
	mcpClients     map[string]*mcp.Client // Map of MCP clients keyed by server name
	primaryClient  *mcp.Client            // Default client for tool calls
	logger         *log.Logger
	// Tool cache for quick lookup during tool call validation
	availableTools map[string]string // Map of tool name to server name
}

// NewLLMMCPBridge creates a new bridge between LLM and MCP with pre-discovered tools.
func NewLLMMCPBridge(mcpClients map[string]*mcp.Client, logger *log.Logger, discoveredTools map[string]string) *LLMMCPBridge {
	if logger == nil {
		logger = log.New(log.Writer(), "LLM_MCP_BRIDGE: ", log.LstdFlags|log.Lshortfile)
	}

	bridge := &LLMMCPBridge{
		mcpClients:     mcpClients,
		logger:         logger,
		availableTools: make(map[string]string),
	}

	// Set the primary client
	for name, client := range mcpClients {
		bridge.primaryClient = client
		bridge.logger.Printf("Set primary client to '%s'", name)
		break
	}

	// Use the pre-discovered tools instead of querying again
	if discoveredTools != nil && len(discoveredTools) > 0 {
		bridge.availableTools = discoveredTools
		bridge.logger.Printf("Using %d pre-discovered tools", len(discoveredTools))
	} else {
		// Fallback to discovering tools if none were provided
		bridge.logger.Printf("No pre-discovered tools provided, will discover dynamically")
		bridge.discoverAvailableTools()
	}

	return bridge
}

// discoverAvailableTools queries all MCP servers for available tools
func (b *LLMMCPBridge) discoverAvailableTools() {
	ctx := context.Background()
	
	for serverName, client := range b.mcpClients {
		b.logger.Printf("Discovering tools from MCP server '%s'...", serverName)
		
		tools, err := client.GetAvailableTools(ctx)
		if err != nil {
			b.logger.Printf("Warning: Failed to retrieve available tools from MCP server '%s': %v", serverName, err)
			continue
		}

		// Store the available tools in the map for quick lookup
		for _, tool := range tools {
			// If a tool is provided by multiple servers, prefer to keep the first one found
			if existingServer, exists := b.availableTools[tool]; !exists {
				b.availableTools[tool] = serverName
				b.logger.Printf("Discovered MCP tool: '%s' from server '%s'", tool, serverName)
			} else {
				b.logger.Printf("Tool '%s' already available from server '%s', ignoring from '%s'", 
					tool, existingServer, serverName)
			}
		}
	}
	
	b.logger.Printf("Total discovered tools: %d", len(b.availableTools))
}

// ProcessLLMResponse processes an LLM response and detects if it should trigger an MCP tool call.
// If a tool call is detected, it executes the tool and returns the result.
// If no tool call is detected, it returns the original LLM response.
func (b *LLMMCPBridge) ProcessLLMResponse(ctx context.Context, llmResponse, userPrompt string) (string, error) {
	b.logger.Printf("Processing LLM response for potential tool calls")
	
	// Check for JSON tool call format
	if toolCall := b.detectJSONToolCall(llmResponse); toolCall != nil {
		b.logger.Printf("Detected JSON tool call: %v", toolCall)
		return b.executeToolCall(ctx, toolCall)
	}

	// Check for natural language tool requests
	if toolRequest := b.detectNaturalLanguageToolRequest(userPrompt, llmResponse); toolRequest != nil {
		b.logger.Printf("Detected natural language tool request: %v", toolRequest)
		return b.executeToolCall(ctx, toolRequest)
	}

	// No tool call detected, return original response
	b.logger.Printf("No tool call detected in LLM response")
	return llmResponse, nil
}

// Tool represents a detected tool call
type Tool struct {
	Name   string                 `json:"tool"`
	Action string                 `json:"action,omitempty"`
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
			// Verify that this tool exists in the available tools
			if _, exists := b.availableTools[tool.Name]; exists {
				return &tool
			} else {
				b.logger.Printf("Warning: Detected tool '%s' is not available from any MCP server", tool.Name)
			}
		}
	}
	return nil
}

// detectNaturalLanguageToolRequest looks for natural language patterns that might indicate tool usage
func (b *LLMMCPBridge) detectNaturalLanguageToolRequest(userPrompt, llmResponse string) *Tool {
	// Check for filesystem list operations
	if path := b.detectFilesystemListPattern(userPrompt, llmResponse); path != "" && b.isToolAvailable("filesystem") {
		return &Tool{
			Name:   "filesystem",
			Action: "list",
			Args: map[string]interface{}{
				"path": path,
			},
		}
	}

	// Check for Ollama requests
	if params := b.detectOllamaPattern(userPrompt, llmResponse); params != nil && b.isToolAvailable("ollama") {
		args := map[string]interface{}{
			"model":  params.Model,
			"prompt": params.Prompt,
		}

		if params.Options != nil && len(params.Options) > 0 {
			args["options"] = params.Options
		}

		return &Tool{
			Name: "ollama",
			Args: args,
		}
	}

	// Check for GitHub repository operations if github tool is available
	if repo := b.detectGitHubRepoPattern(userPrompt, llmResponse); repo != "" && b.isToolAvailable("github") {
		return &Tool{
			Name: "github",
			Action: "repo-info",
			Args: map[string]interface{}{
				"repo": repo,
			},
		}
	}

	return nil
}

// isToolAvailable checks if the specified tool is available from any MCP server
func (b *LLMMCPBridge) isToolAvailable(toolName string) bool {
	_, exists := b.availableTools[toolName]
	return exists
}

// getClientForTool returns the appropriate client for a given tool
func (b *LLMMCPBridge) getClientForTool(toolName string) *mcp.Client {
	if serverName, exists := b.availableTools[toolName]; exists {
		if client, clientExists := b.mcpClients[serverName]; clientExists {
			return client
		}
	}
	// Fallback to primary client if specific client not found
	return b.primaryClient
}

// detectFilesystemListPattern looks for patterns indicating the user wants to list files
func (b *LLMMCPBridge) detectFilesystemListPattern(userPrompt, llmResponse string) string {
	// Common patterns for filesystem list requests
	patterns := []string{
		`(?i)what(?:'s| is) (?:in|available in|inside) ([/\w\s\.-]+)`,
		`(?i)list (?:the )?(?:files|contents) (?:in|of|from) ([/\w\s\.-]+)`,
		`(?i)show (?:me )?(?:the )?(?:files|contents) (?:in|of|from) ([/\w\s\.-]+)`,
		`(?i)show (?:me )?(?:the )?(?:files|contents) of ([/\w\s\.-]+)`,
		`(?i)want to see what(?:'s| is) inside ([/\w\s\.-]+)`,
		`(?i)see what(?:'s| is) inside ([/\w\s\.-]+)`,
		`(?i)show (?:me )?(?:the )? ([/\w\s\.-]+) directory`,
		`(?i)use mcp for ([/\w\s\.-]+)`,
		// Special direct path pattern to catch paths directly
		`(?i)(?:directory|folder|path)?\s*(/[/\w\s\.-]+)`,
	}

	b.logger.Printf("Checking user prompt for filesystem patterns: %s", userPrompt)
	
	// First check user prompt (higher priority)
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(userPrompt)
		if len(matches) > 1 {
			path := strings.TrimSpace(matches[1])
			b.logger.Printf("Detected filesystem request for path: %s", path)
			return path
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

// detectGitHubRepoPattern looks for patterns indicating GitHub repository requests
func (b *LLMMCPBridge) detectGitHubRepoPattern(userPrompt, llmResponse string) string {
	patterns := []string{
		`(?i)show (?:me )?(?:the )?(?:github )?repo(?:sitory)? ([a-zA-Z0-9\-_]+/[a-zA-Z0-9\-_\.]+)`,
		`(?i)get info (?:about|on) (?:the )?(?:github )?repo(?:sitory)? ([a-zA-Z0-9\-_]+/[a-zA-Z0-9\-_\.]+)`,
		`(?i)github\.com/([a-zA-Z0-9\-_]+/[a-zA-Z0-9\-_\.]+)`,
	}
	
	// First check user prompt (higher priority)
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(userPrompt)
		if len(matches) > 1 {
			repo := strings.TrimSpace(matches[1])
			b.logger.Printf("Detected GitHub repository request: %s", repo)
			return repo
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

// OllamaParams represents parameters for Ollama tool
type OllamaParams struct {
	Model   string                 `json:"model"`
	Prompt  string                 `json:"prompt"`
	Options map[string]interface{} `json:"options,omitempty"`
}

// detectOllamaPattern detects Ollama tool usage patterns
func (b *LLMMCPBridge) detectOllamaPattern(userPrompt, llmResponse string) *OllamaParams {
	// Check for explicit Ollama requests in the user prompt
	ollamaRegex := regexp.MustCompile(`(?i)use ollama(?: model)?[:\s]+([a-zA-Z0-9\-:]+)(?:[:\s]+to)?\s+(.+)`)
	matches := ollamaRegex.FindStringSubmatch(userPrompt)

	if len(matches) > 2 {
		return &OllamaParams{
			Model:  strings.TrimSpace(matches[1]),
			Prompt: strings.TrimSpace(matches[2]),
		}
	}

	// Check for JSON-formatted Ollama requests in LLM response
	jsonRegex := regexp.MustCompile(`\{[\s\S]*"model"[\s\S]*"prompt"[\s\S]*\}`)
	match := jsonRegex.FindString(llmResponse)

	if match != "" {
		var params OllamaParams
		if err := json.Unmarshal([]byte(match), &params); err == nil && params.Model != "" && params.Prompt != "" {
			return &params
		}
	}

	return nil
}

// executeToolCall executes a detected tool call
func (b *LLMMCPBridge) executeToolCall(ctx context.Context, tool *Tool) (string, error) {
	// Get the appropriate client for this tool
	client := b.getClientForTool(tool.Name)
	
	if client == nil {
		return "", fmt.Errorf("no suitable MCP client available for tool '%s'", tool.Name)
	}

	// Check if the tool is available
	serverName := b.availableTools[tool.Name]
	if serverName == "" {
		return "", fmt.Errorf("tool '%s' is not available from any MCP server", tool.Name)
	}

	b.logger.Printf("Calling MCP tool '%s' on server '%s' with args: %v", tool.Name, serverName, tool.Args)

	// Call the tool with the generic CallTool method
	result, err := client.CallTool(ctx, tool.Name, tool.Args)
	if err != nil {
		b.logger.Printf("Error calling MCP tool %s: %v", tool.Name, err)
		return "", fmt.Errorf("failed to execute tool %s: %w", tool.Name, err)
	}

	b.logger.Printf("Successfully executed MCP tool: %s", tool.Name)
	
	// Format the result for user consumption
	formattedResult := b.formatToolResult(tool.Name, result)
	return formattedResult, nil
}

// formatToolResult formats the result from a tool call for better readability
func (b *LLMMCPBridge) formatToolResult(toolName, result string) string {
	switch toolName {
	case "filesystem":
		return fmt.Sprintf("📁 **File System Results**:\n```\n%s\n```", result)
	case "github":
		return fmt.Sprintf("🔍 **GitHub Results**:\n```\n%s\n```", result)
	case "ollama":
		return fmt.Sprintf("🤖 **Ollama Results**:\n%s", result)
	default:
		return fmt.Sprintf("**Tool Results (%s)**:\n%s", toolName, result)
	}
}