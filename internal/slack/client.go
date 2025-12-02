// Package slackbot implements the Slack integration for the MCP client
// It provides event handling, message processing, and integration with LLM services
package slackbot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/llms"
	customErrors "github.com/tuannvm/slack-mcp-client/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/handlers"
	"github.com/tuannvm/slack-mcp-client/internal/llm"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
	"github.com/tuannvm/slack-mcp-client/internal/observability"
	"github.com/tuannvm/slack-mcp-client/internal/rag"
)

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
	discoveredTools map[string]mcp.ToolInfo
	tracingHandler  observability.TracingHandler
}

// Message represents a message in the conversation history
type Message struct {
	Role           string    // "user", "assistant", or "tool"
	Content        string    // The message content
	Timestamp      time.Time // When the message was sent/received
	SlackTimestamp string    // Slack's timestamp format (string)
	UserID         string
	RealName       string
	Email          string
}

// NewClient creates a new Slack client instance.
func NewClient(userFrontend UserFrontend, stdLogger *logging.Logger, mcpClients map[string]*mcp.Client,
	discoveredTools map[string]mcp.ToolInfo, cfg *config.Config) (*Client, error) {

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
			toolName, toolInfo.ToolDescription, toolInfo.InputSchema, toolInfo.ServerName)
	}

	// Create a map of raw clients to pass to the bridge
	rawClientMap := make(map[string]interface{})
	for name, client := range mcpClients {
		rawClientMap[name] = client
		clientLogger.DebugKV("Adding MCP client to raw map for bridge", "name", name)
	}

	// Check if RAG client is available in config and add it
	if cfg.RAG.Enabled {
		clientLogger.InfoKV("RAG enabled, creating client for bridge integration", "provider", cfg.RAG.Provider)

		// Use the legacy API for now until we properly update the RAG package
		// Convert structured config to legacy format
		ragConfig := map[string]interface{}{
			"provider": cfg.RAG.Provider,
		}

		// Add provider-specific settings
		if providerSettings, exists := cfg.RAG.Providers[cfg.RAG.Provider]; exists {
			switch cfg.RAG.Provider {
			case "simple":
				ragConfig["database_path"] = providerSettings.DatabasePath
			case "openai":
				if providerSettings.IndexName != "" {
					ragConfig["vector_store_name"] = providerSettings.IndexName
				}
				if providerSettings.VectorStoreID != "" {
					ragConfig["vector_store_id"] = providerSettings.VectorStoreID
				}
				if providerSettings.Dimensions > 0 {
					ragConfig["dimensions"] = providerSettings.Dimensions
				}
				if providerSettings.SimilarityMetric != "" {
					ragConfig["similarity_metric"] = providerSettings.SimilarityMetric
				}
				if providerSettings.MaxResults > 0 {
					ragConfig["max_results"] = providerSettings.MaxResults
				}
				if providerSettings.ScoreThreshold > 0 {
					ragConfig["score_threshold"] = providerSettings.ScoreThreshold
				}
				if providerSettings.RewriteQuery {
					ragConfig["rewrite_query"] = providerSettings.RewriteQuery
				}
				if providerSettings.VectorStoreNameRegex != "" {
					ragConfig["vector_store_name_regex"] = providerSettings.VectorStoreNameRegex
				}
				if providerSettings.VectorStoreMetadataKey != "" {
					ragConfig["vs_metadata_key"] = providerSettings.VectorStoreMetadataKey
				}
				if providerSettings.VectorStoreMetadataValue != "" {
					ragConfig["vs_metadata_value"] = providerSettings.VectorStoreMetadataValue
				}
				// Add OpenAI API key from LLM config or environment
				if openaiConfig, exists := cfg.LLM.Providers["openai"]; exists && openaiConfig.APIKey != "" {
					ragConfig["api_key"] = openaiConfig.APIKey
				}
			}
		}

		// Set chunk size
		if cfg.RAG.ChunkSize > 0 {
			ragConfig["chunk_size"] = cfg.RAG.ChunkSize
		}

		ragClient, err := rag.NewClientWithProvider(cfg.RAG.Provider, ragConfig)
		if err != nil {
			clientLogger.ErrorKV("Failed to create RAG client", "error", err)
		} else {
			rawClientMap["rag"] = ragClient
			clientLogger.DebugKV("Added RAG client to raw map for bridge", "name", "rag")
		}
	}

	logLevel := getLogLevel(stdLogger)

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

	// Load custom prompt from file if specified and customPrompt is empty
	if cfg.LLM.CustomPromptFile != "" && cfg.LLM.CustomPrompt == "" {
		content, err := os.ReadFile(cfg.LLM.CustomPromptFile)
		if err != nil {
			clientLogger.ErrorKV("Failed to read custom prompt file", "file", cfg.LLM.CustomPromptFile, "error", err)
			return nil, customErrors.WrapConfigError(err, "custom_prompt_file_read_failed", "Failed to read custom prompt file")
		}
		cfg.LLM.CustomPrompt = string(content)
		clientLogger.InfoKV("Loaded custom prompt from file", "file", cfg.LLM.CustomPromptFile)
	}

	// Pass the raw map to the bridge with the configured log level
	llmMCPBridge := handlers.NewLLMMCPBridgeFromClientsWithLogLevel(
		rawClientMap,
		clientLogger.StdLogger(),
		discoveredTools,
		logLevel,
		registry,
		cfg,
	)
	clientLogger.InfoKV("LLM-MCP bridge initialized", "clients", len(mcpClients), "tools", len(discoveredTools))

	// Initialize observability
	tracingHandler := observability.NewTracingHandler(cfg, clientLogger)

	// --- Create and return Client instance ---
	return &Client{
		logger:          clientLogger,
		userFrontend:    userFrontend,
		mcpClients:      mcpClients,
		llmMCPBridge:    llmMCPBridge,
		llmRegistry:     registry,
		cfg:             cfg,
		messageHistory:  make(map[string][]Message),
		historyLimit:    cfg.Slack.MessageHistory, // Store configured number of messages per channel
		discoveredTools: discoveredTools,
		tracingHandler:  tracingHandler,
	}, nil
}

