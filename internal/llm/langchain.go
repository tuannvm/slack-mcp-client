// Package llm provides implementations for language model providers
package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/llms"

	"github.com/tuannvm/slack-mcp-client/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

const (
	langchainProviderName = "langchain"
)

// LangChainProvider implements the LLMProvider interface using LangChainGo
// It acts as a gateway, configured to use various LLM providers underneath.
type LangChainProvider struct {
	llm          llms.Model
	providerType string // The underlying provider type (e.g., "openai", "ollama")
	modelName    string // The specific model configured (e.g., "gpt-4o", "llama3")
	logger       *logging.Logger
}

// LangChainModelFactory defines an interface for creating LangChain model instances
type LangChainModelFactory interface {
	// Create returns a new LangChain model instance based on the provided configuration
	Create(config map[string]interface{}, logger *logging.Logger) (llms.Model, error)
	// Validate checks if the configuration is valid for this factory
	Validate(config map[string]interface{}) error
}

// langChainModelFactories stores registered model factories
var langChainModelFactories = make(map[string]LangChainModelFactory)

// init registers the LangChain provider factory and built-in model factories.
func init() {
	// Register the LangChain provider factory
	err := RegisterProviderFactory(langchainProviderName, NewLangChainProviderFactory)
	if err != nil {
		panic(fmt.Sprintf("Failed to register langchain provider factory: %v", err))
	}

	// Register built-in model factories
	RegisterLangChainModelFactory(ProviderTypeOpenAI, &OpenAIModelFactory{})
	RegisterLangChainModelFactory(ProviderTypeOllama, &OllamaModelFactory{})
}

// RegisterLangChainModelFactory registers a new model factory for the given provider type
func RegisterLangChainModelFactory(providerType string, factory LangChainModelFactory) {
	langChainModelFactories[providerType] = factory
}

// GetSupportedLangChainProviders returns a list of supported provider types
func GetSupportedLangChainProviders() []string {
	providers := make([]string, 0, len(langChainModelFactories))
	for provider := range langChainModelFactories {
		providers = append(providers, provider)
	}
	return providers
}

// NewLangChainProviderFactory creates a LangChain provider instance from configuration.
func NewLangChainProviderFactory(config map[string]interface{}, logger *logging.Logger) (LLMProvider, error) {
	underlyingProviderType, ok := config["type"].(string)
	if !ok || underlyingProviderType == "" {
		return nil, fmt.Errorf("langchain config requires 'type' (string)")
	}

	// Get the factory for the requested provider type
	factory, exists := langChainModelFactories[underlyingProviderType]
	if !exists {
		supportedProviders := GetSupportedLangChainProviders()
		return nil, fmt.Errorf("unsupported langchain provider type '%s', supported types: %v",
			underlyingProviderType, supportedProviders)
	}

	modelName, ok := config["model"].(string)
	if !ok || modelName == "" {
		return nil, fmt.Errorf("langchain config requires 'model' (string)")
	}

	// Validate the configuration for this provider type
	if err := factory.Validate(config); err != nil {
		return nil, err
	}

	// Create a named logger for this provider
	providerLogger := logger.WithName("langchain-provider")
	providerLogger.InfoKV("Configuring LangChain provider", "type", underlyingProviderType, "model", modelName)

	// Create the LLM client using the factory
	llmClient, err := factory.Create(config, providerLogger)
	if err != nil {
		providerLogger.ErrorKV("Failed to initialize LangChainGo client", "type", underlyingProviderType, "error", err)
		return nil, fmt.Errorf("failed to initialize langchain %s client: %w", underlyingProviderType, err)
	}

	return &LangChainProvider{
		llm:          llmClient,
		providerType: underlyingProviderType,
		modelName:    modelName,
		logger:       providerLogger, // Assign the named logger
	}, nil
}

