package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

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

// Initialize explicitly initializes the MCP client.
// This should be called before making any tool calls.
func (c *Client) Initialize(ctx context.Context) error {
	if c.client == nil {
		return fmt.Errorf("MCP client is nil")
	}

	c.log.Printf("Initializing MCP client for %s...", c.serverAddr)

	// Different initialization based on client type
	switch {
	case strings.HasPrefix(c.serverAddr, "http"):
		// For HTTP/SSE mode, we need a different approach
		// The SSE client doesn't support echo tool, so we'll use a direct HTTP request
		// to check if the server is available
		c.log.Printf("HTTP/SSE mode detected, checking server availability")
		
		// Create HTTP client with timeout
		httpClient := &http.Client{Timeout: 5 * time.Second}
		
		// Create a simple JSON-RPC request for the echo tool
		reqBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "callTool",
			"params": map[string]interface{}{
				"name": "echo",
				"arguments": map[string]interface{}{
					"message": "ping",
				},
			},
		}
		
		// Marshal the request to JSON
		reqJSON, err := json.Marshal(reqBody)
		if err != nil {
			c.log.Printf("Error marshaling JSON-RPC request: %v", err)
			return fmt.Errorf("error marshaling JSON-RPC request: %w", err)
		}
		
		// Send the request to the server
		resp, err := httpClient.Post(c.serverAddr, "application/json", bytes.NewBuffer(reqJSON))
		if err != nil {
			c.log.Printf("Error connecting to MCP server: %v", err)
			return fmt.Errorf("error connecting to MCP server: %w", err)
		}
		defer resp.Body.Close()
		
		// Check the response status
		if resp.StatusCode != http.StatusOK {
			c.log.Printf("MCP server returned non-OK status: %s", resp.Status)
			return fmt.Errorf("MCP server returned non-OK status: %s", resp.Status)
		}
		
		c.log.Printf("HTTP/SSE MCP client successfully initialized")
		return nil
		
	default:
		// For stdio mode and other modes, use the original approach
		c.log.Printf("Using standard initialization for mode: %s", c.serverAddr)
		
		// Create a simple ping request
		req := mcp.CallToolRequest{}
		req.Params.Name = "echo"
		req.Params.Arguments = map[string]interface{}{
			"message": "ping",
		}
		
		// Try to call the tool with a timeout
		ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		
		resultCh := make(chan error, 1)
		go func() {
			_, err := c.client.CallTool(ctxWithTimeout, req)
			resultCh <- err
		}()
		
		// Wait for the result or timeout
		select {
		case err := <-resultCh:
			if err != nil {
				c.log.Printf("Failed to initialize MCP client: %v", err)
				return fmt.Errorf("failed to initialize MCP client: %w", err)
			}
			c.log.Printf("MCP client successfully initialized")
			return nil
		case <-ctxWithTimeout.Done():
			c.log.Printf("Timeout initializing MCP client")
			return fmt.Errorf("timeout initializing MCP client")
		}
	}
}

// CallTool delegates the tool call to the official MCP client.
func (c *Client) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("MCP client not initialized")
	}
	
	// Ensure the client is initialized before making any tool calls
	if err := c.Initialize(ctx); err != nil {
		c.log.Printf("Failed to initialize MCP client: %v", err)
		return "", fmt.Errorf("failed to initialize MCP client: %w", err)
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
