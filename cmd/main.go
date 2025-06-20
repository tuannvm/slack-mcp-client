// Package main implements the Slack MCP client application
// It provides a bridge between Slack and MCP servers
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/tuannvm/slack-mcp-client/internal/common"
	customErrors "github.com/tuannvm/slack-mcp-client/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/mcp" // Use the internal mcp package

	// internal/mcp is no longer needed here - This comment is now incorrect

	slackbot "github.com/tuannvm/slack-mcp-client/internal/slack"
)

// ToolInfo definition is moved to internal/common/types.go

var (
	// Define command-line flags
	configFile = flag.String("config", "mcp-servers.json", "Path to the MCP server configuration JSON file")
	debug      = flag.Bool("debug", false, "Enable debug logging")
	mcpDebug   = flag.Bool("mcpdebug", false, "Enable debug logging for MCP clients")
)

func main() {
	flag.Parse()

	// Set LLM_PROVIDER=openai by default if not already set
	if os.Getenv("LLM_PROVIDER") == "" {
		if err := os.Setenv("LLM_PROVIDER", "openai"); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to set LLM_PROVIDER environment variable: %v\n", err)
		}
	}

	// Setup logging with structured logger
	logger := setupLogging()
	logger.Info("Starting Slack MCP Client (debug=%v)", *debug)

	// Load and prepare configuration
	cfg := loadAndPrepareConfig(logger)

	// Initialize MCP clients and discover tools
	mcpClients, discoveredTools := initializeMCPClients(logger, cfg)

	// Initialize and run Slack client
	startSlackClient(logger, mcpClients, discoveredTools, cfg)
}

// setupLogging initializes the logging system
func setupLogging() *logging.Logger {
	// Determine log level from debug flag or existing environment variable
	logLevel := logging.LevelInfo
	logLevelName := "info"

	// Check if LOG_LEVEL is already set in the environment
	envLogLevel := os.Getenv("LOG_LEVEL")
	if envLogLevel != "" {
		// Use the environment variable if it's set
		logLevel = logging.ParseLevel(envLogLevel)
		logLevelName = envLogLevel
	} else if *debug {
		// Otherwise use the debug flag
		logLevel = logging.LevelDebug
		logLevelName = "debug"

		// Set LOG_LEVEL environment variable for other components
		if err := os.Setenv("LOG_LEVEL", logLevelName); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to set LOG_LEVEL environment variable: %v\n", err)
		}
	}

	logger := logging.New("slack-mcp-client", logLevel)
	logger.Info("Log level set to: %s", logLevelName)

	// Setup library debugging if requested
	if *mcpDebug {
		if err := os.Setenv("MCP_DEBUG", "true"); err != nil {
			logger.Error("Failed to set MCP_DEBUG environment variable: %v", err)
		}
		logger.Info("MCP_DEBUG environment variable set to true")
	}

	return logger
}

// loadAndPrepareConfig loads the configuration and applies any overrides
func loadAndPrepareConfig(logger *logging.Logger) *config.Config {
	// Load configuration
	cfg, err := config.LoadConfig(*configFile, logger)
	if err != nil {
		logger.Fatal("Failed to load configuration: %v", err)
	}

	// Validate LLM provider - Check against known providers from the factory
	// This validation might be better placed after registry initialization if needed
	// For now, just log the configured provider.
	logger.Info("LLM Provider specified in config: %s", cfg.LLMProvider)

	// Apply command-line overrides AFTER loading config
	if err := applyCommandLineOverrides(logger, cfg); err != nil {
		logger.Fatal("Error applying command-line flags: %v", err)
	}

	// Log configuration information
	logger.Info("Configuration loaded. Slack Bot Token Present: %t, Slack App Token Present: %t",
		cfg.SlackBotToken != "", cfg.SlackAppToken != "")
	logLLMSettings(logger, cfg) // Log LLM settings
	logger.Info("MCP Servers Configured (in file): %d", len(cfg.Servers))

	return cfg
}

