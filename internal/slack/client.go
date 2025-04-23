// Package slackbot implements the Slack integration for the MCP client
// It provides event handling, message processing, and integration with LLM services
package slackbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/tuannvm/slack-mcp-client/internal/bridge"
	"github.com/tuannvm/slack-mcp-client/internal/common"
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
)

// Client represents the Slack client application.
type Client struct {
	log           *log.Logger
	api           *slack.Client
	Socket        *socketmode.Client
	botUserID     string
	botMentionRgx *regexp.Regexp
	mcpClients    map[string]*mcp.Client
	llmMCPBridge  *bridge.LLMMCPBridge
	cfg           *config.Config // Holds the application configuration
	httpClient    *http.Client   // HTTP client for LLM communication
	// Message history for context (limited per channel)
	messageHistory map[string][]Message
	historyLimit   int
	// Use common.ToolInfo
	discoveredTools map[string]common.ToolInfo
}

// Message represents a message in the conversation history
type Message struct {
	Role      string    // "user" or "assistant"
	Content   string    // The message content
	Timestamp time.Time // When the message was sent/received
}

// NewClient creates a new Slack client instance.
func NewClient(botToken, appToken string, logger *log.Logger, mcpClients map[string]*mcp.Client,
	discoveredTools map[string]common.ToolInfo, cfg *config.Config) (*Client, error) {
	if botToken == "" {
		return nil, fmt.Errorf("SLACK_BOT_TOKEN must be set")
	}
	if appToken == "" {
		return nil, fmt.Errorf("SLACK_APP_TOKEN must be set")
	}
	if !strings.HasPrefix(appToken, "xapp-") {
		return nil, fmt.Errorf("SLACK_APP_TOKEN must have the prefix \"xapp-\"")
	}
	if len(mcpClients) == 0 {
		return nil, fmt.Errorf("mcpClients map cannot be empty")
	}
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Basic validation of essential LLM config
	// We now assume OpenAI is the ONLY provider the client will directly use.
	if cfg.LLMProvider != config.ProviderOpenAI {
		// but we will force OpenAI path later anyway.
		logger.Printf("Warning: Configured LLM provider is '%s', but this client is hardcoded to use OpenAI directly.", cfg.LLMProvider)
		// We will force the provider to OpenAI for internal logic consistency,
		// assuming the intention is to *only* use OpenAI via this client.
		cfg.LLMProvider = config.ProviderOpenAI
	}
	if cfg.OpenAIModelName == "" {
		return nil, fmt.Errorf("OpenAIModelName is empty in config")
	}
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable is not set")
	}

	// --- Slack API setup ---
	api := slack.New(
		botToken,
		slack.OptionDebug(false),
		slack.OptionLog(log.New(os.Stdout, "slack-api: ", log.Lshortfile|log.LstdFlags)),
		slack.OptionAppLevelToken(appToken),
	)

	authTest, err := api.AuthTestContext(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with Slack: %w", err)
	}
	botUserID := authTest.UserID
	logger.Printf("Authenticated as Slack bot user: %s (%s)", botUserID, authTest.User)

	socketClient := socketmode.New(
		api,
		socketmode.OptionDebug(false),
		socketmode.OptionLog(log.New(os.Stdout, "slack-socket: ", log.Lshortfile|log.LstdFlags)),
	)

	mentionRegex := regexp.MustCompile(fmt.Sprintf(`^\s*<@%s>`, botUserID))

	httpClient := &http.Client{
		Timeout: 3 * time.Minute, // Increased timeout for potentially long LLM calls
	}

	// --- MCP/Bridge setup ---
	logger.Printf("Available MCP servers (%d):", len(mcpClients))
	for name := range mcpClients {
		logger.Printf("- %s", name)
	}

	logger.Printf("Available tools (%d):", len(discoveredTools))
	for toolName, toolInfo := range discoveredTools {
		logger.Printf("- %s (Desc: %s, Schema: %v, Server: %s)",
			toolName, toolInfo.Description, toolInfo.InputSchema, toolInfo.ServerName)
	}

	llmMCPBridge := bridge.NewLLMMCPBridge(mcpClients, logger, discoveredTools)
	logger.Printf("LLM-MCP bridge initialized with %d MCP clients and %d tools", len(mcpClients), len(discoveredTools))

	// --- Log final config (always OpenAI now for this client) ---
	logger.Printf("Client configured to use LLM provider: %s", cfg.LLMProvider)
	logger.Printf("OpenAI model: %s", cfg.OpenAIModelName)

	// --- Create and return Client instance ---
	return &Client{
		log:             logger,
		api:             api,
		Socket:          socketClient,
		botUserID:       botUserID,
		botMentionRgx:   mentionRegex,
		mcpClients:      mcpClients,
		llmMCPBridge:    llmMCPBridge,
		cfg:             cfg, // Store the config object
		httpClient:      httpClient,
		messageHistory:  make(map[string][]Message),
		historyLimit:    10, // Store the last 10 messages per channel
		discoveredTools: discoveredTools,
	}, nil
}

