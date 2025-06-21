package llm

import (
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	customErrors "github.com/tuannvm/slack-mcp-client/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

// AnthropicModelFactory creates Anthropic LangChain model instances
type AnthropicModelFactory struct{}

// Validate checks if the configuration is valid for Anthropic
func (f *AnthropicModelFactory) Validate(config map[string]interface{}) error {
	// API key is required for Anthropic
	apiKey, ok := config["api_key"].(string)
	if !ok || apiKey == "" {
		return customErrors.NewLLMError("missing_config", "Anthropic config requires 'api_key' (string)")
	}
	return nil
}

// Create returns a new Anthropic LangChain model instance
func (f *AnthropicModelFactory) Create(config map[string]interface{}, logger *logging.Logger) (llms.Model, error) {
	modelName, _ := config["model"].(string)  // Already validated in parent factory
	apiKey, _ := config["api_key"].(string)   // Already validated in Validate method
	baseURL, _ := config["base_url"].(string) // Optional custom base URL

	opts := []anthropic.Option{
		anthropic.WithModel(modelName), // Set model during initialization
		anthropic.WithToken(apiKey),    // API key is required
	}

	if baseURL != "" {
		opts = append(opts, anthropic.WithBaseURL(baseURL))
		logger.InfoKV("Configuring LangChain with Anthropic", "base_url", baseURL, "model", modelName)
	} else {
		logger.InfoKV("Configuring LangChain with Anthropic (default endpoint)", "model", modelName)
	}

	llmClient, err := anthropic.New(opts...)
	if err != nil {
		logger.ErrorKV("Failed to initialize LangChainGo Anthropic client", "error", err)

		// Create a domain-specific error with additional context
		domainErr := customErrors.WrapLLMError(err, "initialization_failed", "Failed to initialize Anthropic client")

		// Add additional context data
		domainErr = domainErr.WithData("model", modelName)
		if baseURL != "" {
			domainErr = domainErr.WithData("base_url", baseURL)
		}

		return nil, domainErr
	}

	return llmClient, nil
}
