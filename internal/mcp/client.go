package mcp

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Client handles interactions with the MCP server using the official MCP Go client.
type Client struct {
	log        *log.Logger
	client     client.MCPClient
	serverAddr string
}

// NewClient creates a new MCP client handler with the configured transport mode.
func NewClient(mode, address string, logger *log.Logger) (*Client, error) {
	if logger == nil {
		logger = log.New(os.Stderr, "MCP_CLIENT: ", log.LstdFlags|log.Lshortfile)
	}

	logger.Printf("Creating new MCP client with mode=%s, address=%s", mode, address)

	var mcpClient client.MCPClient
	var err error
	
	// Create client based on mode
	switch strings.ToLower(mode) {
	case "http":
		// HTTP mode is handled via SSE client in the official library
		logger.Printf("Using SSE client for HTTP mode")
		mcpClient, err = client.NewSSEMCPClient(address)
		if err != nil {
			return nil, fmt.Errorf("failed to create SSE client: %w", err)
		}
		
	case "sse":
		// Create SSE client
		logger.Printf("Using SSE client for SSE mode")
		mcpClient, err = client.NewSSEMCPClient(address)
		if err != nil {
			return nil, fmt.Errorf("failed to create SSE client: %w", err)
		}
		
	case "stdio":
		// For stdio, parse the command and args
		logger.Printf("Using stdio client for stdio mode")
		parts := strings.Fields(address)
		if len(parts) == 0 {
			return nil, fmt.Errorf("invalid stdio command: %s", address)
		}
		
		command := parts[0]
		args := []string{}
		if len(parts) > 1 {
			args = parts[1:]
		}
		
		// Create stdio client
		mcpClient, err = client.NewStdioMCPClient(command, os.Environ(), args...)
		if err != nil {
			return nil, fmt.Errorf("failed to create stdio client: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported MCP mode: %s. Must be 'stdio', 'sse', or 'http'", mode)
	}

	// Create and return the client
	return &Client{
		log:        logger,
		client:     mcpClient,
		serverAddr: address,
	}, nil
}

// StartListener connects to the MCP server and listens for events.
// This should be run in a goroutine.
func (c *Client) StartListener(ctx context.Context) error {
	c.log.Printf("Starting event listener for %s...", c.serverAddr)

	// The official MCP Go client handles the connection and event listening
	// This is a no-op for now as the client library handles this internally
	return nil
}

// CallTool delegates the tool call to the official MCP client.
func (c *Client) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("MCP client not initialized")
	}
	
	c.log.Printf("Calling tool '%s' with args: %v", toolName, args)
	
	// Create a proper CallToolRequest
	req := mcp.CallToolRequest{}
	// Set the tool name and arguments in the params field
	req.Params.Name = toolName
	req.Params.Arguments = args
	
	// Call the tool using the official client
	result, err := c.client.CallTool(ctx, req)
	if err != nil {
		c.log.Printf("Error calling tool '%s': %v", toolName, err)
		return "", fmt.Errorf("failed to call tool '%s': %w", toolName, err)
	}
	
	// Check if the tool call resulted in an error
	if result.IsError {
		c.log.Printf("Tool '%s' returned an error", toolName)
		return "", fmt.Errorf("tool '%s' execution failed", toolName)
	}
	
	// Extract text content from the result
	var resultText string
	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			resultText += textContent.Text
		}
	}
	
	c.log.Printf("Tool '%s' call successful", toolName)
	return resultText, nil
}

// Close cleans up the MCP client resources.
func (c *Client) Close() {
	c.log.Println("Closing MCP client resources...")
	
	// Close the client if it implements io.Closer
	if closer, ok := c.client.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			c.log.Printf("Error closing MCP client: %v", err)
		}
	}
}
