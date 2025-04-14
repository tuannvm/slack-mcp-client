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

	// First try to unmarshal using the new schema with mcpServers
	var servers []ServerConfig

	// Try the new format first (mcpServers object)
	tempConfigNew := struct {
		McpServers map[string]ServerConfig `json:"mcpServers"`
	}{}

	err = json.Unmarshal(configFileBytes, &tempConfigNew)
	if err == nil && len(tempConfigNew.McpServers) > 0 {
		// Convert the map to a slice of ServerConfig
		for name, server := range tempConfigNew.McpServers {
			// Set the name field from the map key
			server.Name = name
			servers = append(servers, server)
		}
	} else {
		// Fall back to the old format (servers array)
		tempConfigOld := struct {
			Servers []ServerConfig `json:"servers"`
		}{}

		err = json.Unmarshal(configFileBytes, &tempConfigOld)
		if err != nil {
			return nil, fmt.Errorf("failed to parse JSON config file '%s': %w", configFilePath, err)
		}

		servers = tempConfigOld.Servers
	}

	// Validate loaded server configurations
	if len(servers) == 0 {
		return nil, fmt.Errorf("no servers defined in config file '%s'", configFilePath)
	}

	serverNames := make(map[string]bool)
	for i, server := range servers {
		// Skip disabled servers
		if server.Disabled {
			continue
		}

		if server.Name == "" {
			return nil, fmt.Errorf("server definition at index %d in '%s' is missing a 'name'", i, configFilePath)
		}
		if serverNames[server.Name] {
			return nil, fmt.Errorf("duplicate server name '%s' found in '%s'", server.Name, configFilePath)
		}
		serverNames[server.Name] = true

		// For new schema servers, derive mode and address from command/env if needed
		if server.Command != "" {
			// If mode is not set but we have a command, default to stdio mode
			if server.Mode == "" {
				server.Mode = "stdio"
			}
			
			// Always set the address to the command for the stdio mode
			// This ensures backward compatibility with the main.go file
			server.Address = server.Command
		} else if server.Address == "" {
			return nil, fmt.Errorf("server '%s' in '%s' is missing both 'command' and 'address'", server.Name, configFilePath)
		}

		// Check for mode
		if server.Mode == "" {
			// Check if mode is in environment variables
			if mcpMode, exists := server.Env["MCP_MODE"]; exists && mcpMode != "" {
				server.Mode = mcpMode
			} else {
				return nil, fmt.Errorf("server '%s' in '%s' is missing a 'mode'", server.Name, configFilePath)
			}
		}

		switch server.Mode {
		case "http", "sse", "stdio":
			// Valid mode
		default:
			return nil, fmt.Errorf("server '%s' in '%s' has invalid mode '%s'. Must be 'http', 'sse', or 'stdio'", server.Name, configFilePath, server.Mode)
		}
	}

	cfg.Servers = servers // Assign validated servers to the main config

	return cfg, nil
}
