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
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
	"github.com/tuannvm/slack-mcp-client/internal/types"
)

// Client represents the Slack client application.
type Client struct {
	log             *log.Logger
	api             *slack.Client
	Socket          *socketmode.Client
	botUserID       string
	botMentionRgx   *regexp.Regexp
	mcpClients      map[string]*mcp.Client
	llmMCPBridge    *bridge.LLMMCPBridge
	cfg             *config.Config // Holds the application configuration
	httpClient      *http.Client // HTTP client for LLM communication
	// Message history for context (limited per channel)
	messageHistory  map[string][]Message
	historyLimit    int
	// Use types.ToolInfo
	discoveredTools map[string]types.ToolInfo
}

// Message represents a message in the conversation history
type Message struct {
	Role      string    // "user" or "assistant"
	Content   string    // The message content
	Timestamp time.Time // When the message was sent/received
}

// NewClient creates a new Slack client instance.
func NewClient(botToken, appToken string, logger *log.Logger, mcpClients map[string]*mcp.Client, 
               discoveredTools map[string]types.ToolInfo, cfg *config.Config) (*Client, error) {
	if botToken == "" {
		return nil, fmt.Errorf("SLACK_BOT_TOKEN must be set")
	}
	if appToken == "" {
		return nil, fmt.Errorf("SLACK_APP_TOKEN must be set")
	}
	if !strings.HasPrefix(appToken, "xapp-") {
		return nil, fmt.Errorf("SLACK_APP_TOKEN must have the prefix \"xapp-\"")
	}
	if mcpClients == nil || len(mcpClients) == 0 {
		return nil, fmt.Errorf("mcpClients map cannot be nil or empty")
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
			} else if !isDirectMessage {
				// Log other messages if needed, but don't process
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
func (c *Client) getContextFromHistory(channelID string) string {
	history, exists := c.messageHistory[channelID]
	if !exists || len(history) == 0 {
		return ""
	}
	
	var contextBuilder strings.Builder
	contextBuilder.WriteString("Previous conversation context:\n---\n") // Clearer start marker
	
	for _, msg := range history {
		prefix := "User"
		if msg.Role == "assistant" {
			prefix = "Assistant"
		} else if msg.Role == "tool" { // Add handling for tool results in history
			prefix = "Tool Result"
		}
		// Sanitize content slightly for logging/prompting (remove potential newlines)
		sanitizedContent := strings.ReplaceAll(msg.Content, "\n", " \\n ") 
		contextBuilder.WriteString(fmt.Sprintf("%s: %s\n", prefix, sanitizedContent))
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
	promptBuilder.WriteString("You have access to the following tools. Analyze the user's request to determine if a tool is needed.\n")
	promptBuilder.WriteString("**If** a tool is appropriate and you can extract **all** required arguments from the user's request based on the tool's Input Schema, respond ONLY with a single JSON object matching the specified format. Do not include any other text, explanation, or conversational filler before or after the JSON object.\n")
	promptBuilder.WriteString("**If** a tool seems appropriate but the user's request is missing necessary arguments, **do not** generate the JSON. Instead, ask the user clarifying questions to obtain the missing information.\n")
	promptBuilder.WriteString("**If** no tool is needed, respond naturally to the user.\n\n")
	
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

	promptBuilder.WriteString("\nJSON Format for Tool Call (use ONLY if tool is needed AND all arguments are available):\n")
	promptBuilder.WriteString("{\n")
	promptBuilder.WriteString("  \"tool\": \"<tool_name>\",\n")
	promptBuilder.WriteString("  \"args\": { <arguments matching the tool's input schema> }\n")
	promptBuilder.WriteString("}\n")

	return promptBuilder.String()
}

// OpenAI API request and response structures
type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	Temperature float64         `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
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

// handleOpenAIPrompt sends the user's text to the OpenAI API and posts the response.
func (c *Client) handleOpenAIPrompt(userPrompt, channelID, threadTS string) {
	c.log.Printf("Sending prompt to OpenAI (Model: %s): %s", c.cfg.OpenAIModelName, userPrompt)
	
	// Add "typing" indicator
	c.api.PostMessage(channelID, slack.MsgOptionText("...", false), slack.MsgOptionTS(threadTS))

	// Prepare messages for OpenAI
	messages := []openaiMessage{}

	// --- Add System Prompt with Tool Info --- 
	systemPrompt := c.generateToolPrompt()
	if systemPrompt != "" {
		c.log.Printf("Adding system prompt with tool instructions")
		messages = append(messages, openaiMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	} else {
		c.log.Printf("No tools available, not adding tool system prompt.")
	}

	// Add conversation history
	history, exists := c.messageHistory[channelID]
	if exists && len(history) > 0 {
		c.log.Printf("Adding %d messages from conversation history", len(history))
		for _, msg := range history {
			role := "user"
			if msg.Role == "assistant" {
				role = "assistant"
			}
			messages = append(messages, openaiMessage{
				Role:    role,
				Content: msg.Content,
			})
		}
	}
	
	// Add the current user prompt
	messages = append(messages, openaiMessage{
		Role:    "user",
		Content: userPrompt,
	})

	// Prepare the request payload
	reqPayload := openaiRequest{
		Model:       c.cfg.OpenAIModelName,
		Messages:    messages,
		Temperature: 0.7,  
		MaxTokens:   2048, 
	}
	
	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		c.log.Printf("Error marshalling OpenAI request payload: %v", err)
		c.postMessage(channelID, threadTS, "Sorry, there was an internal error preparing your request.")
		return
	}

	// Create the HTTP request
	ctx, cancel := context.WithTimeout(context.Background(), c.httpClient.Timeout)
	defer cancel()
	
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, 
		"https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqBody))
	if err != nil {
		c.log.Printf("Error creating OpenAI HTTP request: %v", err)
		c.postMessage(channelID, threadTS, "Sorry, there was an internal error preparing your request.")
		return
	}
	
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+os.Getenv("OPENAI_API_KEY"))

	// Send the request to the OpenAI API
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.log.Printf("Error sending request to OpenAI API: %v", err)
		c.postMessage(channelID, threadTS, fmt.Sprintf("Sorry, I couldn't reach the OpenAI API. Error: %v", err))
		return
	}
	defer httpResp.Body.Close()

	// Read the response body
	respBodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		c.log.Printf("Error reading OpenAI response body: %v", err)
		c.postMessage(channelID, threadTS, "Sorry, there was an error reading the response from the OpenAI API.")
		return
	}

	// Check response status code
	if httpResp.StatusCode != http.StatusOK {
		c.log.Printf("OpenAI API returned non-OK status: %s. Body: %s", httpResp.Status, string(respBodyBytes))
		c.postMessage(channelID, threadTS, fmt.Sprintf("Sorry, the OpenAI API returned an error (Status: %s). Response: ```%s```", httpResp.Status, string(respBodyBytes)))
		return
	}

	// Parse the JSON response from OpenAI
	var openaiResp openaiResponse
	if err := json.Unmarshal(respBodyBytes, &openaiResp); err != nil {
		c.log.Printf("Error parsing OpenAI JSON response: %v. Body: %s", err, string(respBodyBytes))
		c.postMessage(channelID, threadTS, "Sorry, I received an unexpected response format from the OpenAI API.")
		return
	}

	// Check if we have choices in the response
	if len(openaiResp.Choices) == 0 {
		c.log.Printf("OpenAI API returned no choices in response")
		c.postMessage(channelID, threadTS, "Sorry, OpenAI provided no response.")
		return
	}

	// Extract the text from the first choice
	openaiText := strings.TrimSpace(openaiResp.Choices[0].Message.Content)
	c.log.Printf("Received response from OpenAI. Length: %d", len(openaiText))

	// Process the LLM response through the MCP pipeline
	c.processLLMResponseAndReply(openaiText, userPrompt, channelID, threadTS)
}

// processLLMResponseAndReply processes the LLM response, handles tool results with re-prompting, and sends the final reply.
func (c *Client) processLLMResponseAndReply(llmResponse, userPrompt, channelID, threadTS string) {
	// Process the LLM response through the LLM-MCP bridge to detect and execute tool calls
	var finalResponse string
	var isToolResult bool = false

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

	// --- Re-prompting Logic --- 
	if isToolResult {
		c.log.Printf("DEBUG: Tool executed. Re-prompting LLM with tool result.")
		// Construct a new prompt incorporating the original prompt and the tool result
		rePrompt := fmt.Sprintf("The user asked: '%s'\n\nI used a tool and received the following result:\n```\n%s\n```\nPlease formulate a concise and helpful natural language response to the user based *only* on the user's original question and the tool result provided.", userPrompt, finalResponse)

		// Add history
		c.addToHistory(channelID, "assistant", llmResponse) 
		c.addToHistory(channelID, "tool", finalResponse) 

		c.log.Printf("DEBUG: Re-prompting LLM with: %s", rePrompt)
		
		// Always assume OpenAI path for re-prompting (or handle generic re-prompt)
		c.log.Printf("TODO: Re-implement OpenAI re-prompt call") 
		finalResponse = fmt.Sprintf("Tool Result:\n```%s```", finalResponse) // Temporary: show raw result
		
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
				c.api.DeleteMessage(channelID, msg.Timestamp)
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