// Run starts the Socket Mode event loop and event handling.
func (c *Client) Run() error {
	go c.handleEvents()
	c.log.Println("Starting Slack Socket Mode listener...")
	return c.Socket.Run()
}

// handleEvents listens for incoming events and dispatches them.
func (c *Client) handleEvents() {
	for evt := range c.Socket.Events {
		switch evt.Type {
		case socketmode.EventTypeConnecting:
			c.log.Println("Connecting to Slack...")
		case socketmode.EventTypeConnectionError:
			c.log.Println("Connection failed. Retrying...")
		case socketmode.EventTypeConnected:
			c.log.Println("Connected to Slack!")
		case socketmode.EventTypeEventsAPI:
			eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				c.log.Printf("Ignored unexpected EventsAPI event type: %T", evt.Data)
				continue
			}
			c.Socket.Ack(*evt.Request)
			c.log.Printf("Received EventsAPI event: Type=%s", eventsAPIEvent.Type)
			c.handleEventMessage(eventsAPIEvent)
		default:
			c.log.Printf("Ignored event type: %s", evt.Type)
		}
	}
	c.log.Println("Slack event channel closed.")
}

// handleEventMessage processes specific EventsAPI messages.
func (c *Client) handleEventMessage(event slackevents.EventsAPIEvent) {
	switch event.Type {
	case slackevents.CallbackEvent:
		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			c.log.Printf("Received app mention in channel %s from user %s: %s", ev.Channel, ev.User, ev.Text)
			messageText := c.botMentionRgx.ReplaceAllString(ev.Text, "")
			// Add to message history
			c.addToHistory(ev.Channel, "user", messageText)
			// Use handleUserPrompt for app mentions too, for consistency
			go c.handleUserPrompt(strings.TrimSpace(messageText), ev.Channel, ev.TimeStamp)

		case *slackevents.MessageEvent:
			isDirectMessage := strings.HasPrefix(ev.Channel, "D")
			isValidUser := ev.User != "" && ev.User != c.botUserID
			isNotEdited := ev.SubType != "message_changed"
			isBot := ev.BotID != "" || ev.SubType == "bot_message"

			if isDirectMessage && isValidUser && isNotEdited && !isBot {
				c.log.Printf("Received direct message in channel %s from user %s: %s", ev.Channel, ev.User, ev.Text)
				// Add to message history
				c.addToHistory(ev.Channel, "user", ev.Text)
				go c.handleUserPrompt(ev.Text, ev.Channel, ev.ThreadTimeStamp) // Use goroutine to avoid blocking event loop
			}

		default:
			c.log.Printf("Unsupported inner event type: %T", innerEvent.Data)
		}
	default:
		c.log.Printf("Unsupported outer event type: %s", event.Type)
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
	c.log.Printf("DEBUG: Built conversation context for channel %s:\n%s", channelID, contextString) // Log the built context
	return contextString
}

// handleUserPrompt ALWAYS sends the user's text to the OpenAI provider.
func (c *Client) handleUserPrompt(userPrompt, channelID, threadTS string) {
	// Log the provider value (will likely be OpenAI now)
	c.log.Printf("DEBUG: handleUserPrompt - Configured LLM provider: '%s'", c.cfg.LLMProvider)
	c.log.Printf("DEBUG: User prompt: '%s'", userPrompt)

	c.addToHistory(channelID, "user", userPrompt) // Add user message to history

	// Route based on the configured LLM provider - ALWAYS GO TO OPENAI NOW
	c.log.Printf("DEBUG: handleUserPrompt - Forcing branch to OpenAI")
	c.handleOpenAIPrompt(userPrompt, channelID, threadTS)
}

// Function to generate the system prompt string
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
			c.log.Printf("Error marshaling schema for tool %s: %v", name, err)
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

