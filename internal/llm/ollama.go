// Package llm provides implementations for language model providers
package llm

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

// OllamaProvider implements the LLMProvider interface for the Ollama API
type OllamaProvider struct {
	APIBaseURL string
	Model      string
	Logger     *logging.Logger
}

// CreateMCPOllamaHandler creates a new MCP handler for Ollama
func CreateMCPOllamaHandler(logger *logging.Logger) *MCPHandler {
	provider := NewOllamaProvider(logger)

	// Set up the MCP tool for Ollama
	ollamaTool := mcp.NewTool(
		"ollama",
		mcp.WithDescription("Generate text using the Ollama local LLM provider"),
		mcp.WithString("prompt",
			mcp.Description("The prompt to send to the Ollama API"),
			mcp.Required(),
		),
		mcp.WithString("model",
			mcp.Description("The model to use (default is the OLLAMA_DEFAULT_MODEL environment variable or 'llama2')"),
		),
		mcp.WithNumber("temperature",
			mcp.Description("Sampling temperature to use (default is 0.7)"),
		),
	)

	// Create the handler with a custom implementation
	return &MCPHandler{
		Name:        "ollama",
		Description: "Generate text using Ollama's local LLM capabilities",
		Tool:        &ollamaTool,
		Logger:      logger,
		Provider:    provider,
		HandleFunc: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract parameters from the request
			prompt, ok := request.Params.Arguments["prompt"].(string)
			if !ok || prompt == "" {
				return nil, fmt.Errorf("invalid parameter 'prompt': expected non-empty string")
			}

			// Optional parameters with defaults
			modelParam, hasModel := request.Params.Arguments["model"].(string)
			model := os.Getenv("OLLAMA_DEFAULT_MODEL")
			if model == "" {
				model = "llama2" // Default model
			}
			if hasModel && modelParam != "" {
				model = modelParam
			}

			// Temperature parameter (optional)
			temperature := 0.7 // Default
			if tempParam, hasTemp := request.Params.Arguments["temperature"]; hasTemp {
				if tempFloat, ok := tempParam.(float64); ok {
					temperature = tempFloat
				} else if tempStr, ok := tempParam.(string); ok {
					if tempFloat, err := strconv.ParseFloat(tempStr, 64); err == nil {
						temperature = tempFloat
					}
				}
			}

			// Create provider options
			options := ProviderOptions{
				Model:       model,
				Temperature: temperature,
				MaxTokens:   2048, // Default max tokens
			}

			// Generate completion using the provider
			provider := NewOllamaProvider(logger)
			result, err := provider.GenerateCompletion(ctx, prompt, options)
			if err != nil {
				return nil, fmt.Errorf("failed to generate completion: %w", err)
			}

			// Return the result as an MCP response
			return CreateMCPResult(result), nil
		},
	}
}

// NewOllamaProvider creates a new Ollama provider instance
func NewOllamaProvider(logger *logging.Logger) *OllamaProvider {
	// Get the base URL from environment variable, default to localhost:11434
	baseURL := os.Getenv("OLLAMA_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	return &OllamaProvider{
		APIBaseURL: baseURL,
		Model:      os.Getenv("OLLAMA_DEFAULT_MODEL"),
		Logger:     logger,
	}
}

// GenerateCompletion implements the LLMProvider interface for Ollama
func (p *OllamaProvider) GenerateCompletion(ctx context.Context, prompt string, options ProviderOptions) (string, error) {
	// For now, just pass through to chat completion
	messages := []RequestMessage{
		{
			Role:    "user",
			Content: prompt,
		},
	}
	return p.GenerateChatCompletion(ctx, messages, options)
}

// GenerateChatCompletion implements the LLMProvider interface for Ollama
func (p *OllamaProvider) GenerateChatCompletion(ctx context.Context, messages []RequestMessage, options ProviderOptions) (string, error) {
	// In a real implementation, this would make HTTP requests to the Ollama API
	// For now, we'll just return a placeholder message
	p.Logger.Info("Ollama provider is not yet fully implemented - would call Ollama API")

	// Build a message about what would happen if implemented
	model := options.Model
	if model == "" && p.Model != "" {
		model = p.Model
	}
	if model == "" {
		model = "llama2" // Default fallback
	}

	// Return an informative message
	return fmt.Sprintf("[Ollama provider would call %s model at %s with %d messages]",
		model, p.APIBaseURL, len(messages)), nil
}

// GetInfo implements the LLMProvider interface
func (p *OllamaProvider) GetInfo() ProviderInfo {
	return ProviderInfo{
		Name:            "ollama",
		DefaultModel:    "llama2",
		SupportedModels: []string{"llama2", "mistral", "codellama"},
		Endpoint:        p.APIBaseURL,
	}
}

// IsAvailable checks if the Ollama provider is properly configured
func (p *OllamaProvider) IsAvailable() bool {
	// In a real implementation, this would check if the Ollama server is running
	// For now, we'll just check if the base URL is set
	return p.APIBaseURL != ""
}
