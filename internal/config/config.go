package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// ServerConfig defines the configuration for a single MCP server.
type ServerConfig struct {
	Name    string `json:"name"`    // Unique name for the server (e.g., "trino-dev")
	Address string `json:"address"` // Connection address/identifier (e.g., "http://localhost:8080", "/path/to/bin")
	Mode    string `json:"mode"`    // Communication mode: "http", "sse", "stdio"
}

// Default Ollama API endpoint
const defaultOllamaAPIEndpoint = "http://localhost:11434/api/generate"

// Config holds the application configuration.
type Config struct {
	SlackBotToken    string         `json:"-"` // Loaded from env
	SlackAppToken    string         `json:"-"` // Loaded from env
	OllamaAPIEndpoint string         `json:"-"` // Loaded from env, with default
	OllamaModelName  string         `json:"-"` // Loaded from env
	Servers          []ServerConfig `json:"servers"` // Loaded from JSON config file
}

// LoadConfig loads configuration from environment variables and a JSON file.
func LoadConfig(configFilePath string) (*Config, error) {
	// Attempt to load .env file, but don't fail if it doesn't exist
	_ = godotenv.Load()

	ollamaEndpoint := os.Getenv("OLLAMA_API_ENDPOINT")
	if ollamaEndpoint == "" {
		ollamaEndpoint = defaultOllamaAPIEndpoint
		fmt.Printf("Warning: OLLAMA_API_ENDPOINT not set, defaulting to %s\n", defaultOllamaAPIEndpoint)
	}

	cfg := &Config{
		SlackBotToken:    os.Getenv("SLACK_BOT_TOKEN"),
		SlackAppToken:    os.Getenv("SLACK_APP_TOKEN"),
		OllamaAPIEndpoint: ollamaEndpoint,
		OllamaModelName:  os.Getenv("OLLAMA_MODEL"),
		Servers:          []ServerConfig{}, // Initialize empty slice
	}

	if cfg.SlackBotToken == "" {
		return nil, fmt.Errorf("SLACK_BOT_TOKEN environment variable not set")
	}
	if cfg.SlackAppToken == "" {
		return nil, fmt.Errorf("SLACK_APP_TOKEN environment variable not set")
	}
	if cfg.OllamaModelName == "" {
		return nil, fmt.Errorf("OLLAMA_MODEL environment variable not set (e.g., 'llama3')")
	}

	// Load server configurations from the specified JSON file
	if configFilePath == "" {
		return nil, fmt.Errorf("configuration file path (--config) not provided")
	}

	configFileBytes, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file '%s': %w", configFilePath, err)
	}

	// Use a temporary struct to unmarshal just the 'servers' key
	tempConfig := struct {
		Servers []ServerConfig `json:"servers"`
	}{}

	err = json.Unmarshal(configFileBytes, &tempConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON config file '%s': %w", configFilePath, err)
	}

	// Validate loaded server configurations
	if len(tempConfig.Servers) == 0 {
		return nil, fmt.Errorf("no servers defined in config file '%s'", configFilePath)
	}

	serverNames := make(map[string]bool)
	for i, server := range tempConfig.Servers {
		if server.Name == "" {
			return nil, fmt.Errorf("server definition at index %d in '%s' is missing a 'name'", i, configFilePath)
		}
		if serverNames[server.Name] {
			return nil, fmt.Errorf("duplicate server name '%s' found in '%s'", server.Name, configFilePath)
		}
		serverNames[server.Name] = true

		if server.Address == "" {
			return nil, fmt.Errorf("server '%s' in '%s' is missing an 'address'", server.Name, configFilePath)
		}
		if server.Mode == "" {
			return nil, fmt.Errorf("server '%s' in '%s' is missing a 'mode'", server.Name, configFilePath)
		}
		switch server.Mode {
		case "http", "sse", "stdio":
			// Valid mode
		default:
			return nil, fmt.Errorf("server '%s' in '%s' has invalid mode '%s'. Must be 'http', 'sse', or 'stdio'", server.Name, configFilePath, server.Mode)
		}
	}

	cfg.Servers = tempConfig.Servers // Assign validated servers to the main config

	return cfg, nil
}
