// Package llm provides implementations for language model providers
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

const (
	openaiProviderName = "openai"
)

// OpenAIProvider implements the LLMProvider interface for OpenAI
type OpenAIProvider struct {
	apiKey       string
	apiEndpoint  string
	httpClient   *httpClient.Client
	defaultModel string
	logger       *logging.Logger
}

// init registers the OpenAI provider factory.
func init() {
	err := RegisterProviderFactory(openaiProviderName, NewOpenAIProviderFactory)
	if err != nil {
		panic(fmt.Sprintf("Failed to register openai provider factory: %v", err))
	}
}

// NewOpenAIProviderFactory creates an OpenAI provider instance from configuration.
func NewOpenAIProviderFactory(config map[string]interface{}, logger logging.Logger) (LLMProvider, error) {
	apiKey, ok := config["api_key"].(string)
	if !ok || apiKey == "" {
		baseURL, _ := config["base_url"].(string)
		if baseURL == "" {
			return nil, fmt.Errorf("openai config requires 'api_key' (string) or 'base_url' (string)")
		}
		logger.Warn("OpenAI API key not found, relying on base_url")
	}

	baseURL, _ := config["base_url"].(string)
	apiEndpoint := baseURL // Use base_url if provided
	if apiEndpoint == "" {
		apiEndpoint = "https://api.openai.com/v1" // Default OpenAI endpoint
	}
	chatCompletionEndpoint := strings.TrimSuffix(apiEndpoint, "/") + "/chat/completions"

	modelName, ok := config["model"].(string)
	if !ok || modelName == "" {
		modelName = "gpt-4o" // Default model
		logger.Info("OpenAI model not specified in config, defaulting to", "model", modelName)
	}

	providerLogger := logger.WithName("openai-provider")
	providerLogger.Info("Creating OpenAI provider", "model", modelName, "endpoint", chatCompletionEndpoint)

	// Set up HTTP client with logging
	options := httpClient.DefaultOptions()
	options.Timeout = 60 * 1000000000 // 60 seconds
	options.RequestLogger = func(_, url string, _ []byte) {
		providerLogger.Debug("OpenAI API Request", "url", url)
	}
	options.ResponseLogger = func(status int, _ []byte, err error) {
		if err != nil {
			providerLogger.Error("OpenAI API Response error", "error", err)
			return
		}
		providerLogger.Debug("OpenAI API Response received", "status", status)
	}

	return &OpenAIProvider{
		apiKey:       apiKey,
		apiEndpoint:  chatCompletionEndpoint,
		httpClient:   httpClient.NewClient(options),
		defaultModel: modelName,
		logger:       providerLogger,
	}, nil
}

// GetInfo returns information about the provider.
func (p *OpenAIProvider) GetInfo() ProviderInfo {
	description := fmt.Sprintf("Connects to OpenAI API endpoint at %s", p.apiEndpoint)
	if !strings.Contains(p.apiEndpoint, "api.openai.com") {
		description = fmt.Sprintf("Connects to OpenAI-compatible API endpoint at %s", p.apiEndpoint)
	}
	return ProviderInfo{
		Name:          openaiProviderName,
		DisplayName:   fmt.Sprintf("OpenAI (%s)", p.defaultModel),
		Description:   description,
		Configured:    true, // Assumes if created via factory, it's configured
		Available:     p.IsAvailable(), // Check availability dynamically
		Configuration: map[string]string{
			"Endpoint": p.apiEndpoint,
			"Model":    p.defaultModel,
		},
	}
}

// IsAvailable checks if the provider is likely available (has API key or custom endpoint).
func (p *OpenAIProvider) IsAvailable() bool {
	// Available if API key is present OR if a non-default endpoint is configured
	return p.apiKey != "" || !strings.Contains(p.apiEndpoint, "api.openai.com")
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
		// Convert the RequestMessage to OpenAIMessage directly
		openaiMessages = append(openaiMessages, OpenAIMessage(msg))
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
