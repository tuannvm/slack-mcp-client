// Package config handles loading and managing application configuration
package config

import (
	"os"
	"strconv"
)

// Constants for provider types
const (
	ProviderOpenAI    = "openai"
	ProviderOllama    = "ollama"
	ProviderAnthropic = "anthropic"
)

// Config represents the main application configuration
type Config struct {
	Version    string                     `json:"version"`
	Slack      SlackConfig                `json:"slack"`
	LLM        LLMConfig                  `json:"llm"`
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
	RAG        RAGConfig                  `json:"rag,omitempty"`
	Monitoring MonitoringConfig           `json:"monitoring,omitempty"`
}

// SlackConfig contains Slack-specific configuration
type SlackConfig struct {
	BotToken string `json:"botToken"`
	AppToken string `json:"appToken"`
}

// LLMConfig contains LLM provider configuration
type LLMConfig struct {
	Provider          string                       `json:"provider"`
	UseNativeTools    bool                         `json:"useNativeTools,omitempty"`
	UseAgent          bool                         `json:"useAgent,omitempty"`
	CustomPrompt      string                       `json:"customPrompt,omitempty"`
	CustomPromptFile  string                       `json:"customPromptFile,omitempty"`
	ReplaceToolPrompt bool                         `json:"replaceToolPrompt,omitempty"`
	Providers         map[string]LLMProviderConfig `json:"providers"`
}

// LLMProviderConfig contains provider-specific settings
type LLMProviderConfig struct {
	Model       string  `json:"model"`
	APIKey      string  `json:"apiKey,omitempty"`
	BaseURL     string  `json:"baseUrl,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"maxTokens,omitempty"`
}

// MCPServerConfig contains MCP server configuration
type MCPServerConfig struct {
	Command                  string            `json:"command,omitempty"`
	Args                     []string          `json:"args,omitempty"`
	URL                      string            `json:"url,omitempty"`
	Transport                string            `json:"transport,omitempty"`
	Env                      map[string]string `json:"env,omitempty"`
	Disabled                 bool              `json:"disabled,omitempty"`
	InitializeTimeoutSeconds *int              `json:"initializeTimeoutSeconds,omitempty"`
	Tools                    MCPToolsConfig    `json:"tools,omitempty"`
}

// GetTransport returns the transport type, inferring from other fields if not explicitly set
func (mcp *MCPServerConfig) GetTransport() string {
	if mcp.Transport != "" {
		return mcp.Transport
	}
	if mcp.Command != "" {
		return "stdio" // Default: if command is specified, use stdio
	}
	if mcp.URL != "" {
		return "sse" // Default: if URL is specified, use sse
	}
	return "stdio" // Fallback default
}

// GetInitializeTimeout returns the timeout with default fallback
func (mcp *MCPServerConfig) GetInitializeTimeout() int {
	if mcp.InitializeTimeoutSeconds != nil {
		return *mcp.InitializeTimeoutSeconds
	}
	return 30 // Default timeout: 30 seconds
}

// MCPToolsConfig contains tool filtering configuration
type MCPToolsConfig struct {
	AllowList []string `json:"allowList,omitempty"`
	BlockList []string `json:"blockList,omitempty"`
}

// RAGConfig contains RAG system configuration
type RAGConfig struct {
	Enabled   bool                         `json:"enabled,omitempty"`
	Provider  string                       `json:"provider,omitempty"`
	ChunkSize int                          `json:"chunkSize,omitempty"`
	Providers map[string]RAGProviderConfig `json:"providers,omitempty"`
}

// RAGProviderConfig contains RAG provider-specific settings
type RAGProviderConfig struct {
	DatabasePath     string `json:"databasePath,omitempty"`     // Simple provider: path to JSON database
	IndexName        string `json:"indexName,omitempty"`        // OpenAI provider: vector store name
	VectorStoreID    string `json:"vectorStoreId,omitempty"`    // OpenAI provider: existing vector store ID
	Dimensions       int    `json:"dimensions,omitempty"`       // OpenAI provider: embedding dimensions
	SimilarityMetric string `json:"similarityMetric,omitempty"` // OpenAI provider: similarity metric
	MaxResults       int    `json:"maxResults,omitempty"`       // OpenAI provider: maximum search results
}

// MonitoringConfig contains monitoring and observability settings
type MonitoringConfig struct {
	Enabled      bool   `json:"enabled,omitempty"`
	MetricsPort  int    `json:"metricsPort,omitempty"`
	LoggingLevel string `json:"loggingLevel,omitempty"`
}

