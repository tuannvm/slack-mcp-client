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

	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
	slackbot "github.com/tuannvm/slack-mcp-client/internal/slack"
	"github.com/tuannvm/slack-mcp-client/internal/types"
)

// ToolInfo definition is moved to internal/types/types.go

var (
	// Define command-line flags
	configFile = flag.String("config", "", "Path to the MCP server configuration JSON file")
	debug      = flag.Bool("debug", false, "Enable debug logging")
	mcpDebug   = flag.Bool("mcpdebug", false, "Enable debug logging for MCP clients")
	openaiModel = flag.String("openai-model", "", "OpenAI model to use (overrides config/env)")
)

func main() {
	flag.Parse()

	// Setup logging
	logFlags := log.LstdFlags | log.Lshortfile
	if *debug {
		logFlags |= log.Lmicroseconds
	}
	logger := log.New(os.Stdout, "slack-mcp-client: ", logFlags)
	logger.Printf("Starting Slack MCP Client (debug=%v)", *debug)

	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Force provider to OpenAI in config loaded, as client only supports OpenAI directly now
	if cfg.LLMProvider != config.ProviderOpenAI {
		logger.Printf("Warning: Config/Env specified LLM provider '%s', but client is hardcoded for OpenAI. Forcing OpenAI.", cfg.LLMProvider)
		cfg.LLMProvider = config.ProviderOpenAI
	}

	// Apply command-line overrides AFTER loading config
	if err := applyCommandLineOverrides(logger, cfg); err != nil {
		logger.Fatalf("Error applying command-line flags: %v", err)
	}

	logger.Printf("Configuration loaded. Slack Bot Token Present: %t, Slack App Token Present: %t",
		cfg.SlackBotToken != "", cfg.SlackAppToken != "")
	logger.Printf("Final LLM Provider: %s", cfg.LLMProvider) // Will always be OpenAI now
	logLLMSettings(logger, cfg)
	logger.Printf("MCP Servers Configured (in file): %d", len(cfg.Servers))

	// If mcpDebug flag is set, enable library debug output
	if *mcpDebug {
		os.Setenv("MCP_DEBUG", "true")
		logger.Printf("MCP_DEBUG environment variable set to true")
	}

	// Initialize MCP Clients and Discover Tools Sequentially
	mcpClients := make(map[string]*mcp.Client)
	allDiscoveredTools := make(map[string]types.ToolInfo) // Map: toolName -> types.ToolInfo
	failedServers := []string{}
	initializedClientCount := 0

	logger.Println("--- Starting MCP Client Initialization and Tool Discovery --- ")
	for serverName, serverConf := range cfg.Servers {
		logger.Printf("Processing server: '%s'", serverName)

		// Skip disabled servers
		if serverConf.Disabled {
			logger.Printf("  Skipping disabled server '%s'", serverName)
			continue
		}

		// Determine mode
		mode := strings.ToLower(serverConf.Mode)
		if mode == "" {
			if serverConf.Command != "" {
				mode = "stdio"
			} else {
				mode = "http"
			}
			logger.Printf("  WARNING: No mode specified for server '%s', defaulting to '%s'", serverName, mode)
		} else {
			logger.Printf("  Mode: '%s'", mode)
		}

		// Create dedicated logger for this MCP client
		mcpLogger := log.New(os.Stdout, fmt.Sprintf("mcp-%s: ", strings.ToLower(serverName)), logFlags)

		// --- 1. Create Client Instance ---
		var mcpClient *mcp.Client
		var createErr error
		if mode == "stdio" {
			if serverConf.Command == "" {
				logger.Printf("  ERROR: Skipping stdio server '%s': 'command' field is required.", serverName)
				failedServers = append(failedServers, serverName+"(create: missing command)")
				continue
			}
			logger.Printf("  Creating stdio MCP client for command: '%s' with args: %v", serverConf.Command, serverConf.Args)
			mcpClient, createErr = mcp.NewClient(mode, serverConf.Command, serverConf.Args, serverConf.Env, mcpLogger)
		} else { // http or sse
			address := serverConf.Address
			if address == "" && serverConf.URL != "" {
				logger.Printf("  Using 'url' field as address for %s server '%s': %s", mode, serverName, serverConf.URL)
				address = serverConf.URL
			} else if address == "" {
				logger.Printf("  ERROR: Skipping %s server '%s': No 'address' or 'url' specified.", mode, serverName)
				failedServers = append(failedServers, serverName+"(create: missing address/url)")
				continue
			}
			logger.Printf("  Creating %s MCP client for address: %s", mode, address)
			mcpClient, createErr = mcp.NewClient(mode, address, nil, serverConf.Env, mcpLogger) // Pass nil for args
		}

		if createErr != nil {
			logger.Printf("  ERROR: Failed to create MCP client instance for '%s': %v", serverName, createErr)
			failedServers = append(failedServers, serverName+"(create failed)")
			continue // Cannot proceed with this server
		}
		logger.Printf("  Successfully created MCP client instance for '%s'", serverName)

		// Defer client closure immediately after successful creation
		defer func(name string, client *mcp.Client) {
			if client != nil {
				logger.Printf("Closing MCP client: %s", name)
				client.Close()
			}
		}(serverName, mcpClient)

		// --- 2. Initialize Client ---
		initCtx, initCancel := context.WithTimeout(context.Background(), 1*time.Second)
		logger.Printf("  Attempting to initialize MCP client '%s' (timeout: 1s)...", serverName)
		initErr := mcpClient.Initialize(initCtx)
		initCancel()

		if initErr != nil {
			logger.Printf("  WARNING: Failed to initialize MCP client '%s': %v", serverName, initErr)
			logger.Printf("  Client '%s' will not be used for tool discovery or execution.", serverName)
			failedServers = append(failedServers, serverName+"(initialize failed)")
			continue // Cannot discover tools if init fails
		}
		logger.Printf("  MCP client '%s' successfully initialized.", serverName)
		mcpClients[serverName] = mcpClient // Store successfully initialized client
		initializedClientCount++

		// --- 3. Discover Tools ---
		logger.Printf("  Discovering tools from '%s' (timeout: 20s)...", serverName)
		discoveryCtx, discoveryCancel := context.WithTimeout(context.Background(), 20*time.Second)
		
		listResult, toolsErr := mcpClient.GetAvailableTools(discoveryCtx)
		discoveryCancel()

		if toolsErr != nil {
			logger.Printf("  WARNING: Failed to retrieve tools from initialized server '%s': %v", serverName, toolsErr)
			failedServers = append(failedServers, serverName+"(tool discovery failed)")
			continue 
		}

		if listResult == nil || len(listResult.Tools) == 0 {
			logger.Printf("  Warning: Server '%s' initialized but returned 0 tools.", serverName)
			continue
		}

		logger.Printf("  Discovered %d tools from server '%s'", len(listResult.Tools), serverName)
		for _, toolDef := range listResult.Tools {
			toolName := toolDef.Name
			if _, exists := allDiscoveredTools[toolName]; !exists {
				var inputSchemaMap map[string]interface{}
				// Marshal the ToolInputSchema struct to JSON bytes
				schemaBytes, err := json.Marshal(toolDef.InputSchema)
				if err != nil {
					logger.Printf("    ERROR: Failed to marshal input schema struct for tool '%s': %v", toolName, err)
					inputSchemaMap = make(map[string]interface{}) // Use empty map on error
				} else {
					// Unmarshal the JSON bytes into the map
					if err := json.Unmarshal(schemaBytes, &inputSchemaMap); err != nil {
						logger.Printf("    ERROR: Failed to unmarshal input schema JSON for tool '%s': %v", toolName, err)
						inputSchemaMap = make(map[string]interface{}) // Use empty map on error
					}
				}

				// Use types.ToolInfo
				allDiscoveredTools[toolName] = types.ToolInfo{
					ServerName:  serverName,
					Description: toolDef.Description,
					InputSchema: inputSchemaMap,
				}
				if *mcpDebug {
				    logger.Printf("    Stored tool: '%s' (Desc: %s, Schema: %v)", toolName, toolDef.Description, inputSchemaMap)
				}
			} else {
				existingInfo := allDiscoveredTools[toolName]
				logger.Printf("  Warning: Tool '%s' is available from multiple servers ('%s' and '%s'). Using the first one found ('%s').",
					toolName, existingInfo.ServerName, serverName, existingInfo.ServerName)
			}
		}
	}
	logger.Println("--- Finished MCP Client Initialization and Tool Discovery --- ")

	// Log summary
	logger.Printf("Successfully initialized %d MCP clients: %v", initializedClientCount, getMapKeys(mcpClients))
	if len(failedServers) > 0 {
		logger.Printf("Failed to fully initialize/get tools from %d servers: %v", len(failedServers), failedServers)
	}
	logger.Printf("Total unique discovered tools across all initialized servers: %d", len(allDiscoveredTools))

	// Check if we have at least one usable client
	if initializedClientCount == 0 {
		logger.Fatalf("No MCP clients could be successfully initialized. Check configuration and server status.")
	}
	
	// Initialize Slack Bot Client using the successfully initialized clients and discovered tools
	slackLogger := log.New(os.Stdout, "slack: ", logFlags)
	client, err := slackbot.NewClient(
		cfg.SlackBotToken,
		cfg.SlackAppToken,
		slackLogger,
		mcpClients,         // Pass the map of *initialized* clients
		allDiscoveredTools, // Pass the map of types.ToolInfo
		cfg,                // Pass the whole config object
	)
	if err != nil {
		logger.Fatalf("Failed to initialize Slack client: %v", err)
	}

	// Start the Slack client
	startSlackClient(logger, client)
}

