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
)

// Ollama API Request Structure
type ollamaRequest struct {
	Model      string  `json:"model"`
	Prompt     string  `json:"prompt"`
	Stream     bool    `json:"stream"` // Ensure we set this to false for a single response
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	// Add other parameters like system prompt, options if needed
	// Options map[string]interface{} `json:"options,omitempty"`
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
	httpClient      *http.Client // HTTP client for Ollama communication
	// Message history for context (limited per channel)
	messageHistory  map[string][]Message
	historyLimit    int
	// Map of available tools to their server names
	discoveredTools map[string]string
}

// Message represents a message in the conversation history
type Message struct {
	Role      string    // "user" or "assistant"
	Content   string    // The message content
	Timestamp time.Time // When the message was sent/received
}

// NewClient creates a new Slack client instance configured to talk to an Ollama server.
func NewClient(botToken, appToken string, logger *log.Logger, mcpClients map[string]*mcp.Client, discoveredTools map[string]string, ollamaAPIEndpoint, ollamaModelName string) (*Client, error) {
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
	if ollamaAPIEndpoint == "" {
		return nil, fmt.Errorf("Ollama API Endpoint cannot be empty")
	}
	if ollamaModelName == "" {
		return nil, fmt.Errorf("Ollama Model Name cannot be empty")
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
	for toolName, serverName := range discoveredTools {
		logger.Printf("- %s (from server: %s)", toolName, serverName)
	}
	
	// Initialize the LLM-MCP bridge with all available MCP clients and discovered tools
	llmMCPBridge := bridge.NewLLMMCPBridge(mcpClients, logger, discoveredTools)
	logger.Printf("LLM-MCP bridge initialized with %d MCP clients and %d tools", len(mcpClients), len(discoveredTools))

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

// handleUserPrompt sends the user's text to the Ollama API and posts the response.
func (c *Client) handleUserPrompt(userPrompt, channelID, threadTS string) {
	c.log.Printf("Sending prompt to Ollama (Model: %s): %s", c.ollamaModelName, userPrompt)
	
	// Add conversation context if available
	conversationContext := c.getContextFromHistory(channelID)
	fullPrompt := userPrompt
	if conversationContext != "" {
		fullPrompt = fmt.Sprintf("%s\n\nCurrent question: %s", conversationContext, userPrompt)
		c.log.Printf("Added conversation context (%d previous messages)", 
			len(c.messageHistory[channelID])-1) // -1 because the current message is already in history
	}

	// Add a "typing" indicator while processing
	c.api.PostMessage(
		channelID,
		slack.MsgOptionText("...", false),
		slack.MsgOptionTS(threadTS),
	)

	// Prepare the request payload for Ollama /api/generate
	reqPayload := ollamaRequest{
		Model:       c.ollamaModelName,
		Prompt:      fullPrompt, // Use the prompt with context
		Stream:      false,      // Request a single complete response
		Temperature: 0.7,        // Set moderate temperature for some creativity
		MaxTokens:   1024,       // Limit max output tokens to 1024
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

	// Process the LLM response through the LLM-MCP bridge to detect and execute tool calls
	var finalResponse string
	if c.llmMCPBridge != nil {
		c.log.Printf("Processing LLM response through LLM-MCP bridge")
		ctx := context.Background()
		processedResponse, err := c.llmMCPBridge.ProcessLLMResponse(ctx, ollamaGeneratedText, userPrompt)
		if err != nil {
			c.log.Printf("Error processing LLM response through bridge: %v", err)
			// Fall back to original response if bridge processing fails
			finalResponse = ollamaGeneratedText
		} else {
			finalResponse = processedResponse
		}
	} else {
		// No bridge available, use original response
		finalResponse = ollamaGeneratedText
	}

	// Add assistant response to history
	c.addToHistory(channelID, "assistant", finalResponse)

	// Send the final response back to Slack
	if finalResponse == "" {
		c.postMessage(channelID, threadTS, "(Ollama returned an empty response)")
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
