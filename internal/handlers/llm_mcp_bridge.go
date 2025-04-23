// Package handlers provides implementation for MCP tool handlers.
// It handles tool registration, execution, and management.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tuannvm/slack-mcp-client/internal/common"
)

// MCPClientInterface defines the interface for an MCP client
// This allows us to break the circular dependency between packages
type MCPClientInterface interface {
	CallTool(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// LLMMCPBridge provides a bridge between LLM responses and MCP tool calls.
// It detects when an LLM response should trigger a tool call and executes it.
type LLMMCPBridge struct {
	mcpClients     map[string]MCPClientInterface // Map of MCP clients keyed by server name
	logger         *log.Logger
	availableTools map[string]common.ToolInfo // Map of tool names to info about the tool
}

// NewLLMMCPBridge creates a new LLMMCPBridge with the given MCP clients and tools
func NewLLMMCPBridge(mcpClients map[string]MCPClientInterface, logger *log.Logger, discoveredTools map[string]common.ToolInfo) *LLMMCPBridge {
	return &LLMMCPBridge{
		mcpClients:     mcpClients,
		logger:         logger,
		availableTools: discoveredTools,
	}
}

// NewLLMMCPBridgeFromClients creates a new LLMMCPBridge with the given MCP Client objects
// This is a convenience function that wraps the concrete clients in the interface
func NewLLMMCPBridgeFromClients(mcpClients interface{}, logger *log.Logger, discoveredTools map[string]common.ToolInfo) *LLMMCPBridge {
	// Convert the concrete client map to the interface map
	// This is a workaround for the type system to avoid import cycles
	interfaceClients := make(map[string]MCPClientInterface)
	
	// Type assertion to get the original map
	if clientMap, ok := mcpClients.(map[string]interface{}); ok {
		for name, client := range clientMap {
			if mcpClient, ok := client.(MCPClientInterface); ok {
				interfaceClients[name] = mcpClient
			}
		}
	}
	
	return NewLLMMCPBridge(interfaceClients, logger, discoveredTools)
}

// ProcessLLMResponse processes an LLM response, expecting a specific JSON tool call format.
// It no longer uses natural language detection.
func (b *LLMMCPBridge) ProcessLLMResponse(ctx context.Context, llmResponse, _ string) (string, error) {
	// Check for a tool call in JSON format
	if toolCall := b.detectSpecificJSONToolCall(llmResponse); toolCall != nil {
		// Execute the tool call
		result, err := b.executeToolCall(ctx, toolCall)
		if err != nil {
			b.logger.Printf("Error executing tool call: %v", err)
			return fmt.Sprintf("Error executing tool call: %v", err), nil
		}
		return result, nil
	}

	// Just return the LLM response as-is if no tool call was detected
	return llmResponse, nil
}

// ToolCall represents the expected JSON structure for a tool call from the LLM
type ToolCall struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args"`
}

// detectSpecificJSONToolCall attempts to find and parse the *specific* JSON tool call structure.
// It validates the tool name against available tools.
func (b *LLMMCPBridge) detectSpecificJSONToolCall(response string) *ToolCall {
	b.logger.Printf("DEBUG: Beginning JSON tool call detection")

	// Try different parsing strategies in order
	if toolCall := b.tryDirectJSONParsing(response); toolCall != nil {
		return toolCall
	}

	if toolCall := b.tryCodeBlockJSONParsing(response); toolCall != nil {
		return toolCall
	}

	if toolCall := b.tryRegexJSONExtraction(response); toolCall != nil {
		return toolCall
	}

	b.logger.Printf("DEBUG: No valid JSON tool call detected")
	return nil
}

// tryDirectJSONParsing attempts direct JSON parsing of the entire response
func (b *LLMMCPBridge) tryDirectJSONParsing(response string) *ToolCall {
	b.logger.Printf("DEBUG: Attempting direct JSON parsing")
	response = strings.TrimSpace(response)

	var toolCall ToolCall
	if err := json.Unmarshal([]byte(response), &toolCall); err == nil {
		if b.isValidToolCall(toolCall) {
			b.logger.Printf("DEBUG: Direct JSON parsing successful")
			return &toolCall
		}
	}
	return nil
}

// tryCodeBlockJSONParsing looks for JSON in code blocks
func (b *LLMMCPBridge) tryCodeBlockJSONParsing(response string) *ToolCall {
	b.logger.Printf("DEBUG: Searching for JSON in code blocks")
	codeBlockRegex := regexp.MustCompile("```(?:json)?\\s*({[\\s\\S]*?})\\s*```")
	codeBlockMatches := codeBlockRegex.FindAllStringSubmatch(response, -1)

	for _, match := range codeBlockMatches {
		if len(match) >= 2 {
			jsonContent := match[1]
			b.logger.Printf("DEBUG: Found potential JSON in code block: %s", jsonContent)

			var toolCall ToolCall
			if err := json.Unmarshal([]byte(jsonContent), &toolCall); err == nil {
				if b.isValidToolCall(toolCall) {
					b.logger.Printf("DEBUG: JSON code block parsing successful")
					return &toolCall
				}
			} else {
				b.logger.Printf("DEBUG: JSON code block parsing failed: %v", err)
			}
		}
	}
	return nil
}

// tryRegexJSONExtraction looks for tool calls using regex patterns
func (b *LLMMCPBridge) tryRegexJSONExtraction(response string) *ToolCall {
	b.logger.Printf("DEBUG: Searching for JSON objects in text")
	// More lenient regex that can handle various JSON formats with the key elements we need
	jsonRegex := regexp.MustCompile("(?i)[\\{\\s]*[\"']?tool[\"']?\\s*[\\:\\=]\\s*[\"']([^\"']+)[\"']\\s*[\\,\\s]*[\"']?args[\"']?\\s*[\\:\\=]\\s*\\{([\\s\\S]*?)\\}[\\s\\}]*")
	jsonMatches := jsonRegex.FindAllStringSubmatch(response, -1)

	for _, match := range jsonMatches {
		if len(match) >= 3 {
			toolName := match[1]
			argsJSON := "{" + match[2] + "}" // Ensure it's wrapped in braces
			b.logger.Printf("DEBUG: Found potential JSON object: tool=%s, args=%s", toolName, argsJSON)

			// Try to construct the tool call manually
			if toolCall := b.constructToolCall(toolName, argsJSON); toolCall != nil {
				return toolCall
			}
		}
	}
	return nil
}

// constructToolCall attempts to parse JSON args and create a ToolCall struct
func (b *LLMMCPBridge) constructToolCall(toolName string, argsJSON string) *ToolCall {
	var args map[string]interface{}
	err := json.Unmarshal([]byte(argsJSON), &args)
	if err == nil {
		toolCall := &ToolCall{
			Tool: toolName,
			Args: args,
		}

		if _, exists := b.availableTools[toolCall.Tool]; exists {
			b.logger.Printf("DEBUG: Manual JSON construction successful")
			return toolCall
		}

		b.logger.Printf("Warning: LLM requested tool '%s' in regex match, but it is not available.", toolCall.Tool)
		return nil
	}

	b.logger.Printf("DEBUG: JSON args parsing failed: %v", err)

	// Try even more lenient approach - extract key-value pairs
	if simpleArgs, ok := b.extractSimpleKeyValuePairs(argsJSON); ok && len(simpleArgs) > 0 {
		toolCall := &ToolCall{
			Tool: toolName,
			Args: simpleArgs,
		}

		if _, exists := b.availableTools[toolCall.Tool]; exists {
			b.logger.Printf("DEBUG: Simplified key-value extraction successful")
			return toolCall
		}
	}

	return nil
}

// isValidToolCall validates if a tool call has the required fields and refers to an available tool
func (b *LLMMCPBridge) isValidToolCall(toolCall ToolCall) bool {
	if toolCall.Tool != "" && toolCall.Args != nil {
		if _, exists := b.availableTools[toolCall.Tool]; exists {
			return true
		}

		b.logger.Printf("Warning: LLM requested tool '%s', but it is not available.", toolCall.Tool)
	}
	return false
}

// getClientForTool returns the appropriate client for a given tool (using the new map)
func (b *LLMMCPBridge) getClientForTool(toolName string) MCPClientInterface {
	if toolInfo, exists := b.availableTools[toolName]; exists {
		if client, clientExists := b.mcpClients[toolInfo.ServerName]; clientExists {
			return client
		}

		b.logger.Printf("Warning: Tool '%s' found, but its server '%s' is not in the client map.", toolName, toolInfo.ServerName)
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

	// Create a CallToolRequest with the correct structure
	request := mcp.CallToolRequest{}
	request.Params.Name = toolCall.Tool
	request.Params.Arguments = toolCall.Args

	// Call the tool using the MCPClientInterface
	result, err := client.CallTool(ctx, request)
	if err != nil {
		b.logger.Printf("Error calling MCP tool %s: %v", toolCall.Tool, err)
		return "", fmt.Errorf("failed to execute tool %s: %w", toolCall.Tool, err) // Propagate error
	}

	b.logger.Printf("Successfully executed MCP tool: %s", toolCall.Tool)

	// Format the result based on its type
	var resultStr string
	if result != nil {
		// Convert the result to a string representation
		resultBytes, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("failed to marshal tool result: %w", err)
		}
		resultStr = string(resultBytes)
	} else {
		resultStr = "{}"
	}

	// Return the result string
	return resultStr, nil
}

// extractSimpleKeyValuePairs attempts to extract simple key-value pairs from text
// that might look like JSON but not be valid JSON syntax
func (b *LLMMCPBridge) extractSimpleKeyValuePairs(text string) (map[string]interface{}, bool) {
	result := make(map[string]interface{})
	// Match key: "value" or key: value patterns
	pairRegex := regexp.MustCompile(`\s*"?([^"{}:,]+)"?\s*:\s*(?:"([^"]*)"|(true|false|\d+(?:\.\d+)?))\s*,?`)
	matches := pairRegex.FindAllStringSubmatch(text, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			key := strings.TrimSpace(match[1])
			var value interface{}

			// If the third capture group has a value, it's a non-string value
			if match[3] != "" {
				// Handle boolean values
				if match[3] == "true" {
					value = true
				} else if match[3] == "false" {
					value = false
				} else {
					// Handle numeric values
					if strings.Contains(match[3], ".") {
						// Float value
						if floatVal, err := strconv.ParseFloat(match[3], 64); err == nil {
							value = floatVal
						} else {
							value = match[3] // Fallback to string if parsing fails
						}
					} else {
						// Integer value
						if intVal, err := strconv.Atoi(match[3]); err == nil {
							value = intVal
						} else {
							value = match[3] // Fallback to string if parsing fails
						}
					}
				}
			} else {
				// String value (from the second capture group)
				value = match[2]
			}

			result[key] = value
		}
	}

	return result, len(result) > 0
}
