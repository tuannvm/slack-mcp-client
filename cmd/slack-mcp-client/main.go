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
	appLogger := setupLogging()
	appLogger.Info("Starting Slack MCP Client (debug=%v)", *debug)

	// Load and prepare configuration
	cfg := loadAndPrepareConfig(appLogger)

	// Initialize MCP clients and discover tools
	mcpClients, discoveredTools := initializeMCPClients(appLogger, cfg)

	// Initialize and run Slack client
	startSlackClient(appLogger, mcpClients, discoveredTools, cfg)
}

// setupLogging initializes the logging system
func setupLogging() *logging.Logger {
	logLevel := logging.LevelInfo
	if *debug {
		logLevel = logging.LevelDebug
	}

	appLogger := logging.New("slack-mcp-client", logLevel)
	
	// Setup library debugging if requested
	if *mcpDebug {
		if err := os.Setenv("MCP_DEBUG", "true"); err != nil {
			appLogger.Error("Failed to set MCP_DEBUG environment variable: %v", err)
		}
		appLogger.Info("MCP_DEBUG environment variable set to true")
	}
	
	return appLogger
}

// loadAndPrepareConfig loads the configuration and applies any overrides
func loadAndPrepareConfig(logger *logging.Logger) *config.Config {
	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		logger.Fatal("Failed to load configuration: %v", err)
	}

	// Force provider to OpenAI in config loaded, as client only supports OpenAI directly now
	if cfg.LLMProvider != config.ProviderOpenAI {
		logger.Warn("Config/Env specified LLM provider '%s', but client is hardcoded for OpenAI. Forcing OpenAI.", cfg.LLMProvider)
		cfg.LLMProvider = config.ProviderOpenAI
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
	
	appLogger.Info("Final LLM Provider: %s", cfg.LLMProvider) // Will always be OpenAI now
	logLLMSettings(appLogger, cfg)
	appLogger.Info("MCP Servers Configured (in file): %d", len(cfg.Servers))

	// If mcpDebug flag is set, enable library debug output
	if *mcpDebug {
		if err := os.Setenv("MCP_DEBUG", "true"); err != nil {
			appLogger.Error("Failed to set MCP_DEBUG environment variable: %v", err)
		}
		appLogger.Info("MCP_DEBUG environment variable set to true")
	}

	// Initialize MCP Clients and Discover Tools Sequentially
	mcpClients := make(map[string]*mcp.Client)
	allDiscoveredTools := make(map[string]common.ToolInfo) // Map: toolName -> common.ToolInfo
	failedServers := []string{}
	initializedClientCount := 0

	appLogger.Info("--- Starting MCP Client Initialization and Tool Discovery --- ")
	for serverName, serverConf := range cfg.Servers {
		appLogger.Info("Processing server: '%s'", serverName)

		// Skip disabled servers
		if serverConf.Disabled {
			appLogger.Info("  Skipping disabled server '%s'", serverName)
			continue
		}

		// Create a component-specific logger for this server
		serverLogger := appLogger.WithName(serverName)

		// Determine mode
		mode := strings.ToLower(serverConf.Mode)
		if mode == "" {
			if serverConf.Command != "" {
				mode = "stdio"
			} else {
				mode = "http"
			}
			serverLogger.Warn("No mode specified, defaulting to '%s'", mode)
		} else {
			serverLogger.Debug("Mode: '%s'", mode)
		}

		// Create dedicated logger for this MCP client
		// Continue using standard logger for MCP clients for now, as they expect *log.Logger
		mcpLoggerStd := log.New(os.Stdout, fmt.Sprintf("mcp-%s: ", strings.ToLower(serverName)), log.LstdFlags)

		// --- 1. Create Client Instance ---
		var mcpClient *mcp.Client
		var createErr error
		if mode == "stdio" {
			if serverConf.Command == "" {
				serverLogger.Error("Skipping stdio server: 'command' field is required")
				failedServers = append(failedServers, serverName+"(create: missing command)")
				continue
			}
			serverLogger.Info("Creating stdio MCP client for command: '%s' with args: %v", serverConf.Command, serverConf.Args)
			mcpClient, createErr = mcp.NewClient(mode, serverConf.Command, serverConf.Args, serverConf.Env, mcpLoggerStd)
		} else { // http or sse
			address := serverConf.Address
			if address == "" && serverConf.URL != "" {
				serverLogger.Info("Using 'url' field as address: %s", serverConf.URL)
				address = serverConf.URL
			} else if address == "" {
				serverLogger.Error("Skipping %s server: No 'address' or 'url' specified", mode)
				failedServers = append(failedServers, serverName+"(create: missing address/url)")
				continue
			}
			serverLogger.Info("Creating %s MCP client for address: %s", mode, address)
			mcpClient, createErr = mcp.NewClient(mode, address, nil, serverConf.Env, mcpLoggerStd) // Pass nil for args
		}

		if createErr != nil {
			serverLogger.Error("Failed to create MCP client instance: %v", createErr)
			failedServers = append(failedServers, serverName+"(create failed)")
			continue // Cannot proceed with this server
		}
		serverLogger.Info("Successfully created MCP client instance")

		// Defer client closure immediately after successful creation
		defer func(name string, client *mcp.Client, sLogger *logging.Logger) {
			if client != nil {
				sLogger.Info("Closing MCP client")
				client.Close()
			}
		}(serverName, mcpClient, serverLogger)

		// --- 2. Initialize Client ---
		initCtx, initCancel := context.WithTimeout(context.Background(), 1*time.Second)
		serverLogger.Info("Attempting to initialize MCP client (timeout: 1s)...")
		initErr := mcpClient.Initialize(initCtx)
		initCancel()

		if initErr != nil {
			serverLogger.Warn("Failed to initialize MCP client: %v", initErr)
			serverLogger.Warn("Client will not be used for tool discovery or execution")
			failedServers = append(failedServers, serverName+"(initialize failed)")
			continue // Cannot discover tools if init fails
		}
		serverLogger.Info("MCP client successfully initialized")
		mcpClients[serverName] = mcpClient // Store successfully initialized client
		initializedClientCount++

		// --- 3. Discover Tools ---
		serverLogger.Info("Discovering tools (timeout: 20s)...")
		discoveryCtx, discoveryCancel := context.WithTimeout(context.Background(), 20*time.Second)

		listResult, toolsErr := mcpClient.GetAvailableTools(discoveryCtx)
		discoveryCancel()

		if toolsErr != nil {
			serverLogger.Warn("Failed to retrieve tools: %v", toolsErr)
			failedServers = append(failedServers, serverName+"(tool discovery failed)")
			continue
		}

		if listResult == nil || len(listResult.Tools) == 0 {
			serverLogger.Warn("Server initialized but returned 0 tools")
			continue
		}

		serverLogger.Info("Discovered %d tools", len(listResult.Tools))
		for _, toolDef := range listResult.Tools {
			toolName := toolDef.Name
			if _, exists := allDiscoveredTools[toolName]; !exists {
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
				allDiscoveredTools[toolName] = common.ToolInfo{
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
				existingInfo := allDiscoveredTools[toolName]
				serverLogger.Warn("Tool '%s' is available from multiple servers ('%s' and '%s'). Using the first one found ('%s').",
					toolName, existingInfo.ServerName, serverName, existingInfo.ServerName)
			}
		}
	}
	appLogger.Info("--- Finished MCP Client Initialization and Tool Discovery --- ")

	// Log summary
	appLogger.Info("Successfully initialized %d MCP clients: %v", initializedClientCount, getMapKeys(mcpClients))
	if len(failedServers) > 0 {
		appLogger.Info("Failed to fully initialize/get tools from %d servers: %v", len(failedServers), failedServers)
	}
	appLogger.Info("Total unique discovered tools across all initialized servers: %d", len(allDiscoveredTools))

	// Check if we have at least one usable client
	if initializedClientCount == 0 {
		appLogger.Fatal("No MCP clients could be successfully initialized. Check configuration and server status.")
	}

	// Initialize Slack Bot Client using the successfully initialized clients and discovered tools
	// Create a custom logger for the slack client
	// Note: We'll fully integrate our custom logger with the Slack client in a future update
	// For now we'll continue using the standard logger as it expects *log.Logger
	// slackLoggerLevel := logger.LevelInfo
	// if *debug {
	//	slackLoggerLevel = logger.LevelDebug
	// }
	// slackAppLogger := logger.New(os.Stdout, "slack: ", logFlags, slackLoggerLevel)

	// Continue using standard logger for Slack client for now, as it expects *log.Logger
	slackLogger := log.New(os.Stdout, "slack: ", log.LstdFlags)
	client, err := slackbot.NewClient(
		cfg.SlackBotToken,
		cfg.SlackAppToken,
		slackLogger,
		mcpClients,         // Pass the map of *initialized* clients
		allDiscoveredTools, // Pass the map of common.ToolInfo
		cfg,                // Pass the whole config object
	)
	if err != nil {
		appLogger.Fatal("Failed to initialize Slack client: %v", err)
	}

	// Start the Slack client
	startSlackClient(appLogger, client)
}

// applyCommandLineOverrides applies command-line flags directly to the loaded config
func applyCommandLineOverrides(logger *logging.Logger, cfg *config.Config) error {
	// Provider is now forced to OpenAI earlier, so only check for OpenAI model override.
	if *openaiModel != "" {
		// Ensure the provider in config is actually OpenAI before overriding model
		// (This should always be true due to the check in main, but good for safety)
		if cfg.LLMProvider == config.ProviderOpenAI {
			logger.Info("Overriding OpenAI model from command line: %s", *openaiModel)
			cfg.OpenAIModelName = *openaiModel
		} else {
			// This case should technically not be reachable anymore
			logger.Warn("Warning: --openai-model flag provided, but configured provider is not OpenAI ('%s'). Flag ignored.", cfg.LLMProvider)
		}
	}
	return nil // No errors
}

// logLLMSettings logs the current LLM configuration
func logLLMSettings(logger *logging.Logger, cfg *config.Config) {
	logger.Info("OpenAI Model: %s", cfg.OpenAIModelName)
}

// startSlackClient starts the Slack client and handles shutdown
func startSlackClient(logger *logging.Logger, client *slackbot.Client) {
	logger.Info("Starting Slack client...")

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
