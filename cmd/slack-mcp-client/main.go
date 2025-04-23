// Package main implements the Slack MCP client application
// It provides a bridge between Slack and MCP servers
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tuannvm/slack-mcp-client/internal/common"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
	slackbot "github.com/tuannvm/slack-mcp-client/internal/slack"
)

// ToolInfo definition is moved to internal/common/types.go

var (
	// Define command-line flags
	configFile  = flag.String("config", "", "Path to the MCP server configuration JSON file")
	debug       = flag.Bool("debug", false, "Enable debug logging")
	mcpDebug    = flag.Bool("mcpdebug", false, "Enable debug logging for MCP clients")
	openaiModel = flag.String("openai-model", "", "OpenAI model to use (overrides config/env)")
)

func main() {
	flag.Parse()

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
	logLevel := logging.LevelInfo
	if *debug {
		logLevel = logging.LevelDebug
	}

	logger := logging.New("slack-mcp-client", logLevel)

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
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		logger.Fatal("Failed to load configuration: %v", err)
	}

	// Validate LLM provider - now supporting both OpenAI direct and LangChain
	if cfg.LLMProvider != config.ProviderOpenAI && cfg.LLMProvider != config.ProviderLangChain {
		logger.Warn("Config/Env specified unsupported LLM provider '%s'. Supported: 'openai' or 'langchain'. Defaulting to LangChain.", cfg.LLMProvider)
		cfg.LLMProvider = config.ProviderLangChain
	}

	// Apply command-line overrides AFTER loading config
	if err := applyCommandLineOverrides(logger, cfg); err != nil {
		logger.Fatal("Error applying command-line flags: %v", err)
	}

	// Log configuration information
	logger.Info("Configuration loaded. Slack Bot Token Present: %t, Slack App Token Present: %t",
		cfg.SlackBotToken != "", cfg.SlackAppToken != "")
	logger.Info("Final LLM Provider: %s", cfg.LLMProvider)
	logLLMSettings(logger, cfg)
	logger.Info("MCP Servers Configured (in file): %d", len(cfg.Servers))

	return cfg
}

// initializeMCPClients initializes all MCP clients and discovers available tools
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
	logger.Info("Successfully initialized %d MCP clients: %v", initializedClientCount, getMapKeys(mcpClients))
	if len(failedServers) > 0 {
		logger.Info("Failed to fully initialize/get tools from %d servers: %v", len(failedServers), failedServers)
	}
	logger.Info("Total unique discovered tools across all initialized servers: %d", len(allDiscoveredTools))

	// Check if we have at least one usable client
	if initializedClientCount == 0 {
		logger.Fatal("No MCP clients could be successfully initialized. Check configuration and server status.")
	}

	return mcpClients, allDiscoveredTools
}

