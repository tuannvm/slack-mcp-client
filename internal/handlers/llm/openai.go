package llm

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	customErrors "github.com/tuannvm/slack-mcp-client/internal/common/errors"
	httpClient "github.com/tuannvm/slack-mcp-client/internal/common/http"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/handlers"
)

// OpenAIMessage represents a message in the OpenAI API request
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIChatRequest represents a request to the OpenAI chat completions API
type OpenAIChatRequest struct {
	Model               string          `json:"model"`
	Messages            []OpenAIMessage `json:"messages"`
	Temperature         float64         `json:"temperature,omitempty"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
}

// OpenAIChatResponseChoice represents a choice in the OpenAI API response
type OpenAIChatResponseChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// OpenAIChatResponseUsage represents usage statistics in the OpenAI API response
type OpenAIChatResponseUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAIChatResponse represents a response from the OpenAI chat completions API
type OpenAIChatResponse struct {
	ID      string                     `json:"id"`
	Object  string                     `json:"object"`
	Created int64                      `json:"created"`
	Model   string                     `json:"model"`
	Choices []OpenAIChatResponseChoice `json:"choices"`
	Usage   OpenAIChatResponseUsage    `json:"usage"`
}

// OpenAIHandler implements the OpenAI tool
type OpenAIHandler struct {
	handlers.BaseHandler
	apiKey      string
	apiEndpoint string
	httpClient  *httpClient.Client
}

// NewOpenAIHandler creates a new OpenAIHandler
func NewOpenAIHandler(logger *logging.Logger) *OpenAIHandler {
	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	apiEndpoint := os.Getenv("OPENAI_API_ENDPOINT")

	if apiEndpoint == "" {
		apiEndpoint = "https://api.openai.com/v1/chat/completions"
	}

	// Set up HTTP client with logging
	options := httpClient.DefaultOptions()
	options.Timeout = 60 * 1000000000 // 60 seconds
	options.RequestLogger = func(_, url string, _ []byte) {
		logger.Debug("OpenAI API Request: %s", url)
	}
	options.ResponseLogger = func(_ int, _ []byte, err error) {
		if err != nil {
			logger.Error("OpenAI API Response error: %v", err)
			return
		}
		logger.Debug("OpenAI API Response received successfully")
	}

	// Create tool definition
	tool := mcp.NewTool(
		"openai",
		mcp.WithDescription("Process text using OpenAI models"),
		mcp.WithString("model",
			mcp.Description("The OpenAI model to use"),
			mcp.Required(),
		),
		mcp.WithString("prompt",
			mcp.Description("The prompt to send to OpenAI (alternative to messages)"),
		),
		mcp.WithArray("messages",
			mcp.Description("Array of messages to send to OpenAI (alternative to prompt)"),
		),
		mcp.WithNumber("temperature",
			mcp.Description("Temperature for response generation"),
		),
		mcp.WithNumber("max_tokens",
			mcp.Description("Maximum number of tokens to generate"),
		),
	)

	return &OpenAIHandler{
		BaseHandler: handlers.BaseHandler{
			Name:        "openai",
			Description: "Process text using OpenAI models",
			Tool:        tool,
			Logger:      logger.WithName("openai-tool"),
		},
		apiKey:      apiKey,
		apiEndpoint: apiEndpoint,
		httpClient:  httpClient.NewClient(options),
	}
}

// Handle processes an OpenAI tool request
func (h *OpenAIHandler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.Logger.Debug("Received OpenAI tool request")

	// Check if API key is configured
	if h.apiKey == "" {
		return nil, customErrors.NewOpenAIError("API key not configured", "missing_key", nil)
	}

	// Extract parameters from request
	args := request.Params.Arguments
	h.Logger.Debug("Received arguments: %+v", args)

	// Get required model parameter
	model, ok := args["model"].(string)
	if !ok {
		return nil, customErrors.ErrBadRequest
	}

	// Process messages and other parameters
	chatReq, err := h.buildChatRequest(model, args)
	if err != nil {
		return nil, err
	}

	// Make the API request
	chatResp, err := h.makeAPIRequest(ctx, chatReq)
	if err != nil {
		return nil, err
	}

	// Create and return MCP tool response
	return h.createToolResponse(chatResp)
}

// buildChatRequest constructs the OpenAI API request
func (h *OpenAIHandler) buildChatRequest(model string, args map[string]interface{}) (OpenAIChatRequest, error) {
	// Build the chat request
	var messages []OpenAIMessage

	// First, check if there's a simple "prompt" parameter as a shortcut
	if prompt, ok := args["prompt"].(string); ok {
		messages = []OpenAIMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		}
	} else if rawMessages, ok := args["messages"].([]interface{}); ok {
		// Process array of message objects
		messages = h.processMessageArray(rawMessages)
		if len(messages) == 0 {
			return OpenAIChatRequest{}, customErrors.ErrBadRequest
		}
	} else {
		return OpenAIChatRequest{}, customErrors.ErrBadRequest
	}

	// Build the request
	chatReq := OpenAIChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}

	// Apply optional parameters
	h.applyOptionalParameters(&chatReq, args, model)

	return chatReq, nil
}

// processMessageArray converts a raw message array to OpenAIMessages
func (h *OpenAIHandler) processMessageArray(rawMessages []interface{}) []OpenAIMessage {
	var messages []OpenAIMessage

	for _, rawMsg := range rawMessages {
		if msgMap, ok := rawMsg.(map[string]interface{}); ok {
			role, _ := msgMap["role"].(string)
			content, _ := msgMap["content"].(string)

			if role == "" || content == "" {
				return nil // Invalid message format
			}

			messages = append(messages, OpenAIMessage{
				Role:    role,
				Content: content,
			})
		} else {
			return nil // Invalid message format
		}
	}

	return messages
}

// applyOptionalParameters adds optional parameters to the request
func (h *OpenAIHandler) applyOptionalParameters(chatReq *OpenAIChatRequest, args map[string]interface{}, model string) {
	// Handle temperature
	if temp, ok := args["temperature"].(float64); ok {
		chatReq.Temperature = temp
	}

	// Handle token limits based on model type
	if maxTokens, ok := args["max_tokens"].(float64); ok {
		// Use appropriate token parameter based on model type
		if strings.Contains(model, "o3-") {
			h.Logger.Debug("Using max_completion_tokens for o3 model: %s", model)
			chatReq.MaxCompletionTokens = int(maxTokens)
		} else {
			chatReq.MaxTokens = int(maxTokens)
		}
	}
}

// makeAPIRequest sends the request to the OpenAI API
func (h *OpenAIHandler) makeAPIRequest(ctx context.Context, chatReq OpenAIChatRequest) (OpenAIChatResponse, error) {
	// Prepare headers
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + h.apiKey,
	}

	// Make the request
	var chatResp OpenAIChatResponse
	statusCode, err := h.httpClient.DoJSONRequest(ctx, "POST", h.apiEndpoint, chatReq, &chatResp, headers)

	if err != nil {
		h.Logger.Error("OpenAI API request failed: %v", err)
		if customErrors.Is(err, customErrors.ErrTooManyRequests) {
			return OpenAIChatResponse{}, customErrors.NewLLMError("rate_limit_exceeded", "Rate limit exceeded for OpenAI API").WithData("provider", "openai")
		}
		return OpenAIChatResponse{}, customErrors.WrapLLMError(err, "request_failed", "OpenAI API request failed")
	}

	if statusCode != 200 {
		return OpenAIChatResponse{}, customErrors.WrapLLMError(
			customErrors.StatusCodeToError(statusCode),
			"api_error",
			fmt.Sprintf("OpenAI API returned error status: %d", statusCode),
		).WithData("status_code", statusCode)
	}

	return chatResp, nil
}

// createToolResponse creates an MCP tool response from the API response
func (h *OpenAIHandler) createToolResponse(chatResp OpenAIChatResponse) (*mcp.CallToolResult, error) {
	// Check if we have choices in the response
	if len(chatResp.Choices) == 0 {
		return nil, customErrors.NewLLMError("no_choices", "OpenAI API returned no choices in response").WithData("model", chatResp.Model)
	}

	// Extract the message content
	aiResponse := chatResp.Choices[0].Message.Content
	h.Logger.Debug("Received response of length %d", len(aiResponse))

	// Create MCP tool response
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: aiResponse,
			},
		},
	}, nil
}

// IsConfigured returns true if the handler is properly configured
func (h *OpenAIHandler) IsConfigured() bool {
	return h.apiKey != ""
}
