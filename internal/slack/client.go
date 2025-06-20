// Package slackbot implements the Slack integration for the MCP client
// It provides event handling, message processing, and integration with LLM services
package slackbot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/tuannvm/slack-mcp-client/internal/common"
	customErrors "github.com/tuannvm/slack-mcp-client/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/handlers"
	"github.com/tuannvm/slack-mcp-client/internal/llm"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
)

const thinkingMessage = "Thinking..."

// Client represents the Slack client application.
type Client struct {
	logger          *logging.Logger // Structured logger
	userFrontend    UserFrontend
	mcpClients      map[string]*mcp.Client
	llmMCPBridge    *handlers.LLMMCPBridge
	llmRegistry     *llm.ProviderRegistry // LLM provider registry
	cfg             *config.Config        // Holds the application configuration
	messageHistory  map[string][]Message
	historyLimit    int
	discoveredTools map[string]common.ToolInfo
}

// Message represents a message in the conversation history
type Message struct {
	Role      string    // "user", "assistant", or "tool"
	Content   string    // The message content
	Timestamp time.Time // When the message was sent/received
}

// NewClient creates a new Slack client instance.
func NewClient(userFrontend UserFrontend, stdLogger *logging.Logger, mcpClients map[string]*mcp.Client,
	discoveredTools map[string]common.ToolInfo, cfg *config.Config) (*Client, error) {

	// MCP clients are now optional - if none are provided, we'll just use LLM capabilities
	if mcpClients == nil {
		mcpClients = make(map[string]*mcp.Client)
		stdLogger.Printf("No MCP clients provided, running in LLM-only mode")
	}
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	clientLogger := userFrontend.GetLogger()

	// --- MCP/Bridge setup ---
	clientLogger.Printf("Available MCP servers (%d):", len(mcpClients))
	for name := range mcpClients {
		clientLogger.Printf("- %s", name)
	}

	clientLogger.Printf("Available tools (%d):", len(discoveredTools))
	for toolName, toolInfo := range discoveredTools {
		clientLogger.Printf("- %s (Desc: %s, Schema: %v, Server: %s)",
			toolName, toolInfo.Description, toolInfo.InputSchema, toolInfo.ServerName)
	}

	// Create a map of raw clients to pass to the bridge
	rawClientMap := make(map[string]interface{})
	for name, client := range mcpClients {
		rawClientMap[name] = client
		clientLogger.DebugKV("Adding MCP client to raw map for bridge", "name", name)
	}

	logLevel := getLogLevel(stdLogger)

	// Pass the raw map to the bridge with the configured log level
	llmMCPBridge := handlers.NewLLMMCPBridgeFromClientsWithLogLevel(rawClientMap, clientLogger.StdLogger(), discoveredTools, logLevel)
	clientLogger.InfoKV("LLM-MCP bridge initialized", "clients", len(mcpClients), "tools", len(discoveredTools))

	// --- Initialize the LLM provider registry using the config ---
	// Use the internal structured logger for the registry with the same log level as the bridge
	registryLogger := logging.New("llm-registry", logLevel)
	registry, err := llm.NewProviderRegistry(cfg, registryLogger)
	if err != nil {
		// Log the error using the structured logger
		clientLogger.ErrorKV("Failed to initialize LLM provider registry", "error", err)
		return nil, customErrors.WrapLLMError(err, "llm_registry_init_failed", "Failed to initialize LLM provider registry")
	}
	clientLogger.Info("LLM provider registry initialized successfully")
	// Set the primary provider
	primaryProvider := cfg.LLMProvider
	if primaryProvider == "" {
		clientLogger.Warn("No LLM provider specified in config, using default")
		primaryProvider = "openai" // Default to OpenAI if not specified
	}
	clientLogger.InfoKV("Primary LLM provider set", "provider", primaryProvider)

	// --- Create and return Client instance ---
	return &Client{
		logger:          clientLogger,
		userFrontend:    userFrontend,
		mcpClients:      mcpClients,
		llmMCPBridge:    llmMCPBridge,
		llmRegistry:     registry,
		cfg:             cfg,
		messageHistory:  make(map[string][]Message),
		historyLimit:    50, // Store up to 50 messages per channel
		discoveredTools: discoveredTools,
	}, nil
}

