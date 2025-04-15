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

	// Setup logging with appropriate level
	logFlags := log.LstdFlags | log.Lshortfile
	if *debug {
		logFlags |= log.Lmicroseconds
	}
	logger := log.New(os.Stdout, "slack-mcp-client: ", logFlags)
	logger.Printf("Starting Slack MCP Client (debug=%v)", *debug)

	// Validate and apply LLM provider settings
	setupLLMProvider(logger)

	// Load configuration from environment variables and optional config file
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Apply command-line model overrides if specified
	applyModelOverrides(logger, cfg)

	logger.Printf("Configuration loaded. Slack Bot Token Present: %t, Slack App Token Present: %t",
		cfg.SlackBotToken != "", cfg.SlackAppToken != "")
	logger.Printf("LLM Provider: %s", cfg.LLMProvider)
	logLLMSettings(logger, cfg)
	
	logger.Printf("MCP Servers Configured: %d", len(cfg.Servers))

	// Initialize MCP Clients based on configuration
	mcpClients, err := initializeMCPClients(logger, cfg, logFlags)
	if err != nil {
		logger.Fatalf("Failed to initialize MCP clients: %v", err)
	}

	// Discover tools from initialized clients
	allDiscoveredTools := discoverTools(logger, mcpClients)
	
	// Initialize Slack Bot Client
	slackLogger := log.New(os.Stdout, "slack: ", logFlags)
	client, err := slackbot.NewClient(
		cfg.SlackBotToken,
		cfg.SlackAppToken,
		slackLogger,
		mcpClients,
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

// initializeMCPClients initializes all MCP clients from configuration
func initializeMCPClients(logger *log.Logger, cfg *config.Config, logFlags int) (map[string]*mcp.Client, error) {
	mcpClients := make(map[string]*mcp.Client)
	
	// Check if we have any server configurations
	if len(cfg.Servers) == 0 {
		logger.Fatalf("No MCP servers configured in '%s'. At least one MCP server is required.", *configFile)
	}
	
	// DEBUGGING: Print all configured servers and their details
	logger.Printf("Found %d MCP servers in configuration", len(cfg.Servers))
	for name, conf := range cfg.Servers {
		if conf.Disabled {
			logger.Printf("  Server '%s' is disabled", name)
			continue
		}
		
		logger.Printf("  Server '%s' - Mode: '%s'", name, conf.Mode)
		if conf.Mode == "stdio" {
			logger.Printf("    Command: '%s'", conf.Command)
			if len(conf.Args) > 0 {
				logger.Printf("    Args: %v", conf.Args)
			}
			if len(conf.Env) > 0 {
				logger.Printf("    Env vars: %d defined", len(conf.Env))
				if *mcpDebug {
					for k, v := range conf.Env {
						if strings.Contains(strings.ToLower(k), "password") {
							logger.Printf("      %s: <redacted>", k)
						} else {
							logger.Printf("      %s: %s", k, v)
						}
					}
				}
			}
		} else {
			if conf.Address != "" {
				logger.Printf("    Address: '%s'", conf.Address)
			}
			if conf.URL != "" {
				logger.Printf("    URL: '%s'", conf.URL)
			}
		}
	}
	
	// If the mcpDebug flag is set, set the MCP_DEBUG environment variable
	if *mcpDebug {
		os.Setenv("MCP_DEBUG", "true")
		logger.Printf("MCP_DEBUG environment variable set to true")
	}
	
	// Initialize each server from configuration
	for serverName, serverConf := range cfg.Servers {
		// Skip disabled servers
		if serverConf.Disabled {
			logger.Printf("Skipping disabled MCP server '%s'", serverName)
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
			logger.Printf("WARNING: No mode specified for server '%s', defaulting to '%s'", serverName, mode)
		}
		logger.Printf("Using mode '%s' for server '%s'", mode, serverName)

		// Create a dedicated logger for this MCP client
		mcpLogger := log.New(os.Stdout, fmt.Sprintf("mcp-%s: ", strings.ToLower(serverName)), logFlags)

		var mcpClient *mcp.Client
		var err error

		// Create client based on mode, passing components separately for stdio
		if mode == "stdio" {
			if serverConf.Command == "" {
				logger.Printf("ERROR: Skipping stdio server '%s': 'command' field is required.", serverName)
				continue
			}
			logger.Printf("Creating stdio MCP client for command: '%s' with args: %v", serverConf.Command, serverConf.Args)
			mcpClient, err = mcp.NewClient(mode, serverConf.Command, serverConf.Args, serverConf.Env, mcpLogger)
		} else { // http or sse
			address := serverConf.Address
			if address == "" && serverConf.URL != "" {
				logger.Printf("Using 'url' field as address for %s server '%s': %s", mode, serverName, serverConf.URL)
				address = serverConf.URL
			} else if address == "" {
				logger.Printf("ERROR: Skipping %s server '%s': No 'address' or 'url' specified.", mode, serverName)
				continue
			}
			logger.Printf("Creating %s MCP client for address: %s", mode, address)
			mcpClient, err = mcp.NewClient(mode, address, nil, serverConf.Env, mcpLogger) // Pass nil for args in non-stdio modes
		}

		// Handle client creation error
		if err != nil {
			logger.Printf("ERROR: Failed to create MCP client '%s': %v", serverName, err)
			// Decide whether to fail fast or continue. Let's continue but log the error.
			// return nil, fmt.Errorf("failed to create MCP client '%s': %w", serverName, err)
			continue
		}
		logger.Printf("Successfully created MCP client instance for '%s'", serverName)
		
		// Print environment for debugging (optional, controlled by mcpDebug flag)
		if *mcpDebug {
			mcpClient.PrintEnvironment()
		}

		// Try to initialize the client
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		logger.Printf("Attempting to initialize MCP client '%s' with 15s timeout...", serverName)
		
		initErr := mcpClient.Initialize(ctx)
		cancel()
		
		if initErr != nil {
			logger.Printf("WARNING: Failed to initialize MCP client '%s': %v", serverName, initErr)
			logger.Printf("Client '%s' might not be usable.", serverName)
			// Continue anyway, but the client might fail later
		} else {
			logger.Printf("MCP client '%s' successfully initialized.", serverName)
		}

		mcpClients[serverName] = mcpClient

		// Defer client closure
		defer func(name string, client *mcp.Client) {
			if client != nil {
				logger.Printf("Closing MCP client: %s", name)
				client.Close()
			}
		}(serverName, mcpClient)
	}

	// Check if *any* clients were successfully created and potentially initialized
	if len(mcpClients) == 0 {
		return nil, fmt.Errorf("no MCP clients could be created based on the configuration, check logs for errors")
	}

	logger.Printf("Finished MCP client setup. Created %d clients.", len(mcpClients))
	return mcpClients, nil
}

// discoverTools discovers available tools from all initialized clients
func discoverTools(logger *log.Logger, mcpClients map[string]*mcp.Client) map[string]string {
	allDiscoveredTools := make(map[string]string) // Map: toolName -> serverName
	failedServers := []string{}
	
	logger.Println("Attempting to discover tools from initialized clients...")
	for serverName, client := range mcpClients {
		logger.Printf("Discovering tools from MCP server '%s'...", serverName)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // Increased timeout for tool discovery
		tools, err := client.GetAvailableTools(ctx)
		cancel()
		
		if err != nil {
			logger.Printf("Warning: Failed to retrieve available tools from MCP server '%s': %v", serverName, err)
			if strings.Contains(err.Error(), "client not initialized") {
				logger.Printf("Attempting to reinitialize client '%s' before getting tools...", serverName)
				// Try initializing again with longer timeout
				initCtx, initCancel := context.WithTimeout(context.Background(), 30*time.Second)
				if initErr := client.Initialize(initCtx); initErr != nil {
					logger.Printf("Reinitialization also failed: %v", initErr)
				} else {
					// Try getting tools again after successful reinitialization
					toolsCtx, toolsCancel := context.WithTimeout(context.Background(), 20*time.Second)
					retryTools, retryErr := client.GetAvailableTools(toolsCtx)
					toolsCancel()
					if retryErr == nil {
						logger.Printf("Successfully retrieved %d tools after reinitialization from server '%s': %v", 
							len(retryTools), serverName, retryTools)
						for _, tool := range retryTools {
							if existingServer, exists := allDiscoveredTools[tool]; !exists {
								allDiscoveredTools[tool] = serverName
							} else {
								logger.Printf("Warning: Tool '%s' is available from multiple servers ('%s' and '%s'). Using the first one found ('%s').",
									tool, existingServer, serverName, existingServer)
							}
						}
						initCancel()
						continue
					} else {
						logger.Printf("Retry after reinitialization also failed: %v", retryErr)
					}
				}
				initCancel()
			}
			
			failedServers = append(failedServers, serverName)
			continue // Skip this server if tool discovery fails, but continue with others
		}
		
		if len(tools) == 0 {
			logger.Printf("Warning: Server '%s' didn't return any tools", serverName)
			continue
		}
		
		logger.Printf("Discovered %d tools from server '%s': %v", len(tools), serverName, tools)
		for _, tool := range tools {
			if existingServer, exists := allDiscoveredTools[tool]; !exists {
				allDiscoveredTools[tool] = serverName
			} else {
				logger.Printf("Warning: Tool '%s' is available from multiple servers ('%s' and '%s'). Using the first one found ('%s').",
					tool, existingServer, serverName, existingServer)
			}
		}
	}
	
	if len(failedServers) > 0 {
		logger.Printf("Warning: Failed to retrieve tools from %d servers: %v", len(failedServers), failedServers)
	}
	
	logger.Printf("Total unique discovered tools across all servers: %d", len(allDiscoveredTools))
	return allDiscoveredTools
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
}
