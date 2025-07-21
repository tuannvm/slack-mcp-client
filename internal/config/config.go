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
	Timeouts   TimeoutConfig              `json:"timeouts,omitempty"`
	Retry      RetryConfig                `json:"retry,omitempty"`
	Reload     ReloadConfig               `json:"reload,omitempty"`
}

// SlackConfig contains Slack-specific configuration
type SlackConfig struct {
	BotToken        string `json:"botToken"`
	AppToken        string `json:"appToken"`
	MessageHistory  int    `json:"messageHistory,omitempty"`  // Max messages to keep in history per channel (default: 50)
	ThinkingMessage string `json:"thinkingMessage,omitempty"` // Custom "thinking" message (default: "Thinking...")
}

// LLMConfig contains LLM provider configuration
type LLMConfig struct {
	Provider           string                       `json:"provider"`
	UseNativeTools     bool                         `json:"useNativeTools,omitempty"`
	UseAgent           bool                         `json:"useAgent,omitempty"`
	CustomPrompt       string                       `json:"customPrompt,omitempty"`
	CustomPromptFile   string                       `json:"customPromptFile,omitempty"`
	ReplaceToolPrompt  bool                         `json:"replaceToolPrompt,omitempty"`
	MaxAgentIterations int                          `json:"maxAgentIterations,omitempty"` // Maximum agent iterations (default: 20)
	Providers          map[string]LLMProviderConfig `json:"providers"`
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
	DatabasePath             string  `json:"databasePath,omitempty"`             // Simple provider: path to JSON database
	IndexName                string  `json:"indexName,omitempty"`                // OpenAI provider: vector store name
	VectorStoreID            string  `json:"vectorStoreId,omitempty"`            // OpenAI provider: existing vector store ID
	Dimensions               int     `json:"dimensions,omitempty"`               // OpenAI provider: embedding dimensions
	SimilarityMetric         string  `json:"similarityMetric,omitempty"`         // OpenAI provider: similarity metric
	MaxResults               int     `json:"maxResults,omitempty"`               // OpenAI provider: maximum search results
	ScoreThreshold           float64 `json:"scoreThreshold,omitempty"`           // OpenAI provider: score threshold
	RewriteQuery             bool    `json:"rewriteQuery,omitempty"`             // OpenAI provider: rewrite query
	VectorStoreNameRegex     string  `json:"vectorStoreNameRegex,omitempty"`     // OpenAI provider: vector store name regex
	VectorStoreMetadataKey   string  `json:"vectorStoreMetadataKey,omitempty"`   // OpenAI provider: vector store metadata key
	VectorStoreMetadataValue string  `json:"vectorStoreMetadataValue,omitempty"` // OpenAI provider: vector store metadata value
}

// MonitoringConfig contains monitoring and observability settings
type MonitoringConfig struct {
	Enabled      bool   `json:"enabled,omitempty"`
	MetricsPort  int    `json:"metricsPort,omitempty"`
	LoggingLevel string `json:"loggingLevel,omitempty"`
}

// TimeoutConfig contains timeout settings for various operations
type TimeoutConfig struct {
	HTTPRequestTimeout     string `json:"httpRequestTimeout,omitempty"`     // HTTP client timeout (default: "30s")
	MCPInitTimeout         string `json:"mcpInitTimeout,omitempty"`         // MCP client initialization (default: "30s")
	ToolProcessingTimeout  string `json:"toolProcessingTimeout,omitempty"`  // Tool call processing (default: "3m")
	BridgeOperationTimeout string `json:"bridgeOperationTimeout,omitempty"` // Bridge operation timeout (default: "3m")
	PingTimeout            string `json:"pingTimeout,omitempty"`            // Health check ping timeout (default: "5s")
	ResponseProcessing     string `json:"responseProcessing,omitempty"`     // Slack response processing (default: "1m")
}

// RetryConfig contains retry and resilience settings
type RetryConfig struct {
	MaxAttempts          int    `json:"maxAttempts,omitempty"`          // Max retry attempts (default: 3)
	BaseBackoff          string `json:"baseBackoff,omitempty"`          // Base backoff duration (default: "500ms")
	MaxBackoff           string `json:"maxBackoff,omitempty"`           // Maximum backoff duration (default: "5s")
	MCPReconnectAttempts int    `json:"mcpReconnectAttempts,omitempty"` // MCP SSE reconnection attempts (default: 5)
	MCPReconnectBackoff  string `json:"mcpReconnectBackoff,omitempty"`  // MCP reconnection backoff (default: "1s")
}

// ReloadConfig contains settings for periodic application reload
type ReloadConfig struct {
	Enabled  bool   `json:"enabled,omitempty"`  // Enable periodic reload (default: false)
	Interval string `json:"interval,omitempty"` // Reload interval (default: "30m")
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

	// Slack defaults
	if c.Slack.MessageHistory == 0 {
		c.Slack.MessageHistory = 50
	}
	if c.Slack.ThinkingMessage == "" {
		c.Slack.ThinkingMessage = "Thinking..."
	}

	// LLM defaults
	if c.LLM.MaxAgentIterations == 0 {
		c.LLM.MaxAgentIterations = 20
	}

	// Timeout defaults
	if c.Timeouts.HTTPRequestTimeout == "" {
		c.Timeouts.HTTPRequestTimeout = "30s"
	}
	if c.Timeouts.MCPInitTimeout == "" {
		c.Timeouts.MCPInitTimeout = "30s"
	}
	if c.Timeouts.ToolProcessingTimeout == "" {
		c.Timeouts.ToolProcessingTimeout = "3m"
	}
	if c.Timeouts.BridgeOperationTimeout == "" {
		c.Timeouts.BridgeOperationTimeout = "3m"
	}
	if c.Timeouts.PingTimeout == "" {
		c.Timeouts.PingTimeout = "5s"
	}
	if c.Timeouts.ResponseProcessing == "" {
		c.Timeouts.ResponseProcessing = "1m"
	}

	// Retry defaults
	if c.Retry.MaxAttempts == 0 {
		c.Retry.MaxAttempts = 3
	}
	if c.Retry.BaseBackoff == "" {
		c.Retry.BaseBackoff = "500ms"
	}
	if c.Retry.MaxBackoff == "" {
		c.Retry.MaxBackoff = "5s"
	}
	if c.Retry.MCPReconnectAttempts == 0 {
		c.Retry.MCPReconnectAttempts = 5
	}
	if c.Retry.MCPReconnectBackoff == "" {
		c.Retry.MCPReconnectBackoff = "1s"
	}

	// Reload defaults
	if c.Reload.Interval == "" {
		c.Reload.Interval = "30m"
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
