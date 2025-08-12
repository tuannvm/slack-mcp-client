// Package config handles loading and managing application configuration
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

// ValidateAfterDefaults validates configuration after defaults and env substitution
func (c *Config) ValidateAfterDefaults() error {
	// Validate required fields after environment substitution
	if c.Slack.BotToken == "" || strings.HasPrefix(c.Slack.BotToken, "${") {
		return fmt.Errorf("SLACK_BOT_TOKEN environment variable not set")
	}
	if c.Slack.AppToken == "" || strings.HasPrefix(c.Slack.AppToken, "${") {
		return fmt.Errorf("SLACK_APP_TOKEN environment variable not set")
	}

	// Validate LLM provider exists
	if _, exists := c.LLM.Providers[c.LLM.Provider]; !exists {
		return fmt.Errorf("LLM provider '%s' not configured", c.LLM.Provider)
	}

	// Validate provider-specific requirements
	providerConfig := c.LLM.Providers[c.LLM.Provider]
	switch c.LLM.Provider {
	case ProviderOpenAI:
		if providerConfig.APIKey == "" || strings.HasPrefix(providerConfig.APIKey, "${") {
			return fmt.Errorf("OPENAI_API_KEY environment variable not set")
		}
	case ProviderAnthropic:
		if providerConfig.APIKey == "" || strings.HasPrefix(providerConfig.APIKey, "${") {
			return fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")
		}
	}

	// Validate observability configuration
	if c.Observability.Enabled {
		if c.Observability.Provider == "langfuse-otel" {
			if c.Observability.Endpoint == "" || strings.HasPrefix(c.Observability.Endpoint, "${") {
				return fmt.Errorf("OBSERVABILITY_ENDPOINT environment variable not set for Langfuse")
			}
			if c.Observability.PublicKey == "" || strings.HasPrefix(c.Observability.PublicKey, "${") {
				return fmt.Errorf("LANGFUSE_PUBLIC_KEY environment variable not set")
			}
			if c.Observability.SecretKey == "" || strings.HasPrefix(c.Observability.SecretKey, "${") {
				return fmt.Errorf("LANGFUSE_SECRET_KEY environment variable not set")
			}
		}
	}

	return nil
}

// ValidateConfig performs comprehensive validation of the configuration structure
// using the JSON schema at schema/config-schema.json
func (c *Config) ValidateConfig() error {
	// Convert config to JSON for validation
	configJSON, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config for validation: %w", err)
	}

	// Find the schema file relative to the binary
	schemaPath, err := findSchemaPath()
	if err != nil {
		return fmt.Errorf("failed to find schema file: %w", err)
	}

	// Load the JSON schema
	schema, err := jsonschema.Compile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to compile JSON schema: %w", err)
	}

	// Parse the config JSON for validation
	var configData interface{}
	if err := json.Unmarshal(configJSON, &configData); err != nil {
		return fmt.Errorf("failed to unmarshal config for validation: %w", err)
	}

	// Validate against schema
	if err := schema.Validate(configData); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	return nil
}

// findSchemaPath locates the config schema file
func findSchemaPath() (string, error) {
	// Try various paths relative to working directory and executable
	possiblePaths := []string{
		"schema/config-schema.json",
		"./schema/config-schema.json",
		"../schema/config-schema.json",
		"../../schema/config-schema.json",
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return filepath.Abs(path)
		}
	}

	return "", fmt.Errorf("config schema file not found in any of: %v", possiblePaths)
}

// isLegacyFormat checks if the configuration data is in legacy mcp-servers.json format
func isLegacyFormat(configData []byte) bool {
	var rawConfig map[string]interface{}
	if err := json.Unmarshal(configData, &rawConfig); err != nil {
		return false
	}

	// Legacy format has "mcpServers" at root level and no "version", "slack", "llm" fields
	_, hasMcpServers := rawConfig["mcpServers"]
	_, hasVersion := rawConfig["version"]
	_, hasSlack := rawConfig["slack"]
	_, hasLLM := rawConfig["llm"]

	// It's legacy if it has mcpServers but none of the new top-level fields
	return hasMcpServers && !hasVersion && !hasSlack && !hasLLM
}

