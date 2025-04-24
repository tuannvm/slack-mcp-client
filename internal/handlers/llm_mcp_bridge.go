// Package handlers provides implementation for MCP tool handlers.
// It handles tool registration, execution, and management.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/tuannvm/slack-mcp-client/internal/common"
	customErrors "github.com/tuannvm/slack-mcp-client/internal/common/errors"
)

// MCPClientInterface defines the interface for an MCP client
// This allows us to break the circular dependency between packages
type MCPClientInterface interface {
	CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error)
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

	// Log the type of mcpClients for debugging
	logger.Printf("DEBUG: mcpClients type: %T", mcpClients)

	// Try different type assertions based on the actual type
	switch typedClients := mcpClients.(type) {
	case map[string]interface{}:
		// Original implementation for map[string]interface{}
		for name, client := range typedClients {
			if mcpClient, ok := client.(MCPClientInterface); ok {
				interfaceClients[name] = mcpClient
				logger.Printf("DEBUG: Added client for '%s' from map[string]interface{}", name)
			}
		}

	case map[string]MCPClientInterface:
		// Direct case if already the right type
		for name, client := range typedClients {
			interfaceClients[name] = client
			logger.Printf("DEBUG: Added client for '%s' from map[string]MCPClientInterface", name)
		}

	default:
		// Try to reflect and extract the map
		logger.Printf("DEBUG: Using reflection to extract clients from %T", mcpClients)

		// Use reflection to iterate over the map
		val := reflect.ValueOf(mcpClients)
		if val.Kind() == reflect.Map {
			iter := val.MapRange()
			for iter.Next() {
				key := iter.Key().String()
				value := iter.Value().Interface()

				// Try to convert the value to MCPClientInterface
				if client, ok := value.(MCPClientInterface); ok {
					interfaceClients[key] = client
					logger.Printf("DEBUG: Added client for '%s' using reflection", key)
				}
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

			// Check if it's already a domain error
			var errorMessage string
			if customErrors.IsDomainError(err) {
				// Extract structured information from the domain error
				code, _ := customErrors.GetErrorCode(err)
				errorMessage = fmt.Sprintf("Error executing tool call: %v (code: %s)", err, code)
			} else {
				errorMessage = fmt.Sprintf("Error executing tool call: %v", err)
			}

			return errorMessage, nil
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

	// Call the tool directly with the tool name and args
	result, err := client.CallTool(ctx, toolCall.Tool, toolCall.Args)
	if err != nil {
		b.logger.Printf("Error calling MCP tool %s: %v", toolCall.Tool, err)

		// Create a domain-specific error with additional context
		domainErr := customErrors.WrapMCPError(err, "tool_execution_failed",
			fmt.Sprintf("Failed to execute MCP tool '%s'", toolCall.Tool))

		// Add additional context data
		domainErr = domainErr.WithData("tool_name", toolCall.Tool)
		domainErr = domainErr.WithData("server_name", serverName)
		domainErr = domainErr.WithData("args", toolCall.Args)

		return "", domainErr
	}

	b.logger.Printf("Successfully executed MCP tool: %s", toolCall.Tool)

	// The result is already a string with the updated interface
	if result == "" {
		return "{}", nil
	}

	return result, nil
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
				// Handle boolean and numeric values
				switch match[3] {
				case "true":
					value = true
				case "false":
					value = false
				default:
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
