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
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
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
	logger         *logging.Logger
	stdLogger      *log.Logger                // Standard logger for backward compatibility
	availableTools map[string]common.ToolInfo // Map of tool names to info about the tool
}

// NewLLMMCPBridge creates a new LLMMCPBridge with the given MCP clients and tools
// Uses INFO as the default log level
func NewLLMMCPBridge(mcpClients map[string]MCPClientInterface, stdLogger *log.Logger, discoveredTools map[string]common.ToolInfo) *LLMMCPBridge {
	// Create a structured logger from the standard logger with INFO level by default
	// If debug logging is needed, use NewLLMMCPBridgeWithLogLevel instead
	return NewLLMMCPBridgeWithLogLevel(mcpClients, stdLogger, discoveredTools, logging.LevelInfo)
}

// NewLLMMCPBridgeWithLogLevel creates a new LLMMCPBridge with the given MCP clients, tools, and log level
func NewLLMMCPBridgeWithLogLevel(mcpClients map[string]MCPClientInterface, stdLogger *log.Logger,
	discoveredTools map[string]common.ToolInfo, logLevel logging.LogLevel) *LLMMCPBridge {
	// Create a structured logger with the specified log level
	structLogger := logging.New("llm-mcp-bridge", logLevel)

	return &LLMMCPBridge{
		mcpClients:     mcpClients,
		logger:         structLogger,
		stdLogger:      stdLogger,
		availableTools: discoveredTools,
	}
}

// NewLLMMCPBridgeFromClients creates a new LLMMCPBridge with the given MCP Client objects
// This is a convenience function that wraps the concrete clients in the interface
// Uses INFO as the default log level
func NewLLMMCPBridgeFromClients(mcpClients interface{}, stdLogger *log.Logger, discoveredTools map[string]common.ToolInfo) *LLMMCPBridge {
	// If debug logging is needed, use NewLLMMCPBridgeFromClientsWithLogLevel instead
	return NewLLMMCPBridgeFromClientsWithLogLevel(mcpClients, stdLogger, discoveredTools, logging.LevelInfo)
}

// NewLLMMCPBridgeFromClientsWithLogLevel creates a new LLMMCPBridge with the given MCP Client objects and log level
// This is a convenience function that wraps the concrete clients in the interface
func NewLLMMCPBridgeFromClientsWithLogLevel(mcpClients interface{}, stdLogger *log.Logger,
	discoveredTools map[string]common.ToolInfo, logLevel logging.LogLevel) *LLMMCPBridge {
	// Create a structured logger with the specified log level
	structLogger := logging.New("llm-mcp-bridge", logLevel)

	// Convert the concrete client map to the interface map
	// This is a workaround for the type system to avoid import cycles
	interfaceClients := make(map[string]MCPClientInterface)

	// Log the type of mcpClients for debugging
	structLogger.DebugKV("Initializing bridge with clients", "client_type", fmt.Sprintf("%T", mcpClients))

	// Try different type assertions based on the actual type
	switch typedClients := mcpClients.(type) {
	case map[string]interface{}:
		// Original implementation for map[string]interface{}
		for name, client := range typedClients {
			if mcpClient, ok := client.(MCPClientInterface); ok {
				interfaceClients[name] = mcpClient
				structLogger.DebugKV("Added MCP client", "name", name, "source", "map[string]interface{}")
			}
		}

	case map[string]MCPClientInterface:
		// Direct case if already the right type
		for name, client := range typedClients {
			interfaceClients[name] = client
			structLogger.DebugKV("Added MCP client", "name", name, "source", "map[string]MCPClientInterface")
		}

	default:
		// Try to reflect and extract the map
		structLogger.DebugKV("Using reflection to extract clients", "type", fmt.Sprintf("%T", mcpClients))

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
					structLogger.DebugKV("Added MCP client", "name", key, "source", "reflection")
				}
			}
		}
	}

	return NewLLMMCPBridgeWithLogLevel(interfaceClients, stdLogger, discoveredTools, logLevel)
}

// ProcessLLMResponse processes an LLM response, expecting a specific JSON tool call format.
// It no longer uses natural language detection.
func (b *LLMMCPBridge) ProcessLLMResponse(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	// Check for a tool call in JSON format
	// Execute the tool call
	toolCall := &ToolCall{
		Tool: name,
		Args: args,
	}
	result, err := b.executeToolCall(ctx, toolCall)
	if err != nil {
		// Check if it's already a domain error
		var errorMessage string
		if customErrors.IsDomainError(err) {
			// Extract structured information from the domain error
			code, _ := customErrors.GetErrorCode(err)
			b.logger.ErrorKV("Failed to execute tool call",
				"error", err.Error(),
				"error_code", code,
				"tool", toolCall.Tool)
			errorMessage = fmt.Sprintf("Error executing tool call: %v (code: %s)", err, code)
		} else {
			b.logger.ErrorKV("Failed to execute tool call",
				"error", err.Error(),
				"tool", toolCall.Tool)
			errorMessage = fmt.Sprintf("Error executing tool call: %v", err)
		}

		return errorMessage, nil
	}
	return result, nil
}

// ToolCall represents the expected JSON structure for a tool call from the LLM
type ToolCall struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args"`
}

// detectSpecificJSONToolCall attempts to find and parse the *specific* JSON tool call structure.
// It validates the tool name against available tools.
func (b *LLMMCPBridge) detectSpecificJSONToolCall(response string) *ToolCall {
	b.logger.DebugKV("Beginning JSON tool call detection")

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

	b.logger.DebugKV("No valid JSON tool call detected")
	return nil
}