// GenerateCompletion generates a completion using LangChainGo
func (p *LangChainProvider) GenerateCompletion(ctx context.Context, prompt string, options ProviderOptions) (string, error) {
	if p.llm == nil {
		return "", errors.NewLLMError("client_not_initialized", "LangChainGo client not initialized")
	}

	p.logger.DebugKV("Calling LangChainGo GenerateCompletion", "prompt_length", len(prompt))
	callOptions := p.buildOptions(options)

	completion, err := llms.GenerateFromSinglePrompt(ctx, p.llm, prompt, callOptions...)
	if err != nil {
		p.logger.ErrorKV("LangChainGo GenerateCompletion request failed", "error", err)
		return "", errors.WrapLLMError(err, "request_failed", "Failed to generate completion from LangChainGo")
	}

	p.logger.DebugKV("Received GenerateCompletion response", "length", len(completion))
	return completion, nil
}

// GenerateChatCompletion generates a chat completion using LangChainGo
// Note: LangChainGo's basic llms.Model interface doesn't directly support chat messages.
// We simulate it by formatting messages into a single prompt.
func (p *LangChainProvider) GenerateChatCompletion(ctx context.Context, messages []RequestMessage, options ProviderOptions) (string, error) {
	if p.llm == nil {
		return "", errors.NewLLMError("client_not_initialized", "LangChainGo client not initialized")
	}

	p.logger.DebugKV("Calling LangChainGo GenerateChatCompletion", "num_messages", len(messages))

	// Convert our message format to a single prompt string
	var promptBuilder strings.Builder
	for _, msg := range messages {
		promptBuilder.WriteString(fmt.Sprintf("%s: %s\n", strings.ToUpper(msg.Role), msg.Content))
	}
	prompt := promptBuilder.String()
	// Add one final assistant prefix to indicate where the response should go
	// This might need adjustment depending on the specific model's fine-tuning
	prompt += "ASSISTANT: "

	// Call the underlying GenerateCompletion method with the formatted prompt
	return p.GenerateCompletion(ctx, prompt, options)
}

// GetInfo returns information about the provider.
func (p *LangChainProvider) GetInfo() ProviderInfo {
	displayName := fmt.Sprintf("LangChain (%s - %s)", p.providerType, p.modelName)
	description := fmt.Sprintf("LangChainGo gateway using %s model %s", p.providerType, p.modelName)

	return ProviderInfo{
		Name:        langchainProviderName,
		DisplayName: displayName,
		Description: description,
		Configured:  p.llm != nil,    // Configured if the client was successfully created
		Available:   p.IsAvailable(), // Check availability dynamically (basic check for now)
		Configuration: map[string]string{
			"Underlying Provider": p.providerType,
			"Model":               p.modelName,
			// Add other relevant non-sensitive info if needed (e.g., base URL if applicable)
		},
	}
}

// IsAvailable checks if the underlying client is initialized.
// A more thorough check could involve a test API call.
func (p *LangChainProvider) IsAvailable() bool {
	return p.llm != nil
}

// buildOptions creates LangChainGo-specific options from our generic options.
func (p *LangChainProvider) buildOptions(options ProviderOptions) []llms.CallOption {
	var callOptions []llms.CallOption

	// Model: LangChainGo model is typically set at client initialization.
	// However, some LLMs might allow overriding it per call.
	// We prioritize the model from options if provided.
	modelToUse := p.modelName // Default to the configured model
	if options.Model != "" {
		modelToUse = options.Model
		p.logger.DebugKV("Overriding model per-request", "new_model", modelToUse)
	}
	// Add WithModel only if it differs from the initialized one or if the underlying LLM supports it.
	// For simplicity now, we always add it if options.Model is set.
	if options.Model != "" {
		callOptions = append(callOptions, llms.WithModel(modelToUse))
	}

	// Temperature: Apply if > 0
	if options.Temperature > 0 {
		callOptions = append(callOptions, llms.WithTemperature(options.Temperature))
		p.logger.DebugKV("Adding Temperature option", "value", options.Temperature)
	}

	// MaxTokens: Apply if > 0
	if options.MaxTokens > 0 {
		callOptions = append(callOptions, llms.WithMaxTokens(options.MaxTokens))
		p.logger.DebugKV("Adding MaxTokens option", "value", options.MaxTokens)
	}

	// Note: options.TargetProvider is handled during factory creation, not here.

	return callOptions
}
