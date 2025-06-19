// Package handlers provides implementation for MCP tool handlers.
// It handles tool registration, execution, and management.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/tools"
	"github.com/tuannvm/slack-mcp-client/internal/llm"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
	"log"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	customErrors "github.com/tuannvm/slack-mcp-client/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

// LLMMCPBridge provides a bridge between LLM responses and MCP tool calls.
// It detects when an LLM response should trigger a tool call and executes it.
type LLMMCPBridge struct {
	mcpClients     map[string]mcp.MCPClientInterface // Map of MCP clients keyed by server name
	logger         *logging.Logger
	stdLogger      *log.Logger             // Standard logger for backward compatibility
	availableTools map[string]mcp.ToolInfo // Map of tool names to info about the tool
	llmRegistry    *llm.ProviderRegistry   // LLM provider registry
	useNativeTools bool                    // Flag to indicate if native tools should be used. If false, tools are provided through the system prompt.
	UseAgent       bool                    // Flag to indicate if the agent should be used instead of chat
}

// generateToolPrompt generates the prompt string for available tools
func (b *LLMMCPBridge) generateToolPrompt() string {
	if len(b.availableTools) == 0 {
		return "" // No tools available
	}

	var promptBuilder strings.Builder
	promptBuilder.WriteString("You have access to the following tools. Analyze the user's request to determine if a tool is needed.\n\n")

	// Clear instructions on how to format the JSON response
	promptBuilder.WriteString("TOOL USAGE INSTRUCTIONS:\n")
	promptBuilder.WriteString("1. If a tool is appropriate AND you have ALL required arguments from the user's request, respond with ONLY the JSON object.\n")
	promptBuilder.WriteString("2. The JSON MUST be properly formatted with no additional text before or after.\n")
	promptBuilder.WriteString("3. Do NOT include explanations, markdown formatting, or extra text with the JSON.\n")
	promptBuilder.WriteString("4. If any required arguments are missing, do NOT generate the JSON. Instead, ask the user for the missing information.\n")
	promptBuilder.WriteString("5. If no tool is needed, respond naturally to the user's request.\n\n")

	promptBuilder.WriteString("Available Tools:\n")

	for name, toolInfo := range b.availableTools {
		promptBuilder.WriteString(fmt.Sprintf("\nTool Name: %s\n", name))
		promptBuilder.WriteString(fmt.Sprintf("  Description: %s\n", toolInfo.ToolDescription))
		// Attempt to marshal the input schema map into a JSON string for display
		schemaBytes, err := json.MarshalIndent(toolInfo.InputSchema, "  ", "  ")
		if err != nil {
			b.logger.ErrorKV("Error marshaling schema for tool", "tool", name, "error", err)
			promptBuilder.WriteString("  Input Schema: (Error rendering schema)\n")
		} else {
			promptBuilder.WriteString(fmt.Sprintf("  Input Schema (JSON):\n  %s\n", string(schemaBytes)))
		}
	}

	// Add example formats for clarity
	promptBuilder.WriteString("\nEXACT JSON FORMAT FOR TOOL CALLS:\n")
	promptBuilder.WriteString("{\n")
	promptBuilder.WriteString("  \"tool\": \"<tool_name>\",\n")
	promptBuilder.WriteString("  \"args\": { <arguments matching the tool's input schema> }\n")
	promptBuilder.WriteString("}\n\n")

	// Add a concrete example
	promptBuilder.WriteString("EXAMPLE:\n")
	promptBuilder.WriteString("If the user asks 'Show me the files in the current directory' and 'list_dir' is an available tool:\n")
	promptBuilder.WriteString("{\n")
	promptBuilder.WriteString("  \"tool\": \"list_dir\",\n")
	promptBuilder.WriteString("  \"args\": { \"relative_workspace_path\": \".\" }\n")
	promptBuilder.WriteString("}\n\n")

	// Emphasize again to help model handle this correctly
	promptBuilder.WriteString("IMPORTANT: Return ONLY the raw JSON object with no explanations or formatting when using a tool.\n")

	return promptBuilder.String()
}

// NewLLMMCPBridge creates a new LLMMCPBridge with the given MCP clients and tools
// Uses INFO as the default log level
func NewLLMMCPBridge(mcpClients map[string]mcp.MCPClientInterface, stdLogger *log.Logger, discoveredTools map[string]mcp.ToolInfo,
	useNativeTools bool, useAgent bool, llmRegistry *llm.ProviderRegistry) *LLMMCPBridge {
	// Create a structured logger from the standard logger with INFO level by default
	// If debug logging is needed, use NewLLMMCPBridgeWithLogLevel instead
	return NewLLMMCPBridgeWithLogLevel(mcpClients, stdLogger, discoveredTools, logging.LevelInfo, useNativeTools, useAgent, llmRegistry)
}

