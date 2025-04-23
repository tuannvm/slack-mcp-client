// Package llm provides implementations for language model providers
package llm

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"

	"github.com/tuannvm/slack-mcp-client/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

// LangChainProvider implements the LLMProvider interface using LangChainGo
type LangChainProvider struct {
	llm             llms.Model
	defaultModel    string
	providerType    ProviderType
	endpoint        string
	logger          *logging.Logger
	supportedModels []string
}

// NewLangChainProvider creates a new LangChain provider
func NewLangChainProvider(logger *logging.Logger) *LangChainProvider {
	// Get environment variables
	apiKey := os.Getenv("OPENAI_API_KEY")
	apiEndpoint := os.Getenv("OPENAI_API_ENDPOINT")
	defaultModel := os.Getenv("OPENAI_MODEL")
	ollamaEndpoint := os.Getenv("OLLAMA_API_ENDPOINT")
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	providerType := os.Getenv("LLM_PROVIDER_TYPE")

	// Set defaults
	if defaultModel == "" {
		defaultModel = "gpt-4o"
		logger.Info("No OPENAI_MODEL specified, defaulting to %s", defaultModel)
	}

	if ollamaModel == "" {
		ollamaModel = "llama3"
		logger.Info("No OLLAMA_MODEL specified, defaulting to %s", ollamaModel)
	}

	if ollamaEndpoint == "" {
		ollamaEndpoint = "http://localhost:11434"
		logger.Info("No OLLAMA_API_ENDPOINT specified, defaulting to %s", ollamaEndpoint)
	}

	if providerType == "" {
		providerType = string(ProviderTypeOpenAI)
		logger.Info("No LLM_PROVIDER_TYPE specified, defaulting to %s", providerType)
	}

	// Initialize provider based on type
	var llmClient llms.Model
	var err error
	var endpoint string
	var selectedModel string
	var supportedModels []string
	providerTypeEnum := ProviderType(strings.ToLower(providerType))

	switch providerTypeEnum {
	case ProviderTypeOpenAI:
		// OpenAI models
		supportedModels = []string{
			"gpt-4o",
			"gpt-4-turbo",
			"gpt-4",
			"gpt-3.5-turbo",
		}
		selectedModel = defaultModel
		endpoint = apiEndpoint

		// Initialize OpenAI client
		opts := []openai.Option{
			openai.WithToken(apiKey),
		}

		// If custom API endpoint is set, use it
		if apiEndpoint != "" {
			opts = append(opts, openai.WithBaseURL(apiEndpoint))
		}

		llmClient, err = openai.New(opts...)
		if err != nil {
			logger.Error("Failed to initialize LangChainGo OpenAI client: %v", err)
		}

	case ProviderTypeOllama:
		// Ollama models
		supportedModels = []string{
			"llama3",
			"mistral",
			"codellama",
			"phi",
		}
		selectedModel = ollamaModel
		endpoint = ollamaEndpoint

		// Initialize Ollama client
		opts := []ollama.Option{
			ollama.WithModel(ollamaModel),
		}

		// Set the server URL for Ollama
		if ollamaEndpoint != "" {
			logger.Info("Using Ollama endpoint: %s", ollamaEndpoint)
			opts = append(opts, ollama.WithServerURL(ollamaEndpoint))
		}

		llmClient, err = ollama.New(opts...)
		if err != nil {
			logger.Error("Failed to initialize LangChainGo Ollama client: %v", err)
			// If Ollama fails, fall back to OpenAI if API key is available
			if apiKey != "" {
				logger.Warn("Falling back to OpenAI due to Ollama initialization failure")
				providerTypeEnum = ProviderTypeOpenAI
				// Update supportedModels and selectedModel for OpenAI
				supportedModels = []string{
					"gpt-4o",
					"gpt-4-turbo",
					"gpt-4",
					"gpt-3.5-turbo",
				}
				selectedModel = defaultModel
				endpoint = apiEndpoint

				llmClient, err = openai.New(openai.WithToken(apiKey))
				if err != nil {
					logger.Error("Failed to initialize fallback OpenAI client: %v", err)
				}
			}
		}

	default:
		logger.Warn("Unknown provider type '%s', falling back to OpenAI", providerType)
		providerTypeEnum = ProviderTypeOpenAI
		// OpenAI models
		supportedModels = []string{
			"gpt-4o",
			"gpt-4-turbo",
			"gpt-4",
			"gpt-3.5-turbo",
		}
		selectedModel = defaultModel
		endpoint = apiEndpoint

		// Initialize OpenAI as fallback
		llmClient, err = openai.New(openai.WithToken(apiKey))
		if err != nil {
			logger.Error("Failed to initialize fallback OpenAI client: %v", err)
		}
	}

	// Include support for tool calling in MCPServer functionality
	return &LangChainProvider{
		llm:             llmClient,
		defaultModel:    selectedModel,
		providerType:    providerTypeEnum,
		endpoint:        endpoint,
		logger:          logger.WithName("langchain-provider"),
		supportedModels: supportedModels,
	}
}

