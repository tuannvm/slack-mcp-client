// Package slackbot implements the Slack integration for the MCP client
// It provides event handling, message processing, and integration with LLM services
package slackbot

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/slack-go/slack"
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
	"github.com/tuannvm/slack-mcp-client/internal/rag"
)

// Client represents the Slack client application.
type Client struct {
	logger             *logging.Logger // Structured logger
	userFrontend       UserFrontend
	mcpClients         map[string]*mcp.Client
	llmMCPBridge       *handlers.LLMMCPBridge
	llmRegistry        *llm.ProviderRegistry // LLM provider registry
	cfg                *config.Config        // Holds the application configuration
	messageHistory     map[string][]Message
	historyLimit       int
	discoveredTools    map[string]mcp.ToolInfo
	participatedThreads map[string]bool // Track threads where bot has responded (key: "channel:threadTS")
	showThoughts       map[string]bool // Track showThoughts preference per channel/thread (key: "channel:threadTS")
}

// Message represents a message in the conversation history
type Message struct {
	Role      string    // "user", "assistant", or "tool"
	Content   string    // The message content
	Timestamp time.Time // When the message was sent/received
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

	// Create a map of raw clients to pass to the bridge
	rawClientMap := make(map[string]interface{})

	// Add canvas tools if enabled
	var canvasTool *CanvasTool
	if cfg.Canvas.Enabled {
		// Get Slack client from userFrontend (assuming it's a *SlackClient)
		if slackClient, ok := userFrontend.(*SlackClient); ok {
			canvasTool = NewCanvasTool(slackClient.api, clientLogger)
			
			// Add canvas tools to discoveredTools
			createTool := canvasTool.CreateCanvasToolInfo()
			editTool := canvasTool.EditCanvasToolInfo()
			
			discoveredTools["canvas_create"] = createTool
			discoveredTools["canvas_edit"] = editTool
			
			// Add canvas tool to raw client map
			rawClientMap["slack-native"] = canvasTool
			
			clientLogger.InfoKV("Canvas tools enabled", "tools", []string{"canvas_create", "canvas_edit"})
		}
	}

	clientLogger.Printf("Available tools (%d):", len(discoveredTools))
	for toolName, toolInfo := range discoveredTools {
		clientLogger.Printf("- %s (Desc: %s, Schema: %v, Server: %s)",
			toolName, toolInfo.ToolDescription, toolInfo.InputSchema, toolInfo.ServerName)
	}

	// rawClientMap was already created above
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

	// --- Create and return Client instance ---
	return &Client{
		logger:              clientLogger,
		userFrontend:        userFrontend,
		mcpClients:          mcpClients,
		llmMCPBridge:        llmMCPBridge,
		llmRegistry:         registry,
		cfg:                 cfg,
		messageHistory:      make(map[string][]Message),
		historyLimit:        cfg.Slack.MessageHistory, // Store configured number of messages per channel
		discoveredTools:     discoveredTools,
		participatedThreads: make(map[string]bool),
		showThoughts:        make(map[string]bool),
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
		case socketmode.EventTypeSlashCommand:
			cmd, ok := evt.Data.(slack.SlashCommand)
			if !ok {
				c.logger.WarnKV("Ignored unexpected SlashCommand event type", "type", fmt.Sprintf("%T", evt.Data))
				continue
			}
			c.userFrontend.Ack(*evt.Request)
			c.logger.InfoKV("Received slash command", "command", cmd.Command, "user", cmd.UserID)
			c.handleSlashCommandEvent(cmd)
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

			userInfo, err := c.userFrontend.GetUserInfo(ev.User)
			if err != nil {
				c.logger.ErrorKV("Failed to get user info", "user", ev.User, "error", err)
				return
			}

			// Use handleUserPrompt for app mentions too, for consistency
			go c.handleUserPrompt(strings.TrimSpace(messageText), ev.Channel, ev.TimeStamp, ev.TimeStamp, userInfo.Profile.DisplayName)

		case *slackevents.MessageEvent:
			isDirectMessage := strings.HasPrefix(ev.Channel, "D")
			isValidUser := c.userFrontend.IsValidUser(ev.User)
			isNotEdited := ev.SubType != "message_changed"
			isBot := ev.BotID != "" || ev.SubType == "bot_message"
			isInParticipatedThread := c.hasParticipatedInThread(ev.Channel, ev.ThreadTimeStamp)

			userInfo, err := c.userFrontend.GetUserInfo(ev.User)
			if err != nil {
				c.logger.ErrorKV("Failed to get user info", "user", ev.User, "error", err)
				return
			}

			// Process message if:
			// 1. It's a direct message, OR
			// 2. It's in a thread where the bot has participated
			if (isDirectMessage || isInParticipatedThread) && isValidUser && isNotEdited && !isBot {
				c.logger.InfoKV("Received message", "channel", ev.Channel, "user", ev.User, "text", ev.Text, 
					"isDM", isDirectMessage, "isThread", isInParticipatedThread, "threadTS", ev.ThreadTimeStamp)

				// For thread messages, use the thread timestamp; for channel messages, start a new thread
				threadTS := ev.ThreadTimeStamp
				if threadTS == "" && !isDirectMessage {
					// If it's not a DM and not in a thread, this shouldn't happen based on our logic
					// but if it does, use the message timestamp as thread start
					threadTS = ev.TimeStamp
				}

				// For thread replies, we need the original message timestamp for reactions
				messageTS := ev.TimeStamp
				if threadTS == "" {
					// This is a new message, use its timestamp for reactions
					messageTS = ev.TimeStamp
				}
				
				go c.handleUserPrompt(ev.Text, ev.Channel, threadTS, messageTS, userInfo.Profile.DisplayName) // Use goroutine to avoid blocking event loop
			}

		default:
			c.logger.DebugKV("Unsupported inner event type", "type", fmt.Sprintf("%T", innerEvent.Data))
		}
	default:
		c.logger.DebugKV("Unsupported outer event type", "type", event.Type)
	}
}

