package llm

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	customErrors "github.com/tuannvm/slack-mcp-client/v2/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/v2/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/v2/internal/monitoring"
)

type handleContentEndFunc func(res *llms.ContentResponse)

type openaiCallbackHandler struct {
	callbacks.SimpleHandler
	handleContentEndFunc
}

func (handler *openaiCallbackHandler) HandleLLMGenerateContentEnd(_ context.Context, res *llms.ContentResponse) {
	handler.handleContentEndFunc(res)
}

// OpenAIModelFactory creates OpenAI LangChain model instances
type OpenAIModelFactory struct{}

// Validate checks if the configuration is valid for OpenAI
func (f *OpenAIModelFactory) Validate(config map[string]interface{}) error {
	// API key is optional if base_url points to compatible API
	// Model is already validated in the parent factory
	return nil
}

// Create returns a new OpenAI LangChain model instance
func (f *OpenAIModelFactory) Create(config map[string]interface{}, logger *logging.Logger) (llms.Model, error) {
	modelName, _ := config["model"].(string) // Already validated in parent factory
	apiKey, _ := config["api_key"].(string)  // API key is optional if base_url points to compatible API
	baseURL, _ := config["base_url"].(string)

	opts := []openai.Option{
		openai.WithModel(modelName), // Set model during initialization
		openai.WithCallback(&openaiCallbackHandler{
			SimpleHandler: callbacks.SimpleHandler{},
			handleContentEndFunc: func(res *llms.ContentResponse) {
				if len(res.Choices) > 0 {
					choice := res.Choices[0]
					if choice.GenerationInfo != nil {
						for key, value := range choice.GenerationInfo {
							valInt, ok := value.(int)
							if !ok {
								logger.WarnKV("unexpected non-int value for LLM token count", "key", key, "value", value)
								continue
							}
							monitoring.LLMTokensPerRequest.
								With(prometheus.Labels{
									monitoring.MetricLabelType:  key,
									monitoring.MetricLabelModel: modelName,
								}).
								Observe(float64(valInt))
						}
					}
				}
			},
		}),
	}

	if apiKey != "" {
		opts = append(opts, openai.WithToken(apiKey))
	}

	if baseURL != "" {
		opts = append(opts, openai.WithBaseURL(baseURL))
		logger.InfoKV("Configuring LangChain with OpenAI", "base_url", baseURL, "model", modelName)
	} else {
		logger.InfoKV("Configuring LangChain with OpenAI (default endpoint)", "model", modelName)
	}

	llmClient, err := openai.New(opts...)
	if err != nil {
		logger.ErrorKV("Failed to initialize LangChainGo OpenAI client", "error", err)

		// Create a domain-specific error with additional context
		domainErr := customErrors.WrapLLMError(err, "initialization_failed", "Failed to initialize OpenAI client")

		// Add additional context data
		domainErr = domainErr.WithData("model", modelName)
		if baseURL != "" {
			domainErr = domainErr.WithData("base_url", baseURL)
		}

		return nil, domainErr
	}

	return llmClient, nil
}
