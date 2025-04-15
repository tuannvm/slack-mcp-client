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
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
	"github.com/tuannvm/slack-mcp-client/internal/types"
)

// Ollama API Request Structure
type ollamaRequest struct {
	Model      string                 `json:"model"`
	Prompt     string                 `json:"prompt"`
	System     string                 `json:"system,omitempty"`
	Stream     bool                   `json:"stream"`
	Options    map[string]interface{} `json:"options,omitempty"`
}

// Ollama API Response Structure (Simplified for non-streaming)
type ollamaResponse struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Response  string    `json:"response"`
	Done      bool      `json:"done"`
	// Context   []int     `json:"context,omitempty"` // Add if needed for context handling
	// Other fields like total_duration, load_duration etc. are ignored for now
}

// Client represents the Slack client application.
type Client struct {
	log             *log.Logger
	api             *slack.Client
	Socket          *socketmode.Client
	botUserID       string
	botMentionRgx   *regexp.Regexp
	mcpClients      map[string]*mcp.Client
	llmMCPBridge    *bridge.LLMMCPBridge
	ollamaEndpoint  string // Ollama API endpoint (e.g., http://host:port/api/generate)
	ollamaModelName string // Name of the Ollama model to use
	openaiModelName string // Name of the OpenAI model to use
	llmProvider     string // Which LLM provider to use ("ollama" or "openai")
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

// NewClient creates a new Slack client instance configured to talk to an Ollama or OpenAI server.
func NewClient(botToken, appToken string, logger *log.Logger, mcpClients map[string]*mcp.Client, 
               discoveredTools map[string]types.ToolInfo, ollamaAPIEndpoint, ollamaModelName string,
               llmProvider, openaiModelName string) (*Client, error) {
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
	
	// Validate LLM provider settings
	if llmProvider != "ollama" && llmProvider != "openai" {
		return nil, fmt.Errorf("llmProvider must be either 'ollama' or 'openai'")
	}
	
	if llmProvider == "ollama" {
		if ollamaAPIEndpoint == "" {
			return nil, fmt.Errorf("Ollama API Endpoint cannot be empty when provider is 'ollama'")
		}
		if ollamaModelName == "" {
			return nil, fmt.Errorf("Ollama Model Name cannot be empty when provider is 'ollama'")
		}
	} else { // openai
		if openaiModelName == "" {
			return nil, fmt.Errorf("OpenAI Model Name cannot be empty when provider is 'openai'")
		}
		// Check if OPENAI_API_KEY is set
		if os.Getenv("OPENAI_API_KEY") == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY environment variable must be set when provider is 'openai'")
		}
	}

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

	// List available MCP servers
	logger.Printf("Available MCP servers (%d):", len(mcpClients))
	for name := range mcpClients {
		logger.Printf("- %s", name)
	}
	
	// Report discovered tools
	logger.Printf("Available tools (%d):", len(discoveredTools))
	for toolName, toolInfo := range discoveredTools {
		logger.Printf("- %s (Desc: %s, Schema: %v, Server: %s)", 
			toolName, toolInfo.Description, toolInfo.InputSchema, toolInfo.ServerName)
	}
	
	// Initialize the LLM-MCP bridge with all available MCP clients and discovered tools
	llmMCPBridge := bridge.NewLLMMCPBridge(mcpClients, logger, discoveredTools)
	logger.Printf("LLM-MCP bridge initialized with %d MCP clients and %d tools", len(mcpClients), len(discoveredTools))

	// Log which LLM provider will be used
	logger.Printf("Using LLM provider: %s", llmProvider)
	if llmProvider == "ollama" {
		logger.Printf("Ollama model: %s at %s", ollamaModelName, ollamaAPIEndpoint)
	} else {
		logger.Printf("OpenAI model: %s", openaiModelName)
	}

	return &Client{
		log:             logger,
		api:             api,
		Socket:          socketClient,
		botUserID:       botUserID,
		botMentionRgx:   mentionRegex,
		mcpClients:      mcpClients,
		llmMCPBridge:    llmMCPBridge,
		ollamaEndpoint:  ollamaAPIEndpoint,
		ollamaModelName: ollamaModelName,
		openaiModelName: openaiModelName,
		llmProvider:     llmProvider,
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
	
	var context strings.Builder
	context.WriteString("Previous conversation:\n")
	
	for _, msg := range history {
		prefix := "User"
		if msg.Role == "assistant" {
			prefix = "Assistant"
		}
		context.WriteString(fmt.Sprintf("%s: %s\n", prefix, msg.Content))
	}
	
	return context.String()
}

// handleUserPrompt sends the user's text to the selected LLM provider and posts the response.
func (c *Client) handleUserPrompt(userPrompt, channelID, threadTS string) {
	if c.llmProvider == "ollama" {
		c.handleOllamaPrompt(userPrompt, channelID, threadTS)
	} else {
		c.handleOpenAIPrompt(userPrompt, channelID, threadTS)
	}
}

// Function to generate the system prompt string
func (c *Client) generateToolPrompt() string {
	if len(c.discoveredTools) == 0 {
		return "" // No tools available
	}

	var promptBuilder strings.Builder
	promptBuilder.WriteString("You have access to the following tools. If you need to use one to fulfill the user's request, respond ONLY with a single JSON object matching the specified format. Do not include any other text, explanation, or conversational filler before or after the JSON object.\n\nAvailable Tools:\n")

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

	promptBuilder.WriteString("\nJSON Format for Tool Call:\n")
	promptBuilder.WriteString("{\n")
	promptBuilder.WriteString("  \"tool\": \"<tool_name>\",\n")
	promptBuilder.WriteString("  \"args\": { <arguments matching the tool's input schema> }\n")
	promptBuilder.WriteString("}\n")

	return promptBuilder.String()
}

// handleOllamaPrompt sends the user's text to the Ollama API and posts the response.
func (c *Client) handleOllamaPrompt(userPrompt, channelID, threadTS string) {
	c.log.Printf("Sending prompt to Ollama (Model: %s): %s", c.ollamaModelName, userPrompt)
	
	// Add conversation context
	conversationContext := c.getContextFromHistory(channelID)
	fullPrompt := userPrompt
	if conversationContext != "" {
		fullPrompt = fmt.Sprintf("%s\n\nCurrent question: %s", conversationContext, userPrompt)
	}

	// Generate the system prompt with tool info
	systemPrompt := c.generateToolPrompt()
	if systemPrompt != "" {
		c.log.Printf("Adding system prompt with tool instructions")
	} else {
		c.log.Printf("No tools available, not adding tool system prompt.")
	}

	// Add "typing" indicator
	c.api.PostMessage(channelID, slack.MsgOptionText("...", false), slack.MsgOptionTS(threadTS))

	// Prepare the request payload
	reqPayload := ollamaRequest{
		Model:   c.ollamaModelName,
		Prompt:  fullPrompt,
		System:  systemPrompt, // Include the system prompt
		Stream:  false,
		Options: map[string]interface{}{"temperature": 0.7, "num_predict": 1024}, // Using Options map
	}
	
	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		c.log.Printf("Error marshalling Ollama request payload: %v", err)
		c.postMessage(channelID, threadTS, "Sorry, there was an internal error preparing your request.")
		return
	}

	// Create the HTTP request
	ctx, cancel := context.WithTimeout(context.Background(), c.httpClient.Timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ollamaEndpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		c.log.Printf("Error creating Ollama HTTP request: %v", err)
		c.postMessage(channelID, threadTS, "Sorry, there was an internal error preparing your request.")
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json") // Ollama responds with JSON

	// Send the request to the Ollama server
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.log.Printf("Error sending request to Ollama server at %s: %v", c.ollamaEndpoint, err)
		c.postMessage(channelID, threadTS, fmt.Sprintf("Sorry, I couldn't reach the Ollama server. Please ensure it's running and accessible. Error: %v", err))
		return
	}
	defer httpResp.Body.Close()

	// Read the response body
	respBodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		c.log.Printf("Error reading Ollama response body: %v", err)
		c.postMessage(channelID, threadTS, "Sorry, there was an error reading the response from the Ollama server.")
		return
	}

	// Check response status code
	if httpResp.StatusCode != http.StatusOK {
		c.log.Printf("Ollama server returned non-OK status: %s. Body: %s", httpResp.Status, string(respBodyBytes))
		c.postMessage(channelID, threadTS, fmt.Sprintf("Sorry, the Ollama server returned an error (Status: %s). Response: ```%s```", httpResp.Status, string(respBodyBytes)))
		return
	}

	// Parse the JSON response from Ollama
	var ollamaResp ollamaResponse
	if err := json.Unmarshal(respBodyBytes, &ollamaResp); err != nil {
		c.log.Printf("Error parsing Ollama JSON response: %v. Body: %s", err, string(respBodyBytes))
		c.postMessage(channelID, threadTS, "Sorry, I received an unexpected response format from the Ollama server.")
		return
	}

	if !ollamaResp.Done {
		c.log.Printf("Warning: Ollama response indicated 'done: false', but we expected a complete response (stream: false). Response: %s", ollamaResp.Response)
		// Proceed anyway, but this might indicate an issue or incomplete response
	}

	ollamaGeneratedText := strings.TrimSpace(ollamaResp.Response)
	c.log.Printf("Received response from Ollama. Length: %d", len(ollamaGeneratedText))

	// Process the LLM response through the MCP pipeline
	c.processLLMResponseAndReply(ollamaGeneratedText, userPrompt, channelID, threadTS)
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
	c.log.Printf("Sending prompt to OpenAI (Model: %s): %s", c.openaiModelName, userPrompt)
	
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
		Model:       c.openaiModelName,
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

		// Call the appropriate LLM handler AGAIN with the re-prompt
		// NOTE: This is a simplified re-prompt. It doesn't include previous history or the system prompt again.
		// A more robust implementation might need a dedicated re-prompt function.
		
		// Add assistant's previous turn (the tool call JSON) and the tool result to history 
		// before calling LLM again
		c.addToHistory(channelID, "assistant", llmResponse) // Add the JSON tool call itself
		c.addToHistory(channelID, "tool", finalResponse) // Add the tool result (might need a 'tool' role)

		c.log.Printf("DEBUG: Re-prompting LLM with: %s", rePrompt)
		
		if c.llmProvider == "ollama" {
			// Call Ollama again (need to adapt handleOllamaPrompt or create a helper)
			// For simplicity here, let's just log and maybe return the raw result for now
			c.log.Printf("TODO: Re-implement Ollama re-prompt call") 
			finalResponse = fmt.Sprintf("Tool Result:\n```%s```", finalResponse) // Temporary: show raw result
			// Ideally: call handleOllamaPrompt(rePrompt, channelID, threadTS) - but this would cause a loop
			// Need to refactor LLM call logic to handle re-prompting without infinite loops.
		} else { // openai
			// Call OpenAI again (need to adapt handleOpenAIPrompt or create a helper)
			c.log.Printf("TODO: Re-implement OpenAI re-prompt call") 
			finalResponse = fmt.Sprintf("Tool Result:\n```%s```", finalResponse) // Temporary: show raw result
		}
		// After the LLM call completes, 'finalResponse' would be updated with the LLM's summary.
		// For now, we are just showing the raw tool result as a placeholder.
		
	} else {
		// No tool was executed, or bridge failed. Add the assistant's response to history.
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