// addToHistory adds a message to the channel/thread history
func (c *Client) addToHistory(channelID, threadTS, role, content string) {
	// Use thread-aware key: for threads use "channel:threadTS", for non-threads just use channelID
	historyKey := channelID
	if threadTS != "" {
		historyKey = fmt.Sprintf("%s:%s", channelID, threadTS)
	}
	
	history, exists := c.messageHistory[historyKey]
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

	c.messageHistory[historyKey] = history
	c.logger.DebugKV("Added to history", "key", historyKey, "role", role, "contentLength", len(content))
}

// trackThreadParticipation marks a thread as participated by the bot
func (c *Client) trackThreadParticipation(channelID, threadTS string) {
	if threadTS != "" {
		threadKey := fmt.Sprintf("%s:%s", channelID, threadTS)
		c.participatedThreads[threadKey] = true
		c.logger.DebugKV("Tracked thread participation", "channel", channelID, "thread", threadTS)
	}
}

// hasParticipatedInThread checks if the bot has participated in a thread
func (c *Client) hasParticipatedInThread(channelID, threadTS string) bool {
	if threadTS == "" {
		return false
	}
	threadKey := fmt.Sprintf("%s:%s", channelID, threadTS)
	return c.participatedThreads[threadKey]
}

// getContextFromHistory builds a context string from message history
//
//nolint:unused // Reserved for future use
func (c *Client) getContextFromHistory(channelID, threadTS string) string {
	// Use thread-aware key: for threads use "channel:threadTS", for non-threads just use channelID
	historyKey := channelID
	if threadTS != "" {
		historyKey = fmt.Sprintf("%s:%s", channelID, threadTS)
	}
	
	history, exists := c.messageHistory[historyKey]
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
	c.logger.DebugKV("Built conversation context", "key", historyKey, "messages", len(history)) // Log the built context
	return contextString
}