// Run starts the Socket Mode event loop and event handling.
func (c *Client) Run() error {
	go c.handleEvents()
	c.logger.Info("Starting Slack Socket Mode listener...")
	return c.userFrontend.Run()
}

// handleEvents listens for incoming events and dispatches them.
func (c *Client) handleEvents() {
	for evt := range c.userFrontend.GetEventChannel() {
		switch evt.Type {
		case socketmode.EventTypeConnecting:
			c.logger.Info("Connecting to Slack...")
		case socketmode.EventTypeConnectionError:
			c.logger.Warn("Connection failed. Retrying...")
		case socketmode.EventTypeConnected:
			c.logger.Info("Connected to Slack!")
		case socketmode.EventTypeEventsAPI:
			eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				c.logger.WarnKV("Ignored unexpected EventsAPI event type", "type", fmt.Sprintf("%T", evt.Data))
				continue
			}
			c.userFrontend.Ack(*evt.Request)
			c.logger.InfoKV("Received EventsAPI event", "type", eventsAPIEvent.Type)
			c.handleEventMessage(eventsAPIEvent)
		default:
			c.logger.DebugKV("Ignored event type", "type", evt.Type)
		}
	}
	c.logger.Info("Slack event channel closed.")
}

// handleEventMessage processes specific EventsAPI messages.
func (c *Client) handleEventMessage(event slackevents.EventsAPIEvent) {
	switch event.Type {
	case slackevents.CallbackEvent:
		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			c.logger.InfoKV("Received app mention in channel", "channel", ev.Channel, "user", ev.User, "text", ev.Text)
			messageText := c.userFrontend.RemoveBotMention(ev.Text)
			// Add to message history
			c.addToHistory(ev.Channel, "user", messageText)
			// Use handleUserPrompt for app mentions too, for consistency
			go c.handleUserPrompt(strings.TrimSpace(messageText), ev.Channel, ev.TimeStamp)

		case *slackevents.MessageEvent:
			isDirectMessage := strings.HasPrefix(ev.Channel, "D")
			isValidUser := c.userFrontend.IsValidUser(ev.User)
			isNotEdited := ev.SubType != "message_changed"
			isBot := ev.BotID != "" || ev.SubType == "bot_message"

			if isDirectMessage && isValidUser && isNotEdited && !isBot {
				c.logger.InfoKV("Received direct message in channel", "channel", ev.Channel, "user", ev.User, "text", ev.Text)
				// Add to message history
				c.addToHistory(ev.Channel, "user", ev.Text)
				go c.handleUserPrompt(ev.Text, ev.Channel, ev.ThreadTimeStamp) // Use goroutine to avoid blocking event loop
			}

		default:
			c.logger.DebugKV("Unsupported inner event type", "type", fmt.Sprintf("%T", innerEvent.Data))
		}
	default:
		c.logger.DebugKV("Unsupported outer event type", "type", event.Type)
	}
}

// addToHistory adds a message to the channel history
func (c *Client) addToHistory(channelID, role, content string) {
	history, exists := c.messageHistory[channelID]
	if !exists {
		history = []Message{}
	}

	// Add the new message
	message := Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}
	history = append(history, message)

	// Limit history size
	if len(history) > c.historyLimit {
		history = history[len(history)-c.historyLimit:]
	}

	c.messageHistory[channelID] = history
}