// ApplyDefaults applies default values to the configuration
func (c *Config) ApplyDefaults() {
	// Set version if not specified
	if c.Version == "" {
		c.Version = "2.0"
	}

	// LLM defaults
	if c.LLM.Provider == "" {
		c.LLM.Provider = ProviderOpenAI
	}

	// Ensure providers map exists
	if c.LLM.Providers == nil {
		c.LLM.Providers = make(map[string]LLMProviderConfig)
	}

	// Set default provider configurations if they don't exist
	if _, exists := c.LLM.Providers[ProviderOpenAI]; !exists {
		c.LLM.Providers[ProviderOpenAI] = LLMProviderConfig{
			Model:       "gpt-4o",
			Temperature: 0.7,
		}
	}

	if _, exists := c.LLM.Providers[ProviderAnthropic]; !exists {
		c.LLM.Providers[ProviderAnthropic] = LLMProviderConfig{
			Model:       "claude-3-5-sonnet-20241022",
			Temperature: 0.7,
		}
	}

	if _, exists := c.LLM.Providers[ProviderOllama]; !exists {
		c.LLM.Providers[ProviderOllama] = LLMProviderConfig{
			Model:       "llama3",
			BaseURL:     "http://localhost:11434",
			Temperature: 0.7,
		}
	}

	// RAG defaults
	if c.RAG.Provider == "" {
		c.RAG.Provider = "simple"
	}
	if c.RAG.ChunkSize == 0 {
		c.RAG.ChunkSize = 1000
	}
	if c.RAG.Providers == nil {
		c.RAG.Providers = make(map[string]RAGProviderConfig)
	}
	if _, exists := c.RAG.Providers["simple"]; !exists {
		c.RAG.Providers["simple"] = RAGProviderConfig{
			DatabasePath: "./rag.db",
		}
	}
	if _, exists := c.RAG.Providers["openai"]; !exists {
		c.RAG.Providers["openai"] = RAGProviderConfig{
			IndexName:  "slack-mcp-rag",
			Dimensions: 1536,
		}
	}

	// Monitoring defaults
	c.Monitoring.Enabled = true // Default to enabled
	if c.Monitoring.MetricsPort == 0 {
		c.Monitoring.MetricsPort = 8080
	}
	if c.Monitoring.LoggingLevel == "" {
		c.Monitoring.LoggingLevel = "info"
	}

	// Ensure MCP servers map exists
	if c.MCPServers == nil {
		c.MCPServers = make(map[string]MCPServerConfig)
	}
}

// ApplyEnvironmentVariables applies environment variable overrides
func (c *Config) ApplyEnvironmentVariables() {
	// Slack configuration
	if token := os.Getenv("SLACK_BOT_TOKEN"); token != "" {
		c.Slack.BotToken = token
	}
	if token := os.Getenv("SLACK_APP_TOKEN"); token != "" {
		c.Slack.AppToken = token
	}

	// LLM provider override
	if provider := os.Getenv("LLM_PROVIDER"); provider != "" {
		c.LLM.Provider = provider
	}

	// Custom prompt override
	if prompt := os.Getenv("CUSTOM_PROMPT"); prompt != "" {
		c.LLM.CustomPrompt = prompt
	}

	// Monitoring overrides
	if enabled := os.Getenv("MONITORING_ENABLED"); enabled != "" {
		if val, err := strconv.ParseBool(enabled); err == nil {
			c.Monitoring.Enabled = val
		}
	}

	// Apply API keys to provider configurations
	if c.LLM.Providers == nil {
		c.LLM.Providers = make(map[string]LLMProviderConfig)
	}

	// OpenAI configuration
	if openaiConfig, exists := c.LLM.Providers[ProviderOpenAI]; exists {
		if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
			openaiConfig.APIKey = apiKey
		}
		if model := os.Getenv("OPENAI_MODEL"); model != "" {
			openaiConfig.Model = model
		}
		c.LLM.Providers[ProviderOpenAI] = openaiConfig
	}

	// Anthropic configuration
	if anthropicConfig, exists := c.LLM.Providers[ProviderAnthropic]; exists {
		if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
			anthropicConfig.APIKey = apiKey
		}
		if model := os.Getenv("ANTHROPIC_MODEL"); model != "" {
			anthropicConfig.Model = model
		}
		c.LLM.Providers[ProviderAnthropic] = anthropicConfig
	}

	// Ollama configuration
	if ollamaConfig, exists := c.LLM.Providers[ProviderOllama]; exists {
		if baseURL := os.Getenv("OLLAMA_BASE_URL"); baseURL != "" {
			ollamaConfig.BaseURL = baseURL
		}
		if model := os.Getenv("OLLAMA_MODEL"); model != "" {
			ollamaConfig.Model = model
		}
		c.LLM.Providers[ProviderOllama] = ollamaConfig
	}
}
