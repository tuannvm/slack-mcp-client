// Package config handles loading and managing application configuration
package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

// Constants for provider types
const (
	ProviderOpenAI    = "openai"
	ProviderOllama    = "ollama"
	ProviderLangChain = "langchain" // Keep for potential direct LangChain use if needed
)

// ServerConfig defines the configuration for a single MCP server
type ServerConfig struct {
	URL      string `json:"url"`
	Disabled bool   `json:"disabled"`
}

// Config defines the overall application configuration
type Config struct {
	SlackBotToken string                            `json:"slack_bot_token"`
	SlackAppToken string                            `json:"slack_app_token"`
	Servers       map[string]ServerConfig           `json:"servers"`
	LLMProvider   string                            `json:"llm_provider"`  // Name of the provider to USE (e.g., "openai", "ollama")
	LLMProviders  map[string]map[string]interface{} `json:"llm_providers"` // Configuration for ALL potential providers
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configFile string, logger *logging.Logger) (*Config, error) {
	// Initialize default config
	cfg := &Config{
		LLMProvider: ProviderOpenAI, // Default to OpenAI if not specified
		LLMProviders: map[string]map[string]interface{}{
			ProviderOpenAI: {"type": "openai", "model": "gpt-4o"},                                       // Default OpenAI model
			ProviderOllama: {"type": "ollama", "model": "llama3", "base_url": "http://localhost:11434"}, // Default Ollama settings
			// Add other providers with defaults here if needed
		},
		Servers: make(map[string]ServerConfig),
	}

	// Read config file if provided
	if configFile != "" {
		// Ensure the file exists
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			return nil, fmt.Errorf("config file does not exist: %s", configFile)
		}

		// Read and parse the config file
		configData, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %s", err)
		}

		// Parse JSON into config struct
		if err := json.Unmarshal(configData, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %s", err)
		}

		if logger != nil {
			logger.InfoKV("Loaded configuration from file", "file", configFile)
		}
	}

	// Override with environment variables

	// Slack tokens
	if slackBotToken := os.Getenv("SLACK_BOT_TOKEN"); slackBotToken != "" {
		cfg.SlackBotToken = slackBotToken
	}

	if slackAppToken := os.Getenv("SLACK_APP_TOKEN"); slackAppToken != "" {
		cfg.SlackAppToken = slackAppToken
	}

	// Check for LLM provider override from environment variable
	if llmProviderEnv := os.Getenv("LLM_PROVIDER"); llmProviderEnv != "" {
		if logger != nil {
			logger.InfoKV("Overriding LLM provider from environment variable", "provider", llmProviderEnv)
		}
		cfg.LLMProvider = llmProviderEnv
	}

	// Ensure provider maps exist
	if cfg.LLMProviders == nil {
		cfg.LLMProviders = make(map[string]map[string]interface{})
	}

	// Ensure the selected provider has a configuration
	if _, ok := cfg.LLMProviders[cfg.LLMProvider]; !ok {
		// Create default config for the selected provider
		switch cfg.LLMProvider {
		case ProviderOpenAI:
			cfg.LLMProviders[ProviderOpenAI] = map[string]interface{}{
				"type":  "openai",
				"model": "gpt-4o",
			}
		case ProviderOllama:
			cfg.LLMProviders[ProviderOllama] = map[string]interface{}{
				"type":     "ollama",
				"model":    "llama3",
				"base_url": "http://localhost:11434",
			}
		default:
			// For any other provider, create a basic config with langchain as the type
			cfg.LLMProviders[cfg.LLMProvider] = map[string]interface{}{
				"type":  "langchain",
				"model": "default",
			}
		}
	}

	// Apply environment variables for API keys if available
	if providerConfig, ok := cfg.LLMProviders[ProviderOpenAI]; ok {
		if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
			providerConfig["api_key"] = apiKey
		}
		if model := os.Getenv("OPENAI_MODEL"); model != "" {
			providerConfig["model"] = model
		}
		cfg.LLMProviders[ProviderOpenAI] = providerConfig
	}

	return cfg, nil
}
