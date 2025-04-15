package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// ServerConfig defines the configuration for a single MCP server.
type ServerConfig struct {
	Name      string            `json:"name"`      // Name of the server (derived from the key in mcpServers)
	Command   string            `json:"command"`   // Command to execute
	Args      []string          `json:"args"`      // Arguments for the command
	Env       map[string]string `json:"env"`       // Environment variables for the command
	Disabled  bool              `json:"disabled"`  // Whether the server is disabled
	// Legacy fields for backward compatibility
	Address   string            `json:"address"`   // Connection address (for backward compatibility)
	Mode      string            `json:"mode"`      // Communication mode (for backward compatibility)
}

// Default Ollama API endpoint
const defaultOllamaAPIEndpoint = "http://localhost:11434/api/generate"

// Config holds the application configuration.
type Config struct {
	SlackBotToken     string                     `json:"-"` // Loaded from env
	SlackAppToken     string                     `json:"-"` // Loaded from env
	OllamaAPIEndpoint string                     `json:"-"` // Loaded from env, with default
	OllamaModelName   string                     `json:"-"` // Loaded from env
	Servers           map[string]ServerConfig    `json:"servers"` // Map of server configs by name
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
		SlackBotToken:     os.Getenv("SLACK_BOT_TOKEN"),
		SlackAppToken:     os.Getenv("SLACK_APP_TOKEN"),
		OllamaAPIEndpoint: ollamaEndpoint,
		OllamaModelName:   os.Getenv("OLLAMA_MODEL"),
		Servers:           make(map[string]ServerConfig), // Initialize empty map
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

	// Try the new format with mcpServers object
	tempConfig := struct {
		McpServers map[string]ServerConfig `json:"mcpServers"`
	}{}

	if err := json.Unmarshal(configFileBytes, &tempConfig); err != nil {
		return nil, fmt.Errorf("failed to parse JSON config file '%s': %w", configFilePath, err)
	}

	// No servers defined in config file
	if len(tempConfig.McpServers) == 0 {
		return nil, fmt.Errorf("no MCP servers defined in config file '%s'", configFilePath)
	}

	// Process and validate each server
	for name, server := range tempConfig.McpServers {
		// Skip disabled servers
		if server.Disabled {
			continue
		}

		// Set the name from the key in the map
		server.Name = name
		
		// For stdio mode servers, derive mode and address from command/env if needed
		if server.Command != "" {
			// If mode is not set but we have a command, default to stdio mode
			if server.Mode == "" {
				server.Mode = "stdio"
				server.Address = server.Command
			}
		} else if server.Address == "" {
			return nil, fmt.Errorf("server '%s' in '%s' is missing both 'command' and 'address'", name, configFilePath)
		}

		// Check for mode
		if server.Mode == "" {
			// Check if mode is in environment variables
			if mcpMode, exists := server.Env["MCP_MODE"]; exists && mcpMode != "" {
				server.Mode = mcpMode
			} else {
				// Default to stdio if not specified
				server.Mode = "stdio"
			}
		}

		// Validate mode
		switch server.Mode {
		case "http", "sse", "stdio":
			// Valid mode
		default:
			return nil, fmt.Errorf("server '%s' in '%s' has invalid mode '%s'. Must be 'http', 'sse', or 'stdio'", name, configFilePath, server.Mode)
		}

		// Add the validated server to the config
		cfg.Servers[name] = server
	}

	// Ensure we have at least one enabled server
	if len(cfg.Servers) == 0 {
		return nil, fmt.Errorf("no enabled MCP servers found in config file '%s'", configFilePath)
	}

	return cfg, nil
}