// getContextFromHistory builds a context string from message history
//
//nolint:unused // Reserved for future use
func (c *Client) getContextFromHistory(channelID string) string {
	history, exists := c.messageHistory[channelID]
	if !exists || len(history) == 0 {
		return ""
	}

	var contextBuilder strings.Builder
	contextBuilder.WriteString("Previous conversation context:\n---\n") // Clearer start marker

	for _, msg := range history {
		switch msg.Role {
		case "assistant":
			prefix := "Assistant"
			sanitizedContent := strings.ReplaceAll(msg.Content, "\n", " \\n ")
			contextBuilder.WriteString(fmt.Sprintf("%s: %s\n", prefix, sanitizedContent))
		case "tool":
			prefix := "Tool Result"
			sanitizedContent := strings.ReplaceAll(msg.Content, "\n", " \\n ")
			contextBuilder.WriteString(fmt.Sprintf("%s: %s\n", prefix, sanitizedContent))
		default: // "user" or any other role
			prefix := "User"
			sanitizedContent := strings.ReplaceAll(msg.Content, "\n", " \\n ")
			contextBuilder.WriteString(fmt.Sprintf("%s: %s\n", prefix, sanitizedContent))
		}
	}
	contextBuilder.WriteString("---\n") // Clearer end marker

	contextString := contextBuilder.String()
	c.logger.DebugKV("Built conversation context", "channel", channelID, "context", contextString) // Log the built context
	return contextString
}

// handleUserPrompt sends the user's text to the configured LLM provider.
func (c *Client) handleUserPrompt(userPrompt, channelID, threadTS string) {
	// Determine the provider to use from config
	providerName := c.cfg.LLMProvider // Get the primary provider name from config
	c.logger.DebugKV("Routing prompt via configured provider", "provider", providerName)
	c.logger.DebugKV("User prompt", "text", userPrompt)

	c.addToHistory(channelID, "user", userPrompt) // Add user message to history

	// Show a temporary "typing" indicator
	c.userFrontend.SendMessage(channelID, threadTS, thinkingMessage)

	// Get context from history
	contextHistory := c.getContextFromHistory(channelID)

	// Call LLM using the integrated logic
	llmResponse, err := c.callLLM(providerName, userPrompt, contextHistory)
	if err != nil {
		c.logger.ErrorKV("Error from LLM provider", "provider", providerName, "error", err)
		c.userFrontend.SendMessage(channelID, threadTS, fmt.Sprintf("Sorry, I encountered an error with the LLM provider ('%s'): %v", providerName, err))
		return
	}

	c.logger.InfoKV("Received response from LLM", "provider", providerName, "length", len(llmResponse))

	// Process the LLM response through the MCP pipeline
	c.processLLMResponseAndReply(llmResponse, userPrompt, channelID, threadTS)
}

