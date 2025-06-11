// Package mcp provides MCP client and server implementations
package mcp

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	customErrors "github.com/tuannvm/slack-mcp-client/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

// Client provides an interface for interacting with an MCP server.
// It handles tool discovery and execution of tool calls.
type Client struct {
	logger      *logging.Logger
	client      client.MCPClient
	serverAddr  string
	initialized bool // Track if the client has been successfully initialized

	cancel    context.CancelFunc // Cancel the MCP client
	closeOnce sync.Once          // Ensures close logic runs only once
	closeMu   sync.Mutex         // Protects access during close
}

// NewClient creates a new MCP client handler.
// For stdio mode, addressOrCommand should be the command path, and args should be provided.
// For http/sse modes, addressOrCommand is the URL, and args is ignored.
func NewClient(mode, addressOrCommand string, args []string, env map[string]string, stdLogger *logging.Logger) (*Client, error) {
	// Determine log level from environment variable
	logLevel := logging.LevelInfo // Default to INFO
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		logLevel = logging.ParseLevel(envLevel)
	}

	// Create a structured logger for the MCP client
	mcpLogger := logging.New("mcp-client", logLevel)

	mcpLogger.InfoKV("Creating new MCP client", "mode", mode)

	// Create underlying MCP client based on mode
	modeLower := strings.ToLower(mode)
	var mcpClient *client.Client
	var err error
	switch modeLower {
	case "stdio":
		// Build environment slice
		processEnv := os.Environ()
		envMap := make(map[string]string)
		for _, e := range processEnv {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				envMap[parts[0]] = parts[1]
			}
		}
		for k, v := range env {
			envMap[k] = v
		}
		finalEnv := make([]string, 0, len(envMap))
		for k, v := range envMap {
			finalEnv = append(finalEnv, fmt.Sprintf("%s=%s", k, v))
		}
		mcpClient, err = client.NewStdioMCPClient(addressOrCommand, finalEnv, args...)
	case "http", "sse":
		mcpClient, err = client.NewSSEMCPClient(addressOrCommand)
	default:
		return nil, customErrors.NewMCPError("invalid_mode", fmt.Sprintf("Unsupported MCP mode: %s", mode))
	}
	if err != nil {
		return nil, customErrors.WrapMCPError(err, "client_creation", fmt.Sprintf("Failed to create MCP client for %s", addressOrCommand))
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := mcpClient.Start(ctx); err != nil {
		cancel()
		return nil, customErrors.WrapMCPError(err, "client_creation", fmt.Sprintf("Failed to start MCP client for %s", addressOrCommand))
	}

	// Create the wrapper client
	wrapperClient := &Client{
		logger:      mcpLogger,
		client:      mcpClient,
		serverAddr:  addressOrCommand,
		initialized: false,
		cancel:      cancel,
	}

	return wrapperClient, nil
}

// StartListener connects to the MCP server and listens for events.
// This should be run in a goroutine.
func (c *Client) StartListener(_ context.Context) error { // nolint:revive // Using underscore for unused parameter
	c.logger.InfoKV("Starting event listener", "server", c.serverAddr)

	// The official MCP Go client handles the connection and event listening
	// This is a no-op for now as the client library handles this internally
	return nil
}

// Initialize initializes the MCP client by connecting to the server and discovering tools.
func (c *Client) Initialize(ctx context.Context) error {
	if c.client == nil {
		return customErrors.NewMCPError("client_nil", "MCP client is nil")
	}

	// Check if already initialized
	if c.initialized {
		c.logger.DebugKV("Client already initialized, skipping Initialize call", "server", c.serverAddr)
		return nil
	}

	c.logger.InfoKV("Attempting to initialize MCP client", "server", c.serverAddr)

	var initErr error
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION

	// Call Initialize on the underlying MCP client
	_, initErr = c.client.Initialize(ctx, initReq)

	// Handle the result
	if initErr != nil {
		c.logger.ErrorKV("MCP client initialization failed", "server", c.serverAddr, "error", initErr)

	}

	c.logger.InfoKV("Initialize request successful", "server", c.serverAddr)
	c.initialized = true // Set flag ONLY on success
	return nil
}

// CallTool delegates the tool call to the official MCP client.
func (c *Client) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	if c.client == nil {
		return "", customErrors.NewMCPError("client_nil", "MCP client reference is nil")
	}

	// Ensure the client is initialized before making any tool calls.
	if !c.initialized {
		c.logger.Warn("Client not initialized, attempting to initialize before tool call")
		if err := c.Initialize(ctx); err != nil {
			c.logger.ErrorKV("Failed to initialize client", "error", err)
			return "", customErrors.WrapMCPError(err, "client_not_initialized", "MCP client not initialized before tool call")
		}
	}

	c.logger.InfoKV("Calling tool", "tool", toolName, "server", c.serverAddr)

	// Create a proper CallToolRequest
	req := mcp.CallToolRequest{}
	// Set the tool name and arguments in the params field
	req.Params.Name = toolName
	req.Params.Arguments = args

	// Call the tool using the official client
	result, err := c.client.CallTool(ctx, req)
	if err != nil {
		c.logger.ErrorKV("Tool call failed", "tool", toolName, "error", err)
		return "", customErrors.WrapMCPError(err, "tool_call_failed", fmt.Sprintf("Failed to call tool '%s'", toolName))
	}

	// Check if the tool call resulted in an error
	if result.IsError {
		// Attempt to extract error message if available
		var errMsgText string
		if len(result.Content) > 0 {
			if textContent, ok := result.Content[0].(mcp.TextContent); ok {
				errMsgText = textContent.Text
			} else {
				errMsgText = "Unknown error"
			}
		} else {
			errMsgText = "Unknown error"
		}

		c.logger.ErrorKV("Tool execution error", "tool", toolName, "error", errMsgText)
		return "", customErrors.NewMCPError("tool_execution_error",
			fmt.Sprintf("Tool '%s' returned an error", toolName)).WithData("error_message", errMsgText)
	}

	// Extract text content from the result
	var resultText string
	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			resultText += textContent.Text
		}
	}

	c.logger.InfoKV("Tool call successful", "tool", toolName)
	return resultText, nil
}

