// Package bridge provides integration between LLM responses and MCP tool execution.
// It handles detection and execution of tool calls in LLM responses.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/tuannvm/slack-mcp-client/internal/common"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
)

// LLMMCPBridge provides a bridge between LLM responses and MCP tool calls.
// It detects when an LLM response should trigger a tool call and executes it.
type LLMMCPBridge struct {
	mcpClients    map[string]*mcp.Client // Map of MCP clients keyed by server name
	primaryClient *mcp.Client            // Default client for tool calls
	logger        *log.Logger
	// Use common.ToolInfo for the tool map
	availableTools map[string]common.ToolInfo // Map of tool name to server name
}

// NewLLMMCPBridge creates a new bridge between LLM and MCP with pre-discovered tools.
func NewLLMMCPBridge(mcpClients map[string]*mcp.Client, logger *log.Logger, discoveredTools map[string]common.ToolInfo) *LLMMCPBridge {
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
		bridge.availableTools = make(map[string]common.ToolInfo)
		logger.Printf("Initialized with %d MCP clients and NO pre-discovered tools.", len(mcpClients))
	} else {
		logger.Printf("Initialized with %d MCP clients and %d pre-discovered tools.", len(mcpClients), len(discoveredTools))
	}

	return bridge
}

// ProcessLLMResponse processes an LLM response, expecting a specific JSON tool call format.
// It no longer uses natural language detection.
func (b *LLMMCPBridge) ProcessLLMResponse(ctx context.Context, llmResponse, _ string) (string, error) {
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
			return "", fmt.Errorf("error executing tool '%s': %w", toolCall.Tool, err)
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
	trimmedResponse := strings.TrimSpace(response)
	if strings.HasPrefix(trimmedResponse, "{") && strings.HasSuffix(trimmedResponse, "}") {
		var toolCall ToolCall
		if err := json.Unmarshal([]byte(trimmedResponse), &toolCall); err == nil {
			// Validate the tool call
			if b.isValidToolCall(toolCall) {
				b.logger.Printf("DEBUG: Direct JSON parsing successful")
				return &toolCall
			}
		} else {
			b.logger.Printf("DEBUG: Direct JSON parsing failed: %v", err)
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

// constructToolCall attempts to construct a tool call from a tool name and args JSON string
func (b *LLMMCPBridge) constructToolCall(toolName string, argsJSON string) *ToolCall {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err == nil {
		toolCall := &ToolCall{
			Tool: toolName,
			Args: args,
		}

		if _, exists := b.availableTools[toolCall.Tool]; exists {
			b.logger.Printf("DEBUG: Manual JSON construction successful")
			return toolCall
		} else {
			b.logger.Printf("Warning: LLM requested tool '%s' in regex match, but it is not available.", toolCall.Tool)
		}
	} else {
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
	}
	return nil
}

// isValidToolCall validates if a tool call has the required fields and refers to an available tool
func (b *LLMMCPBridge) isValidToolCall(toolCall ToolCall) bool {
	if toolCall.Tool != "" && toolCall.Args != nil {
		if _, exists := b.availableTools[toolCall.Tool]; exists {
			return true
		} else {
			b.logger.Printf("Warning: LLM requested tool '%s', but it is not available.", toolCall.Tool)
		}
	}
	return false
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

// extractSimpleKeyValuePairs attempts to extract simple key-value pairs from text
// that might look like JSON but not be valid JSON syntax
func (b *LLMMCPBridge) extractSimpleKeyValuePairs(text string) (map[string]interface{}, bool) {
	result := make(map[string]interface{})

	// Look for patterns like "key": "value" or "key": 123
	kvRegex := regexp.MustCompile("\"([^\"]+)\"\\s*\\:\\s*(\"[^\"]*\"|\\d+|true|false)")
	matches := kvRegex.FindAllStringSubmatch(text, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			key := match[1]
			valueStr := match[2]

			// Try to determine the value type
			if strings.HasPrefix(valueStr, "\"") && strings.HasSuffix(valueStr, "\"") {
				// It's a string
				result[key] = valueStr[1 : len(valueStr)-1] // Remove quotes
			} else if valueStr == "true" {
				result[key] = true
			} else if valueStr == "false" {
				result[key] = false
			} else {
				// Try to parse as number
				if num, err := strconv.ParseFloat(valueStr, 64); err == nil {
					result[key] = num
				} else {
					result[key] = valueStr // Keep as string if all else fails
				}
			}
		}
	}

	return result, len(result) > 0
}
