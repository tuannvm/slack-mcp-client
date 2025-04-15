package mcp

import (
	"context"
	"encoding/json"
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
		// For stdio, carefully parse the command and args to ensure proper splitting
		logger.Printf("Using stdio client for stdio mode with command line: %s", address)

		// Use a more careful approach to split the command line to handle quoted arguments
		var args []string
		var command string

		// Simple approach: try to handle basic quoting
		inQuote := false
		var currentArg strings.Builder
		var quoteChar rune

		// Process the command line character by character
		for _, c := range address {
			switch {
			case (c == '"' || c == '\'') && !inQuote:
				// Start of a quoted section
				inQuote = true
				quoteChar = c
			case c == quoteChar && inQuote:
				// End of a quoted section
				inQuote = false
			case c == ' ' && !inQuote:
				// Space outside quotes - end of an argument
				if currentArg.Len() > 0 {
					args = append(args, currentArg.String())
					currentArg.Reset()
				}
			default:
				// Regular character - add to current argument
				currentArg.WriteRune(c)
			}
		}

		// Add the last argument if there is one
		if currentArg.Len() > 0 {
			args = append(args, currentArg.String())
		}

		// Ensure we have at least a command
		if len(args) == 0 {
			return nil, fmt.Errorf("invalid stdio command: %s", address)
		}

		// First argument is the command
		command = args[0]
		args = args[1:] // Rest are arguments

		// Create stdio client with the command and properly separated args
		logger.Printf("Creating stdio client with command: %s, args: %v", command, args)
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

	// Check if already initialized to avoid redundant calls, if the library supports it easily.
	// For now, we rely on the caller or library's internal state.
	c.log.Printf("Initializing MCP client for %s...", c.serverAddr)

	// Different initialization based on client type
	switch typedClient := c.client.(type) {
	case *client.SSEMCPClient:
		// SSEMCPClient (used for HTTP/SSE) initialization might involve ensuring connection
		// or performing an initial handshake if the library requires it.
		// The library's Initialize method handles this.
		c.log.Printf("DEBUG: Attempting initialization via library's Initialize for SSE/HTTP client")
		initReq := mcp.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		if _, err := typedClient.Initialize(ctx, initReq); err != nil {
			// Log the error but maybe don't fail hard, allow connection retries?
			c.log.Printf("DEBUG: SSE/HTTP client Initialize call failed: %v", err)
			// Returning error might be too strict if connection can recover.
			// For now, let's return it.
			return fmt.Errorf("SSE/HTTP client initialization failed: %w", err)
		}
		c.log.Printf("DEBUG: SSE/HTTP client successfully initialized via library call")
		return nil

	case *client.StdioMCPClient:
		// StdioMCPClient also has an Initialize method for the handshake.
		c.log.Printf("DEBUG: Attempting initialization via library's Initialize for Stdio client")
		initReq := mcp.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		if _, err := typedClient.Initialize(ctx, initReq); err != nil {
			c.log.Printf("DEBUG: Stdio client Initialize call failed: %v", err)
			return fmt.Errorf("Stdio client initialization failed: %w", err)
		}
		// *** REMOVED the call to c.GetAvailableTools here ***
		c.log.Printf("DEBUG: Stdio client successfully initialized via library call")
		return nil
	default:
		// Fallback for unknown client types
		c.log.Printf("Warning: Unknown MCP client type (%T), attempting generic ping for initialization check.", c.client)
		if err := c.client.Ping(ctx); err != nil {
			c.log.Printf("DEBUG: Generic ping failed: %v", err)
			return fmt.Errorf("generic ping initialization check failed: %w", err)
		}
		c.log.Printf("DEBUG: Generic ping successful for unknown client type.")
		return nil
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

// GetAvailableTools retrieves the list of available tools from the MCP server.
func (c *Client) GetAvailableTools(ctx context.Context) ([]string, error) {
	c.log.Printf("Attempting to get available tools from %s", c.serverAddr)

	// Ensure client is initialized before trying to get tools
	if err := c.Initialize(ctx); err != nil {
		c.log.Printf("DEBUG: Failed to retrieve available tools: %v", err)
		c.log.Printf("DEBUG: Will proceed with initialization anyway")
	}

	// *** Interface for clients that implement ListTools directly (preferred) ***
	type toolLister interface {
		ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error)
	}

	if lister, ok := c.client.(toolLister); ok {
		c.log.Println("Using native ListTools method")
		req := mcp.ListToolsRequest{} // Assuming no pagination needed for now
		listResult, err := lister.ListTools(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("failed to get tools via native ListTools method: %w", err)
		}
		toolNames := make([]string, 0, len(listResult.Tools))
		for _, toolDef := range listResult.Tools {
			toolNames = append(toolNames, toolDef.Name)
		}
		c.log.Printf("Native ListTools returned %d tools: %v", len(toolNames), toolNames)
		return toolNames, nil
	}

	// *** Workaround for StdioMCPClient using sendRequest ***
	c.log.Printf("Warning: Underlying MCP client does not implement ListTools natively. Attempting manual 'tools/list' call...")

	// Define an interface for the sendRequest method (based on stdio.go source)
	type requestSender interface {
		sendRequest(ctx context.Context, method string, params interface{}) (*json.RawMessage, error)
	}

	if sender, ok := c.client.(requestSender); ok {
		c.log.Println("Client implements sendRequest. Manually calling 'tools/list'.")
		req := mcp.ListToolsRequest{}
		var params interface{} = req.Params
		if params == nil {
			params = map[string]interface{}{} 
		}

		rawResponse, err := sender.sendRequest(ctx, "tools/list", params)
		if err != nil {
			return nil, fmt.Errorf("manual 'tools/list' call failed: %w", err)
		}
		if rawResponse == nil {
			return nil, fmt.Errorf("manual 'tools/list' call returned nil response")
		}

		var listResult mcp.ListToolsResult
		if err := json.Unmarshal(*rawResponse, &listResult); err != nil {
			return nil, fmt.Errorf("failed to unmarshal manual 'tools/list' response: %w. Response: %s", err, string(*rawResponse))
		}

		toolNames := make([]string, 0, len(listResult.Tools))
		for _, toolDef := range listResult.Tools {
			toolNames = append(toolNames, toolDef.Name)
		}
		c.log.Printf("Manual 'tools/list' call returned %d tools: %v", len(toolNames), toolNames)
		return toolNames, nil
	}

	// --- If neither interface is met --- 
	c.log.Printf("Error: Underlying MCP client (%T) implements neither ListTools nor the assumed sendRequest interface. Cannot discover tools.", c.client)
	return []string{}, fmt.Errorf("underlying client %T cannot discover tools via ListTools or sendRequest workaround", c.client)
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

// SetEnvironment sets environment variables for the MCP client (only effective for stdio mode).
func (c *Client) SetEnvironment(env map[string]string) {
	if env == nil || len(env) == 0 {
		return
	}

	// This method only has an effect if the client is using stdio mode
	_, ok := c.client.(*client.StdioMCPClient)
	if !ok {
		c.log.Printf("Warning: SetEnvironment called on non-stdio client - ignoring")
		return
	}

	// For stdio, set the environment variables directly on the client
	// Since the internal MCP client doesn't expose a way to set environment after creation,
	// we'll log this limitation for now
	c.log.Printf("Note: Environment variables can only be set when creating a stdio client")
}

// SetArguments sets command-line arguments for the MCP client (only effective for stdio mode).
func (c *Client) SetArguments(args []string) {
	if args == nil || len(args) == 0 {
		return
	}

	// This method only has an effect if the client is using stdio mode
	_, ok := c.client.(*client.StdioMCPClient)
	if !ok {
		c.log.Printf("Warning: SetArguments called on non-stdio client - ignoring")
		return
	}

	// For stdio, set the arguments directly on the client
	// Since the internal MCP client doesn't expose a way to set arguments after creation,
	// we'll log this limitation for now
	c.log.Printf("Note: Arguments can only be set when creating a stdio client")

	// For future implementations, if the client library is updated to support setting args after creation:
	// stdioClient.SetArgs(args)
}
