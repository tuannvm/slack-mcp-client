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
	options.RequestLogger = func(method, url string, body []byte) {
		logger.Debug("OpenAI API Request: %s %s", method, url)
	}
	options.ResponseLogger = func(statusCode int, body []byte, err error) {
		if err != nil {
			logger.Error("OpenAI API Response error: %v", err)
			return
		}
		logger.Debug("OpenAI API Response: status=%d, body_length=%d", statusCode, len(body))
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

	// Get required parameters
	model, ok := args["model"].(string)
	if !ok {
		return nil, customErrors.ErrBadRequest
	}

	// Handle messages parameter
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
		for _, rawMsg := range rawMessages {
			if msgMap, ok := rawMsg.(map[string]interface{}); ok {
				role, _ := msgMap["role"].(string)
				content, _ := msgMap["content"].(string)

				if role == "" || content == "" {
					return nil, customErrors.ErrBadRequest
				}

				messages = append(messages, OpenAIMessage{
					Role:    role,
					Content: content,
				})
			} else {
				return nil, customErrors.ErrBadRequest
			}
		}
	} else {
		return nil, customErrors.ErrBadRequest
	}

	// Build the request
	chatReq := OpenAIChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}

	// Optional parameters
	if temp, ok := args["temperature"].(float64); ok {
		chatReq.Temperature = temp
	}

	if maxTokens, ok := args["max_tokens"].(float64); ok {
		// Use appropriate token parameter based on model type
		if strings.Contains(model, "o3-") {
			h.Logger.Debug("Using max_completion_tokens for o3 model: %s", model)
			chatReq.MaxCompletionTokens = int(maxTokens)
		} else {
			chatReq.MaxTokens = int(maxTokens)
		}
	}

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
			return nil, customErrors.NewOpenAIError("Rate limit exceeded", "rate_limit_exceeded", err)
		}
		return nil, customErrors.NewOpenAIError("API request failed", "request_failed", err)
	}

	if statusCode != 200 {
		return nil, customErrors.NewOpenAIError(
			fmt.Sprintf("API returned error status: %d", statusCode),
			"api_error",
			customErrors.StatusCodeToError(statusCode),
		)
	}

	// Check if we have choices in the response
	if len(chatResp.Choices) == 0 {
		return nil, customErrors.NewOpenAIError("API returned no choices", "no_choices", nil)
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