// generateToolPrompt generates the prompt string for available tools
func (c *Client) generateToolPrompt() string {
	if len(c.discoveredTools) == 0 {
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

	for name, toolInfo := range c.discoveredTools {
		promptBuilder.WriteString(fmt.Sprintf("\nTool Name: %s\n", name))
		promptBuilder.WriteString(fmt.Sprintf("  Description: %s\n", toolInfo.Description))
		// Attempt to marshal the input schema map into a JSON string for display
		schemaBytes, err := json.MarshalIndent(toolInfo.InputSchema, "  ", "  ")
		if err != nil {
			c.logger.ErrorKV("Error marshaling schema for tool", "tool", name, "error", err)
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

// callLLM generates a text completion using the specified provider from the registry.
func (c *Client) callLLM(providerName, prompt, contextHistory string) (string, error) {
	// Create a context with appropriate timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Generate the system prompt with tool information
	systemPrompt := c.generateToolPrompt()

	// Prepare messages with system prompt and context history
	messages := []llm.RequestMessage{}

	// Add system prompt with tool info if available
	if systemPrompt != "" {
		messages = append(messages, llm.RequestMessage{
			Role:    "system",
			Content: systemPrompt,
		})
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

	// Build options based on the config (provider might override or use these)
	// Note: TargetProvider is removed as it's handled by config/factory
	options := llm.ProviderOptions{
		// Model: Let the provider use its configured default or handle overrides if needed.
		// Model: c.cfg.OpenAIModelName, // Example: If you still want a global default hint
		Temperature: 0.7,  // Consider making configurable
		MaxTokens:   2048, // Consider making configurable
	}

	// --- Use the specified provider via the registry ---
	c.logger.InfoKV("Attempting to use LLM provider for chat completion", "provider", providerName)

	// Call the registry's method which includes availability check
	completion, err := c.llmRegistry.GenerateChatCompletion(ctx, providerName, messages, options)
	if err != nil {
		// Error already logged by registry method potentially, but log here too for context
		c.logger.ErrorKV("GenerateChatCompletion failed", "provider", providerName, "error", err)
		return "", customErrors.WrapSlackError(err, "llm_request_failed", fmt.Sprintf("LLM request failed for provider '%s'", providerName))
	}

	c.logger.InfoKV("Successfully received chat completion", "provider", providerName)
	return completion, nil
}

// processLLMResponseAndReply processes the LLM response, handles tool results with re-prompting, and sends the final reply.
// Incorporates logic previously in LLMClient.ProcessToolResponse.
func (c *Client) processLLMResponseAndReply(llmResponse, userPrompt, channelID, threadTS string) {
	// Log the raw LLM response for debugging
	c.logger.DebugKV("Raw LLM response", "response", truncateForLog(llmResponse, 500))

	// Create a context with timeout for tool processing
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// --- Process Tool Response (Logic from LLMClient.ProcessToolResponse) ---
	var finalResponse string
	var isToolResult bool
	var toolProcessingErr error

	if c.llmMCPBridge == nil {
		// If bridge is nil, just use the original response
		finalResponse = llmResponse
		isToolResult = false
		toolProcessingErr = nil
		c.logger.Warn("LLMMCPBridge is nil, skipping tool processing")
	} else {
		// Process the response through the bridge
		processedResponse, err := c.llmMCPBridge.ProcessLLMResponse(ctx, llmResponse, userPrompt)
		if err != nil {
			finalResponse = fmt.Sprintf("Sorry, I encountered an error while trying to use a tool: %v", err)
			isToolResult = false
			toolProcessingErr = err // Store the error
		} else {
			// If the processed response is different from the original, a tool was executed
			if processedResponse != llmResponse {
				finalResponse = processedResponse
				isToolResult = true
			} else {
				// No tool was executed
				finalResponse = llmResponse
				isToolResult = false
			}
		}
	}
	// --- End of Process Tool Response Logic ---

	if toolProcessingErr != nil {
		c.logger.ErrorKV("Tool processing error", "error", toolProcessingErr)
		c.userFrontend.SendMessage(channelID, threadTS, finalResponse) // Post the error message
		return
	}

	if isToolResult {
		c.logger.Debug("Tool executed. Re-prompting LLM with tool result.")
		c.logger.DebugKV("Tool result", "result", truncateForLog(finalResponse, 500))

		// Construct a new prompt incorporating the original prompt and the tool result
		rePrompt := fmt.Sprintf("The user asked: '%s'\n\nI used a tool and received the following result:\n```\n%s\n```\nPlease formulate a concise and helpful natural language response to the user based *only* on the user's original question and the tool result provided.", userPrompt, finalResponse)

		// Add history
		c.addToHistory(channelID, "assistant", llmResponse) // Original LLM response (tool call JSON)
		c.addToHistory(channelID, "tool", finalResponse)    // Tool execution result

		c.logger.DebugKV("Re-prompting LLM", "prompt", rePrompt)

		// Re-prompt using the LLM client
		var repromptErr error
		// Get the provider name from config again for the re-prompt
		providerName := c.cfg.LLMProvider
		finalResponse, repromptErr = c.callLLM(providerName, rePrompt, c.getContextFromHistory(channelID))
		if repromptErr != nil {
			c.logger.ErrorKV("Error during LLM re-prompt", "error", repromptErr)
			// Fallback: Show the tool result and the error
			finalResponse = fmt.Sprintf("Tool Result:\n```%s```\n\n(Error generating final response: %v)", finalResponse, repromptErr)
		}
	} else {
		// No tool was executed, add assistant response to history
		c.addToHistory(channelID, "assistant", finalResponse)
	}

	// Send the final response back to Slack
	if finalResponse == "" {
		c.userFrontend.SendMessage(channelID, threadTS, "(LLM returned an empty response)")
	} else {
		c.userFrontend.SendMessage(channelID, threadTS, finalResponse)
	}
}

// truncateForLog truncates a string for log output
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