// tryDirectJSONParsing attempts direct JSON parsing of the entire response
func (b *LLMMCPBridge) tryDirectJSONParsing(response string) *ToolCall {
	b.logger.DebugKV("Attempting direct JSON parsing")
	response = strings.TrimSpace(response)

	var toolCall ToolCall
	if err := json.Unmarshal([]byte(response), &toolCall); err == nil {
		if b.isValidToolCall(toolCall) {
			b.logger.DebugKV("Direct JSON parsing successful", "tool", toolCall.Tool)
			return &toolCall
		}
	}
	return nil
}

// tryCodeBlockJSONParsing looks for JSON in code blocks
func (b *LLMMCPBridge) tryCodeBlockJSONParsing(response string) *ToolCall {
	b.logger.DebugKV("Searching for JSON in code blocks")
	codeBlockRegex := regexp.MustCompile("```(?:json)?\\s*({[\\s\\S]*?})\\s*```")
	codeBlockMatches := codeBlockRegex.FindAllStringSubmatch(response, -1)

	for _, match := range codeBlockMatches {
		if len(match) >= 2 {
			jsonContent := match[1]
			b.logger.DebugKV("Found potential JSON in code block", "content", jsonContent)

			var toolCall ToolCall
			if err := json.Unmarshal([]byte(jsonContent), &toolCall); err == nil {
				if b.isValidToolCall(toolCall) {
					b.logger.DebugKV("JSON code block parsing successful", "tool", toolCall.Tool)
					return &toolCall
				}
			} else {
				b.logger.DebugKV("JSON code block parsing failed", "error", err.Error())
			}
		}
	}
	return nil
}

// tryRegexJSONExtraction looks for tool calls using regex patterns
func (b *LLMMCPBridge) tryRegexJSONExtraction(response string) *ToolCall {
	b.logger.DebugKV("Searching for JSON objects in text")
	// More lenient regex that can handle various JSON formats with the key elements we need
	jsonRegex := regexp.MustCompile("(?i)[\\{\\s]*[\"']?tool[\"']?\\s*[\\:\\=]\\s*[\"']([^\"']+)[\"']\\s*[\\,\\s]*[\"']?args[\"']?\\s*[\\:\\=]\\s*\\{([\\s\\S]*?)\\}[\\s\\}]*")
	jsonMatches := jsonRegex.FindAllStringSubmatch(response, -1)

	for _, match := range jsonMatches {
		if len(match) >= 3 {
			toolName := match[1]
			argsJSON := "{" + match[2] + "}" // Ensure it's wrapped in braces
			b.logger.DebugKV("Found potential JSON object", "tool", toolName, "args", argsJSON)

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
			b.logger.DebugKV("Manual JSON construction successful", "tool", toolCall.Tool)
			return toolCall
		}

		b.logger.WarnKV("Tool not available", "tool", toolCall.Tool, "source", "regex_match")
		return nil
	}

	b.logger.DebugKV("JSON args parsing failed", "error", err.Error(), "tool", toolName)

	// Try even more lenient approach - extract key-value pairs
	if simpleArgs, ok := b.extractSimpleKeyValuePairs(argsJSON); ok && len(simpleArgs) > 0 {
		toolCall := &ToolCall{
			Tool: toolName,
			Args: simpleArgs,
		}

		if _, exists := b.availableTools[toolCall.Tool]; exists {
			b.logger.DebugKV("Simplified key-value extraction successful", "tool", toolCall.Tool)
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

		b.logger.WarnKV("Tool not available", "tool", toolCall.Tool)
	}
	return false
}

// getClientForTool returns the appropriate client for a given tool (using the new map)
func (b *LLMMCPBridge) getClientForTool(toolName string) MCPClientInterface {
	if toolInfo, exists := b.availableTools[toolName]; exists {
		if client, clientExists := b.mcpClients[toolInfo.ServerName]; clientExists {
			return client
		}

		b.logger.WarnKV("Server not found for tool", "tool", toolName, "server", toolInfo.ServerName)
	}
	// Fallback or error handling if tool/server not found
	b.logger.WarnKV("Client not found for tool", "tool", toolName, "fallback", false)
	return nil // Return nil, executeToolCall should handle this
}

// executeToolCall executes a detected tool call (using the new ToolCall struct)
func (b *LLMMCPBridge) executeToolCall(ctx context.Context, toolCall *ToolCall) (string, error) {
	client := b.getClientForTool(toolCall.Tool)
	if client == nil {
		b.logger.ErrorKV("No MCP client available", "tool", toolCall.Tool)
		return "", customErrors.NewMCPError("client_not_found", fmt.Sprintf("No MCP client available for tool '%s'", toolCall.Tool))
	}

	serverName := b.availableTools[toolCall.Tool].ServerName // Get server name for logging
	b.logger.InfoKV("Calling MCP tool",
		"tool", toolCall.Tool,
		"server", serverName,
		"args", fmt.Sprintf("%v", toolCall.Args))

	// Call the tool directly with the tool name and args
	result, err := client.CallTool(ctx, toolCall.Tool, toolCall.Args)
	if err != nil {
		// Create a domain-specific error with additional context
		domainErr := customErrors.WrapMCPError(err, "tool_execution_failed",
			fmt.Sprintf("Failed to execute MCP tool '%s'", toolCall.Tool))

		// Add additional context data
		domainErr = domainErr.WithData("tool_name", toolCall.Tool)
		domainErr = domainErr.WithData("server_name", serverName)
		domainErr = domainErr.WithData("args", toolCall.Args)

		return "", domainErr
	}

	b.logger.InfoKV("Successfully executed MCP tool", "tool", toolCall.Tool)

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
