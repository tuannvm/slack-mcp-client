// Package config handles loading and managing application configuration
package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

// Constants for provider types
const (
	ProviderOpenAI    = "openai"
	ProviderOllama    = "ollama"
	ProviderAnthropic = "anthropic"
	ProviderLangChain = "langchain" // Keep for potential direct LangChain use if needed
)

// ServerConfig defines the configuration for a single MCP server
type ServerConfig struct {
	URL                      string            `json:"url,omitempty"`                        // For HTTP/SSE mode
	Mode                     string            `json:"mode,omitempty"`                       // Mode of operation (e.g., "http", "sse")
	Command                  string            `json:"command,omitempty"`                    // For stdio mode
	Args                     []string          `json:"args,omitempty"`                       // Command arguments
	Env                      map[string]string `json:"env,omitempty"`                        // Environment variables
	Disabled                 bool              `json:"disabled,omitempty"`                   // Whether this server is disabled
	InitializeTimeoutSeconds *int              `json:"initialize_timeout_seconds,omitempty"` // Timeout for server initialization

	AllowList []string `json:"allow_list,omitempty"` // List of allowed tools
	BlockList []string `json:"block_list,omitempty"` // List of blocked tools
}

// MCPServersConfig is the top-level structure for the MCP servers configuration
type MCPServersConfig struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// Config defines the overall application configuration
type Config struct {
	UseStdIOClient *bool `json:"use_stdio_client,omitempty"` // Use stdio client instead of Slack API

	SlackBotToken string `json:"slack_bot_token"`
	SlackAppToken string `json:"slack_app_token"`

	Servers map[string]ServerConfig `json:"servers"`

	UseNativeTools *bool                             `json:"use_native_tools,omitempty"` // Use MCP bridge for LLMs
	LLMProvider    string                            `json:"llm_provider"`               // Name of the provider to USE (e.g., "openai", "ollama")
	LLMProviders   map[string]map[string]interface{} `json:"llm_providers"`              // Configuration for ALL potential providers

	// Custom prompt configuration
	CustomPrompt      string `json:"custom_prompt,omitempty"`       // Custom system prompt to prepend to tool instructions
	ReplaceToolPrompt *bool  `json:"replace_tool_prompt,omitempty"` // If true, completely replace tool prompt with custom prompt
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configFile string, logger *logging.Logger) (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		// Only log as info since .env file is optional
		if logger != nil {
			logger.InfoKV("No .env file loaded", "error", err)
		}
	} else if logger != nil {
		logger.InfoKV("Loaded environment variables from .env file", "success", true)
	}
	// Initialize default config
	cfg := &Config{
		LLMProvider: ProviderOpenAI, // Default to OpenAI if not specified
		LLMProviders: map[string]map[string]interface{}{
			ProviderOpenAI:    {"type": "openai", "model": "gpt-4o"},                                       // Default OpenAI model
			ProviderOllama:    {"type": "ollama", "model": "llama3", "base_url": "http://localhost:11434"}, // Default Ollama settings
			ProviderAnthropic: {"type": "anthropic", "model": "claude-3-5-sonnet-20241022"},                // Default Anthropic model
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

		// First try to parse as MCPServersConfig (new format)
		var mcpConfig MCPServersConfig
		if err := json.Unmarshal(configData, &mcpConfig); err == nil && len(mcpConfig.MCPServers) > 0 {
			// Successfully parsed as MCPServersConfig
			cfg.Servers = mcpConfig.MCPServers
			if logger != nil {
				logger.InfoKV("Loaded MCP servers configuration from file", "file", configFile, "server_count", len(mcpConfig.MCPServers))
			}
		} else {
			// Try to parse as regular Config
			if err := json.Unmarshal(configData, cfg); err != nil {
				return nil, fmt.Errorf("failed to parse config file: %s", err)
			}

			if logger != nil {
				logger.InfoKV("Loaded configuration from file", "file", configFile)
			}
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
		case ProviderAnthropic:
			cfg.LLMProviders[ProviderAnthropic] = map[string]interface{}{
				"type":  "anthropic",
				"model": "claude-3-5-sonnet-20241022",
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

	if providerConfig, ok := cfg.LLMProviders[ProviderAnthropic]; ok {
		if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
			providerConfig["api_key"] = apiKey
		}
		if model := os.Getenv("ANTHROPIC_MODEL"); model != "" {
			providerConfig["model"] = model
		}
		cfg.LLMProviders[ProviderAnthropic] = providerConfig
	}

	if cfg.UseNativeTools == nil {
		// Default to false if not set
		defaultUseLLMMCPBridge := false
		cfg.UseNativeTools = &defaultUseLLMMCPBridge
	}

	return cfg, nil
}