// Run starts the Socket Mode event loop and event handling.
func (c *Client) Run() error {
	go c.handleEvents()
	c.logger.Info("Starting Slack Socket Mode listener...")
	return c.userFrontend.Run()
}

// Close gracefully closes the Slack client
func (c *Client) Close() error {
	c.logger.Info("Closing Slack client...")
	// Note: socketmode.Client doesn't have a public Close method
	// The client will stop when the context is cancelled or when there's a connection error
	return nil
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
			c.logger.InfoKV("Received app mention in channel", "channel", ev.Channel, "user", ev.User, "text", ev.Text, "ThreadTS", ev.ThreadTimeStamp)
			messageText := c.userFrontend.RemoveBotMention(ev.Text)
			profile, err := c.userFrontend.GetUserInfo(ev.User)
			if err != nil {
				c.logger.WarnKV("Failed to get user info", "user", ev.User, "error", err)
				profile = &UserProfile{userId: ev.User, realName: "Unknown", email: ""}
			}

			parentTS := ev.ThreadTimeStamp
			if parentTS == "" {
				parentTS = ev.TimeStamp // Use the original message timestamp if no thread
			}
			// Use handleUserPrompt for app mentions too, for consistency
			go c.handleUserPrompt(strings.TrimSpace(messageText), ev.Channel, parentTS, ev.TimeStamp, profile)

		case *slackevents.MessageEvent:
			isDirectMessage := strings.HasPrefix(ev.Channel, "D")
			isValidUser := c.userFrontend.IsValidUser(ev.User)
			isNotEdited := ev.SubType != "message_changed"
			isBot := ev.BotID != "" || ev.SubType == "bot_message"

			if isDirectMessage && isValidUser && isNotEdited && !isBot {
				c.logger.InfoKV("Received direct message in channel", "channel", ev.Channel, "user", ev.User, "text", ev.Text, "ThreadTS", ev.ThreadTimeStamp)
				profile, err := c.userFrontend.GetUserInfo(ev.User)
				if err != nil {
					c.logger.WarnKV("Failed to get user info", "user", ev.User, "error", err)
					profile = &UserProfile{userId: ev.User, realName: "Unknown", email: ""}
				}

				parentTS := ev.ThreadTimeStamp
				if parentTS == "" {
					parentTS = ev.TimeStamp // Use the original message timestamp if no thread
				}
				go c.handleUserPrompt(ev.Text, ev.Channel, parentTS, ev.TimeStamp, profile) // Use goroutine to avoid blocking event loop
			}

		default:
			c.logger.DebugKV("Unsupported inner event type", "type", fmt.Sprintf("%T", innerEvent.Data))
		}
	default:
		c.logger.DebugKV("Unsupported outer event type", "type", event.Type)
	}
}