// NewLLMMCPBridgeWithLogLevel creates a new LLMMCPBridge with the given MCP clients, tools, and log level
func NewLLMMCPBridgeWithLogLevel(mcpClients map[string]mcp.MCPClientInterface, stdLogger *log.Logger,
	discoveredTools map[string]mcp.ToolInfo, logLevel logging.LogLevel, useNativeTools bool, useAgent bool,
	llmRegistry *llm.ProviderRegistry) *LLMMCPBridge {
	// Create a structured logger with the specified log level
	structLogger := logging.New("llm-mcp-bridge", logLevel)

	return &LLMMCPBridge{
		mcpClients:     mcpClients,
		logger:         structLogger,
		stdLogger:      stdLogger,
		availableTools: discoveredTools,
		useNativeTools: useNativeTools,
		llmRegistry:    llmRegistry,
		UseAgent:       useAgent,
	}
}

// NewLLMMCPBridgeFromClients creates a new LLMMCPBridge with the given MCP Client objects
// This is a convenience function that wraps the concrete clients in the interface
// Uses INFO as the default log level
func NewLLMMCPBridgeFromClients(mcpClients interface{}, stdLogger *log.Logger, discoveredTools map[string]mcp.ToolInfo,
	useNativeTools bool, useAgent bool, llmRegistry *llm.ProviderRegistry) *LLMMCPBridge {
	// If debug logging is needed, use NewLLMMCPBridgeFromClientsWithLogLevel instead
	return NewLLMMCPBridgeFromClientsWithLogLevel(mcpClients, stdLogger, discoveredTools, logging.LevelInfo, useNativeTools, useAgent, llmRegistry)
}

// NewLLMMCPBridgeFromClientsWithLogLevel creates a new LLMMCPBridge with the given MCP Client objects and log level
// This is a convenience function that wraps the concrete clients in the interface
func NewLLMMCPBridgeFromClientsWithLogLevel(mcpClients interface{}, stdLogger *log.Logger,
	discoveredTools map[string]mcp.ToolInfo, logLevel logging.LogLevel, useNativeTools bool, useAgent bool,
	llmRegistry *llm.ProviderRegistry) *LLMMCPBridge {
	// Create a structured logger with the specified log level
	structLogger := logging.New("llm-mcp-bridge", logLevel)

	// Convert the concrete client map to the interface map
	// This is a workaround for the type system to avoid import cycles
	interfaceClients := make(map[string]mcp.MCPClientInterface)

	// Log the type of mcpClients for debugging
	structLogger.DebugKV("Initializing bridge with clients", "client_type", fmt.Sprintf("%T", mcpClients))

	// Try different type assertions based on the actual type
	switch typedClients := mcpClients.(type) {
	case map[string]interface{}:
		// Original implementation for map[string]interface{}
		for name, client := range typedClients {
			if mcpClient, ok := client.(mcp.MCPClientInterface); ok {
				interfaceClients[name] = mcpClient
				structLogger.DebugKV("Added MCP client", "name", name, "source", "map[string]interface{}")
			}
		}

	case map[string]mcp.MCPClientInterface:
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
				if client, ok := value.(mcp.MCPClientInterface); ok {
					interfaceClients[key] = client
					structLogger.DebugKV("Added MCP client", "name", key, "source", "reflection")
				}
			}
		}
	}

	return NewLLMMCPBridgeWithLogLevel(interfaceClients, stdLogger, discoveredTools, logLevel, useNativeTools, useAgent, llmRegistry)
}

