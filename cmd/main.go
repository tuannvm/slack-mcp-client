// Package main implements the Slack MCP client application
// It provides a bridge between Slack and MCP servers
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tuannvm/slack-mcp-client/internal/app"
	customErrors "github.com/tuannvm/slack-mcp-client/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
	"github.com/tuannvm/slack-mcp-client/internal/monitoring"
	"github.com/tuannvm/slack-mcp-client/internal/rag"

	slackbot "github.com/tuannvm/slack-mcp-client/internal/slack"
)

// ToolInfo definition is moved to internal/common/types.go

var (
	// Define command-line flags
	configFile  = flag.String("config", "config.json", "Path to the configuration file (supports both config.json and legacy mcp-servers.json formats)")
	debug       = flag.Bool("debug", false, "Enable debug logging")
	mcpDebug    = flag.Bool("mcpdebug", false, "Enable debug logging for MCP clients")
	metricsPort = flag.String("metrics-port", "8080", "Port for metrics endpoint (default: 8080)")
	// Configuration validation flag
	configValidate = flag.Bool("config-validate", false, "Validate configuration file and exit")
	// Configuration migration flag
	migrateConfig = flag.Bool("migrate-config", false, "Migrate legacy configuration to new format and exit")

	// RAG-related flags
	ragIngest          = flag.String("rag-ingest", "", "Ingest PDF files from directory and exit")
	ragSearch          = flag.String("rag-search", "", "Search RAG database and exit")
	ragDatabase        = flag.String("rag-db", "./knowledge.json", "Path to RAG database file")
	ragProvider        = flag.String("rag-provider", "", "RAG provider to use (simple, openai)")
	ragInit            = flag.Bool("rag-init", false, "Initialize vector store and exit")
	ragList            = flag.Bool("rag-list", false, "List files in vector store and exit")
	ragDelete          = flag.String("rag-delete", "", "Delete files from vector store (comma-separated IDs) and exit")
	ragStats           = flag.Bool("rag-stats", false, "Show RAG statistics and exit")
	ragAssistantName   = flag.String("rag-assistant-name", "", "Name for the OpenAI assistant (for init)")
	ragVectorStoreName = flag.String("rag-vector-store-name", "", "Name for the vector store (for init)")
)

func init() {
	monitoring.RegisterMetrics()
}

func main() {
	flag.Parse()

	// Validate configuration and exit if requested
	if *configValidate {
		// Load and validate config (runtime validation and strict JSON parsing)
		if _, err := config.LoadConfig(*configFile, nil); err != nil {
			fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Configuration is valid")
		os.Exit(0)
	}

	// Migrate configuration and exit if requested
	if *migrateConfig {
		handleConfigMigration(*configFile)
		return
	}

	// Handle RAG utility commands first (these exit after completion)
	if *ragInit {
		handleRAGInit()
		return
	}

	if *ragIngest != "" {
		handleRAGIngest(*ragIngest)
		return
	}

	if *ragSearch != "" {
		handleRAGSearch(*ragSearch)
		return
	}

	if *ragList {
		handleRAGList()
		return
	}

	if *ragDelete != "" {
		handleRAGDelete(*ragDelete)
		return
	}

	if *ragStats {
		handleRAGStats()
		return
	}

	// Set LLM_PROVIDER=openai by default if not already set
	if os.Getenv("LLM_PROVIDER") == "" {
		if err := os.Setenv("LLM_PROVIDER", "openai"); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to set LLM_PROVIDER environment variable: %v\n", err)
		}
	}

	// Setup logging with structured logger
	logger := setupLogging()
	logger.Info("Starting Slack MCP Client (debug=%v)", *debug)

	// Start metrics server
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		logger.Info("Starting metrics server on port %s", *metricsPort)
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", *metricsPort), nil))
	}()

	// Run application with reload capability
	if err := app.RunWithReload(logger, *configFile, runMainApplication); err != nil {
		logger.Fatal("Application failed to start: %v", err)
		os.Exit(1)
	}
}