// OpenAI API request and response structures
type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiRequest struct {
	Model               string          `json:"model"`
	Messages            []openaiMessage `json:"messages"`
	Temperature         float64         `json:"temperature,omitempty"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
}

type openaiChoice struct {
	Index        int           `json:"index"`
	Message      openaiMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openaiResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []openaiChoice `json:"choices"`
}

// callOpenAI sends a prompt to the OpenAI API and returns the response
func (c *Client) callOpenAI(prompt, contextHistory string) (string, error) {
	// Log a warning when falling back to direct OpenAI calls instead of using LangChain as gateway
	c.log.Printf("WARNING: Using direct OpenAI API call instead of LangChain gateway. This is a fallback mechanism and not the preferred approach.")

	// Prepare messages for OpenAI
	messages := []openaiMessage{}

	// Add system prompt with tool info if available
	systemPrompt := c.generateToolPrompt()
	if systemPrompt != "" {
		messages = append(messages, openaiMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	// Add conversation context if provided
	if contextHistory != "" {
		messages = append(messages, openaiMessage{
			Role:    "system",
			Content: "Previous conversation: " + contextHistory,
		})
	}

	// Add the current prompt
	messages = append(messages, openaiMessage{
		Role:    "user",
		Content: prompt,
	})

	// Prepare the basic request payload
	reqPayload := openaiRequest{
		Model:    c.cfg.OpenAIModelName,
		Messages: messages,
	}

	// Use model-specific parameters
	if strings.Contains(c.cfg.OpenAIModelName, "o3-") {
		reqPayload.MaxCompletionTokens = 2048
	} else if strings.Contains(c.cfg.OpenAIModelName, "gpt-4o") {
		reqPayload.MaxCompletionTokens = 2048
		reqPayload.Temperature = 0.7
	} else {
		reqPayload.MaxTokens = 2048
		reqPayload.Temperature = 0.7
	}

	// Marshal the request body
	jsonBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("error marshaling OpenAI request: %w", err)
	}

	// Create the HTTP request
	ctx, cancel := context.WithTimeout(context.Background(), c.httpClient.Timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("error creating OpenAI HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+os.Getenv("OPENAI_API_KEY"))

	// Send the request to the OpenAI API
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("error sending request to OpenAI API: %w", err)
	}

	// Set up a defer to close the response body
	defer func() {
		if httpResp != nil && httpResp.Body != nil {
			_ = httpResp.Body.Close()
		}
	}()

	// Read the response body
	respBodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading OpenAI response: %w", err)
	}

	// Check response status code
	if httpResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API returned error (Status: %s): %s", httpResp.Status, string(respBodyBytes))
	}

	// Parse the JSON response from OpenAI
	var openaiResp openaiResponse
	if err := json.Unmarshal(respBodyBytes, &openaiResp); err != nil {
		return "", fmt.Errorf("error parsing OpenAI response: %w", err)
	}

	// Check if we have choices in the response
	if len(openaiResp.Choices) == 0 {
		return "", fmt.Errorf("OpenAI API returned no choices")
	}

	// Extract the text from the first choice
	return strings.TrimSpace(openaiResp.Choices[0].Message.Content), nil
}

// handleOpenAIPrompt sends the user's text to the OpenAI API and posts the response.
func (c *Client) handleOpenAIPrompt(userPrompt, channelID, threadTS string) {
	c.log.Printf("Sending prompt to OpenAI (Model: %s): %s", c.cfg.OpenAIModelName, userPrompt)

	// Show a temporary "typing" indicator
	if _, _, err := c.api.PostMessage(channelID, slack.MsgOptionText("...", false), slack.MsgOptionTS(threadTS)); err != nil {
		c.log.Printf("Error posting typing indicator: %v", err)
	}

	// Get context from history
	contextHistory := c.getContextFromHistory(channelID)

	// Call LLM based on configured provider (OpenAI direct or LangChain)
	llmResponse, err := c.callLLM(userPrompt, contextHistory)
	if err != nil {
		c.log.Printf("Error from LLM provider (%s): %v", c.cfg.LLMProvider, err)
		c.postMessage(channelID, threadTS, fmt.Sprintf("Sorry, I encountered an error: %v", err))
		return
	}

	c.log.Printf("Received response from LLM (%s). Length: %d", c.cfg.LLMProvider, len(llmResponse))

	// Process the LLM response through the MCP pipeline
	c.processLLMResponseAndReply(llmResponse, userPrompt, channelID, threadTS)
}

// processLLMResponseAndReply processes the LLM response, handles tool results with re-prompting, and sends the final reply.
func (c *Client) processLLMResponseAndReply(llmResponse, userPrompt, channelID, threadTS string) {
	// Log the raw LLM response for debugging
	c.log.Printf("DEBUG: Raw LLM response (first 500 chars): %s", truncateForLog(llmResponse, 500))

	// Process the LLM response to see if it contains a tool call
	var isToolResult = false
	var finalResponse string

	if c.llmMCPBridge != nil {
		c.log.Printf("DEBUG: Processing LLM response through bridge...")
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute) // Timeout for tool execution
		processedResponse, bridgeErr := c.llmMCPBridge.ProcessLLMResponse(ctx, llmResponse, userPrompt)
		cancel()

		if bridgeErr != nil {
			// Bridge returned an error (e.g., tool execution failed)
			c.log.Printf("ERROR: LLM-MCP Bridge returned error: %v", bridgeErr)
			// Inform the user about the tool failure
			finalResponse = fmt.Sprintf("Sorry, I encountered an error while trying to use a tool: %v", bridgeErr)
			// Don't mark as tool result, just post the error message
		} else if processedResponse != llmResponse {
			// Bridge returned a different response, indicating a successful tool call
			c.log.Printf("DEBUG: Bridge returned tool result. Length: %d", len(processedResponse))
			c.log.Printf("DEBUG: Tool result (first 500 chars): %s", truncateForLog(processedResponse, 500))
			finalResponse = processedResponse // This is the raw tool result
			isToolResult = true
		} else {
			// Bridge returned the original response, meaning no tool was called
			c.log.Printf("DEBUG: Bridge did not execute a tool.")
			finalResponse = llmResponse
		}
	} else {
		// No bridge available, use original response
		c.log.Printf("DEBUG: LLM-MCP bridge not available.")
		finalResponse = llmResponse
	}

	// Set finalResponse to llmResponse if it's still empty
	if finalResponse == "" {
		finalResponse = llmResponse
	}

	// --- Re-prompting Logic ---
	if isToolResult {
		c.log.Printf("DEBUG: Tool executed. Re-prompting LLM with tool result.")
		// Construct a new prompt incorporating the original prompt and the tool result
		rePrompt := fmt.Sprintf("The user asked: '%s'\n\nI used a tool and received the following result:\n```\n%s\n```\nPlease formulate a concise and helpful natural language response to the user based *only* on the user's original question and the tool result provided.", userPrompt, finalResponse)

		// Add history
		c.addToHistory(channelID, "assistant", llmResponse)
		c.addToHistory(channelID, "tool", finalResponse)

		c.log.Printf("DEBUG: Re-prompting LLM with: %s", rePrompt)

		// Re-prompt using the configured LLM provider with the tool result
		var repromptErr error
		finalResponse, repromptErr = c.callLLM(rePrompt, c.getContextFromHistory(channelID))
		if repromptErr != nil {
			c.log.Printf("Error during LLM re-prompt: %v", repromptErr)
			finalResponse = fmt.Sprintf("Tool Result:\n```%s```\n\n(Error re-prompting LLM: %v)", finalResponse, repromptErr)
		}

	} else {
		// No tool was executed, add assistant response to history
		c.addToHistory(channelID, "assistant", finalResponse)
	}

	// Send the final response back to Slack
	if finalResponse == "" {
		c.postMessage(channelID, threadTS, "(LLM returned an empty response)")
	} else {
		c.postMessage(channelID, threadTS, finalResponse)
	}
}

// truncateForLog truncates a string for log output
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// postMessage sends a message back to Slack, replying in a thread if threadTS is provided.
func (c *Client) postMessage(channelID, threadTS, text string) {
	if text == "" {
		c.log.Println("Attempted to send empty message, skipping.")
		return
	}

	// Delete "typing" indicator messages if any
	// This is a simplistic approach - more sophisticated approaches might track message IDs
	history, err := c.api.GetConversationHistory(&slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Limit:     10,
	})
	if err == nil && history != nil {
		for _, msg := range history.Messages {
			if msg.User == c.botUserID && msg.Text == "..." {
				_, _, err := c.api.DeleteMessage(channelID, msg.Timestamp)
				if err != nil {
					c.log.Printf("Error deleting typing indicator message: %v", err)
				}
				break // Just delete the most recent one
			}
		}
	}

	_, _, err = c.api.PostMessage(
		channelID,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTS),
	)
	if err != nil {
		c.log.Printf("Error posting message to channel %s: %v", channelID, err)
	}
}