// GetAvailableTools retrieves the list of available tools from the MCP server.
// It now returns the full ListToolsResult to include schema information.
func (c *Client) GetAvailableTools(ctx context.Context) (*mcp.ListToolsResult, error) {
	c.logger.InfoKV("Discovering tools", "server", c.serverAddr)

	// Ensure the client is initialized. Attempt once with a longer timeout if not.
	if !c.initialized {
		// Attempt to initialize with a timeout
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		c.logger.DebugKV("Sending initialize request", "server", c.serverAddr)
		if err := c.Initialize(ctx); err != nil {
			c.logger.ErrorKV("Failed to initialize client", "error", err)
			return nil, customErrors.WrapMCPError(err, "client_not_initialized", "MCP client not initialized before getting available tools")
		}
	}

	// Define the necessary interface
	type toolLister interface {
		ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error)
	}

	// Check if the client implements the ListTools method
	if lister, ok := c.client.(toolLister); ok {
		c.logger.DebugKV("Client implements toolLister", "server", c.serverAddr)
		req := mcp.ListToolsRequest{}

		listResult, err := lister.ListTools(ctx, req)

		// Simple retry logic if first attempt fails
		if err != nil {
			c.logger.WarnKV("First ListTools attempt failed", "server", c.serverAddr, "error", err)
			pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
			if pingErr := c.client.Ping(pingCtx); pingErr != nil {
				pingCancel()
				c.logger.WarnKV("Ping also failed", "server", c.serverAddr, "error", pingErr)
			} else {
				pingCancel()
				c.logger.InfoKV("Ping succeeded, retrying ListTools", "server", c.serverAddr)
				listResult, err = lister.ListTools(ctx, req) // Retry the call
			}
		}

		// Process result if successful
		if err == nil {
			if listResult == nil {
				c.logger.WarnKV("ListTools returned nil result", "server", c.serverAddr)
				// Return an empty result struct instead of just nil
				return &mcp.ListToolsResult{}, nil
			}
			// Log discovered tools
			for _, tool := range listResult.Tools {
				c.logger.DebugKV("Discovered tool", "name", tool.Name, "description", tool.Description)
			}
			c.logger.InfoKV("Tool discovery completed", "server", c.serverAddr, "tools", len(listResult.Tools))
			return listResult, nil // <-- Return the full result struct
		}

		// If we got here, ListTools failed even after potential retry
		c.logger.ErrorKV("Tool discovery failed", "server", c.serverAddr, "error", err)
		return nil, customErrors.WrapMCPError(err, "tool_discovery_failed", fmt.Sprintf("Failed to discover tools for %s", c.serverAddr))
	}

	// --- Fallback if client type does not implement ListTools ---
	c.logger.WarnKV("Client does not implement toolLister", "server", c.serverAddr)
	// Return nil struct and error
	return nil, customErrors.NewMCPError("unsupported_operation", fmt.Sprintf("Client type %T does not support tool discovery", c.client))
}

// GetClientMapKeys extracts the keys (server names) from a map of MCP clients.
func GetClientMapKeys(m map[string]*Client) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Close closes the MCP client connection.
func (c *Client) Close() error {
	c.logger.InfoKV("Closing MCP client", "server", c.serverAddr)

	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	var closeErr error
	// Use sync.Once to ensure we only run the close logic once
	c.closeOnce.Do(func() {
		c.logger.InfoKV("Closing MCP client resources", "server", c.serverAddr)

		// Close the underlying library client if possible
		if closer, ok := c.client.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				c.logger.ErrorKV("Error closing MCP client", "error", err)
				closeErr = err
			}
		} else {
			c.logger.WarnKV("Underlying client type does not implement io.Closer", "server", c.serverAddr)
		}

		if c.cancel != nil {
			c.cancel()
		}

		c.logger.InfoKV("Finished closing MCP client", "server", c.serverAddr)
	})
	return closeErr
}

// PrintEnvironment logs all environment variables for debugging
func (c *Client) PrintEnvironment() {
	c.logger.InfoKV("Environment variables for MCP client", "server", c.serverAddr)

	// Print relevant env vars that might affect MCP clients
	relevantVars := []string{
		"TRINO_HOST", "TRINO_PORT", "TRINO_USER", "TRINO_PASSWORD",
		"MCP_DEBUG", "MCP_MODE", "MCP_SERVER_PORT",
	}

	for _, env := range relevantVars {
		val := os.Getenv(env)
		if val != "" {
			c.logger.DebugKV("Environment variable", "name", env, "value", val)
		} else {
			c.logger.DebugKV("Environment variable not set", "name", env)
		}
	}

	// For stdio mode, check if command exists
	if c.serverAddr == "http://localhost:8080" {
		c.logger.DebugKV("Checking mcp-trino command", "server", c.serverAddr)
		path, err := exec.LookPath("mcp-trino")
		if err != nil {
			c.logger.WarnKV("mcp-trino not found in PATH", "error", err)
		} else {
			c.logger.DebugKV("mcp-trino found", "path", path)
		}
	}
}
