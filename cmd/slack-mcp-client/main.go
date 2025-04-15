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

	// Load configuration from environment variables and optional config file
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	logger.Printf("Configuration loaded. Slack Bot Token Present: %t, Slack App Token Present: %t",
		cfg.SlackBotToken != "", cfg.SlackAppToken != "")
	logger.Printf("Ollama Endpoint: %s, Ollama Model: %s", cfg.OllamaAPIEndpoint, cfg.OllamaModelName)
	logger.Printf("MCP Servers Configured: %d", len(cfg.Servers))

	// Initialize MCP Clients based on configuration from mcp-servers.json only
	mcpClients := make(map[string]*mcp.Client)

	// Only initialize servers from the mcp-servers.json file
	if len(cfg.Servers) == 0 {
		logger.Fatalf("No MCP servers found in configuration file. Please check your mcp-servers.json file.")
	}

	for serverName, serverConf := range cfg.Servers {
		// Skip disabled servers
		if serverConf.Disabled {
			logger.Printf("Skipping disabled MCP server '%s'", serverName)
			continue
		}

		// Determine mode
		mode := strings.ToLower(serverConf.Mode)
		if mode == "" {
			logger.Printf("Warning: No mode specified for server '%s', defaulting to 'stdio'", serverName)
			mode = "stdio"
		}

		var addressOrCommand string

		// Handle different modes appropriately
		if mode == "stdio" {
			// For stdio mode, combine command and args into a single string
			if serverConf.Command != "" {
				// Build the full command string from command and args
				commandParts := []string{serverConf.Command}
				commandParts = append(commandParts, serverConf.Args...)
				addressOrCommand = strings.Join(commandParts, " ")

				logger.Printf("Using command and args for stdio server '%s': %s", serverName, addressOrCommand)
			} else if serverConf.Address != "" {
				// Fallback to address if specified (legacy format)
				addressOrCommand = serverConf.Address
				logger.Printf("Using legacy 'address' field for stdio server '%s'", serverName)
			} else {
				logger.Printf("Skipping MCP server '%s': No command or address specified.", serverName)
				continue
			}
		} else {
			// For HTTP/SSE modes, use the address field
			if serverConf.Address != "" {
				addressOrCommand = serverConf.Address
			} else {
				logger.Printf("Skipping MCP server '%s': No address specified for %s mode.", serverName, mode)
				continue
			}
		}

		// Create a dedicated logger for this MCP client
		mcpLogger := log.New(os.Stdout,
			fmt.Sprintf("mcp-%s: ", strings.ToLower(serverName)),
			logFlags)

		logger.Printf("Initializing MCP client '%s' in %s mode with: %s",
			serverName, mode, addressOrCommand)

		// Create client with appropriate mode and address/command
		mcpClient, err := mcp.NewClient(mode, addressOrCommand, mcpLogger)
		if err != nil {
			logger.Fatalf("Failed to create MCP client '%s': %v", serverName, err)
		}

		// Set environment variables if specified in the schema
		if len(serverConf.Env) > 0 && mode == "stdio" {
			logger.Printf("Setting environment variables for MCP client '%s'", serverName)
			mcpClient.SetEnvironment(serverConf.Env)
		}

		// Try to initialize the client but don't fail if it doesn't initialize immediately
		// This allows the server to start up in the background
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		logger.Printf("Attempting to initialize MCP client '%s'...", serverName)
		if err := mcpClient.Initialize(ctx); err != nil {
			logger.Printf("Warning: Failed to initialize MCP client '%s': %v", serverName, err)
			logger.Printf("Will continue anyway as the client may initialize later when used")
		} else {
			logger.Printf("MCP client '%s' successfully initialized", serverName)
		}

		mcpClients[serverName] = mcpClient

		// Ensure client is closed on exit
		defer func(name string, client *mcp.Client) {
			logger.Printf("Closing MCP client: %s", name)
			client.Close()
		}(serverName, mcpClient)
	}

	// Verify that at least one MCP client was created
	if len(mcpClients) == 0 {
		logger.Fatalf("No MCP clients were initialized. Please check your mcp-servers.json configuration.")
	}

	logger.Printf("Successfully initialized %d MCP clients", len(mcpClients))

	// *** Explicitly discover tools AFTER initialization ***
	allDiscoveredTools := make(map[string]string) // Map: toolName -> serverName
	logger.Println("Attempting to discover tools from initialized clients...")
	for serverName, client := range mcpClients {
		logger.Printf("Discovering tools from MCP server '%s'...", serverName)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Timeout for tool discovery
		tools, err := client.GetAvailableTools(ctx)
		cancel()
		if err != nil {
			logger.Printf("Warning: Failed to retrieve available tools from MCP server '%s': %v", serverName, err)
			continue // Skip this server if tool discovery fails
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
	logger.Printf("Total unique discovered tools across all servers: %d", len(allDiscoveredTools))

	// Initialize Slack Bot Client
	slackLogger := log.New(os.Stdout, "slack: ", logFlags)
	client, err := slackbot.NewClient(
		cfg.SlackBotToken,
		cfg.SlackAppToken,
		slackLogger,
		mcpClients,
		allDiscoveredTools, // Pass discovered tools directly to the client/bridge
		cfg.OllamaAPIEndpoint,
		cfg.OllamaModelName,
	)
	if err != nil {
		logger.Fatalf("Failed to initialize Slack client: %v", err)
	}

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
	// MCP client closures are handled by defer statements above
}