func historyKey(channelID, threadTS string) string {
	return fmt.Sprintf("%s:%s", channelID, threadTS)
}

// addToHistory adds a message to the channel history
func (c *Client) addToHistory(channelID, threadTS, timestamp, role, content, userID, realName, email string) {
	key := historyKey(channelID, threadTS)
	history, exists := c.messageHistory[key]
	if !exists {
		history = []Message{}
	}

	// Add the new message
	message := Message{
		Role:           role,
		Content:        content,
		Timestamp:      time.Now(),
		SlackTimestamp: timestamp,
		UserID:         userID,
		RealName:       realName,
		Email:          email,
	}
	history = append(history, message)

	// Limit history size
	if len(history) > c.historyLimit {
		history = history[len(history)-c.historyLimit:]
	}

	c.messageHistory[key] = history
}

// getContextFromHistory builds a context string from message history
//
//nolint:unused // Reserved for future use
func (c *Client) getContextFromHistory(channelID string, threadTS string) string {
	history, exists := c.messageHistory[historyKey(channelID, threadTS)]
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
			userInfo := ""
			if msg.UserID != "" {
				userInfo = fmt.Sprintf(" (User: %s, Name: %s, Email: %s)", msg.UserID, msg.RealName, msg.Email)
			}
			sanitizedContent := strings.ReplaceAll(msg.Content, "\n", " \\n ")
			contextBuilder.WriteString(fmt.Sprintf("%s: %s%s\n", prefix, sanitizedContent, userInfo))
		}
	}
	contextBuilder.WriteString("---\n") // Clearer end marker

	contextString := contextBuilder.String()
	c.logger.DebugKV("Built conversation context", "channel", channelID, "context", contextString) // Log the built context
	return contextString
}