// applyCommandLineOverrides applies command-line flags directly to the loaded config
func applyCommandLineOverrides(logger *log.Logger, cfg *config.Config) error {
	// Provider is now forced to OpenAI earlier, so only check for OpenAI model override.
	if *openaiModel != "" {
		// Ensure the provider in config is actually OpenAI before overriding model
		// (This should always be true due to the check in main, but good for safety)
		if cfg.LLMProvider == config.ProviderOpenAI {
			logger.Printf("Overriding OpenAI model from command line: %s", *openaiModel)
			cfg.OpenAIModelName = *openaiModel
		} else {
			// This case should technically not be reachable anymore
			logger.Printf("Warning: --openai-model flag provided, but configured provider is not OpenAI ('%s'). Flag ignored.", cfg.LLMProvider)
		}
	} 
	return nil // No errors
}

// logLLMSettings logs the current LLM configuration
func logLLMSettings(logger *log.Logger, cfg *config.Config) {
	logger.Printf("OpenAI Model: %s", cfg.OpenAIModelName)
}

// startSlackClient starts the Slack client and handles shutdown
func startSlackClient(logger *log.Logger, client *slackbot.Client) {
	logger.Println("Starting Slack client...")

	// Start listening for Slack events in a separate goroutine
	go func() {
		if err := client.Run(); err != nil {
			logger.Fatalf("Slack client error: %v", err)
		}
	}()

	logger.Println("Slack MCP Client is now running. Press Ctrl+C to exit.")

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	logger.Printf("Received signal %v, shutting down...", sig)
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
