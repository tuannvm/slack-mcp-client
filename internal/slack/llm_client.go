// Package slackbot implements the Slack integration for the MCP client
package slackbot

import (
	"context"
	"fmt"
	"log"

	"github.com/tuannvm/slack-mcp-client/internal/common"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/handlers"
	"github.com/tuannvm/slack-mcp-client/internal/llm"
)

// LLMClient provides a lightweight wrapper around LLM operations
type LLMClient struct {
	logger          *log.Logger
	discoveredTools map[string]common.ToolInfo
	config          *config.Config
	llmRegistry     *llm.ProviderRegistry
}

// NewLLMClient creates a new LLM client that uses the consolidated handler implementation
func NewLLMClient(logger *log.Logger, discoveredTools map[string]common.ToolInfo, cfg *config.Config) *LLMClient {
	// Create a wrapped logger for the LLM registry
	wrappedLogger := logging.New("llm-client", logging.LevelDebug)

	// Create the LLM provider registry
	registry := llm.NewProviderRegistry(wrappedLogger)

	return &LLMClient{
		logger:          logger,
		discoveredTools: discoveredTools,
		config:          cfg,
		llmRegistry:     registry,
	}
}

// GenerateCompletion generates a text completion using the configured LLM provider
func (c *LLMClient) GenerateCompletion(ctx context.Context, prompt string, systemPrompt string, contextHistory string) (string, error) {
	// Prepare messages with system prompt and context history
	messages := []llm.RequestMessage{}

	// Add system prompt with tool info if available
	if systemPrompt != "" {
		messages = append(messages, llm.RequestMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	// Add conversation context if provided
	if contextHistory != "" {
		messages = append(messages, llm.RequestMessage{
			Role:    "system",
			Content: "Previous conversation: " + contextHistory,
		})
	}

	// Add the user's prompt
	messages = append(messages, llm.RequestMessage{
		Role:    "user",
		Content: prompt,
	})

	// Build options based on the config
	options := llm.ProviderOptions{
		Model:       c.config.OpenAIModelName,
		Temperature: 0.7,
		MaxTokens:   2048,
	}

	// Get the primary provider name from the registry
	providerName := c.llmRegistry.GetPrimaryProvider()
	if providerName == "" {
		return "", fmt.Errorf("no primary LLM provider configured")
	}

	// Get the provider implementation from the registry
	provider, err := c.llmRegistry.GetProvider(providerName)
	if err != nil {
		return "", fmt.Errorf("failed to get primary LLM provider '%s': %w", providerName, err)
	}
	if provider == nil {
		// This case should ideally not happen if GetProvider returns no error, but check defensively.
		return "", fmt.Errorf("primary LLM provider '%s' not found or not initialized", providerName)
	}

	// Call the provider to generate the completion
	return provider.GenerateChatCompletion(ctx, messages, options)
}

// ProcessToolResponse takes the LLM response and processes it through the MCP bridge
// Returns the processed response and a boolean indicating if a tool was executed
func (c *LLMClient) ProcessToolResponse(ctx context.Context, llmResponse, userPrompt string, bridge *handlers.LLMMCPBridge) (string, bool, error) {
	if bridge == nil {
		return llmResponse, false, nil
	}

	// Process the response through the bridge
	processedResponse, err := bridge.ProcessLLMResponse(ctx, llmResponse, userPrompt)
	if err != nil {
		return fmt.Sprintf("Sorry, I encountered an error while trying to use a tool: %v", err), false, err
	}

	// If the processed response is different from the original, a tool was executed
	if processedResponse != llmResponse {
		return processedResponse, true, nil
	}

	// No tool was executed
	return llmResponse, false, nil
}
