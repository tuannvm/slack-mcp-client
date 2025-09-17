package llm

import (
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
	customErrors "github.com/tuannvm/slack-mcp-client/v2/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/v2/internal/common/logging"
)

// OllamaModelFactory creates Ollama LangChain model instances
type OllamaModelFactory struct{}

// Validate checks if the configuration is valid for Ollama
func (f *OllamaModelFactory) Validate(config map[string]interface{}) error {
	baseURL, ok := config["base_url"].(string)
	if !ok || baseURL == "" {
		return customErrors.NewLLMError("missing_config", "Ollama config requires 'base_url' (string)")
	}
	return nil
}

// Create returns a new Ollama LangChain model instance
func (f *OllamaModelFactory) Create(config map[string]interface{}, logger *logging.Logger) (llms.Model, error) {
	modelName, _ := config["model"].(string)  // Already validated in parent factory
	baseURL, _ := config["base_url"].(string) // Already validated in Validate method

	opts := []ollama.Option{
		ollama.WithModel(modelName),
		ollama.WithServerURL(baseURL),
	}

	logger.InfoKV("Configuring LangChain with Ollama", "base_url", baseURL, "model", modelName)

	llmClient, err := ollama.New(opts...)
	if err != nil {
		logger.ErrorKV("Failed to initialize LangChainGo Ollama client", "error", err)

		// Create a domain-specific error with additional context
		domainErr := customErrors.WrapLLMError(err, "initialization_failed", "Failed to initialize Ollama client")

		// Add additional context data
		domainErr = domainErr.WithData("model", modelName)
		domainErr = domainErr.WithData("base_url", baseURL)

		return nil, domainErr
	}

	return llmClient, nil
}
