// Package config handles loading and managing application configuration
package config

import (
	"os"
	"strconv"
	"strings"
)

// Constants for provider types
const (
	ProviderOpenAI    = "openai"
	ProviderOllama    = "ollama"
	ProviderAnthropic = "anthropic"
)

// Observability Providers
const (
	ObservabilityProviderSimple   = "simple-otel"
	ObservabilityProviderLangfuse = "langfuse-otel"
	ObservabilityProviderDisabled = "disabled"
)

// Config represents the main application configuration
type Config struct {
	Version        string                     `json:"version"`
	Slack          SlackConfig                `json:"slack"`
	LLM            LLMConfig                  `json:"llm"`
	MCPServers     map[string]MCPServerConfig `json:"mcpServers"`
	RAG            RAGConfig                  `json:"rag,omitempty"`
	Security       SecurityConfig             `json:"security,omitempty"`
	Monitoring     MonitoringConfig           `json:"monitoring,omitempty"`
	Timeouts       TimeoutConfig              `json:"timeouts,omitempty"`
	Retry          RetryConfig                `json:"retry,omitempty"`
	Reload         ReloadConfig               `json:"reload,omitempty"`
	Observability  ObservabilityConfig        `json:"observability,omitempty"`
	UseStdIOClient bool                       `json:"useStdIOClient,omitempty"` // Use terminal client instead of a real slack bot, for local development
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
	HTTPHeaders              map[string]string `json:"httpHeaders,omitempty"`
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
// TODO: Refactor this to use a common interface for all RAG providers, can use environment variables to configure the different providers
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

// ReloadConfig contains signal-based reload configuration
type ReloadConfig struct {
	Enabled  bool   `json:"enabled,omitempty"`  // Enable periodic reload (default: false)
	Interval string `json:"interval,omitempty"` // Reload interval (default: "30m")
}

type ObservabilityConfig struct {
	Enabled        bool   `json:"enabled,omitempty"`
	Provider       string `json:"provider,omitempty"`
	Endpoint       string `json:"endpoint,omitempty"`
	PublicKey      string `json:"publicKey,omitempty"`
	SecretKey      string `json:"secretKey,omitempty"`
	ServiceName    string `json:"serviceName,omitempty"`
	ServiceVersion string `json:"serviceVersion,omitempty"`
}

// SecurityConfig contains security and access control settings
type SecurityConfig struct {
	Enabled          bool     `json:"enabled,omitempty"`          // Enable/disable security (default: false)
	StrictMode       bool     `json:"strictMode,omitempty"`       // Require both user AND channel whitelisting (default: false)
	AllowedUsers     []string `json:"allowedUsers,omitempty"`     // Comma-separated list of allowed user IDs
	AllowedChannels  []string `json:"allowedChannels,omitempty"`  // Comma-separated list of allowed channel IDs
	AdminUsers       []string `json:"adminUsers,omitempty"`       // Comma-separated list of admin user IDs
	RejectionMessage string   `json:"rejectionMessage,omitempty"` // Custom message for unauthorized users
	LogUnauthorized  bool     `json:"logUnauthorized,omitempty"`  // Log unauthorized access attempts (default: true)
}

// ApplyDefaults applies default values to the configuration
func (c *Config) ApplyDefaults() {
	c.applyVersionDefaults()
	c.applyLLMDefaults()
	c.applyRAGDefaults()
	c.applySlackDefaults()
	c.applySecurityDefaults()
	c.applyTimeoutDefaults()
	c.applyRetryDefaults()
	c.applyMonitoringDefaults()
	c.applyMCPDefaults()
	c.applyObservabilityDefaults()
}

// applyVersionDefaults sets default version if not specified
func (c *Config) applyVersionDefaults() {
	if c.Version == "" {
		c.Version = "2.0"
	}
}

// applyLLMDefaults sets default LLM provider and configurations
func (c *Config) applyLLMDefaults() {
	if c.LLM.Provider == "" {
		c.LLM.Provider = ProviderOpenAI
	}

	if c.LLM.MaxAgentIterations <= 0 || c.LLM.MaxAgentIterations > 100 {
		c.LLM.MaxAgentIterations = 20
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
}

// applyRAGDefaults sets default RAG provider and configurations
func (c *Config) applyRAGDefaults() {
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
}

// applySlackDefaults sets default Slack configuration
func (c *Config) applySlackDefaults() {
	if c.Slack.MessageHistory == 0 {
		c.Slack.MessageHistory = 50
	}
	if c.Slack.ThinkingMessage == "" {
		c.Slack.ThinkingMessage = "Thinking..."
	}
}

// applySecurityDefaults sets default security configuration
func (c *Config) applySecurityDefaults() {
	// Security is disabled by default
	if c.Security.Enabled {
		// Set default rejection message
		if c.Security.RejectionMessage == "" {
			c.Security.RejectionMessage = "I'm sorry, but I don't have permission to respond in this context. Please contact the app administrator if you believe this is an error."
		}

		// LogUnauthorized defaults to true when security is enabled
		// Note: Since LogUnauthorized is a bool (not *bool), we can't distinguish between
		// "not set" and "explicitly set to false" in JSON config. However, environment
		// variables (applied later) can override this default.
		if !c.Security.LogUnauthorized {
			c.Security.LogUnauthorized = true
		}
	}
}

// applyTimeoutDefaults sets default timeout values
func (c *Config) applyTimeoutDefaults() {
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
}

// applyRetryDefaults sets default retry configuration
func (c *Config) applyRetryDefaults() {
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
}

// applyMonitoringDefaults sets default monitoring configuration
func (c *Config) applyMonitoringDefaults() {
	c.Monitoring.Enabled = true // Default to enabled
	if c.Monitoring.MetricsPort == 0 {
		c.Monitoring.MetricsPort = 8080
	}
	if c.Monitoring.LoggingLevel == "" {
		c.Monitoring.LoggingLevel = "info"
	}
}

// applyObservabilityDefaults sets default observability configuration
// Defaults are applied regardless of Enabled; runtime checks decide whether to initialize tracing.
func (c *Config) applyObservabilityDefaults() {

	// Default provider to simple-otel when enabled
	if c.Observability.Provider == "" {
		c.Observability.Provider = ObservabilityProviderSimple
	}

	// Default service name
	if c.Observability.ServiceName == "" {
		c.Observability.ServiceName = "slack-mcp-client"
	}

	// Default service version
	if c.Observability.ServiceVersion == "" {
		c.Observability.ServiceVersion = "1.0.0"
	}
}

// applyMCPDefaults initializes MCP servers map if nil
func (c *Config) applyMCPDefaults() {
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
	// Observability overrides
	if enabled := os.Getenv("OBSERVABILITY_ENABLED"); enabled != "" {
		if val, err := strconv.ParseBool(enabled); err == nil {
			c.Observability.Enabled = val
		}
	}

	if provider := os.Getenv("OBSERVABILITY_PROVIDER"); provider != "" {
		c.Observability.Provider = provider
	}
	if endpoint := os.Getenv("OBSERVABILITY_ENDPOINT"); endpoint != "" {
		c.Observability.Endpoint = endpoint
	}
	if publicKey := os.Getenv("LANGFUSE_PUBLIC_KEY"); publicKey != "" {
		c.Observability.PublicKey = publicKey
	}
	if secretKey := os.Getenv("LANGFUSE_SECRET_KEY"); secretKey != "" {
		c.Observability.SecretKey = secretKey
	}
	if serviceName := os.Getenv("OBSERVABILITY_SERVICE_NAME"); serviceName != "" {
		c.Observability.ServiceName = serviceName
	}
	if serviceVersion := os.Getenv("OBSERVABILITY_SERVICE_VERSION"); serviceVersion != "" {
		c.Observability.ServiceVersion = serviceVersion
	}

	// Security configuration overrides
	if enabled := os.Getenv("SECURITY_ENABLED"); enabled != "" {
		if val, err := strconv.ParseBool(enabled); err == nil {
			c.Security.Enabled = val
		}
	}

	if strictMode := os.Getenv("SECURITY_STRICT_MODE"); strictMode != "" {
		if val, err := strconv.ParseBool(strictMode); err == nil {
			c.Security.StrictMode = val
		}
	}

	// Track if LogUnauthorized was explicitly set via environment variable
	logUnauthorizedEnvSet := false
	if logUnauthorized := os.Getenv("SECURITY_LOG_UNAUTHORIZED"); logUnauthorized != "" {
		if val, err := strconv.ParseBool(logUnauthorized); err == nil {
			c.Security.LogUnauthorized = val
			logUnauthorizedEnvSet = true
		}
	}

	if allowedUsers := os.Getenv("SECURITY_ALLOWED_USERS"); allowedUsers != "" {
		users := strings.Split(allowedUsers, ",")
		filteredUsers := []string{}
		for _, user := range users {
			trimmed := strings.TrimSpace(user)
			if trimmed != "" {
				filteredUsers = append(filteredUsers, trimmed)
			}
		}
		c.Security.AllowedUsers = filteredUsers
	}

	if allowedChannels := os.Getenv("SECURITY_ALLOWED_CHANNELS"); allowedChannels != "" {
		channels := strings.Split(allowedChannels, ",")
		filteredChannels := []string{}
		for _, channel := range channels {
			trimmed := strings.TrimSpace(channel)
			if trimmed != "" {
				filteredChannels = append(filteredChannels, trimmed)
			}
		}
		c.Security.AllowedChannels = filteredChannels
	}

	if adminUsers := os.Getenv("SECURITY_ADMIN_USERS"); adminUsers != "" {
		users := strings.Split(adminUsers, ",")
		filteredAdmins := []string{}
		for _, user := range users {
			trimmed := strings.TrimSpace(user)
			if trimmed != "" {
				filteredAdmins = append(filteredAdmins, trimmed)
			}
		}
		c.Security.AdminUsers = filteredAdmins
	}

	if rejectionMessage := os.Getenv("SECURITY_REJECTION_MESSAGE"); rejectionMessage != "" {
		c.Security.RejectionMessage = rejectionMessage
	}

	// Apply security defaults after environment variables have been processed
	// This ensures defaults are set when security is enabled via environment variables
	// without JSON configuration. We don't call applySecurityDefaults() here to keep
	// the logic explicit and avoid confusing re-application of defaults.
	if c.Security.Enabled {
		// Set default rejection message if not provided
		if c.Security.RejectionMessage == "" {
			c.Security.RejectionMessage = "I'm sorry, but I don't have permission to respond in this context. Please contact the app administrator if you believe this is an error."
		}

		// Set LogUnauthorized to true by default if not explicitly set via env var
		// Only apply this default if the env var wasn't provided, to respect explicit false values
		if !logUnauthorizedEnvSet && !c.Security.LogUnauthorized {
			c.Security.LogUnauthorized = true
		}
	}
}

// SecurityResult represents the result of a security check
type SecurityResult struct {
	Allowed bool   // Whether access is granted
	Reason  string // Reason for the decision (for logging)
}

// ValidateAccess performs security validation based on the current configuration
// Returns SecurityResult indicating whether access should be granted and the reason
func (c *Config) ValidateAccess(userID, channelID string) SecurityResult {
	// If security is disabled, allow all access
	if !c.Security.Enabled {
		return SecurityResult{
			Allowed: true,
			Reason:  "Security disabled",
		}
	}

	// Check if user is an admin (admins always have access regardless of channel restrictions)
	isAdmin := c.isAdminUser(userID)
	if isAdmin {
		return SecurityResult{
			Allowed: true,
			Reason:  "Admin user access",
		}
	}

	// Check user and channel whitelists
	isUserAllowed := c.isUserAllowed(userID)
	isChannelAllowed := c.isChannelAllowed(channelID)

	// Apply access control based on strict mode
	if c.Security.StrictMode {
		// Strict mode: both user AND channel must be whitelisted
		if isUserAllowed && isChannelAllowed {
			return SecurityResult{
				Allowed: true,
				Reason:  "User and channel both whitelisted (strict mode)",
			}
		}
		if !isUserAllowed && !isChannelAllowed {
			return SecurityResult{
				Allowed: false,
				Reason:  "Neither user nor channel whitelisted (strict mode)",
			}
		}
		if !isUserAllowed {
			return SecurityResult{
				Allowed: false,
				Reason:  "User not whitelisted (strict mode)",
			}
		}
		return SecurityResult{
			Allowed: false,
			Reason:  "Channel not whitelisted (strict mode)",
		}
	} else {
		// Flexible mode: user OR channel must be whitelisted
		if isUserAllowed || isChannelAllowed {
			if isUserAllowed && isChannelAllowed {
				return SecurityResult{
					Allowed: true,
					Reason:  "User and channel both whitelisted",
				}
			} else if isUserAllowed {
				return SecurityResult{
					Allowed: true,
					Reason:  "User whitelisted",
				}
			} else {
				return SecurityResult{
					Allowed: true,
					Reason:  "Channel whitelisted",
				}
			}
		}
		return SecurityResult{
			Allowed: false,
			Reason:  "Neither user nor channel whitelisted",
		}
	}
}

// isUserAllowed checks if a user ID is in the allowed users list
func (c *Config) isUserAllowed(userID string) bool {
	for _, allowedUser := range c.Security.AllowedUsers {
		if allowedUser == userID {
			return true
		}
	}
	return false
}

// isChannelAllowed checks if a channel ID is in the allowed channels list
func (c *Config) isChannelAllowed(channelID string) bool {
	for _, allowedChannel := range c.Security.AllowedChannels {
		if allowedChannel == channelID {
			return true
		}
	}
	return false
}

// isAdminUser checks if a user ID is in the admin users list
func (c *Config) isAdminUser(userID string) bool {
	for _, adminUser := range c.Security.AdminUsers {
		if adminUser == userID {
			return true
		}
	}
	return false
}