// ProcessLLMResponse processes an LLM response, expecting a specific JSON tool call format.
// It no longer uses natural language detection.
func (b *LLMMCPBridge) ProcessLLMResponse(ctx context.Context, llmResponse *llms.ContentChoice, _ string) (string, error) {
	var toolCall *ToolCall
	var err error
	funcCall := llmResponse.FuncCall
	// Check for a tool call in JSON format
	if len(llmResponse.ToolCalls) > 0 {
		funcCall = llmResponse.ToolCalls[0].FunctionCall
	}

	if funcCall != nil {
		toolCall, err = b.getToolCall(funcCall)
		if err != nil {
			return "", err
		}
	} else {
		toolCall = b.detectSpecificJSONToolCall(llmResponse.Content)
	}

	if toolCall != nil {
		// Execute the tool call
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

	// Just return the LLM response as-is if no tool call was detected
	return llmResponse.Content, nil
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

func (b *LLMMCPBridge) getToolCall(funcCall *llms.FunctionCall) (*ToolCall, error) {
	var args map[string]interface{}
	err := json.Unmarshal([]byte(funcCall.Arguments), &args)
	if err != nil {
		return nil, customErrors.NewMCPError("invalid_json_args", fmt.Sprintf("Args not valid json for call '%s'", funcCall.Name))
	}
	return &ToolCall{
		Tool: funcCall.Name,
		Args: args,
	}, nil
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
func (b *LLMMCPBridge) getClientForTool(toolName string) mcp.MCPClientInterface {
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

func (b *LLMMCPBridge) CallLLMAgent(providerName, systemPrompt, prompt, contextHistory string, callbackHandler callbacks.Handler) (string, error) {
	// Create a context with an appropriate timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	toolArr := make([]tools.Tool, 0, len(b.availableTools))
	for _, t := range b.availableTools {
		toolArr = append(toolArr, &t)
	}

	// Prepare messages with system prompt and context history
	history := []llm.RequestMessage{}

	// Add conversation context if provided
	if contextHistory != "" {
		history = append(history, llm.RequestMessage{
			Role:    "system",
			Content: "Previous conversation: " + contextHistory,
		})
	}

	// --- Use the specified provider via the registry ---
	b.logger.InfoKV("Attempting to use LLM provider for chat completion", "provider", providerName)

	completion, err := b.llmRegistry.GenerateAgentCompletion(ctx, providerName, systemPrompt, prompt, history, toolArr, callbackHandler)
	if err != nil {
		// Error already logged by registry method potentially, but log here too for context
		b.logger.ErrorKV("GenerateAgentCompletion failed", "provider", providerName, "error", err)
		return "", customErrors.WrapSlackError(err, "llm_request_failed", fmt.Sprintf("LLM request failed for provider '%s'", providerName))
	}

	return completion, nil
}

// CallLLM generates a text completion using the specified provider from the registry.
func (b *LLMMCPBridge) CallLLM(providerName, prompt, contextHistory string) (*llms.ContentChoice, error) {
	// Create a context with appropriate timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Prepare messages with system prompt and context history
	messages := []llm.RequestMessage{}
	// Build options based on the config (provider might override or use these)
	// Note: TargetProvider is removed as it's handled by config/factory
	options := llm.ProviderOptions{
		// Model: Let the provider use its configured default or handle overrides if needed.
		// Model: c.cfg.OpenAIModelName, // Example: If you still want a global default hint
		Temperature: 0.7,  // Consider making configurable
		MaxTokens:   2048, // Consider making configurable
	}

	if !b.useNativeTools {
		// Generate the system prompt with tool information
		systemPrompt := b.generateToolPrompt()

		// Add system prompt with tool info if available
		if systemPrompt != "" {
			messages = append(messages, llm.RequestMessage{
				Role:    "system",
				Content: systemPrompt,
			})
		}
	} else {
		tools := []llms.Tool{}
		for name, tool := range b.availableTools {
			tools = append(tools, llms.Tool{
				Type: "function",
				Function: &llms.FunctionDefinition{
					Name:        name,
					Description: tool.ToolDescription,
					Parameters:  tool.InputSchema,
				},
			})
		}
		options.Tools = tools
	}

	// Add conversation context if provided
	if contextHistory != "" {
		messages = append(messages, llm.RequestMessage{
			Role:    "system",
			Content: "Previous conversation: " + contextHistory,
		})
	}

	// Add the user's prompt
	messages = append(messages, llm.RequestMessage{
		Role:    "user",
		Content: prompt,
	})

	// --- Use the specified provider via the registry ---
	b.logger.InfoKV("Attempting to use LLM provider for chat completion", "provider", providerName)

	// Call the registry's method which includes availability check
	completion, err := b.llmRegistry.GenerateChatCompletion(ctx, providerName, messages, options)
	if err != nil {
		// Error already logged by registry method potentially, but log here too for context
		b.logger.ErrorKV("GenerateChatCompletion failed", "provider", providerName, "error", err)
		return nil, customErrors.WrapSlackError(err, "llm_request_failed", fmt.Sprintf("LLM request failed for provider '%s'", providerName))
	}

	b.logger.InfoKV("Successfully received chat completion", "provider", providerName)

	return completion, nil
}
