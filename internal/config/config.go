// Package config handles loading and managing application configuration
package config

import (
	"fmt"

	"github.com/spf13/viper"
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
	URL      string `mapstructure:"url"`
	Disabled bool   `mapstructure:"disabled"`
}

// Config defines the overall application configuration
type Config struct {
	SlackBotToken string                            `mapstructure:"slack_bot_token"`
	SlackAppToken string                            `mapstructure:"slack_app_token"`
	Servers       map[string]ServerConfig           `mapstructure:"servers"`
	LLMProvider   string                            `mapstructure:"llm_provider"`  // Name of the provider to USE (e.g., "openai", "ollama")
	LLMProviders  map[string]map[string]interface{} `mapstructure:"llm_providers"` // Configuration for ALL potential providers

	// Deprecated/Removed - configuration now within LLMProviders map
	// OpenAIModelName         string `mapstructure:"openai_model_name"`
	// LangChainTargetProvider string `mapstructure:"langchain_target_provider"`
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configFile string, logger *logging.Logger) (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.appname")
	viper.AddConfigPath("/etc/appname")

	// Attempt to read the config file
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("fatal error config file: %s", err)
	}

	// Set default values
	viper.SetDefault("llm_provider", ProviderOpenAI) // Default to OpenAI if not specified
	// Set default structures for known providers to avoid nil maps later
	viper.SetDefault("llm_providers", map[string]map[string]interface{}{
		ProviderOpenAI: {"model": "gpt-4o"},                                       // Default OpenAI model
		ProviderOllama: {"model": "llama3", "endpoint": "http://localhost:11434"}, // Default Ollama settings
		// Add other providers with defaults here if needed
	})

	// Environment variable bindings
	if err := viper.BindEnv("slack_bot_token", "SLACK_BOT_TOKEN"); err != nil {
		if logger != nil {
			logger.WarnKV("Failed to bind env var", "key", "slack_bot_token", "error", err)
		}
	}
	if err := viper.BindEnv("slack_app_token", "SLACK_APP_TOKEN"); err != nil {
		if logger != nil {
			logger.WarnKV("Failed to bind env var", "key", "slack_app_token", "error", err)
		}
	}
	if err := viper.BindEnv("llm_provider", "LLM_PROVIDER"); err != nil {
		if logger != nil {
			logger.WarnKV("Failed to bind env var", "key", "llm_provider", "error", err)
		}
	}

	// Unmarshal config into Config struct
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode into struct, %v", err)
	}

	// Post-load validation or adjustments (optional)
	if cfg.LLMProvider == "" {
		fmt.Println("Warning: llm_provider is empty, defaulting to 'openai'")
		cfg.LLMProvider = ProviderOpenAI
	}

	// Ensure provider maps exist even if not in config file
	if cfg.LLMProviders == nil {
		cfg.LLMProviders = make(map[string]map[string]interface{})
	}
	if _, ok := cfg.LLMProviders[ProviderOpenAI]; !ok {
		cfg.LLMProviders[ProviderOpenAI] = map[string]interface{}{"model": "gpt-4o"}
	}
	if _, ok := cfg.LLMProviders[ProviderOllama]; !ok {
		cfg.LLMProviders[ProviderOllama] = map[string]interface{}{"model": "llama3", "endpoint": "http://localhost:11434"}
	}

	return &cfg, nil
}