// initializeMCPClients initializes all MCP clients and discovers available tools
// Use mcp.Client from the internal mcp package
func initializeMCPClients(logger *logging.Logger, cfg *config.Config) (map[string]*mcp.Client, map[string]common.ToolInfo) {
	// Initialize MCP Clients and Discover Tools Sequentially
	mcpClients := make(map[string]*mcp.Client)
	allDiscoveredTools := make(map[string]common.ToolInfo) // Map: toolName -> common.ToolInfo
	failedServers := []string{}
	initializedClientCount := 0

	logger.Info("--- Starting MCP Client Initialization and Tool Discovery --- ")
	for serverName, serverConf := range cfg.Servers {
		processSingleMCPServer(
			logger,
			serverName,
			serverConf,
			mcpClients,
			allDiscoveredTools,
			&failedServers,
			&initializedClientCount,
		)
	}

	logger.Info("--- Finished MCP Client Initialization and Tool Discovery --- ")

	// Log summary
	// Use the imported function from the mcp package
	logger.Info("Successfully initialized %d MCP clients: %v", initializedClientCount, mcp.GetClientMapKeys(mcpClients))
	if len(failedServers) > 0 {
		logger.Info("Failed to fully initialize/get tools from %d servers: %v", len(failedServers), failedServers)
	}
	logger.Info("Total unique discovered tools across all initialized servers: %d", len(allDiscoveredTools))

	// Log a warning if no clients were initialized, but continue running
	if initializedClientCount == 0 {
		logger.Warn("No MCP clients could be successfully initialized. Application will run with LLM capabilities only.")
	}

	return mcpClients, allDiscoveredTools
}

// processSingleMCPServer processes a single MCP server configuration
func processSingleMCPServer(
	logger *logging.Logger,
	serverName string,
	serverConf config.ServerConfig,
	mcpClients map[string]*mcp.Client, // Use mcp.Client
	discoveredTools map[string]common.ToolInfo,
	failedServers *[]string,
	initializedClientCount *int,
) {
	logger.Info("Processing server: '%s'", serverName)

	// Skip disabled servers
	if serverConf.Disabled {
		logger.Info("  Skipping disabled server '%s'", serverName)
		return
	}

	// Create a component-specific logger for this server
	serverLogger := logger.WithName(serverName)

	// Create dedicated logger for this MCP client
	mcpLoggerStd := log.New(os.Stdout, fmt.Sprintf("mcp-%s: ", strings.ToLower(serverName)), log.LstdFlags)

	// Create client instance (assuming HTTP/SSE based on simplified config)
	// Use mcp.NewClient from the internal package
	mcpClient, err := createMCPClient(serverLogger, serverConf, mcpLoggerStd)
	if err != nil {
		*failedServers = append(*failedServers, serverName+fmt.Sprintf("(create: %s)", err))
		return
	}

	serverLogger.Info("Successfully created MCP client instance")

	// Only close the client if initialization fails
	// We'll keep successful clients open for the lifetime of the application
	closeClientOnFailure := func() {
		if mcpClient != nil && mcpClients[serverName] == nil { // Only close if not stored in mcpClients
			serverLogger.Info("Closing unused MCP client")
			if err := mcpClient.Close(); err != nil {
				serverLogger.ErrorKV("Failed to close MCP client", "error", err)
			}
		}
	}
	defer closeClientOnFailure()

	// Initialize client
	// Use mcp.Client from the internal mcp package (via mcpClient variable)
	if err := initializeMCPClientInstance(serverLogger, mcpClient, serverConf.InitializeTimeoutSeconds); err != nil {
		*failedServers = append(*failedServers, serverName+"(initialize failed)")
		return
	}

	// Store successfully initialized client
	serverLogger.Info("Adding MCP client for '%s' to active client map", serverName)
	mcpClients[serverName] = mcpClient
	*initializedClientCount++

	// Special debugging for Kubernetes server
	if serverName == "kubernetes" {
		serverLogger.Info("Successfully initialized Kubernetes MCP client")
	}

	// Discover tools
	// Use mcp.Client from the internal mcp package (via mcpClient variable)
	serverLogger.Info("Discovering tools (timeout: 20s)...")
	discoveryCtx, discoveryCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer discoveryCancel()

	listResult, toolsErr := mcpClient.GetAvailableTools(discoveryCtx)

	if toolsErr != nil {
		serverLogger.Warn("Failed to retrieve tools: %v", toolsErr)
		*failedServers = append(*failedServers, serverName+"(tool discovery failed)")
		return
	}

	if listResult == nil || len(listResult.Tools) == 0 {
		serverLogger.Warn("Server initialized but returned 0 tools")
		return
	}

	serverLogger.Info("Discovered %d tools", len(listResult.Tools))
	for _, toolDef := range listResult.Tools {
		toolName := toolDef.Name
		if _, exists := discoveredTools[toolName]; !exists {
			var inputSchemaMap map[string]interface{}
			// Marshal the ToolInputSchema struct to JSON bytes
			schemaBytes, err := json.Marshal(toolDef.InputSchema)
			if err != nil {
				serverLogger.Error("    Failed to marshal input schema struct for tool '%s': %v", toolName, err)
				inputSchemaMap = make(map[string]interface{}) // Use empty map on error
			} else {
				// Unmarshal the JSON bytes into the map
				if err := json.Unmarshal(schemaBytes, &inputSchemaMap); err != nil {
					serverLogger.Error("    Failed to unmarshal input schema JSON for tool '%s': %v", toolName, err)
					inputSchemaMap = make(map[string]interface{}) // Use empty map on error
				}
			}

			// Use common.ToolInfo
			discoveredTools[toolName] = common.ToolInfo{
				ServerName:  serverName,
				Description: toolDef.Description,
				InputSchema: inputSchemaMap,
			}
			if *mcpDebug {
				serverLogger.Debug("Stored tool: '%s' (Desc: %s)", toolName, toolDef.Description)
				if *debug {
					// Only log the full schema if debug mode is enabled
					schemaJSON, _ := json.MarshalIndent(inputSchemaMap, "", "  ")
					serverLogger.Debug("Tool schema: %s", string(schemaJSON))
				}
			}
		} else {
			existingInfo := discoveredTools[toolName]
			serverLogger.Warn("Tool '%s' is available from multiple servers ('%s' and '%s'). Using the first one found ('%s').",
				toolName, existingInfo.ServerName, serverName, existingInfo.ServerName)
		}
	}
}

