// Package handlers provides implementation for MCP tool handlers.
package handlers

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	
	"github.com/tuannvm/slack-mcp-client/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/llm"
)

// LLMGatewayHandler implements a centralized gateway for LLM providers
type LLMGatewayHandler struct {
	BaseHandler
	registry      *llm.ProviderRegistry
	defaultModel  string
}

// NewLLMGatewayHandler creates a new LLM gateway handler
func NewLLMGatewayHandler(logger *logging.Logger) *LLMGatewayHandler {
	// Create provider registry
	registry := llm.NewProviderRegistry(logger)
	
	// Register providers (OpenAI direct and LangChain)
	openaiProvider := llm.NewOpenAIProvider(logger)
	langchainProvider := llm.NewLangChainProvider(logger)
	ollamaProvider := llm.NewOllamaProvider(logger)
	
	// Only register providers that are available
	if openaiProvider.IsAvailable() {
		registry.RegisterProvider(openaiProvider)
	}
	
	if langchainProvider.IsAvailable() {
		registry.RegisterProvider(langchainProvider)
		// Set LangChain as primary if available
		registry.SetPrimaryProvider(langchainProvider.GetInfo().Name)
	}
	
	if ollamaProvider.IsAvailable() {
		registry.RegisterProvider(ollamaProvider)
	}
	
	// If OpenAI is available, set it as fallback for LangChain
	if openaiProvider.IsAvailable() && langchainProvider.IsAvailable() {
		registry.SetFallback(langchainProvider.GetInfo().Name, openaiProvider.GetInfo().Name)
	}
	
	// Create tool definition
	tool := mcp.NewTool(
		"llm",
		mcp.WithDescription("Generate text using various language model providers"),
		mcp.WithString("provider",
			mcp.Description("The LLM provider to use (openai, langchain, ollama)"),
		),
		mcp.WithString("model",
			mcp.Description("The model to use"),
		),
		mcp.WithString("prompt",
			mcp.Description("The prompt to send to the LLM (alternative to messages)"),
		),
		mcp.WithArray("messages",
			mcp.Description("Array of messages to send to the LLM (alternative to prompt)"),
		),
		mcp.WithNumber("temperature",
			mcp.Description("Temperature for response generation"),
		),
		mcp.WithNumber("max_tokens",
			mcp.Description("Maximum number of tokens to generate"),
		),
	)

	return &LLMGatewayHandler{
		BaseHandler: BaseHandler{
			Name:        "llm",
			Description: "Generate text using various language model providers",
			Tool:        tool,
			Logger:      logger.WithName("llm-gateway"),
		},
		registry:      registry,
		defaultModel:  "gpt-4o", // Default to GPT-4o
	}
}

// Handle processes a request to the LLM gateway
func (h *LLMGatewayHandler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.Logger.Debug("Received LLM gateway request")

	// Extract parameters from request
	args := request.Params.Arguments
	h.Logger.Debug("Received arguments: %+v", args)

	// Get provider parameter with fallback to primary provider
	providerName, _ := args["provider"].(string)
	if providerName == "" {
		providerName = h.registry.GetPrimaryProvider()
		h.Logger.Debug("Using primary provider: %s", providerName)
	}

	// Get model parameter with fallback to default
	model, _ := args["model"].(string)
	if model == "" {
		model = h.defaultModel
		h.Logger.Debug("Using default model: %s", model)
	}

	// Build options
	options := llm.ProviderOptions{
		Model: model,
	}

	// Apply optional parameters
	if temp, ok := args["temperature"].(float64); ok {
		options.Temperature = temp
	}

	if maxTokens, ok := args["max_tokens"].(float64); ok {
		options.MaxTokens = int(maxTokens)
	}

	// Process either prompt or messages
	var result string
	var err error

	if prompt, ok := args["prompt"].(string); ok && prompt != "" {
		// Use single prompt for the completion
		result, err = h.registry.GenerateCompletion(ctx, providerName, prompt, options)
	} else if rawMessages, ok := args["messages"].([]interface{}); ok && len(rawMessages) > 0 {
		// Convert messages to standard format
		messages := make([]llm.RequestMessage, 0, len(rawMessages))
		for _, rawMsg := range rawMessages {
			if msgMap, ok := rawMsg.(map[string]interface{}); ok {
				role, _ := msgMap["role"].(string)
				content, _ := msgMap["content"].(string)

				if role != "" && content != "" {
					messages = append(messages, llm.RequestMessage{
						Role:    role,
						Content: content,
					})
				}
			}
		}

		// Use messages for chat completion
		result, err = h.registry.GenerateChatCompletion(ctx, providerName, messages, options)
	} else {
		return nil, errors.NewOpenAIError(
			"No prompt or messages provided in request",
			"missing_input",
			nil,
		)
	}

	if err != nil {
		return nil, err
	}

	// Create and return MCP tool response
	return llm.CreateMCPResult(result), nil
}
