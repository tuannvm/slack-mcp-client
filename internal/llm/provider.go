// Package llm provides implementations and interfaces for language model providers
package llm

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp" // Use the correct MCP library types
)

// Constants for provider types and names
const (
	ProviderTypeOpenAI        = "openai"
	ProviderTypeOllama        = "ollama"
	ProviderNameLangChain     = "langchain"
	DefaultLLMGatewayProvider = ProviderNameLangChain
)

// ProviderType defines the type of LLM provider (e.g., OpenAI, Ollama)
type ProviderType string

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
	// GenerateCompletion generates a text completion (less common now, prefer chat)
	GenerateCompletion(ctx context.Context, prompt string, options ProviderOptions) (string, error)

	// GenerateChatCompletion generates a chat completion using a message history
	GenerateChatCompletion(ctx context.Context, messages []RequestMessage, options ProviderOptions) (string, error)

	// GetInfo returns information about the provider
	GetInfo() ProviderInfo

	// IsAvailable returns true if the provider is properly configured and ready
	IsAvailable() bool
}

// MCPHandler represents an MCP handler that wraps an LLM provider or similar functionality
// It adapts the LLMProvider concept to the MCP tool interface.
type MCPHandler struct {
	Name        string
	Description string
	Tool        *mcp.Tool                                                                           // Use mcp.Tool
	Logger      interface{}                                                                         // Using interface{} to support different logger types (e.g., *log.Logger, *logging.Logger)
	HandleFunc  func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) // Use mcp types
	Provider    LLMProvider                                                                         // Optional: Link back to the underlying provider if applicable
}

// GetName returns the name of the tool
func (h *MCPHandler) GetName() string {
	return h.Name
}

// GetDescription returns a human-readable description of the tool
func (h *MCPHandler) GetDescription() string {
	return h.Description
}

// GetToolDefinition returns the MCP Tool definition
func (h *MCPHandler) GetToolDefinition() mcp.Tool {
	if h.Tool == nil {
		// Return an empty tool or handle error appropriately
		return mcp.Tool{}
	}
	return *h.Tool
}

// Handle processes an MCP tool request and returns a result or an error
func (h *MCPHandler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Delegate to the specific handler function defined for this tool
	if h.HandleFunc == nil {
		// Return a standard Go error. The MCP server framework should handle converting this
		// into the appropriate MCP error response.
		return nil, fmt.Errorf("internal error: handler function not implemented for tool %s", h.Name)
	}
	return h.HandleFunc(ctx, request)
}

// CreateMCPResult creates a basic MCP tool result with a string value
func CreateMCPResult(content string) *mcp.CallToolResult {
	// Use the mcp package's way to create a text result
	return mcp.NewToolResultText(content)
}
