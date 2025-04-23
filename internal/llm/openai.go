// Package llm provides implementations for language model providers
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tuannvm/slack-mcp-client/internal/common/errors"
	httpClient "github.com/tuannvm/slack-mcp-client/internal/common/http"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
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

// OpenAIProvider implements the LLMProvider interface for OpenAI
type OpenAIProvider struct {
	apiKey          string
	apiEndpoint     string
	httpClient      *httpClient.Client
	defaultModel    string
	logger          *logging.Logger
	supportedModels []string
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(logger *logging.Logger) *OpenAIProvider {
	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	apiEndpoint := os.Getenv("OPENAI_API_ENDPOINT")
	defaultModel := os.Getenv("OPENAI_MODEL")

	if apiEndpoint == "" {
		apiEndpoint = "https://api.openai.com/v1/chat/completions"
	}

	if defaultModel == "" {
		defaultModel = "gpt-4o"
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

	return &OpenAIProvider{
		apiKey:       apiKey,
		apiEndpoint:  apiEndpoint,
		httpClient:   httpClient.NewClient(options),
		defaultModel: defaultModel,
		logger:       logger.WithName("openai-provider"),
		supportedModels: []string{
			"gpt-4o",
			"gpt-4-turbo",
			"gpt-4",
			"gpt-3.5-turbo",
		},
	}
}

// GetInfo returns information about the provider
func (p *OpenAIProvider) GetInfo() ProviderInfo {
	return ProviderInfo{
		Name:            "openai",
		DefaultModel:    p.defaultModel,
		SupportedModels: p.supportedModels,
		Endpoint:        p.apiEndpoint,
	}
}

// IsAvailable returns true if the provider is properly configured
func (p *OpenAIProvider) IsAvailable() bool {
	return p.apiKey != ""
}

// GenerateCompletion generates a text completion using OpenAI
func (p *OpenAIProvider) GenerateCompletion(ctx context.Context, prompt string, options ProviderOptions) (string, error) {
	// Create a single message with the prompt
	messages := []OpenAIMessage{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	return p.generateWithMessages(ctx, messages, options)
}

// GenerateChatCompletion generates a chat completion using OpenAI
func (p *OpenAIProvider) GenerateChatCompletion(ctx context.Context, messages []RequestMessage, options ProviderOptions) (string, error) {
	// Convert our message format to OpenAI's format
	openaiMessages := make([]OpenAIMessage, 0, len(messages))
	for _, msg := range messages {
		openaiMessages = append(openaiMessages, OpenAIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return p.generateWithMessages(ctx, openaiMessages, options)
}

// generateWithMessages is a helper function that generates text with the given messages
func (p *OpenAIProvider) generateWithMessages(ctx context.Context, messages []OpenAIMessage, options ProviderOptions) (string, error) {
	// Check if API key is configured
	if p.apiKey == "" {
		return "", errors.NewOpenAIError("API key not configured", "missing_key", nil)
	}

	// Determine which model to use
	model := options.Model
	if model == "" {
		model = p.defaultModel
	}

	// Create request
	chatReq := OpenAIChatRequest{
		Model:    model,
		Messages: messages,
	}

	// Apply options
	if options.Temperature > 0 {
		chatReq.Temperature = options.Temperature
	}

	if options.MaxTokens > 0 {
		// Use appropriate token parameter based on model type
		if strings.Contains(model, "o3-") {
			p.logger.Debug("Using max_completion_tokens for o3 model: %s", model)
			chatReq.MaxCompletionTokens = options.MaxTokens
		} else {
			chatReq.MaxTokens = options.MaxTokens
		}
	}

	// Prepare headers
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + p.apiKey,
	}

	// Make the request using DoRequest instead of creating an http.Request
	respBody, statusCode, err := p.httpClient.DoRequest(ctx, "POST", p.apiEndpoint, chatReq, headers)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}

	// Check status code
	if statusCode != 200 {
		return "", fmt.Errorf("API returned error status: %d", statusCode)
	}

	// Parse response
	var chatResp OpenAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Check if we have choices in the response
	if len(chatResp.Choices) == 0 {
		return "", errors.NewOpenAIError("No choices in response", "no_choices", nil)
	}

	// Extract the message content
	return chatResp.Choices[0].Message.Content, nil
}

// CreateMCPOpenAIHandler creates an MCP handler for OpenAI
func CreateMCPOpenAIHandler(logger *logging.Logger) *MCPHandler {
	// Use the provider implementation
	provider := NewOpenAIProvider(logger)

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

	handleFunc := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logger.Debug("Received OpenAI tool request")

		// Check if API key is configured
		if !provider.IsAvailable() {
			return nil, errors.NewOpenAIError("API key not configured", "missing_key", nil)
		}

		// Extract parameters from request
		args := request.Params.Arguments
		logger.Debug("Received arguments: %+v", args)

		// Get model parameter
		model, ok := args["model"].(string)
		if !ok {
			return nil, errors.ErrBadRequest
		}

		// Build options
		options := ProviderOptions{
			Model: model,
		}

		// Extract temperature if provided
		if temp, ok := args["temperature"].(float64); ok {
			options.Temperature = temp
		}

		// Extract max tokens if provided
		if maxTokens, ok := args["max_tokens"].(float64); ok {
			options.MaxTokens = int(maxTokens)
		}

		var result string
		var err error

		// Process input (either prompt or messages)
		if prompt, ok := args["prompt"].(string); ok && prompt != "" {
			// Use the provider to generate response with prompt
			result, err = provider.GenerateCompletion(ctx, prompt, options)
		} else if rawMessages, ok := args["messages"].([]interface{}); ok && len(rawMessages) > 0 {
			// Convert messages to standard format
			messages := make([]RequestMessage, 0, len(rawMessages))
			for _, rawMsg := range rawMessages {
				if msgMap, ok := rawMsg.(map[string]interface{}); ok {
					role, _ := msgMap["role"].(string)
					content, _ := msgMap["content"].(string)

					if role != "" && content != "" {
						messages = append(messages, RequestMessage{
							Role:    role,
							Content: content,
						})
					}
				}
			}

			// Use the provider to generate response with messages
			result, err = provider.GenerateChatCompletion(ctx, messages, options)
		} else {
			return nil, errors.NewLLMError("missing_input", "No prompt or messages provided")
		}

		if err != nil {
			return nil, err
		}

		// Create and return MCP tool response
		return CreateMCPResult(result), nil
	}

	return &MCPHandler{
		Name:        "openai",
		Description: "Process text using OpenAI models",
		Tool:        &tool,
		Logger:      logger.WithName("openai-tool"),
		HandleFunc:  handleFunc,
		Provider:    provider,
	}
}
