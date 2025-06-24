// Package llm provides implementations and interfaces for language model providers
package llm

import (
	"context"
	"fmt"
	"sync"

	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/tools"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

// Constants for provider types and names
const (
	ProviderTypeOpenAI        = "openai"
	ProviderTypeOllama        = "ollama"
	ProviderTypeAnthropic     = "anthropic"
	ProviderNameLangChain     = "langchain"
	DefaultLLMGatewayProvider = ProviderNameLangChain
)

// ProviderFactory defines the function signature for creating an LLMProvider instance.
// It takes provider-specific configuration and a logger.
type ProviderFactory func(config map[string]interface{}, logger *logging.Logger) (LLMProvider, error)

// Global map to store provider factories
var (
	providerFactories = make(map[string]ProviderFactory)
	mu                sync.RWMutex
)

// RegisterProviderFactory registers a factory function for a given provider name.
// This should be called from the init() function of each provider implementation.
func RegisterProviderFactory(name string, factory ProviderFactory) error {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := providerFactories[name]; exists {
		return fmt.Errorf("provider factory for '%s' already registered", name)
	}
	providerFactories[name] = factory
	// Use a temporary logger or standard log during init phase if needed
	// fmt.Printf("Registered LLM provider factory: %s\n", name)
	return nil
}

// GetProviderFactory retrieves a registered provider factory.
func GetProviderFactory(name string) (ProviderFactory, bool) {
	mu.RLock()
	defer mu.RUnlock()
	factory, exists := providerFactories[name]
	return factory, exists
}

// ListProviderFactories returns the names of all registered factories.
func ListProviderFactories() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(providerFactories))
	for name := range providerFactories {
		names = append(names, name)
	}
	return names
}

// ProviderType defines the type of LLM provider (e.g., OpenAI, Ollama)
type ProviderType string

// ProviderInfo contains information about an LLM provider
type ProviderInfo struct {
	Name          string            // Unique identifier (e.g., "openai", "ollama")
	DisplayName   string            // User-friendly name (e.g., "OpenAI GPT-4", "Local Ollama")
	Description   string            // Brief description of the provider/model
	Configured    bool              // Whether the provider has been configured
	Available     bool              // Whether the provider is currently reachable/available
	Configuration map[string]string // Non-sensitive configuration details (e.g., model, base URL)
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
	Tools          []llms.Tool
}

// LLMProvider defines the interface for language model providers
type LLMProvider interface {
	// GenerateCompletion generates a text completion (less common now, prefer chat)
	// Deprecated: Prefer GenerateChatCompletion
	GenerateCompletion(ctx context.Context, prompt string, options ProviderOptions) (*llms.ContentChoice, error)

	// GenerateChatCompletion generates a chat completion using a message history
	GenerateChatCompletion(ctx context.Context, messages []RequestMessage, options ProviderOptions) (*llms.ContentChoice, error)

	// GenerateAgentCompletion generates a chat completion using a message history using a langchain agent
	GenerateAgentCompletion(ctx context.Context, userDisplayName, systemPrompt string, prompt string, messages []RequestMessage, llmTools []tools.Tool, callbackHandler callbacks.Handler) (string, error)

	// GetInfo returns information about the provider
	GetInfo() ProviderInfo

	// IsAvailable returns true if the provider is properly configured and ready
	IsAvailable() bool
}