// handleUserPrompt sends the user's text to the configured LLM provider.
func (c *Client) handleUserPrompt(userPrompt, channelID, threadTS string, timestamp string, profile *UserProfile) {
	c.logger.DebugKV("Routing prompt via configured provider", "provider", c.cfg.LLM.Provider)
	c.logger.DebugKV("User prompt", "text", userPrompt)

	// Security validation check
	securityResult := c.cfg.ValidateAccess(profile.userId, channelID)
	if !securityResult.Allowed {
		// Log unauthorized access attempt if enabled
		if c.cfg.Security.LogUnauthorized != nil && *c.cfg.Security.LogUnauthorized {
			c.logger.WarnKV("security: Access denied",
				"user_id", profile.userId,
				"channel_id", channelID,
				"allowed", false,
				"reason", securityResult.Reason,
				"strict_mode", c.cfg.Security.StrictMode,
			)
		}

		// Send rejection message if configured
		if c.cfg.Security.RejectionMessage != "" {
			c.userFrontend.SendMessage(channelID, threadTS, c.cfg.Security.RejectionMessage)
		}

		// Early return - do not process the request further
		return
	}

	// Log successful access if security is enabled
	if c.cfg.Security.Enabled {
		c.logger.InfoKV("security: Access granted",
			"user_id", profile.userId,
			"channel_id", channelID,
			"allowed", true,
			"reason", securityResult.Reason,
			"strict_mode", c.cfg.Security.StrictMode,
		)
	}

	ctx, span := c.tracingHandler.StartTrace(context.Background(), "slack-user-interaction", userPrompt, map[string]string{
		"session_id":   fmt.Sprintf("%s-%s", channelID, threadTS),
		"user_email":   profile.email,
		"llm_provider": c.cfg.LLM.Provider,
		"use_agent":    fmt.Sprintf("%t", c.cfg.LLM.UseAgent),
	})
	defer span.End()

	// Fetch thread replies from slack
	replies, err := c.userFrontend.GetThreadReplies(channelID, threadTS)
	if err != nil {
		c.logger.ErrorKV("Failed to fetch thread replies", "channel", channelID, "thread_ts", threadTS, "error", err)
	} else {
		c.logger.DebugKV("Fetched thread replies", "channel", channelID, "thread_ts", threadTS, "count", len(replies))
		existingMessages := make(map[string]bool)
		history := c.messageHistory[historyKey(channelID, threadTS)]
		for _, msg := range history {
			// key := fmt.Sprintf("%s:%s", msg.UserID, msg.Content)
			existingMessages[msg.SlackTimestamp] = true
		}
		for _, reply := range replies {
			// replyKey := fmt.Sprintf("%s:%s", reply.User, reply.Text)
			if !existingMessages[reply.Timestamp] {
				role := "user"
				if reply.BotID != "" {
					role = "assistant"
				}
				replyProfile, err := c.userFrontend.GetUserInfo(reply.User)
				if err != nil {
					c.logger.WarnKV("Failed to get user info", "user", reply.User, "error", err)
					replyProfile = &UserProfile{userId: reply.User, realName: "Unknown", email: ""}
				}
				c.addToHistory(channelID, threadTS, reply.Timestamp, role, reply.Text, replyProfile.userId, replyProfile.realName, replyProfile.email)
				existingMessages[reply.Timestamp] = true
			}
		}
	}

	// Get context from history
	contextHistory := c.getContextFromHistory(channelID, threadTS)

	c.addToHistory(channelID, threadTS, timestamp, "user", userPrompt, profile.userId, profile.realName, profile.email) // Add user message to history

	// Show a temporary "typing" indicator
	c.userFrontend.SendMessage(channelID, threadTS, c.cfg.Slack.ThinkingMessage)

	if !c.cfg.LLM.UseAgent {
		// Prepare the final prompt with custom prompt as system instruction
		var finalPrompt string
		customPrompt := c.cfg.LLM.CustomPrompt
		if customPrompt != "" {
			// Use custom prompt as system instruction, then add user prompt
			finalPrompt = fmt.Sprintf("System instructions: %s\n\nUser: %s", customPrompt, userPrompt)
			c.logger.DebugKV("Using custom prompt as system instruction", "custom_prompt_length", len(customPrompt))
		} else {
			finalPrompt = userPrompt
		}

		llmCtx, llmSpan := c.tracingHandler.StartLLMSpan(ctx, "llm-call", c.cfg.LLM.Providers[c.cfg.LLM.Provider].Model, finalPrompt, map[string]interface{}{
			"temperature": c.cfg.LLM.Providers[c.cfg.LLM.Provider].Temperature,
			"max_tokens":  c.cfg.LLM.Providers[c.cfg.LLM.Provider].MaxTokens,
		})

		startTime := time.Now()

		// Call LLM using the integrated logic with system instruction
		llmResponse, err := c.llmMCPBridge.CallLLM(finalPrompt, contextHistory)

		duration := time.Since(startTime)

		// Set duration and handle response
		c.tracingHandler.SetDuration(llmSpan, duration)

		if err != nil {
			c.logger.ErrorKV("Error from LLM provider", "provider", c.cfg.LLM.Provider, "error", err)
			c.userFrontend.SendMessage(channelID, threadTS, fmt.Sprintf("Sorry, I encountered an error with the LLM provider ('%s'): %v", c.cfg.LLM.Provider, err))
			c.tracingHandler.RecordError(llmSpan, err, "ERROR")
			llmSpan.End()
			return
		}

		// set output and token usage
		c.tracingHandler.SetOutput(llmSpan, llmResponse.Content)

		// Extract and set token usage details
		usageDetails := map[string]int{
			"prompt_tokens":    getIntFromMap(llmResponse.GenerationInfo, "PromptTokens"),
			"output_tokens":    getIntFromMap(llmResponse.GenerationInfo, "CompletionTokens"),
			"total_tokens":     getIntFromMap(llmResponse.GenerationInfo, "TotalTokens"),
			"reasoning_tokens": getIntFromMap(llmResponse.GenerationInfo, "ReasoningTokens"),
		}

		if usageDetails["total_tokens"] > 0 {
			c.tracingHandler.SetTokenUsage(llmSpan, usageDetails["prompt_tokens"], usageDetails["output_tokens"], usageDetails["reasoning_tokens"], usageDetails["total_tokens"])
		}

		c.logger.InfoKV("Received response from LLM", "provider", c.cfg.LLM.Provider, "length", len(llmResponse.Content))
		c.tracingHandler.RecordSuccess(llmSpan, "LLM call succeeded")
		llmSpan.End()

		// Process the LLM response through the MCP pipeline
		c.processLLMResponseAndReply(llmCtx, llmResponse, userPrompt, channelID, threadTS)
	} else {
		// Agent path with enhanced tracing
		agentCtx, agentSpan := c.tracingHandler.StartSpan(ctx, "llm-agent-call", "generation", userPrompt, map[string]string{
			"provider": c.cfg.LLM.Provider,
			"is_agent": "true",
		})
		sendMsg := func(msg string) {
			// Trace each messages sent by the agent
			_, msgSpan := c.tracingHandler.StartSpan(agentCtx, "agent-message-send", "event", msg, map[string]string{
				"channel_id":     channelID,
				"thread_ts":      threadTS,
				"message_type":   "agent_intermediate",
				"message_length": fmt.Sprintf("%d", len(msg)),
			})

			c.addToHistory(channelID, threadTS, "", "assistant", msg, "", "", "") // Original LLM response (tool call JSON)
			c.userFrontend.SendMessage(channelID, threadTS, msg)
			c.tracingHandler.RecordSuccess(msgSpan, "Agent message sent successfully")
			msgSpan.End()
		}

		startTime := time.Now()
		llmResponse, err := c.llmMCPBridge.CallLLMAgent(
			profile.realName,
			c.cfg.LLM.CustomPrompt,
			userPrompt,
			contextHistory,
			&agentCallbackHandler{
				callbacks.SimpleHandler{},
				sendMsg,
				c.logger,
			})
		duration := time.Since(startTime)

		// Set duration
		c.tracingHandler.SetDuration(agentSpan, duration)

		if err != nil {
			c.logger.ErrorKV("Error from LLM provider", "provider", c.cfg.LLM.Provider, "error", err)
			c.userFrontend.SendMessage(channelID, threadTS, fmt.Sprintf("Sorry, I encountered an error with the LLM provider ('%s'): %v", c.cfg.LLM.Provider, err))
			c.tracingHandler.RecordError(agentSpan, err, "ERROR")
			agentSpan.End()
			return
		}
		c.logger.InfoKV("Received response from LLM", "provider", c.cfg.LLM.Provider, "length", len(llmResponse))

		// Set Output
		c.tracingHandler.SetOutput(agentSpan, llmResponse)

		// Send the final response back to Slack
		if llmResponse == "" {
			c.userFrontend.SendMessage(channelID, threadTS, "(LLM returned an empty response)")
			c.tracingHandler.RecordError(agentSpan, fmt.Errorf("LLM returned an empty response"), "ERROR")

		} else {
			c.tracingHandler.RecordSuccess(agentSpan, "LLM agent call succeeded")
		}
		agentSpan.End()
	}
}

