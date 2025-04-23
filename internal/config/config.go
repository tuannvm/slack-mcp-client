// Package config handles loading and managing application configuration
package config

import (
	"fmt"

	"github.com/spf13/viper"
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
func LoadConfig(configFile string) (*Config, error) {
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
	viper.BindEnv("slack_bot_token", "SLACK_BOT_TOKEN")
	viper.BindEnv("slack_app_token", "SLACK_APP_TOKEN")
	viper.BindEnv("llm_provider", "LLM_PROVIDER")

	// Bind environment variables for specific provider settings (example for OpenAI API Key)
	// Note: Viper doesn't easily bind nested map structures directly from env vars.
	// API keys are best handled directly via os.Getenv in the provider constructor for security.
	// viper.BindEnv("llm_providers.openai.api_key", "OPENAI_API_KEY") // This might not work as expected
	// viper.BindEnv("llm_providers.openai.model", "OPENAI_MODEL") // Prefer config file for model
	// viper.BindEnv("llm_providers.ollama.endpoint", "OLLAMA_API_ENDPOINT") // Prefer config file
	// viper.BindEnv("llm_providers.ollama.model", "OLLAMA_MODEL") // Prefer config file

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
