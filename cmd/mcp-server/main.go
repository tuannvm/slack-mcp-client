package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
)

func main() {
	logger := log.New(os.Stdout, "mcp-server: ", log.LstdFlags|log.Lshortfile)

	// Create a basic config for the server
	cfg := &config.Config{
		OllamaAPIEndpoint: "http://localhost:11434",
		OllamaModelName:   "phi4-mini",
	}

	// Create and initialize the MCP server
	server, err := mcp.NewServer(cfg, logger)
	if err != nil {
		logger.Fatalf("Failed to create MCP server: %v", err)
	}

	// Create a context that will be canceled on SIGINT or SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Start the server
	if err := server.Run(ctx, ":8080"); err != nil {
		logger.Fatalf("Server error: %v", err)
	}
}