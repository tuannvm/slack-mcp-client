package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
	"github.com/tuannvm/slack-mcp-client/internal/slack"
)

func main() {
	// Setup logger
	logger := log.New(os.Stdout, "slack-mcp-client: ", log.LstdFlags|log.Lshortfile)

	logger.Println("Starting Slack MCP Client...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("Error loading configuration: %v", err)
	}

	logger.Println("Configuration loaded successfully.")
	logger.Printf("Log Level: %s", cfg.LogLevel) // TODO: Implement proper log level handling

	// Initialize MCP Server
	mcpServer, err := mcp.NewServer(cfg, logger)
	if err != nil {
		logger.Fatalf("Error initializing MCP server: %v", err)
	}
	logger.Println("MCP server initialized.")

	// Setup graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(3) // One for Slack client, one for MCP server, one for MCP client listener

	// Start MCP Server in a goroutine
	go func() {
		defer wg.Done()
		if err := mcpServer.Run(ctx, cfg.MCPServerListenAddress); err != nil {
			logger.Printf("MCP Server error: %v", err)
			stop() // Trigger shutdown if server fails unexpectedly
		}
	}()

	logger.Println("Allowing MCP server a moment to start...")
	time.Sleep(200 * time.Millisecond)

	// Initialize MCP Client
	mcpClient, err := mcp.NewClient(cfg, logger)
	if err != nil {
		logger.Fatalf("Error initializing MCP client: %v", err)
	}
	logger.Println("MCP client initialized.")

	// Start MCP Client SSE Listener in a goroutine
	go func() {
		defer wg.Done()
		if err := mcpClient.StartListener(ctx); err != nil {
			logger.Printf("MCP client listener error: %v", err)
			stop() // Signal shutdown on listener error
		}
	}()

	// Initialize Slack Client
	slackClient, err := slack.NewClient(cfg, logger, mcpClient)
	if err != nil {
		logger.Fatalf("Failed to create Slack client: %v", err)
	}
	logger.Println("Slack client initialized.")

	// Start Slack Client in a goroutine
	go func() {
		defer wg.Done()
		// Note: slackClient.Run() blocks until connection error or closure.
		// It doesn't directly use the context for shutdown in its main loop,
		// but shutdown is triggered by the OS signal handling closing the socket.
		if err := slackClient.Run(); err != nil {
			logger.Printf("Slack Client error: %v", err)
			stop() // Trigger shutdown if client fails unexpectedly
		}
	}()

	logger.Println("Application started. Press Ctrl+C to shutdown.")

	// Wait for shutdown signal
	<-ctx.Done()
	logger.Println("Shutdown signal received...")

	// Perform graceful shutdown
	// Give some time for ongoing operations to complete

	// Close MCP client
	if mcpClient != nil {
		mcpClient.Close()
	}

	// TODO: Add graceful shutdown for Slack client if needed
	// TODO: Add graceful shutdown for MCP server if needed (httpServer.Shutdown)

	logger.Println("Shutdown complete.")

	// Wait for goroutines to finish
	wg.Wait()
	logger.Println("Slack MCP Client shut down gracefully.")
}