// GenerateCompletion generates a completion using LangChainGo
func (p *LangChainProvider) GenerateCompletion(ctx context.Context, prompt string, options ProviderOptions) (string, error) {
	// Check if client is initialized
	if p.llm == nil {
		return "", errors.NewLLMError("client_not_initialized", "LangChainGo client not initialized")
	}

	p.logger.Debug("Calling LangChainGo with prompt: %s", prompt)

	// Build options
	callOptions := p.buildOptions(options)

	// Call the LLM with the prompt
	completion, err := llms.GenerateFromSinglePrompt(ctx, p.llm, prompt, callOptions...)
	if err != nil {
		p.logger.Error("LangChainGo request failed: %v", err)
		return "", errors.WrapLLMError(
			err,
			"request_failed",
			"Failed to generate completion from LangChainGo",
		)
	}

	p.logger.Debug("Received response of length %d", len(completion))
	return completion, nil
}

// GenerateChatCompletion generates a chat completion using LangChainGo
func (p *LangChainProvider) GenerateChatCompletion(ctx context.Context, messages []RequestMessage, options ProviderOptions) (string, error) {
	// Check if client is initialized
	if p.llm == nil {
		return "", errors.NewLLMError("client_not_initialized", "LangChainGo client not initialized")
	}

	p.logger.Debug("Calling LangChainGo with %d messages", len(messages))

	// Convert our message format to a prompt
	var promptBuilder strings.Builder

	for _, msg := range messages {
		promptBuilder.WriteString(fmt.Sprintf("%s: %s\n", strings.ToUpper(msg.Role), msg.Content))
	}

	prompt := promptBuilder.String()

	// Add one final assistant prefix to indicate where the response should go
	prompt += "ASSISTANT: "

	// Call the LLM with the constructed prompt
	return p.GenerateCompletion(ctx, prompt, options)
}

// GetInfo returns information about the provider
func (p *LangChainProvider) GetInfo() ProviderInfo {
	return ProviderInfo{
		Name:            string(p.providerType),
		DefaultModel:    p.defaultModel,
		SupportedModels: p.supportedModels,
		Endpoint:        p.endpoint,
	}
}

// IsAvailable returns true if the provider is properly configured
func (p *LangChainProvider) IsAvailable() bool {
	return p.llm != nil
}

// buildOptions creates LangChainGo-specific options from our generic options
func (p *LangChainProvider) buildOptions(options ProviderOptions) []llms.CallOption {
	var callOptions []llms.CallOption

	// Set the model if provided
	if options.Model != "" {
		callOptions = append(callOptions, llms.WithModel(options.Model))
	} else if p.defaultModel != "" {
		callOptions = append(callOptions, llms.WithModel(p.defaultModel))
	}

	// Set temperature if provided
	if options.Temperature > 0 {
		callOptions = append(callOptions, llms.WithTemperature(options.Temperature))
	}

	// Set max tokens if provided
	if options.MaxTokens > 0 {
		callOptions = append(callOptions, llms.WithMaxTokens(options.MaxTokens))
	}

	return callOptions
}

// CreateMCPLangChainHandler creates an MCP handler for LangChain
func CreateMCPLangChainHandler(logger *logging.Logger) *MCPHandler {
	// Use the provider implementation
	provider := NewLangChainProvider(logger)

	// Create tool definition
	tool := mcp.NewTool(
		"langchain",
		mcp.WithDescription("Process text using LangChainGo with various LLM models"),
		mcp.WithString("model",
			mcp.Description("The model to use"),
			mcp.Required(),
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

	handleFunc := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logger.Debug("Received LangChain tool request")

		// Extract parameters from request
		args := request.Params.Arguments
		logger.Debug("Received arguments: %+v", args)

		// Get model parameter with fallback to default
		model, ok := args["model"].(string)
		if !ok || model == "" {
			model = provider.GetInfo().DefaultModel
			logger.Debug("Using default model: %s", model)
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
		Name:        "langchain",
		Description: "Process text using LangChainGo with various LLM models",
		Tool:        &tool,
		Logger:      logger.WithName("langchain-tool"),
		HandleFunc:  handleFunc,
		Provider:    provider,
	}
}