// createMCPClient creates an MCP client based on configuration
// Use mcp.Client and mcp.NewClient from the internal mcp package
func createMCPClient(logger *logging.Logger, serverConf config.ServerConfig, _ *log.Logger) (*mcp.Client, error) {
	// Check if this is a URL-based (HTTP/SSE) configuration
	if serverConf.URL != "" {
		// Assume "sse" mode by default for HTTP-based connections
		mode := serverConf.Mode
		if mode == "" {
			mode = "sse" // Default to SSE if not specified
		}
		logger.InfoKV("Creating MCP client", "mode", mode, "address", serverConf.URL)

		// Use the imported mcp.NewClient from internal/mcp/client.go with structured logger
		mcpClient, createErr := mcp.NewClient(mode, serverConf.URL, nil, nil, logger)
		if createErr != nil {
			logger.Error("Failed to create MCP client for URL %s: %v", serverConf.URL, createErr)
			// Create a domain-specific error with additional context
			domainErr := customErrors.WrapMCPError(createErr, "client_creation_failed",
				fmt.Sprintf("Failed to create MCP client for URL '%s'", serverConf.URL))

			// Add additional context data
			domainErr = domainErr.WithData("mode", mode)
			domainErr = domainErr.WithData("url", serverConf.URL)
			return nil, domainErr
		}
		return mcpClient, nil
	}

	// Check if this is a command-based (stdio) configuration
	if serverConf.Command != "" {
		mode := "stdio"
		logger.InfoKV("Creating MCP client", "mode", mode, "command", serverConf.Command, "args", serverConf.Args)

		// Process environment variables
		env := make(map[string]string)
		for k, v := range serverConf.Env {
			// Handle variable substitution from environment
			if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
				envVar := strings.TrimSuffix(strings.TrimPrefix(v, "${"), "}")
				if envValue := os.Getenv(envVar); envValue != "" {
					env[k] = envValue
					logger.Debug("Substituted environment variable %s for MCP server", envVar)
				} else {
					logger.Warn("Environment variable %s not found for substitution", envVar)
					env[k] = "" // Set empty value
				}
			} else {
				env[k] = v
			}
		}

		// Create the MCP client
		logger.DebugKV("Executing command", "command", serverConf.Command, "args", serverConf.Args, "env", env)
		mcpClient, createErr := mcp.NewClient(mode, serverConf.Command, serverConf.Args, env, logger)
		if createErr != nil {
			logger.Error("Failed to create MCP client: %v", createErr)
			// Create a domain-specific error with additional context
			domainErr := customErrors.WrapMCPError(createErr, "client_creation_failed",
				fmt.Sprintf("Failed to create MCP client for command '%s'", serverConf.Command))

			// Add additional context data
			domainErr = domainErr.WithData("mode", mode)
			domainErr = domainErr.WithData("command", serverConf.Command)
			return nil, domainErr
		}
		return mcpClient, nil
	}

	// Neither URL nor Command specified
	logger.Error("Skipping server: Neither 'url' nor 'command' specified in config")
	return nil, customErrors.NewMCPError("invalid_config", "Missing both URL and command in server configuration")
}

