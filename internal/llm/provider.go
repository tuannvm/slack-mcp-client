// Package llm provides implementations for language model providers
package llm

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// ProviderType represents the type of LLM provider
type ProviderType string

const (
	// ProviderTypeOpenAI represents the OpenAI provider
	ProviderTypeOpenAI ProviderType = "openai"

	// ProviderTypeLangChain represents the LangChain provider
	ProviderTypeLangChain ProviderType = "langchain"

	// ProviderTypeOllama represents the Ollama provider
	ProviderTypeOllama ProviderType = "ollama"
)

// ProviderInfo contains information about an LLM provider
type ProviderInfo struct {
	Name            string   // Provider name
	DefaultModel    string   // Default model to use
	SupportedModels []string // List of supported models
	Endpoint        string   // API endpoint
}

// RequestMessage represents a single message in a chat request
type RequestMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ProviderOptions contains options for LLM requests
type ProviderOptions struct {
	Model          string  // Model to use (specific model name, e.g., gpt-4o)
	Temperature    float64 // Temperature for response generation (0-1)
	MaxTokens      int     // Maximum number of tokens to generate
	TargetProvider string  // For gateway providers: specifies the underlying provider (e.g., "openai", "ollama")
}

// LLMProvider defines the interface for language model providers
type LLMProvider interface {
	// GenerateCompletion generates a text completion
	GenerateCompletion(ctx context.Context, prompt string, options ProviderOptions) (string, error)

	// GenerateChatCompletion generates a chat completion
	GenerateChatCompletion(ctx context.Context, messages []RequestMessage, options ProviderOptions) (string, error)

	// GetInfo returns information about the provider
	GetInfo() ProviderInfo

	// IsAvailable returns true if the provider is properly configured
	IsAvailable() bool
}

// MCPHandler represents an MCP handler that wraps an LLM provider
type MCPHandler struct {
	Name        string
	Description string
	Tool        *mcp.Tool
	Logger      interface{} // Using interface{} to support different logger types
	HandleFunc  func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	Provider    LLMProvider
}

// GetName returns the name of the tool
func (h *MCPHandler) GetName() string {
	return h.Name
}

// GetDescription returns a human-readable description of the tool
func (h *MCPHandler) GetDescription() string {
	return h.Description
}

// GetToolDefinition returns the Tool definition
func (h *MCPHandler) GetToolDefinition() mcp.Tool {
	return *h.Tool
}

// Handle processes an MCP tool request and returns a result or an error
func (h *MCPHandler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return h.HandleFunc(ctx, request)
}

// CreateMCPResult creates a basic MCP tool result with a string value
func CreateMCPResult(content string) *mcp.CallToolResult {
	return mcp.NewToolResultText(content)
}
