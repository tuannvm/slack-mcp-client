package llm

import (
	"fmt"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

// OllamaModelFactory creates Ollama LangChain model instances
type OllamaModelFactory struct{}

// Validate checks if the configuration is valid for Ollama
func (f *OllamaModelFactory) Validate(config map[string]interface{}) error {
	baseURL, ok := config["base_url"].(string)
	if !ok || baseURL == "" {
		return fmt.Errorf("ollama config requires 'base_url' (string)")
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
		return nil, fmt.Errorf("failed to initialize Ollama client: %w", err)
	}

	return llmClient, nil
}
