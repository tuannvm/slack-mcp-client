package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// ServerConfig defines the configuration for a single MCP server.
type ServerConfig struct {
	Name      string            `json:"name"`      // Name of the server (derived from the key in mcpServers)
	Command   string            `json:"command"`   // Command to execute
	Args      []string          `json:"args"`      // Arguments for the command
	Env       map[string]string `json:"env"`       // Environment variables for the command
	Disabled  bool              `json:"disabled"`  // Whether the server is disabled
	// Fields for connection details
	Address   string            `json:"address"`   // Connection address
	URL       string            `json:"url"`       // URL (alternative to address)
	Mode      string            `json:"mode"`      // Communication mode (http, sse, stdio)
}

// LLMProvider specifies which LLM provider to use
type LLMProvider string

const (
	// ProviderOpenAI uses OpenAI models
	ProviderOpenAI LLMProvider = "openai"
)

// Config holds the application configuration.
type Config struct {
	SlackBotToken     string                     `json:"-"` // Loaded from env
	SlackAppToken     string                     `json:"-"` // Loaded from env
	OpenAIModelName   string                     `json:"-"` // Loaded from env
	LLMProvider       LLMProvider               `json:"-"` // Will always be OpenAI now
	Servers           map[string]ServerConfig    `json:"servers"` // Map of server configs by name
}

// LoadConfig loads configuration from environment variables and a JSON file.
func LoadConfig(configFilePath string) (*Config, error) {
	// Attempt to load .env file, but don't fail if it doesn't exist
	_ = godotenv.Load()

	// Create base config from environment variables
	cfg, err := loadConfigFromEnv()
	if err != nil {
		return nil, err
	}

	// If no config file specified, return with environment-only config
	if configFilePath == "" {
		fmt.Println("No configuration file provided, using environment variables only.")
		return cfg, nil
	}

	// Load server configurations from file
	if err := loadServersFromFile(configFilePath, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadConfigFromEnv creates a config from environment variables. Assumes OpenAI is the provider.
func loadConfigFromEnv() (*Config, error) {
	// Initialize cfg fields, setting provider directly to OpenAI
	cfg := &Config{
		SlackBotToken:     os.Getenv("SLACK_BOT_TOKEN"),
		SlackAppToken:     os.Getenv("SLACK_APP_TOKEN"),
		LLMProvider:       ProviderOpenAI, // Set directly
		Servers:           make(map[string]ServerConfig),
	}

	// Load OpenAI model environment variable
	cfg.OpenAIModelName = os.Getenv("OPENAI_MODEL")
	// Validate OpenAI model (and set default)
	if cfg.OpenAIModelName == "" {
		cfg.OpenAIModelName = "gpt-4o"
		fmt.Printf("Warning: OPENAI_MODEL not set, defaulting to %s\n", cfg.OpenAIModelName)
	}

	// Validate essential Slack credentials
	if cfg.SlackBotToken == "" {
		return nil, fmt.Errorf("SLACK_BOT_TOKEN environment variable not set")
	}
	if cfg.SlackAppToken == "" {
		return nil, fmt.Errorf("SLACK_APP_TOKEN environment variable not set")
	}

	return cfg, nil
}

// loadServersFromFile loads MCP server configurations from a JSON file
func loadServersFromFile(configFilePath string, cfg *Config) error {
	// Read the config file
	configFileBytes, err := os.ReadFile(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to read config file '%s': %w", configFilePath, err)
	}

	// Parse JSON using the mcpServers structure
	tempConfig := struct {
		McpServers map[string]ServerConfig `json:"mcpServers"`
	}{}

	if err := json.Unmarshal(configFileBytes, &tempConfig); err != nil {
		return fmt.Errorf("failed to parse JSON config file '%s': %w", configFilePath, err)
	}

	// No servers defined in config file
	if len(tempConfig.McpServers) == 0 {
		return fmt.Errorf("no MCP servers defined in config file '%s'", configFilePath)
	}

	// Process and validate each server
	for name, server := range tempConfig.McpServers {
		// Skip disabled servers early
		if server.Disabled {
			continue
		}

		// Set the name from the key in the map
		server.Name = name
		
		// Handle URL field (move to Address if Address is empty)
		if server.URL != "" && server.Address == "" {
			server.Address = server.URL
		}
		
		// Validate connection details
		if server.Command == "" && server.Address == "" {
			return fmt.Errorf("server '%s' is missing both 'command' and 'address'/'url'", name)
		}

		// Set default mode if not specified
		if server.Mode == "" {
			// Default to stdio if command is provided, otherwise http
			if server.Command != "" {
				server.Mode = "stdio"
			} else {
				server.Mode = "http"
			}
		}

		// Validate mode
		switch strings.ToLower(server.Mode) {
		case "http", "sse", "stdio":
			// Valid mode, normalize to lowercase
			server.Mode = strings.ToLower(server.Mode)
		default:
			return fmt.Errorf("server '%s' has invalid mode '%s'. Must be 'http', 'sse', or 'stdio'", 
				name, server.Mode)
		}

		// Add the validated server to the config
		cfg.Servers[name] = server
	}

	return nil
}