// loadLegacyConfigFile loads and converts legacy mcp-servers.json format
func loadLegacyConfigFile(cfg *Config, configData []byte, logger *logging.Logger) error {
	// Parse the legacy format
	var legacyConfig struct {
		McpServers map[string]MCPServerConfig `json:"mcpServers"`
	}

	if err := json.Unmarshal(configData, &legacyConfig); err != nil {
		return fmt.Errorf("failed to parse legacy config file: %w", err)
	}

	// Convert to new format by setting the MCP servers
	cfg.MCPServers = legacyConfig.McpServers

	if logger != nil {
		logger.InfoKV("Successfully converted legacy configuration",
			"mcpServersCount", len(cfg.MCPServers))
		logger.Warn("Consider migrating to new config.json format using --migrate-config flag")
	}

	return nil
}

// removeSchemaField removes the $schema field from JSON data to avoid strict parsing errors
func removeSchemaField(configData []byte) []byte {
	var rawConfig map[string]interface{}
	if err := json.Unmarshal(configData, &rawConfig); err != nil {
		return configData // Return original if unmarshal fails
	}

	// Remove $schema field if present
	delete(rawConfig, "$schema")

	// Marshal back to JSON
	if cleanData, err := json.Marshal(rawConfig); err == nil {
		return cleanData
	}

	return configData // Return original if marshal fails
}

// SubstituteEnvironmentVariables performs environment variable substitution
func (c *Config) SubstituteEnvironmentVariables() {
	// Substitute in Slack configuration
	c.Slack.BotToken = substituteEnvVars(c.Slack.BotToken)
	c.Slack.AppToken = substituteEnvVars(c.Slack.AppToken)

	// Substitute in LLM provider configurations
	for name, provider := range c.LLM.Providers {
		provider.APIKey = substituteEnvVars(provider.APIKey)
		provider.BaseURL = substituteEnvVars(provider.BaseURL)
		c.LLM.Providers[name] = provider
	}
}

// substituteEnvVars replaces ${VAR_NAME} patterns with environment variable values
func substituteEnvVars(input string) string {
	if strings.HasPrefix(input, "${") && strings.HasSuffix(input, "}") {
		varName := input[2 : len(input)-1]
		if envValue := os.Getenv(varName); envValue != "" {
			return envValue
		}
	}
	return input
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configFile string, logger *logging.Logger) (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		if logger != nil {
			logger.InfoKV("No .env file loaded", "error", err)
		}
	} else if logger != nil {
		logger.InfoKV("Loaded environment variables from .env file", "success", true)
	}

	// Initialize config with defaults
	cfg := &Config{}
	cfg.ApplyDefaults()

	// Apply environment variable overrides BEFORE loading config file
	// This ensures config file has highest priority
	cfg.ApplyEnvironmentVariables()

	// Read config file if provided - this will override environment variables
	if configFile != "" {
		if err := loadConfigFile(cfg, configFile, logger); err != nil {
			return nil, err
		}
	}

	// Perform environment variable substitution (for ${VAR} placeholders only)
	cfg.SubstituteEnvironmentVariables()

	// Validate configuration
	if err := cfg.ValidateAfterDefaults(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// loadConfigFile loads configuration from a file
func loadConfigFile(cfg *Config, configFile string, logger *logging.Logger) error {
	// Ensure the file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", configFile)
	}

	// Read and parse the config file
	configData, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %s", err)
	}

	// Check if this is a legacy format
	if isLegacyFormat(configData) {
		if logger != nil {
			logger.InfoKV("Detected legacy configuration format, converting automatically", "file", configFile)
		}
		return loadLegacyConfigFile(cfg, configData, logger)
	}

	// Parse as new format - first remove $schema field if present, then parse with strict validation
	configData = removeSchemaField(configData)
	dec := json.NewDecoder(bytes.NewReader(configData))
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	if logger != nil {
		logger.InfoKV("Loaded configuration from file", "file", configFile)
	}

	return nil
}