// processSingleMCPServer processes a single MCP server configuration
func processSingleMCPServer(
	logger *logging.Logger,
	serverName string,
	serverConf config.ServerConfig,
	mcpClients map[string]*mcp.Client,
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

	// Determine mode
	mode := determineServerMode(serverLogger, serverConf)

	// Create dedicated logger for this MCP client
	mcpLoggerStd := log.New(os.Stdout, fmt.Sprintf("mcp-%s: ", strings.ToLower(serverName)), log.LstdFlags)

	// Create client instance
	mcpClient, err := createMCPClient(serverLogger, mode, serverConf, mcpLoggerStd)
	if err != nil {
		*failedServers = append(*failedServers, serverName+fmt.Sprintf("(create: %s)", err))
		return
	}

	serverLogger.Info("Successfully created MCP client instance")

	// Defer client closure
	defer func() {
		if mcpClient != nil {
			serverLogger.Info("Closing MCP client")
			mcpClient.Close()
		}
	}()

	// Initialize client
	if err := initializeMCPClientInstance(serverLogger, mcpClient); err != nil {
		*failedServers = append(*failedServers, serverName+"(initialize failed)")
		return
	}

	// Store successfully initialized client
	mcpClients[serverName] = mcpClient
	*initializedClientCount++

	// Discover tools
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

// determineServerMode determines the mode for a server based on configuration
func determineServerMode(logger *logging.Logger, serverConf config.ServerConfig) string {
	mode := strings.ToLower(serverConf.Mode)
	if mode == "" {
		if serverConf.Command != "" {
			mode = "stdio"
		} else {
			mode = "http"
		}
		logger.Warn("No mode specified, defaulting to '%s'", mode)
	} else {
		logger.Debug("Mode: '%s'", mode)
	}
	return mode
}

// createMCPClient creates an MCP client based on mode and configuration
func createMCPClient(logger *logging.Logger, mode string, serverConf config.ServerConfig, mcpLogger *log.Logger) (*mcp.Client, error) {
	var mcpClient *mcp.Client
	var createErr error

	if mode == "stdio" {
		if serverConf.Command == "" {
			logger.Error("Skipping stdio server: 'command' field is required")
			return nil, fmt.Errorf("missing command")
		}
		logger.Info("Creating stdio MCP client for command: '%s' with args: %v", serverConf.Command, serverConf.Args)
		mcpClient, createErr = mcp.NewClient(mode, serverConf.Command, serverConf.Args, serverConf.Env, mcpLogger)
	} else { // http or sse
		address := serverConf.Address
		if address == "" && serverConf.URL != "" {
			logger.Info("Using 'url' field as address: %s", serverConf.URL)
			address = serverConf.URL
		} else if address == "" {
			logger.Error("Skipping %s server: No 'address' or 'url' specified", mode)
			return nil, fmt.Errorf("missing address/url")
		}
		logger.Info("Creating %s MCP client for address: %s", mode, address)
		mcpClient, createErr = mcp.NewClient(mode, address, nil, serverConf.Env, mcpLogger) // Pass nil for args
	}

	return mcpClient, createErr
}

// initializeMCPClientInstance initializes an MCP client with proper timeout
func initializeMCPClientInstance(logger *logging.Logger, client *mcp.Client) error {
	logger.Info("Attempting to initialize MCP client (timeout: 1s)...")
	initCtx, initCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer initCancel()

	initErr := client.Initialize(initCtx)
	if initErr != nil {
		logger.Warn("Failed to initialize MCP client: %v", initErr)
		logger.Warn("Client will not be used for tool discovery or execution")
		return initErr
	}

	logger.Info("MCP client successfully initialized")
	return nil
}

// applyCommandLineOverrides applies command-line flags directly to the loaded config
func applyCommandLineOverrides(logger *logging.Logger, cfg *config.Config) error {
	// Apply OpenAI model override if specified
	if *openaiModel != "" {
		// Both OpenAI direct and LangChain providers can use the OpenAI model setting
		if cfg.LLMProvider == config.ProviderOpenAI || cfg.LLMProvider == config.ProviderLangChain {
			logger.Info("Overriding OpenAI model from command line: %s", *openaiModel)
			cfg.OpenAIModelName = *openaiModel
		} else {
			logger.Warn("Warning: --openai-model flag provided, but configured provider '%s' doesn't use OpenAI models. Flag ignored.", cfg.LLMProvider)
		}
	}
	return nil // No errors
}

// logLLMSettings logs the current LLM configuration
func logLLMSettings(logger *logging.Logger, cfg *config.Config) {
	logger.Info("OpenAI Model: %s", cfg.OpenAIModelName)
}

// startSlackClient starts the Slack client and handles shutdown
func startSlackClient(logger *logging.Logger, mcpClients map[string]*mcp.Client, discoveredTools map[string]common.ToolInfo, cfg *config.Config) {
	logger.Info("Starting Slack client...")

	// Continue using standard logger for Slack client for now, as it expects *log.Logger
	slackLogger := log.New(os.Stdout, "slack: ", log.LstdFlags)
	client, err := slackbot.NewClient(
		cfg.SlackBotToken,
		cfg.SlackAppToken,
		slackLogger,
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
	// Client closures are handled by defer statements
}

// Helper function to get keys from a map[string]*mcp.Client
func getMapKeys(m map[string]*mcp.Client) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
