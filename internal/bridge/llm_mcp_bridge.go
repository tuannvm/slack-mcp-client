package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
//	"regexp"
	"strings"

	"github.com/tuannvm/slack-mcp-client/internal/mcp"
	"github.com/tuannvm/slack-mcp-client/internal/types"
)

// LLMMCPBridge provides a bridge between LLM responses and MCP tool calls.
// It detects when an LLM response should trigger a tool call and executes it.
type LLMMCPBridge struct {
	mcpClients     map[string]*mcp.Client // Map of MCP clients keyed by server name
	primaryClient  *mcp.Client            // Default client for tool calls
	logger         *log.Logger
	// Use types.ToolInfo for the tool map
	availableTools map[string]types.ToolInfo // Map of tool name to server name
}

// NewLLMMCPBridge creates a new bridge between LLM and MCP with pre-discovered tools.
func NewLLMMCPBridge(mcpClients map[string]*mcp.Client, logger *log.Logger, discoveredTools map[string]types.ToolInfo) *LLMMCPBridge {
	if logger == nil {
		logger = log.New(log.Writer(), "LLM_MCP_BRIDGE: ", log.LstdFlags|log.Lshortfile)
	}

	bridge := &LLMMCPBridge{
		mcpClients:     mcpClients,
		logger:         logger,
		availableTools: discoveredTools,
	}

	// Set the primary client
	for name, client := range mcpClients {
		bridge.primaryClient = client
		bridge.logger.Printf("Set primary client to '%s'", name)
		break
	}

	// Log initialization details
	if bridge.availableTools == nil {
		bridge.availableTools = make(map[string]types.ToolInfo)
		logger.Printf("Initialized with %d MCP clients and NO pre-discovered tools.", len(mcpClients))
	} else {
		logger.Printf("Initialized with %d MCP clients and %d pre-discovered tools.", len(mcpClients), len(discoveredTools))
	}

	return bridge
}

// ProcessLLMResponse processes an LLM response, expecting a specific JSON tool call format.
// It no longer uses natural language detection.
func (b *LLMMCPBridge) ProcessLLMResponse(ctx context.Context, llmResponse, userPrompt string) (string, error) {
	b.logger.Printf("Processing LLM response for potential JSON tool call: %s", llmResponse)
	
	// Attempt to detect and parse the specific JSON tool call format
	if toolCall := b.detectSpecificJSONToolCall(llmResponse); toolCall != nil {
		b.logger.Printf("Detected JSON tool call: %v", toolCall)
		// Execute the tool call
		toolResult, err := b.executeToolCall(ctx, toolCall)
		if err != nil {
			// Return the error to the caller (slack client) to handle
			// It might inform the user or try to re-prompt the LLM
			b.logger.Printf("Error executing tool call: %v", err)
			return "", fmt.Errorf("Error executing tool '%s': %w", toolCall.Tool, err)
		}
		// Return the raw tool result (caller will decide how to present/re-prompt)
		b.logger.Printf("Tool '%s' executed successfully.", toolCall.Tool)
		return toolResult, nil // Indicate success by returning result and nil error
	}

	// No valid tool call JSON detected, return original LLM response
	b.logger.Printf("No valid JSON tool call detected.")
	return llmResponse, nil // Indicate no tool call by returning original response and nil error
}

// ToolCall represents the expected JSON structure for a tool call from the LLM
type ToolCall struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args"`
}

// detectSpecificJSONToolCall attempts to find and parse the *specific* JSON tool call structure.
// It validates the tool name against available tools.
func (b *LLMMCPBridge) detectSpecificJSONToolCall(response string) *ToolCall {
	// Trim whitespace and check if it looks like a JSON object
	trimmedResponse := strings.TrimSpace(response)
	if !(strings.HasPrefix(trimmedResponse, "{") && strings.HasSuffix(trimmedResponse, "}")) {
		return nil // Doesn't look like a JSON object
	}

	var toolCall ToolCall
	if err := json.Unmarshal([]byte(trimmedResponse), &toolCall); err != nil {
		b.logger.Printf("DEBUG: Failed to unmarshal potential tool call JSON: %v. JSON: %s", err, trimmedResponse)
		return nil // Not the expected JSON structure
	}

	// Check if the essential fields are present
	if toolCall.Tool == "" || toolCall.Args == nil {
		b.logger.Printf("DEBUG: Parsed JSON missing 'tool' or 'args' field.")
		return nil // Missing required fields
	}

	// *** Validate the tool name against the available tools ***
	if _, exists := b.availableTools[toolCall.Tool]; !exists {
		b.logger.Printf("Warning: LLM requested tool '%s', but it is not available.", toolCall.Tool)
		return nil // Tool not available
	}

	// If we got here, it's a valid, available tool call
	return &toolCall
}

// isToolAvailable checks if the specified tool is available (using the new map)
func (b *LLMMCPBridge) isToolAvailable(toolName string) bool {
	_, exists := b.availableTools[toolName]
	return exists
}

// getClientForTool returns the appropriate client for a given tool (using the new map)
func (b *LLMMCPBridge) getClientForTool(toolName string) *mcp.Client {
	if toolInfo, exists := b.availableTools[toolName]; exists {
		if client, clientExists := b.mcpClients[toolInfo.ServerName]; clientExists {
			return client
		} else {
			b.logger.Printf("Warning: Tool '%s' found, but its server '%s' is not in the client map.", toolName, toolInfo.ServerName)
		}
	}
	// Fallback or error handling if tool/server not found
	b.logger.Printf("Warning: Could not find a specific client for tool '%s'. No fallback configured.", toolName)
	return nil // Return nil, executeToolCall should handle this
}

// executeToolCall executes a detected tool call (using the new ToolCall struct)
func (b *LLMMCPBridge) executeToolCall(ctx context.Context, toolCall *ToolCall) (string, error) {
	client := b.getClientForTool(toolCall.Tool)
	if client == nil {
		return "", fmt.Errorf("no MCP client available for tool '%s'", toolCall.Tool)
	}

	serverName := b.availableTools[toolCall.Tool].ServerName // Get server name for logging
	b.logger.Printf("Calling MCP tool '%s' on server '%s' with args: %v", toolCall.Tool, serverName, toolCall.Args)

	// Call the tool using the generic CallTool method
	result, err := client.CallTool(ctx, toolCall.Tool, toolCall.Args)
	if err != nil {
		b.logger.Printf("Error calling MCP tool %s: %v", toolCall.Tool, err)
		return "", fmt.Errorf("failed to execute tool %s: %w", toolCall.Tool, err) // Propagate error
	}

	b.logger.Printf("Successfully executed MCP tool: %s", toolCall.Tool)
	
	// Return the raw result string - formatting/re-prompting happens in slack client
	return result, nil 
}