// getIntFromMap safely extracts an int value from a map[string]interface{} by key.
func getIntFromMap(m map[string]interface{}, key string) int {
	if m == nil {
		return 0
	}
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case float32:
			return int(v)
		case json.Number:
			i, _ := v.Int64()
			return int(i)
		case string:
			var i int
			_, err := fmt.Sscanf(v, "%d", &i)
			if err == nil {
				return i
			}
		}
	}
	return 0
}

func (c *Client) extractToolNameFromResponse(response string) string {
	// Try to parse JSON to extract tool name
	var toolCall struct {
		Tool string `json:"tool"`
	}

	if err := json.Unmarshal([]byte(response), &toolCall); err == nil && toolCall.Tool != "" {
		return toolCall.Tool
	}

	// Fallback: look for tool names in the response text
	for toolName := range c.discoveredTools {
		if strings.Contains(response, toolName) {
			return toolName
		}
	}

	return "unknown"
}

func (c *Client) detectIfToolUsedLLM(toolName, response string) bool {
	// Simple heuristics to detect if tool used LLM
	switch {
	case strings.Contains(strings.ToLower(toolName), "rag"):
		return true
	case strings.Contains(strings.ToLower(toolName), "search"):
		return len(response) > 500 // Long responses likely used LLM for synthesis
	case strings.Contains(strings.ToLower(toolName), "summarize"):
		return true
	default:
		return false
	}
}