// runMainApplication contains the core application logic that can be reloaded
func runMainApplication(ctx context.Context, logger *logging.Logger) error {
	// Load and prepare configuration
	cfg := loadAndPrepareConfig(logger)

	// Initialize MCP clients and discover tools
	mcpClients, discoveredTools := initializeMCPClients(logger, cfg)

	// Initialize and run Slack client
	startSlackClient(ctx, logger, mcpClients, discoveredTools, cfg)

	return nil
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
	logger.Info("LLM Provider specified in config: %s", cfg.LLM.Provider)

	// Apply command-line overrides AFTER loading config
	if err := applyCommandLineOverrides(logger, cfg); err != nil {
		logger.Fatal("Error applying command-line flags: %v", err)
	}

	// Log configuration information
	logger.Info("Configuration loaded. Slack Bot Token Present: %t, Slack App Token Present: %t",
		cfg.Slack.BotToken != "", cfg.Slack.AppToken != "")
	logLLMSettings(logger, cfg) // Log LLM settings
	logger.Info("MCP Servers Configured (in file): %d", len(cfg.MCPServers))

	return cfg
}

// initializeMCPClients initializes all MCP clients and discovers available tools
// Use mcp.Client from the internal mcp package
func initializeMCPClients(logger *logging.Logger, cfg *config.Config) (map[string]*mcp.Client, map[string]mcp.ToolInfo) {
	// Initialize MCP Clients and Discover Tools Sequentially
	mcpClients := make(map[string]*mcp.Client)
	allDiscoveredTools := make(map[string]mcp.ToolInfo) // Map: toolName -> common.ToolInfo
	failedServers := []string{}
	initializedClientCount := 0

	logger.Info("--- Starting MCP Client Initialization and Tool Discovery --- ")
	for serverName, serverConf := range cfg.MCPServers {
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
	serverConf config.MCPServerConfig,
	mcpClients map[string]*mcp.Client, // Use mcp.Client
	discoveredTools map[string]mcp.ToolInfo,
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
	mcpClient, err := createMCPClient(serverLogger, serverConf, serverName, mcpLoggerStd)
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

	blockListMap := map[string]bool{}
	allowListMap := map[string]bool{}
	for _, toolName := range serverConf.Tools.BlockList {
		blockListMap[toolName] = true
	}
	for _, toolName := range serverConf.Tools.AllowList {
		allowListMap[toolName] = true
	}

	serverLogger.Info("Discovered %d tools", len(listResult.Tools))
	for _, toolDef := range listResult.Tools {
		if _, exists := blockListMap[toolDef.Name]; exists {
			serverLogger.Debug("    Tool '%s' is in block list, skipping", toolDef.Name)
			continue
		}
		if len(allowListMap) > 0 && !allowListMap[toolDef.Name] {
			serverLogger.Debug("    Tool '%s' is not in allow list, skipping", toolDef.Name)
			continue
		}
		toolName := fmt.Sprintf("%s_%s", serverName, toolDef.Name)
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
			discoveredTools[toolName] = mcp.ToolInfo{
				ServerName:      serverName,
				ToolName:        toolName,
				ToolDescription: toolDef.Description,
				InputSchema:     inputSchemaMap,
				Client:          mcpClient,
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

// resolveHTTPHeaders resolves environment variables in HTTP headers
func resolveHTTPHeaders(headers map[string]string, logger *logging.Logger) map[string]string {
	resolvedHeaders := make(map[string]string)
	for k, v := range headers {
		// Handle variable substitution from environment
		if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
			envVar := strings.TrimSuffix(strings.TrimPrefix(v, "${"), "}")
			if envValue := os.Getenv(envVar); envValue != "" {
				resolvedHeaders[k] = envValue
				logger.Debug("Substituted environment variable %s for HTTP header", envVar)
			} else {
				logger.Warn("Environment variable %s not found for HTTP header substitution", envVar)
				resolvedHeaders[k] = "" // Set empty value
			}
		} else {
			resolvedHeaders[k] = v
		}
	}
	return resolvedHeaders
}

// createMCPClient creates an MCP client based on configuration
// Use mcp.Client and mcp.NewClient from the internal mcp package
func createMCPClient(logger *logging.Logger, serverConf config.MCPServerConfig, serverName string, _ *log.Logger) (*mcp.Client, error) {
	// Check if this is a URL-based (HTTP/SSE) configuration
	if serverConf.URL != "" {
		// Assume "sse" transport by default for HTTP-based connections
		transport := serverConf.Transport
		if transport == "" {
			transport = "sse" // Default to SSE if not specified
		}
		logger.InfoKV("Creating MCP client", "transport", transport, "address", serverConf.URL)

		// Resolve HTTPHeaders environment variables for URL-based configurations
		resolvedHeaders := resolveHTTPHeaders(serverConf.HTTPHeaders, logger)

		// Use the imported mcp.NewClient from internal/mcp/client.go with structured logger
		mcpClient, createErr := mcp.NewClient(transport, serverConf.URL, serverName, nil, nil, resolvedHeaders, logger)
		if createErr != nil {
			logger.Error("Failed to create MCP client for URL %s: %v", serverConf.URL, createErr)
			// Create a domain-specific error with additional context
			domainErr := customErrors.WrapMCPError(createErr, "client_creation_failed",
				fmt.Sprintf("Failed to create MCP client for URL '%s'", serverConf.URL))

			// Add additional context data
			domainErr = domainErr.WithData("transport", transport)
			domainErr = domainErr.WithData("url", serverConf.URL)
			return nil, domainErr
		}
		return mcpClient, nil
	}

	// Check if this is a command-based (stdio) configuration
	if serverConf.Command != "" {
		transport := "stdio"
		logger.InfoKV("Creating MCP client", "transport", transport, "command", serverConf.Command, "args", serverConf.Args)

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

		// Resolve HTTPHeaders environment variables
		resolvedHeaders := resolveHTTPHeaders(serverConf.HTTPHeaders, logger)

		// Create the MCP client
		logger.DebugKV("Executing command", "command", serverConf.Command, "args", serverConf.Args, "env", env, "headers", resolvedHeaders)
		mcpClient, createErr := mcp.NewClient(transport, serverConf.Command, serverName, serverConf.Args, env, resolvedHeaders, logger)
		if createErr != nil {
			logger.Error("Failed to create MCP client: %v", createErr)
			// Create a domain-specific error with additional context
			domainErr := customErrors.WrapMCPError(createErr, "client_creation_failed",
				fmt.Sprintf("Failed to create MCP client for command '%s'", serverConf.Command))

			// Add additional context data
			domainErr = domainErr.WithData("transport", transport)
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
	logger.Info("Primary LLM Provider Selected: %s", cfg.LLM.Provider)

	// Check if the provider was set via environment variable
	llmProviderEnv := os.Getenv("LLM_PROVIDER")
	if llmProviderEnv != "" {
		logger.Info("LLM Provider set from environment variable: %s", llmProviderEnv)
	}

	// Log details for the selected provider if available
	if providerConfig, ok := cfg.LLM.Providers[cfg.LLM.Provider]; ok {
		// Log that we're using LangChain as the gateway
		logger.Info("Using LangChain as gateway for provider: %s", cfg.LLM.Provider)
		logger.Info("Configuration for %s: model=%s, temperature=%f", cfg.LLM.Provider, providerConfig.Model, providerConfig.Temperature)
	} else {
		logger.Warn("No specific configuration found for selected provider: %s", cfg.LLM.Provider)
	}
}

// startSlackClient starts the Slack client and handles shutdown
// Use mcp.Client from the internal mcp package
func startSlackClient(ctx context.Context, logger *logging.Logger, mcpClients map[string]*mcp.Client, discoveredTools map[string]mcp.ToolInfo, cfg *config.Config) {
	logger.Info("Starting Slack client...")

	// Initialize RAG client if enabled and add tools to discoveredTools
	// The actual RAG client will be created in the Slack client where it gets added to the bridge
	if cfg.RAG.Enabled {
		logger.InfoKV("RAG enabled, preparing tools for bridge integration", "provider", cfg.RAG.Provider)

		// Add RAG tools to discoveredTools for bridge integration
		if discoveredTools == nil {
			discoveredTools = make(map[string]mcp.ToolInfo)
		}

		// Manually add RAG tools since we'll create the client in the Slack package
		discoveredTools["rag_search"] = mcp.ToolInfo{
			ToolName:        "rag_search",
			ToolDescription: "Search the RAG knowledge base for relevant information",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "The search query to find relevant information",
					},
				},
				"required": []string{"query"},
			},
			ServerName: "rag", // Internal RAG server identifier
		}
		discoveredTools["rag_ingest"] = mcp.ToolInfo{
			ToolName:        "rag_ingest",
			ToolDescription: "Ingest a file into the RAG knowledge base",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the file to ingest",
					},
					"metadata": map[string]interface{}{
						"type":        "object",
						"description": "Optional metadata for the file",
					},
				},
				"required": []string{"file_path"},
			},
			ServerName: "rag", // Internal RAG server identifier
		}
		discoveredTools["rag_stats"] = mcp.ToolInfo{
			ToolName:        "rag_stats",
			ToolDescription: "Get statistics about the RAG knowledge base",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			ServerName: "rag", // Internal RAG server identifier
		}

		logger.InfoKV("Added RAG tools to available tools", "tool_count", 3)
	} else {
		logger.Info("RAG integration disabled in configuration")
	}

	var err error

	var userFrontend slackbot.UserFrontend
	userFrontend, err = slackbot.GetSlackClient(
		cfg.Slack.BotToken,
		cfg.Slack.AppToken,
		logger,
		cfg.Slack.ThinkingMessage,
	)
	if err != nil {
		logger.Fatal("Failed to initialize Slack client: %v", err)
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

	// Create a channel to signal when Slack client exits
	slackDone := make(chan error, 1)

	// Start listening for Slack events in a separate goroutine
	go func() {
		defer close(slackDone)
		if err := client.Run(); err != nil {
			logger.ErrorKV("Slack client error", "error", err)
			slackDone <- err
		}
	}()

	logger.Info("Slack MCP Client is now running. Waiting for shutdown signal...")

	// Wait for termination signal or context cancellation
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	select {
	case sig := <-sigChan:
		logger.Info("Received signal %v, shutting down...", sig)
	case <-ctx.Done():
		logger.Info("Context cancelled, shutting down...")
	case err := <-slackDone:
		if err != nil {
			logger.ErrorKV("Slack client exited with error", "error", err)
		} else {
			logger.Info("Slack client exited normally")
		}
		return // Exit the function if Slack client stopped
	}

	// Try to close Slack client gracefully (if Close method is available)
	logger.Info("Stopping Slack client...")
	if closeErr := client.Close(); closeErr != nil {
		logger.ErrorKV("Failed to close Slack client gracefully", "error", closeErr)
	}

	// Wait for Slack client goroutine to finish with a timeout
	select {
	case <-slackDone:
		logger.Info("Slack client stopped")
	case <-time.After(5 * time.Second):
		logger.Warn("Slack client stop timed out")
	}

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

// handleRAGIngest processes PDF files from a directory and ingests them into the RAG database
func handleRAGIngest(path string) {
	provider := getRAGProvider()
	fmt.Printf("Ingesting PDF files from: %s (provider: %s)\n", path, provider)

	// Create RAG configuration
	config := getRAGConfig(provider)
	ragClient, err := rag.NewClientWithProvider(provider, config)
	if err != nil {
		fmt.Printf("Error creating RAG client: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := ragClient.GetProvider().Close(); err != nil {
			fmt.Printf("Warning: failed to close RAG client: %v\n", err)
		}
	}()

	ctx := context.Background()

	// Use the RAG client to ingest
	result, err := ragClient.CallTool(ctx, "rag_ingest", map[string]interface{}{
		"file_path":    path,
		"is_directory": true,
	})
	if err != nil {
		fmt.Printf("Error during ingestion: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Ingestion complete: %s\n", result)

	// Get stats
	statsResult, err := ragClient.CallTool(ctx, "rag_stats", map[string]interface{}{})
	if err != nil {
		fmt.Printf("Warning: Could not get stats: %v\n", err)
	} else {
		fmt.Printf("Stats: %s\n", statsResult)
	}
}

// handleRAGSearch searches the RAG database and displays results
func handleRAGSearch(query string) {
	provider := getRAGProvider()
	fmt.Printf("Searching RAG database for: %s (provider: %s)\n", query, provider)

	// Create RAG configuration
	config := getRAGConfig(provider)
	ragClient, err := rag.NewClientWithProvider(provider, config)
	if err != nil {
		fmt.Printf("Error creating RAG client: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := ragClient.GetProvider().Close(); err != nil {
			fmt.Printf("Warning: failed to close RAG client: %v\n", err)
		}
	}()

	ctx := context.Background()

	// Use the RAG client to search
	result, err := ragClient.CallTool(ctx, "rag_search", map[string]interface{}{
		"query": query,
	})
	if err != nil {
		fmt.Printf("Error during search: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n%s\n", result)
}

// handleRAGInit initializes the vector store
func handleRAGInit() {
	provider := getRAGProvider()
	if provider == "simple" {
		fmt.Printf("Init not required for simple provider\n")
		return
	}

	fmt.Printf("Initializing vector store (provider: %s)\n", provider)

	// Create RAG configuration
	config := getRAGConfig(provider)
	ragClient, err := rag.NewClientWithProvider(provider, config)
	if err != nil {
		fmt.Printf("Error creating RAG client: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := ragClient.GetProvider().Close(); err != nil {
			fmt.Printf("Warning: failed to close RAG client: %v\n", err)
		}
	}()

	fmt.Printf("Vector store initialized successfully\n")

	// Show stats
	ctx := context.Background()
	statsResult, err := ragClient.CallTool(ctx, "rag_stats", map[string]interface{}{})
	if err != nil {
		fmt.Printf("Warning: Could not get stats: %v\n", err)
	} else {
		fmt.Printf("Stats: %s\n", statsResult)
	}
}

// handleRAGList lists files in the vector store
func handleRAGList() {
	provider := getRAGProvider()
	fmt.Printf("Listing files in vector store (provider: %s)\n", provider)

	if provider == "simple" {
		fmt.Printf("File listing not supported for simple provider\n")
		return
	}

	// Create RAG configuration
	config := getRAGConfig(provider)
	ragClient, err := rag.NewClientWithProvider(provider, config)
	if err != nil {
		fmt.Printf("Error creating RAG client: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := ragClient.GetProvider().Close(); err != nil {
			fmt.Printf("Warning: failed to close RAG client: %v\n", err)
		}
	}()

	// Get the underlying vector provider
	vectorProvider := ragClient.GetProvider()
	ctx := context.Background()
	files, err := vectorProvider.ListFiles(ctx, 100)
	if err != nil {
		fmt.Printf("Error listing files: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d files:\n", len(files))
	for _, file := range files {
		fmt.Printf("  - ID: %s, Name: %s, Size: %d bytes, Status: %s\n",
			file.ID, file.Name, file.Size, file.Status)
	}
}

// handleRAGDelete deletes files from the vector store
func handleRAGDelete(fileIDs string) {
	provider := getRAGProvider()
	fmt.Printf("Deleting files from vector store (provider: %s)\n", provider)

	if provider == "simple" {
		fmt.Printf("File deletion not supported for simple provider\n")
		return
	}

	ids := strings.Split(fileIDs, ",")
	for i, id := range ids {
		ids[i] = strings.TrimSpace(id)
	}

	// Create RAG configuration
	config := getRAGConfig(provider)
	ragClient, err := rag.NewClientWithProvider(provider, config)
	if err != nil {
		fmt.Printf("Error creating RAG client: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := ragClient.GetProvider().Close(); err != nil {
			fmt.Printf("Warning: failed to close RAG client: %v\n", err)
		}
	}()

	// Get the underlying vector provider
	vectorProvider := ragClient.GetProvider()
	ctx := context.Background()
	for _, id := range ids {
		if err := vectorProvider.DeleteFile(ctx, id); err != nil {
			fmt.Printf("Error deleting file %s: %v\n", id, err)
		} else {
			fmt.Printf("Deleted file: %s\n", id)
		}
	}
}

// handleRAGStats shows RAG statistics
func handleRAGStats() {
	provider := getRAGProvider()
	fmt.Printf("RAG Statistics (provider: %s)\n", provider)

	// Create RAG configuration
	config := getRAGConfig(provider)
	ragClient, err := rag.NewClientWithProvider(provider, config)
	if err != nil {
		fmt.Printf("Error creating RAG client: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := ragClient.GetProvider().Close(); err != nil {
			fmt.Printf("Warning: failed to close RAG client: %v\n", err)
		}
	}()

	ctx := context.Background()
	result, err := ragClient.CallTool(ctx, "rag_stats", map[string]interface{}{})
	if err != nil {
		fmt.Printf("Error getting stats: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s\n", result)
}

// getRAGProvider determines the RAG provider to use
func getRAGProvider() string {
	if *ragProvider != "" {
		return *ragProvider
	}

	// Try to infer from LLM provider
	llmProvider := os.Getenv("LLM_PROVIDER")
	return rag.GetProviderFromFlags("", llmProvider)
}

// getRAGConfig creates RAG configuration based on provider and flags
func getRAGConfig(provider string) map[string]interface{} {
	config := make(map[string]interface{})
	config["database_path"] = *ragDatabase
	config["provider"] = provider

	if provider == "openai" {
		openaiConfig := make(map[string]interface{})
		if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
			openaiConfig["api_key"] = apiKey
		}

		// Add CLI flags for naming if provided
		if *ragAssistantName != "" {
			config["assistant_name"] = *ragAssistantName
		}
		if *ragVectorStoreName != "" {
			config["vector_store_name"] = *ragVectorStoreName
		}

		config["openai"] = openaiConfig
	}

	return config
}

// handleConfigMigration handles the configuration migration from legacy format
func handleConfigMigration(inputFile string) {
	fmt.Printf("Migrating configuration from legacy format...\n")

	// Determine input and output files
	if inputFile == "" {
		inputFile = "mcp-servers.json"
	}
	outputFile := "config.json"

	// Check if input file exists
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Input file '%s' not found\n", inputFile)
		fmt.Fprintf(os.Stderr, "Usage: slack-mcp-client --migrate-config [--config input-file]\n")
		os.Exit(1)
	}

	// Check if output file already exists
	if _, err := os.Stat(outputFile); err == nil {
		fmt.Fprintf(os.Stderr, "Error: Output file '%s' already exists\n", outputFile)
		fmt.Fprintf(os.Stderr, "Please move or rename the existing file before migration\n")
		os.Exit(1)
	}

	// Execute the migration script
	if err := executeMigrationScript(inputFile, outputFile); err != nil {
		fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Migration completed successfully!\n")
	fmt.Printf("  Input:  %s\n", inputFile)
	fmt.Printf("  Output: %s\n", outputFile)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("1. Review the generated %s file\n", outputFile)
	fmt.Printf("2. Test with: slack-mcp-client --config %s --config-validate\n", outputFile)
	fmt.Printf("3. Update your deployment scripts to use --config %s\n", outputFile)
}

// executeMigrationScript runs the migration script to convert legacy config
func executeMigrationScript(inputFile, outputFile string) error {
	// Look for the migration script in common locations
	scriptPaths := []string{
		"scripts/migrate-config.sh",
		"./migrate-config.sh",
		"/usr/local/bin/migrate-config.sh",
	}

	var scriptPath string
	for _, path := range scriptPaths {
		if _, err := os.Stat(path); err == nil {
			scriptPath = path
			break
		}
	}

	if scriptPath == "" {
		return fmt.Errorf("migration script not found in any of the expected locations: %v", scriptPaths)
	}

	// Execute the script with input and output files
	cmd := exec.Command("bash", scriptPath, inputFile, outputFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
