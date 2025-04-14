package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"strings"

	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
	slackbot "github.com/tuannvm/slack-mcp-client/internal/slack"
)

var (
	// Define command-line flags
	configFile = flag.String("config", "", "Path to the MCP server configuration JSON file")
)

func main() {
	flag.Parse()

	logger := log.New(os.Stdout, "slack-mcp-client: ", log.LstdFlags|log.Lshortfile)

	// Load configuration from environment variables and optional config file
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	logger.Printf("Configuration loaded. Slack Bot Token Present: %t, Slack App Token Present: %t", cfg.SlackBotToken != "", cfg.SlackAppToken != "")
	logger.Printf("Ollama Endpoint: %s, Ollama Model: %s", cfg.OllamaAPIEndpoint, cfg.OllamaModelName)

	// Initialize MCP Clients based on configuration
	mcpClients := make(map[string]*mcp.Client)
	for _, serverConf := range cfg.Servers {
		var mode, addressOrCommand string
		serverName := serverConf.Name

		if serverName == "" {
			logger.Println("Skipping MCP server with empty name.")
			continue
		}

		// Determine mode and address/command
		mode = strings.ToLower(serverConf.Mode)
		addressOrCommand = serverConf.Address

		if addressOrCommand == "" {
			logger.Printf("Skipping MCP server '%s': No address/command specified.", serverName)
			continue
		}

		if mode == "" {
			// Attempt to infer mode if not provided (basic inference)
			if strings.HasPrefix(addressOrCommand, "http://") || strings.HasPrefix(addressOrCommand, "https://") {
				mode = "http"
				logger.Printf("Warning: Mode not specified for server '%s', inferring '%s' based on address.", serverName, mode)
			} else {
				// Assume stdio if it's not a URL - This might be fragile
				mode = "stdio"
				logger.Printf("Warning: Mode not specified for server '%s', assuming '%s'. Consider specifying mode explicitly.", serverName, mode)
			}
		}

		logger.Printf("Initializing MCP client '%s' in %s mode with address/command: %s", serverName, mode, addressOrCommand)

		mcpClient, err := mcp.NewClient(mode, addressOrCommand, logger)
		if err != nil {
			logger.Fatalf("Failed to create MCP client '%s': %v", serverName, err)
		}
		mcpClients[serverName] = mcpClient

		// Ensure client is closed on exit
		defer func(name string, client *mcp.Client) {
			logger.Printf("Closing MCP client: %s", name)
			client.Close()
		}(serverName, mcpClient)
	}

	// Initialize Slack Bot Client
	client, err := slackbot.NewClient(
		cfg.SlackBotToken,
		cfg.SlackAppToken,
		logger,
		mcpClients,
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

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Println("Shutting down...")
	// MCP client closures are handled by defer
}