func (c *Client) estimateToolTokenUsage(toolName, prompt, response string) int {
	// Rough token estimation (1 token ≈ 4 characters)
	if !c.detectIfToolUsedLLM(toolName, response) {
		return 0 // Tool doesn't use LLM
	}

	// Estimate based on tool type
	switch {
	case strings.Contains(strings.ToLower(toolName), "rag"):
		// RAG typically uses prompt + retrieved context + generation
		return (len(prompt) + len(response) + 1000) / 4 // +1000 for retrieved context
	case strings.Contains(strings.ToLower(toolName), "search"):
		// Search + summarization
		return (len(prompt) + len(response) + 500) / 4 // +500 for search results
	default:
		return (len(prompt) + len(response)) / 4
	}
}

// processLLMResponseAndReply processes the LLM response, handles tool results with re-prompting, and sends the final reply.
// Incorporates logic previously in LLMClient.ProcessToolResponse.
func (c *Client) processLLMResponseAndReply(traceCtx context.Context, llmResponse *llms.ContentChoice, userPrompt, channelID, threadTS string) {
	// Start tool processing span
	ctx, span := c.tracingHandler.StartSpan(traceCtx, "tool-processing", "span", userPrompt, map[string]string{
		"channel_id":      channelID,
		"thread_ts":       threadTS,
		"original_prompt": userPrompt,
		"response_length": fmt.Sprintf("%d", len(llmResponse.Content)),
	})
	defer span.End()
	// Log the raw LLM response for debugging
	c.logger.DebugKV("Raw LLM response", "response", logging.TruncateForLog(fmt.Sprintf("%v", llmResponse), 500))
	extraArgs := map[string]interface{}{
		"channel_id": channelID,
		"thread_ts":  threadTS,
	}
	c.logger.DebugKV("Added extra arguments", "channel_id", channelID, "thread_ts", threadTS)

	// Create a context with timeout for tool processing
	toolCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	// --- Process Tool Response (Logic from LLMClient.ProcessToolResponse) ---
	var finalResponse string
	var isToolResult bool
	var toolProcessingErr error

	if c.llmMCPBridge == nil {
		// If bridge is nil, just use the original response
		finalResponse = llmResponse.Content
		isToolResult = false
		toolProcessingErr = nil
		c.logger.Warn("LLMMCPBridge is nil, skipping tool processing")
	} else {
		// Extract tool name before execution
		executedToolName := c.extractToolNameFromResponse(llmResponse.Content)

		// Start tool execution span
		_, toolExecSpan := c.tracingHandler.StartSpan(ctx, "tool-execution", "event", "", map[string]string{
			"bridge_available": "true",
			"response_type":    "processing",
			"tool_name":        executedToolName,
		})
		startTime := time.Now()
		// Process the response through the bridge
		processedResponse, err := c.llmMCPBridge.ProcessLLMResponse(toolCtx, llmResponse, userPrompt, extraArgs)
		toolDuration := time.Since(startTime)
		c.tracingHandler.SetDuration(toolExecSpan, toolDuration)
		if err != nil {
			finalResponse = fmt.Sprintf("Sorry, I encountered an error while trying to use a tool: %v", err)
			isToolResult = false
			toolProcessingErr = err // Store the error
			c.tracingHandler.RecordError(toolExecSpan, err, "ERROR")
		} else {
			// If the processed response is different from the original, a tool was executed
			if processedResponse != llmResponse.Content {
				finalResponse = processedResponse
				isToolResult = true
				c.tracingHandler.SetOutput(toolExecSpan, processedResponse)
				c.tracingHandler.RecordSuccess(toolExecSpan, "Tool executed successfully")
			} else {
				// No tool was executed
				finalResponse = llmResponse.Content
				isToolResult = false
				c.tracingHandler.SetOutput(toolExecSpan, "No tool execution required")
				c.tracingHandler.RecordSuccess(toolExecSpan, "No tool processing needed")
			}
		}
		toolExecSpan.End()
	}
	// --- End of Process Tool Response Logic ---

	if toolProcessingErr != nil {
		c.tracingHandler.RecordError(span, toolProcessingErr, "ERROR")
		c.logger.ErrorKV("Tool processing error", "error", toolProcessingErr)
		c.userFrontend.SendMessage(channelID, threadTS, finalResponse) // Post the error message
		return
	}

	if isToolResult {
		c.logger.Debug("Tool executed. Re-prompting LLM with tool result.")
		c.logger.DebugKV("Tool result", "result", logging.TruncateForLog(finalResponse, 500))

		// Always re-prompt LLM with tool results for synthesis
		// Construct a new prompt incorporating the original prompt and the tool result
		rePrompt := fmt.Sprintf("The user asked: '%s'\n\nI searched the knowledge base and found the following relevant information:\n```\n%s\n```\n\nPlease analyze and synthesize this retrieved information to provide a comprehensive response to the user's request. Use the detailed information from the search results according to your system instructions.", userPrompt, finalResponse)

		// Start re-prompt span
		executedToolName := c.extractToolNameFromResponse(llmResponse.Content)
		_, repromptSpan := c.tracingHandler.StartLLMSpan(ctx, "llm-reprompt",
			c.cfg.LLM.Providers[c.cfg.LLM.Provider].Model,
			rePrompt,
			map[string]interface{}{
				"is_reprompt":           true,
				"tool_name":             executedToolName,                                                      // Add this
				"tool_estimated_tokens": c.estimateToolTokenUsage(executedToolName, userPrompt, finalResponse), // Add this
			})

		// Add history
		c.addToHistory(channelID, threadTS, "", "assistant", llmResponse.Content, "", "", "") // Original LLM response (tool call JSON)
		c.addToHistory(channelID, threadTS, "", "tool", finalResponse, "", "", "")            // Tool execution result

		c.logger.DebugKV("Re-prompting LLM", "prompt", rePrompt)

		// Re-prompt using the LLM client with custom prompt as system instruction
		var repromptErr error
		// Prepare the re-prompt with custom prompt as system instruction
		var finalRePrompt string
		customPrompt := c.cfg.LLM.CustomPrompt

		if customPrompt != "" {
			// Use custom prompt as system instruction for re-prompt too
			finalRePrompt = fmt.Sprintf("System instructions: %s\n\n%s", customPrompt, rePrompt)
		} else {
			finalRePrompt = rePrompt
		}
		startTime := time.Now()

		finalResStruct, repromptErr := c.llmMCPBridge.CallLLM(finalRePrompt, c.getContextFromHistory(channelID, threadTS))

		duration := time.Since(startTime)
		// Set duration
		c.tracingHandler.SetDuration(repromptSpan, duration)

		if repromptErr != nil {
			c.tracingHandler.RecordError(repromptSpan, repromptErr, "ERROR")
			c.logger.ErrorKV("Error during LLM re-prompt", "error", repromptErr)
			// Fallback: Show the tool result and the error
			finalResponse = fmt.Sprintf("Tool Result:\n```%s```\n\n(Error generating final response: %v)", finalResponse, repromptErr)
			c.tracingHandler.RecordError(span, repromptErr, "ERROR")
		} else {
			c.logger.DebugKV("LLM re-prompt successful", "response", logging.TruncateForLog(fmt.Sprintf("%v", finalResStruct), 500))
			finalResponse = finalResStruct.Content
			repromptUsageDetails := map[string]int{
				"prompt_tokens":     getIntFromMap(finalResStruct.GenerationInfo, "PromptTokens"),
				"completion_tokens": getIntFromMap(finalResStruct.GenerationInfo, "CompletionTokens"),
				"reasoning_tokens":  getIntFromMap(finalResStruct.GenerationInfo, "ReasoningTokens"),
				"total_tokens":      getIntFromMap(finalResStruct.GenerationInfo, "TotalTokens"),
			}
			if repromptUsageDetails["total_tokens"] > 0 {
				c.tracingHandler.SetTokenUsage(repromptSpan,
					repromptUsageDetails["prompt_tokens"],
					repromptUsageDetails["completion_tokens"],
					repromptUsageDetails["reasoning_tokens"],
					repromptUsageDetails["total_tokens"])
			}
			c.tracingHandler.SetOutput(repromptSpan, finalResponse)
			c.tracingHandler.RecordSuccess(repromptSpan, "LLM re-prompt successful")
		}
		repromptSpan.End()
	} else {
		// No tool was executed, add assistant response to history
		c.addToHistory(channelID, threadTS, "", "assistant", finalResponse, "", "", "")
	}

	// Start message sending span
	_, msgSpan := c.tracingHandler.StartSpan(ctx, "slack-message-send", "event", userPrompt, map[string]string{
		"channel_id":            channelID,
		"thread_ts":             threadTS,
		"final_response_length": fmt.Sprintf("%d", len(finalResponse)),
		"is_empty_response":     fmt.Sprintf("%t", finalResponse == ""),
		"had_tool_execution":    fmt.Sprintf("%t", isToolResult),
	})
	// Send the final response back to Slack
	if finalResponse == "" {
		c.userFrontend.SendMessage(channelID, threadTS, "(LLM returned an empty response)")
		c.tracingHandler.RecordError(msgSpan, fmt.Errorf("LLM returned an empty response"), "ERROR")

	} else {
		c.userFrontend.SendMessage(channelID, threadTS, finalResponse)
		c.tracingHandler.RecordSuccess(msgSpan, "Slack message sent successfully")
	}
	msgSpan.End()
	// Set final trace output
	c.tracingHandler.SetOutput(span, finalResponse)
	c.tracingHandler.RecordSuccess(span, "Tool processing completed")
}