// handleUserPrompt sends the user's text to the configured LLM provider.
func (c *Client) handleUserPrompt(userPrompt, channelID, threadTS, messageTS, userDisplayName string) {
	c.logger.DebugKV("Routing prompt via configured provider", "provider", c.cfg.LLM.Provider)
	c.logger.DebugKV("User prompt", "text", userPrompt)

	// Handle commands (both slash-style and text commands)
	trimmedPrompt := strings.TrimSpace(userPrompt)
	lowerPrompt := strings.ToLower(trimmedPrompt)
	
	// Check for thinking mode commands (text or slash style)
	if lowerPrompt == "think silent" || lowerPrompt == "think quietly" || 
	   lowerPrompt == "/think_silent" || lowerPrompt == "/think_quietly" ||
	   lowerPrompt == "!think_silent" || lowerPrompt == "!think_quietly" {
		c.setThinkingMode(false, channelID, threadTS)
		return
	}
	
	if lowerPrompt == "think aloud" || lowerPrompt == "think loud" ||
	   lowerPrompt == "/think_aloud" || lowerPrompt == "/think_loud" ||
	   lowerPrompt == "!think_aloud" || lowerPrompt == "!think_loud" {
		c.setThinkingMode(true, channelID, threadTS)
		return
	}

	// Get context from history
	contextHistory := c.getContextFromHistory(channelID, threadTS)

	c.addToHistory(channelID, threadTS, "user", userPrompt) // Add user message to history

	// Add thinking emoji reaction
	err := c.userFrontend.AddReaction(channelID, messageTS, "thinking_face")
	if err != nil {
		c.logger.ErrorKV("Failed to add thinking reaction", "error", err)
	}

	if !c.cfg.LLM.UseAgent {
		// Prepare the final prompt with custom prompt as system instruction
		var finalPrompt string
		customPrompt := c.cfg.LLM.CustomPrompt

		if customPrompt != "" {
			// Use custom prompt as system instruction, then add user prompt
			// Include channel ID if canvas is enabled
			if c.cfg.Canvas.Enabled && channelID != "" {
				finalPrompt = fmt.Sprintf("System instructions: %s\n\n[SLACK_CHANNEL_ID: %s]\nUser: %s", customPrompt, channelID, userPrompt)
			} else {
				finalPrompt = fmt.Sprintf("System instructions: %s\n\nUser: %s", customPrompt, userPrompt)
			}
			c.logger.DebugKV("Using custom prompt as system instruction", "custom_prompt_length", len(customPrompt))
		} else {
			// Include channel ID if canvas is enabled
			if c.cfg.Canvas.Enabled && channelID != "" {
				finalPrompt = fmt.Sprintf("[SLACK_CHANNEL_ID: %s]\n%s", channelID, userPrompt)
			} else {
				finalPrompt = userPrompt
			}
		}

		// Call LLM using the integrated logic with system instruction
		llmResponse, err := c.llmMCPBridge.CallLLM(finalPrompt, contextHistory)
		if err != nil {
			c.logger.ErrorKV("Error from LLM provider", "provider", c.cfg.LLM.Provider, "error", err)
			// Remove thinking reaction on error
			c.userFrontend.RemoveReaction(channelID, messageTS, "thinking_face")
			c.userFrontend.SendMessage(channelID, threadTS, fmt.Sprintf("Sorry, I encountered an error with the LLM provider ('%s'): %v", c.cfg.LLM.Provider, err))
			return
		}

		c.logger.InfoKV("Received response from LLM", "provider", c.cfg.LLM.Provider, "length", len(llmResponse.Content))

		// Process the LLM response through the MCP pipeline
		c.processLLMResponseAndReply(llmResponse, userPrompt, channelID, threadTS, messageTS)
	} else {
		// Include channel ID in the prompt if canvas is enabled
		agentPrompt := userPrompt
		if c.cfg.Canvas.Enabled && channelID != "" {
			agentPrompt = fmt.Sprintf("[SLACK_CHANNEL_ID: %s]\n%s", channelID, userPrompt)
		}
		
		llmResponse, err := c.llmMCPBridge.CallLLMAgent(
			userDisplayName,
			c.cfg.LLM.CustomPrompt,
			agentPrompt,
			contextHistory,
			&agentCallbackHandler{
				SimpleHandler: callbacks.SimpleHandler{},
				sendMessage: func(msg string) {
					c.userFrontend.SendMessage(channelID, threadTS, msg)
				},
				showThoughts: c.getShowThoughts(channelID, threadTS),
				storeFullMessage: func(msg string) {
					c.addToHistory(channelID, threadTS, "assistant", msg)
				},
			})
		if err != nil {
			c.logger.ErrorKV("Error from LLM provider", "provider", c.cfg.LLM.Provider, "error", err)
			// Remove thinking reaction on error
			c.userFrontend.RemoveReaction(channelID, messageTS, "thinking_face")
			c.userFrontend.SendMessage(channelID, threadTS, fmt.Sprintf("Sorry, I encountered an error with the LLM provider ('%s'): %v", c.cfg.LLM.Provider, err))
			return
		}
		c.logger.InfoKV("Received response from LLM", "provider", c.cfg.LLM.Provider, "length", len(llmResponse))
		// Send the final response back to Slack
		if llmResponse == "" {
			c.userFrontend.SendMessage(channelID, threadTS, "(LLM returned an empty response)")
		}
		// Remove thinking reaction after response
		c.userFrontend.RemoveReaction(channelID, messageTS, "thinking_face")
		// Track that we've participated in this thread
		c.trackThreadParticipation(channelID, threadTS)
	}
}

