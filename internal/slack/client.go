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
	"github.com/tmc/langchaingo/llms"

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
	messageHistory  map[string][]llms.MessageContent
	historyLimit    int
	discoveredTools map[string]common.ToolInfo
	llmsTools       []llms.Tool
	toolCallsLimit  int
}

// NewClient creates a new Slack client instance.
func NewClient(userFrontend UserFrontend, stdLogger *logging.Logger, mcpClients map[string]*mcp.Client,
	discoveredTools map[string]common.ToolInfo, llmsTools []llms.Tool, cfg *config.Config) (*Client, error) {

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
			toolName, toolInfo.Tool.Function.Description, toolInfo.Tool.Function.Parameters, toolInfo.ServerName)
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
		messageHistory:  make(map[string][]llms.MessageContent),
		historyLimit:    50, // Store up to 50 messages per channel
		discoveredTools: discoveredTools,
		llmsTools:       llmsTools,
		toolCallsLimit:  25,
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
			c.addToHistory(ev.Channel, llms.ChatMessageTypeHuman, llms.TextPart(messageText))
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
				c.addToHistory(ev.Channel, llms.ChatMessageTypeHuman, llms.TextPart(ev.Text))
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
func (c *Client) addToHistory(channelID string, role llms.ChatMessageType, parts ...llms.ContentPart) {
	history, exists := c.messageHistory[channelID]
	if !exists {
		history = []llms.MessageContent{}
	}

	// Add the new message
	message := llms.MessageContent{
		Role:  role,
		Parts: parts,
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
func (c *Client) getContextFromHistory(channelID string) []llms.MessageContent {
	c.logger.DebugKV("Built conversation context", "channel", channelID)
	history := c.messageHistory[channelID]
	return history
}

// handleUserPrompt sends the user's text to the configured LLM provider.
func (c *Client) handleUserPrompt(userPrompt, channelID, threadTS string) {
	// Determine the provider to use from config
	c.logger.DebugKV("User prompt", "text", userPrompt)

	c.addToHistory(channelID, llms.ChatMessageTypeHuman, llms.TextPart(userPrompt)) // Add user message to history

	// Show a temporary "typing" indicator
	c.userFrontend.SendMessage(channelID, threadTS, thinkingMessage)

	// Process the LLM response through the MCP pipeline
	c.processLLMResponseAndReply(channelID, threadTS)
}

// callLLM generates a text completion using the specified provider from the registry.
func (c *Client) callLLM(providerName string, messages []llms.MessageContent) (*llms.ContentChoice, error) {
	// Create a context with appropriate timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Generate the system prompt with tool information
	//toolPrompt := c.generateToolPrompt()

	// Build options based on the config (provider might override or use these)
	// Note: TargetProvider is removed as it's handled by config/factory
	options := llm.ProviderOptions{
		// Model: Let the provider use its configured default or handle overrides if needed.
		// Model: c.cfg.OpenAIModelName, // Example: If you still want a global default hint
		Temperature: 0.7,         // Consider making configurable
		MaxTokens:   2048,        // Consider making configurable
		Tools:       c.llmsTools, // Use the tools registered in the client
	}

	// --- Use the specified provider via the registry ---
	c.logger.InfoKV("Attempting to use LLM provider for chat completion", "provider", providerName)

	// Call the registry's method which includes availability check
	resp, err := c.llmRegistry.GenerateChatCompletion(ctx, providerName, messages, options)
	if err != nil {
		// Error already logged by registry method potentially, but log here too for context
		c.logger.ErrorKV("GenerateChatCompletion failed", "provider", providerName, "error", err)
		return nil, customErrors.WrapSlackError(err, "llm_request_failed", fmt.Sprintf("LLM request failed for provider '%s'", providerName))
	}

	c.logger.InfoKV("Successfully received chat completion", "provider", providerName)
	return resp, nil
}

func (c *Client) answer(channelID, threadTS string, response string) {
	if response == "" {
		c.userFrontend.SendMessage(channelID, threadTS, "(LLM returned an empty response)")
	} else {
		c.userFrontend.SendMessage(channelID, threadTS, response)
	}
}

// processLLMResponseAndReply processes the LLM response, handles tool results with re-prompting, and sends the final reply.
// Incorporates logic previously in LLMClient.ProcessToolResponse.
func (c *Client) processLLMResponseAndReply(channelID, threadTS string) {
	providerName := c.cfg.LLMProvider

	for i := 0; i < c.toolCallsLimit; i++ {
		// Get context from history
		contextHistory := c.getContextFromHistory(channelID)
		// Call LLM using the integrated logic
		llmResponse, err := c.callLLM(providerName, contextHistory)
		if err != nil {
			c.logger.ErrorKV("Error from LLM provider", "provider", providerName, "error", err)
			c.userFrontend.SendMessage(channelID, threadTS, fmt.Sprintf("Sorry, I encountered an error with the LLM provider ('%s'): %v", providerName, err))
			return
		}

		c.logger.InfoKV("Received response from LLM", "provider", providerName, "length", len(llmResponse.Content))

		// Log the raw LLM response for debugging
		c.logger.DebugKV("Raw LLM response", "response", truncateForLog(llmResponse.Content, 500))

		// Add the LLM response to history

		// End of the response handling logic
		if len(llmResponse.ToolCalls) == 0 || c.llmMCPBridge == nil {
			if c.llmMCPBridge == nil {
				c.logger.Warn("LLMMCPBridge is nil, skipping tool processing")
			}
			c.addToHistory(channelID, llms.ChatMessageTypeAI, llms.TextPart(llmResponse.Content))

			c.answer(channelID, threadTS, llmResponse.Content)
			return
		}

		parts := []llms.ContentPart{
			llms.TextPart(llmResponse.Content),
		}
		for _, toolCall := range llmResponse.ToolCalls {
			parts = append(parts, toolCall)
		}

		c.addToHistory(channelID, llms.ChatMessageTypeAI, parts...)

		// Create a context with timeout for tool processing
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		for _, toolCall := range llmResponse.ToolCalls {
			// --- Process Tool Response (Logic from LLMClient.ProcessToolResponse) ---
			// Process the response through the bridge
			args := make(map[string]interface{})
			err = json.Unmarshal([]byte(toolCall.FunctionCall.Arguments), &args)
			if err != nil {
				c.logger.ErrorKV("Failed to unmarshal tool call arguments", "error", err, "arguments", toolCall.FunctionCall.Arguments)
				return
			}

			processedResponse, err := c.llmMCPBridge.ProcessLLMResponse(ctx, toolCall.FunctionCall.Name, args)
			if err != nil {
				c.logger.ErrorKV("Tool processing error", "error", err)
				finalResponse := fmt.Sprintf("Sorry, I encountered an error while trying to use a tool: %v", err)
				c.answer(channelID, threadTS, finalResponse)
				return
			}

			c.logger.DebugKV("Tool result", "result", truncateForLog(processedResponse, 500))

			// Add history
			toolResponsePart := llms.ToolCallResponse{
				ToolCallID: toolCall.ID,
				Name:       toolCall.FunctionCall.Name,
				Content:    processedResponse,
			}
			c.addToHistory(channelID, llms.ChatMessageTypeTool, toolResponsePart)
		}
	}
}

// truncateForLog truncates a string for log output
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
