package config

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config holds the application configuration.
// Tags define the environment variables to load.
type Config struct {
	// Slack Configuration
	SlackBotToken string `envconfig:"SLACK_BOT_TOKEN" required:"true"`
	SlackAppToken string `envconfig:"SLACK_APP_TOKEN" required:"true"`

	// MCP Configuration
	MCPServerListenAddress string `envconfig:"MCP_SERVER_LISTEN_ADDRESS" default:":8080"`
	MCPTargetServerAddress string `envconfig:"MCP_TARGET_SERVER_ADDRESS" default:"http://localhost:8080"` // Address of MCP server to call

	// LLM Configuration (Add more as needed)
	// OpenaiApiKey string `envconfig:"OPENAI_API_KEY"`

	// Other Configuration
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
}

// Load reads configuration from environment variables.
// It first attempts to load a .env file if present.
func Load() (*Config, error) {
	// Attempt to load .env file, but ignore errors (it might not exist)
	_ = godotenv.Load()

	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Printf("Error loading configuration: %v", err) // Log the error for debugging
		return nil, err
	}

	return &cfg, nil
}
