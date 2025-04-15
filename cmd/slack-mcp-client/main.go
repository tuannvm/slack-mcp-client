package main

import (
	"context"
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
)

var (
	// Define command-line flags
	configFile = flag.String("config", "", "Path to the MCP server configuration JSON file")
	debug      = flag.Bool("debug", false, "Enable debug logging")
	mcpDebug   = flag.Bool("mcpdebug", false, "Enable debug logging for MCP clients")
	llmProvider = flag.String("llm", "", "LLM provider to use: 'openai' (default) or 'ollama'")
	openaiModel = flag.String("openai-model", "", "OpenAI model to use (only if --llm=openai)")
	ollamaModel = flag.String("ollama-model", "", "Ollama model to use (only if --llm=ollama)")
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

	// Setup LLM provider
	setupLLMProvider(logger)

	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}
	applyModelOverrides(logger, cfg)

	logger.Printf("Configuration loaded. Slack Bot Token Present: %t, Slack App Token Present: %t",
		cfg.SlackBotToken != "", cfg.SlackAppToken != "")
	logger.Printf("LLM Provider: %s", cfg.LLMProvider)
	logLLMSettings(logger, cfg)
	logger.Printf("MCP Servers Configured (in file): %d", len(cfg.Servers))

	// If mcpDebug flag is set, enable library debug output
	if *mcpDebug {
		os.Setenv("MCP_DEBUG", "true")
		logger.Printf("MCP_DEBUG environment variable set to true")
	}

	// Initialize MCP Clients and Discover Tools Sequentially
	mcpClients := make(map[string]*mcp.Client)
	allDiscoveredTools := make(map[string]string) // Map: toolName -> serverName
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
		initCtx, initCancel := context.WithTimeout(context.Background(), 15*time.Second)
		logger.Printf("  Attempting to initialize MCP client '%s' (timeout: 15s)...", serverName)
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
		tools, toolsErr := mcpClient.GetAvailableTools(discoveryCtx)
		discoveryCancel()

		if toolsErr != nil {
			logger.Printf("  WARNING: Failed to retrieve tools from initialized server '%s': %v", serverName, toolsErr)
			failedServers = append(failedServers, serverName+"(tool discovery failed)")
			// Continue with other servers even if one fails discovery
			continue 
		}

		if len(tools) == 0 {
			logger.Printf("  Warning: Server '%s' initialized but returned 0 tools.", serverName)
			continue
		}

		logger.Printf("  Discovered %d tools from server '%s': %v", len(tools), serverName, tools)
		for _, tool := range tools {
			if existingServer, exists := allDiscoveredTools[tool]; !exists {
				allDiscoveredTools[tool] = serverName
			} else {
				logger.Printf("  Warning: Tool '%s' is available from multiple servers ('%s' and '%s'). Using the first one found ('%s').",
					tool, existingServer, serverName, existingServer)
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
		mcpClients, // Pass the map of *initialized* clients
		allDiscoveredTools,
		cfg.OllamaAPIEndpoint,
		cfg.OllamaModelName,
		string(cfg.LLMProvider),
		cfg.OpenAIModelName,
	)
	if err != nil {
		logger.Fatalf("Failed to initialize Slack client: %v", err)
	}

	// Start the Slack client
	startSlackClient(logger, client)
}

// setupLLMProvider validates and applies LLM provider settings from command line
func setupLLMProvider(logger *log.Logger) {
	if *llmProvider != "" {
		if *llmProvider != "openai" && *llmProvider != "ollama" {
			logger.Fatalf("Invalid LLM provider: %s. Must be 'openai' or 'ollama'", *llmProvider)
		}
		
		// Validate model flags match the provider
		if *llmProvider == "openai" && *ollamaModel != "" {
			logger.Fatalf("Ollama model specified with OpenAI provider. Use --openai-model instead")
		}
		if *llmProvider == "ollama" && *openaiModel != "" {
			logger.Fatalf("OpenAI model specified with Ollama provider. Use --ollama-model instead")
		}
		
		os.Setenv("LLM_PROVIDER", *llmProvider)
		logger.Printf("Setting LLM provider from command line: %s", *llmProvider)
		
		// Set the model if specified
		if *llmProvider == "openai" && *openaiModel != "" {
			os.Setenv("OPENAI_MODEL", *openaiModel)
			logger.Printf("Using OpenAI model: %s", *openaiModel)
		} else if *llmProvider == "ollama" && *ollamaModel != "" {
			os.Setenv("OLLAMA_MODEL", *ollamaModel)
			logger.Printf("Using Ollama model: %s", *ollamaModel)
		}
	}
}

// applyModelOverrides applies command-line model overrides to the config
func applyModelOverrides(logger *log.Logger, cfg *config.Config) {
	if *llmProvider == "openai" && *openaiModel != "" {
		cfg.OpenAIModelName = *openaiModel
		logger.Printf("Overriding OpenAI model from command line: %s", *openaiModel)
	} else if *llmProvider == "ollama" && *ollamaModel != "" {
		cfg.OllamaModelName = *ollamaModel
		logger.Printf("Overriding Ollama model from command line: %s", *ollamaModel)
	}

	// Ensure only one model (provider) is used
	if cfg.LLMProvider == config.ProviderOllama {
		// Clear OpenAI settings to avoid confusion
		cfg.OpenAIModelName = ""
	} else {
		// Clear Ollama settings to avoid confusion
		cfg.OllamaModelName = ""
		cfg.OllamaAPIEndpoint = ""
	}
}

// logLLMSettings logs the current LLM configuration
func logLLMSettings(logger *log.Logger, cfg *config.Config) {
	if cfg.LLMProvider == config.ProviderOllama {
		logger.Printf("Ollama Endpoint: %s, Ollama Model: %s", cfg.OllamaAPIEndpoint, cfg.OllamaModelName)
	} else {
		logger.Printf("OpenAI Model: %s", cfg.OpenAIModelName)
	}
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