// initializeMCPClientInstance initializes an MCP client with proper timeout
// Use mcp.Client from the internal mcp package
func initializeMCPClientInstance(logger *logging.Logger, client *mcp.Client, timeoutSeconds *int) error {
	initTimeout := 5 // Default timeout
	if timeoutSeconds != nil {
		initTimeout = *timeoutSeconds
	}
	logger.Info("Attempting to initialize MCP client (timeout: %d)...", initTimeout)
	// Create a context with timeout for initialization
	initCtx, initCancel := context.WithTimeout(context.Background(), time.Duration(initTimeout)*time.Second)
	defer initCancel()

	// Try to initialize the client
	initErr := client.Initialize(initCtx)
	if initErr != nil {
		// Log detailed error information
		logger.Error("Failed to initialize MCP client: %v", initErr)

		// Create a domain-specific error with additional context
		domainErr := customErrors.WrapMCPError(initErr, "initialization_failed", "Failed to initialize MCP client")

		// Check for specific error conditions and add more context
		if strings.Contains(initErr.Error(), "context deadline exceeded") {
			logger.Error("Initialization timed out. The MCP server may be slow to start or not responding.")
			logger.Error("Try increasing the timeout or check if the NPM package is installed correctly.")
			domainErr = domainErr.WithData("timeout_exceeded", true)
			domainErr = domainErr.WithData("suggestion", "Increase timeout or check NPM package installation")
		} else if strings.Contains(initErr.Error(), "file already closed") {
			logger.Error("The MCP server process exited prematurely. Check command and arguments.")
			domainErr = domainErr.WithData("process_exited", true)
			domainErr = domainErr.WithData("suggestion", "Check command and arguments")
		}

		logger.Warn("Client will not be used for tool discovery or execution")
		return domainErr
	}

	logger.Info("MCP client successfully initialized")
	return nil
}

// applyCommandLineOverrides applies command-line flags directly to the loaded config
func applyCommandLineOverrides(logger *logging.Logger, cfg *config.Config) error {
	// Command-line overrides for LLM settings are not applied directly.
	logger.Debug("Command-line overrides for LLM settings are not applied directly to the config map.")
	return nil // No errors
}

// logLLMSettings logs the current LLM configuration
func logLLMSettings(logger *logging.Logger, cfg *config.Config) {
	// Log the primary provider being used
	logger.Info("Primary LLM Provider Selected: %s", cfg.LLMProvider)

	// Check if the provider was set via environment variable
	llmProviderEnv := os.Getenv("LLM_PROVIDER")
	if llmProviderEnv != "" {
		logger.Info("LLM Provider set from environment variable: %s", llmProviderEnv)
	}

	// Log details for the selected provider if available
	if providerConfig, ok := cfg.LLMProviders[cfg.LLMProvider]; ok {
		// Be careful logging sensitive info like API keys
		loggableConfig := make(map[string]interface{})
		for k, v := range providerConfig {
			if k != "api_key" && k != "apiKey" { // Avoid logging keys
				loggableConfig[k] = v
			}
		}

		// Log that we're using LangChain as the gateway
		logger.Info("Using LangChain as gateway for provider: %s", cfg.LLMProvider)
		logger.Info("Configuration for %s: %v", cfg.LLMProvider, loggableConfig)
	} else {
		logger.Warn("No specific configuration found for selected provider: %s", cfg.LLMProvider)
	}
}

// startSlackClient starts the Slack client and handles shutdown
// Use mcp.Client from the internal mcp package
func startSlackClient(logger *logging.Logger, mcpClients map[string]*mcp.Client, discoveredTools map[string]common.ToolInfo, cfg *config.Config) {
	logger.Info("Starting Slack client...")
	var err error

	var userFrontend slackbot.UserFrontend
	if cfg.UseStdIOClient != nil && *cfg.UseStdIOClient {
		userFrontend = slackbot.NewStdioClient(logger)
	} else {
		userFrontend, err = slackbot.GetSlackClient(
			cfg.SlackBotToken,
			cfg.SlackAppToken,
			logger,
		)
		if err != nil {
			logger.Fatal("Failed to initialize Slack client: %v", err)
		}
	}

	// Use the structured logger for the Slack client
	client, err := slackbot.NewClient(
		userFrontend,
		logger,          // Pass the structured logger
		mcpClients,      // Pass the map of initialized clients
		discoveredTools, // Pass the map of tool information
		cfg,             // Pass the whole config object
	)
	if err != nil {
		logger.Fatal("Failed to initialize Slack client: %v", err)
	}

	// Start listening for Slack events in a separate goroutine
	go func() {
		if err := client.Run(); err != nil {
			logger.Fatal("Slack client error: %v", err)
		}
	}()

	logger.Info("Slack MCP Client is now running. Press Ctrl+C to exit.")

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	logger.Info("Received signal %v, shutting down...", sig)

	// Gracefully close all MCP clients
	logger.Info("Closing all MCP clients...")
	for name, client := range mcpClients {
		if client != nil {
			logger.InfoKV("Closing MCP client", "name", name)
			if err := client.Close(); err != nil {
				logger.ErrorKV("Failed to close MCP client", "name", name, "error", err)
			}
		}
	}
}