// processLLMResponseAndReply processes the LLM response, handles tool results with re-prompting, and sends the final reply.
// Incorporates logic previously in LLMClient.ProcessToolResponse.
func (c *Client) processLLMResponseAndReply(llmResponse *llms.ContentChoice, userPrompt, channelID, threadTS, messageTS string) {
	// Log the raw LLM response for debugging
	c.logger.DebugKV("Raw LLM response", "response", logging.TruncateForLog(fmt.Sprintf("%v", llmResponse), 500))

	// Create a context with timeout for tool processing
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
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
		// Process the response through the bridge
		processedResponse, err := c.llmMCPBridge.ProcessLLMResponse(ctx, llmResponse, userPrompt)
		if err != nil {
			finalResponse = fmt.Sprintf("Sorry, I encountered an error while trying to use a tool: %v", err)
			isToolResult = false
			toolProcessingErr = err // Store the error
		} else {
			// If the processed response is different from the original, a tool was executed
			if processedResponse != llmResponse.Content {
				finalResponse = processedResponse
				isToolResult = true
			} else {
				// No tool was executed
				finalResponse = llmResponse.Content
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
		c.logger.DebugKV("Tool result", "result", logging.TruncateForLog(finalResponse, 500))

		// Always re-prompt LLM with tool results for synthesis
		// Construct a new prompt incorporating the original prompt and the tool result
		rePrompt := fmt.Sprintf("The user asked: '%s'\n\nI searched the knowledge base and found the following relevant information:\n```\n%s\n```\n\nPlease analyze and synthesize this retrieved information to provide a comprehensive response to the user's request. Use the detailed information from the search results according to your system instructions.", userPrompt, finalResponse)

		// Add history for non-comprehensive results
		c.addToHistory(channelID, threadTS, "assistant", llmResponse.Content) // Original LLM response (tool call JSON)
		c.addToHistory(channelID, threadTS, "tool", finalResponse)            // Tool execution result

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

		finalResStruct, repromptErr := c.llmMCPBridge.CallLLM(finalRePrompt, c.getContextFromHistory(channelID, threadTS))
		if repromptErr != nil {
			c.logger.ErrorKV("Error during LLM re-prompt", "error", repromptErr)
			// Fallback: Show the tool result and the error
			finalResponse = fmt.Sprintf("Tool Result:\n```%s```\n\n(Error generating final response: %v)", finalResponse, repromptErr)
		} else {
			finalResponse = finalResStruct.Content
		}
	} else {
		// No tool was executed, add assistant response to history
		c.addToHistory(channelID, threadTS, "assistant", finalResponse)
	}

	// Send the final response back to Slack
	if finalResponse == "" {
		c.userFrontend.SendMessage(channelID, threadTS, "(LLM returned an empty response)")
	} else {
		c.userFrontend.SendMessage(channelID, threadTS, finalResponse)
	}
	
	// Remove thinking reaction after response
	c.userFrontend.RemoveReaction(channelID, messageTS, "thinking_face")
	
	// Track that we've participated in this thread
	c.trackThreadParticipation(channelID, threadTS)
}

// handleSlashCommandEvent processes Slack slash command events
func (c *Client) handleSlashCommandEvent(cmd slack.SlashCommand) {
	// Get thread key for per-conversation settings
	channelID := cmd.ChannelID
	threadKey := channelID
	
	switch cmd.Command {
	case "/think_aloud":
		c.showThoughts[threadKey] = true
		c.userFrontend.SendMessage(channelID, "", ":brain: Thinking aloud mode enabled. I'll show my reasoning process.")
		c.logger.InfoKV("Enabled thinking aloud via slash command", "channel", channelID, "user", cmd.UserID)

	case "/think_silent":
		c.showThoughts[threadKey] = false
		c.userFrontend.SendMessage(channelID, "", ":shushing_face: Silent thinking mode enabled. I'll keep my thoughts to myself.")
		c.logger.InfoKV("Enabled silent thinking via slash command", "channel", channelID, "user", cmd.UserID)

	default:
		c.userFrontend.SendMessage(channelID, "", fmt.Sprintf("Unknown command: %s", cmd.Command))
	}
}

// handleSlashCommand processes slash commands like /think_aloud and /think_silent
func (c *Client) handleSlashCommand(command, channelID, threadTS, messageTS string) {
	// Get thread key for per-conversation settings
	threadKey := channelID
	if threadTS != "" {
		threadKey = fmt.Sprintf("%s:%s", channelID, threadTS)
	}

	switch command {
	case "/think_aloud":
		c.showThoughts[threadKey] = true
		c.userFrontend.SendMessage(channelID, threadTS, ":brain: Thinking aloud mode enabled. I'll show my reasoning process.")
		c.logger.InfoKV("Enabled thinking aloud", "channel", channelID, "thread", threadTS)

	case "/think_silent", "/think_quietly":
		c.showThoughts[threadKey] = false
		c.userFrontend.SendMessage(channelID, threadTS, ":shushing_face: Silent thinking mode enabled. I'll keep my thoughts to myself.")
		c.logger.InfoKV("Enabled silent thinking", "channel", channelID, "thread", threadTS)

	default:
		c.userFrontend.SendMessage(channelID, threadTS, fmt.Sprintf("Unknown command: %s\nAvailable commands:\n• `/think_aloud` - Show my reasoning process\n• `/think_silent` - Hide my reasoning process", command))
	}
}

// setThinkingMode sets the thinking mode for a channel/thread
func (c *Client) setThinkingMode(showThoughts bool, channelID, threadTS string) {
	// Get thread key for per-conversation settings
	threadKey := channelID
	if threadTS != "" {
		threadKey = fmt.Sprintf("%s:%s", channelID, threadTS)
	}

	c.showThoughts[threadKey] = showThoughts
	
	if showThoughts {
		c.userFrontend.SendMessage(channelID, threadTS, ":brain: Thinking aloud mode enabled. I'll show my reasoning process.")
		c.logger.InfoKV("Enabled thinking aloud", "channel", channelID, "thread", threadTS)
	} else {
		c.userFrontend.SendMessage(channelID, threadTS, ":shushing_face: Silent thinking mode enabled. I'll keep my thoughts to myself.")
		c.logger.InfoKV("Enabled silent thinking", "channel", channelID, "thread", threadTS)
	}
}

// getShowThoughts returns whether to show thoughts for a given channel/thread
func (c *Client) getShowThoughts(channelID, threadTS string) bool {
	// Check thread-specific setting first
	threadKey := channelID
	if threadTS != "" {
		threadKey = fmt.Sprintf("%s:%s", channelID, threadTS)
	}

	if showThoughts, exists := c.showThoughts[threadKey]; exists {
		return showThoughts
	}

	// Fall back to config default
	if c.cfg.LLM.ShowThoughts != nil {
		return *c.cfg.LLM.ShowThoughts
	}

	// Default to true if not configured
	return true
}
