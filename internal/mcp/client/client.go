// Package client provides an implementation of the Model Context Protocol client
package client

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	customErrors "github.com/tuannvm/slack-mcp-client/internal/common/errors"
)

// Client provides an interface for interacting with an MCP server.
// It handles tool discovery and execution of tool calls.
type Client struct {
	log         *log.Logger
	client      client.MCPClient
	serverAddr  string
	initialized bool // Track if the client has been successfully initialized

	closeOnce sync.Once  // Ensures close logic runs only once
	closeMu   sync.Mutex // Protects access during close
}

// NewClient creates a new MCP client handler.
// For stdio mode, addressOrCommand should be the command path, and args should be provided.
// For http/sse modes, addressOrCommand is the URL, and args is ignored.
func NewClient(mode, addressOrCommand string, args []string, env map[string]string, logger *log.Logger) (*Client, error) {
	if logger == nil {
		logger = log.New(os.Stderr, "MCP_CLIENT: ", log.LstdFlags|log.Lshortfile)
	}

	logger.Printf("Creating new MCP client: mode='%s'", mode)

	// Create underlying MCP client based on mode
	modeLower := strings.ToLower(mode)
	var mcpClient client.MCPClient
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

	// Create the wrapper client
	wrapperClient := &Client{
		log:        logger,
		client:     mcpClient,
		serverAddr: addressOrCommand,
	}

	return wrapperClient, nil
}

// StartListener connects to the MCP server and listens for events.
// This should be run in a goroutine.
func (c *Client) StartListener(_ context.Context) error { // nolint:revive // Using underscore for unused parameter
	c.log.Printf("Starting event listener for %s...", c.serverAddr)

	// The official MCP Go client handles the connection and event listening
	// This is a no-op for now as the client library handles this internally
	return nil
}

// Initialize explicitly initializes the MCP client.
// This should be called before making any tool calls.
func (c *Client) Initialize(ctx context.Context) error {
	if c.client == nil {
		return customErrors.NewMCPError("client_nil", "MCP client is nil")
	}

	// If already successfully initialized, skip
	if c.initialized {
		c.log.Printf("DEBUG: Client for %s already initialized, skipping Initialize call", c.serverAddr)
		return nil
	}

	c.log.Printf("Attempting to initialize MCP client for %s...", c.serverAddr)

	var initErr error
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION

	// Call Initialize on the underlying MCP client
	_, initErr = c.client.Initialize(ctx, initReq)

	// Handle the result
	if initErr != nil {
		c.log.Printf("ERROR: MCP client initialization failed for %s: %v", c.serverAddr, initErr)

		// Specific error hints
		if strings.Contains(initErr.Error(), "endpoint not received") {
			c.log.Printf("  Hint: Server did not respond as an SSE endpoint. Check URL and mode (should be 'sse' with /sse suffix, or maybe 'http' without suffix?).")
		} else if strings.Contains(initErr.Error(), "connection refused") {
			c.log.Printf("  Hint: Connection refused. Ensure the server process/container is running at %s.", c.serverAddr)
		} else if strings.Contains(initErr.Error(), "file already closed") {
			c.log.Printf("  Hint: Stdio process may have exited prematurely. Check command, args, and ensure required env vars are set correctly in mcp-servers.json.")
		}

		return customErrors.WrapMCPError(initErr, "initialization_failed", "MCP client initialization failed")
	}

	c.log.Printf("MCP client for %s successfully initialized.", c.serverAddr)
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
		c.log.Printf("DEBUG: Client for %s not initialized before CallTool, attempting Initialize...", c.serverAddr)
		initCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // 20s timeout for initialization
		if err := c.Initialize(initCtx); err != nil {
			cancel()
			c.log.Printf("ERROR: Failed to initialize MCP client before CallTool: %v", err)
			return "", customErrors.WrapMCPError(err, "init_before_call", "Failed to initialize MCP client before tool call")
		}
		cancel()
		c.log.Printf("DEBUG: Initialization successful during CallTool attempt.") // Added log
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
		// Optional: add transport-specific hints here
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

		c.log.Printf("Error: Tool '%s' returned an error: %s", toolName, errMsgText)
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

	c.log.Printf("Tool '%s' call successful", toolName)
	return resultText, nil
}

// GetAvailableTools retrieves the list of available tools from the MCP server.
// It now returns the full ListToolsResult to include schema information.
func (c *Client) GetAvailableTools(ctx context.Context) (*mcp.ListToolsResult, error) {
	c.log.Printf("Attempting to get available tools from %s", c.serverAddr)

	// Ensure the client is initialized. Attempt once with a longer timeout if not.
	if !c.initialized {
		c.log.Printf("DEBUG: Client for %s not initialized before GetAvailableTools, attempting Initialize...", c.serverAddr)
		initCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // 20s timeout for initialization
		if err := c.Initialize(initCtx); err != nil {
			cancel()
			c.log.Printf("ERROR: Initialization attempt failed during GetAvailableTools: %v", err)
			return nil, customErrors.WrapMCPError(err, "init_before_list", "Client not initialized before getting available tools")
		}
		cancel()
		c.log.Printf("DEBUG: Initialization successful during GetAvailableTools attempt.")
	} else {
		c.log.Printf("DEBUG: Client %s already initialized.", c.serverAddr)
	}

	// Define the necessary interface
	type toolLister interface {
		ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error)
	}

	// Check if the client implements the ListTools method
	if lister, ok := c.client.(toolLister); ok {
		c.log.Printf("DEBUG: Client (%T) implements toolLister. Calling native ListTools method for %s", c.client, c.serverAddr)
		req := mcp.ListToolsRequest{}

		listResult, err := lister.ListTools(ctx, req)

		// Simple retry logic if first attempt fails
		if err != nil {
			c.log.Printf("DEBUG: First ListTools attempt failed for %s: %v. Trying Ping and retrying...", c.serverAddr, err)
			pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
			if pingErr := c.client.Ping(pingCtx); pingErr != nil {
				pingCancel()
				c.log.Printf("DEBUG: Ping also failed for %s: %v. Not retrying ListTools.", c.serverAddr, pingErr)
			} else {
				pingCancel()
				c.log.Printf("DEBUG: Ping succeeded for %s, retrying ListTools", c.serverAddr)
				listResult, err = lister.ListTools(ctx, req) // Retry the call
			}
		}

		// Process result if successful
		if err == nil {
			if listResult == nil {
				c.log.Printf("Warning: ListTools for %s returned nil result despite nil error", c.serverAddr)
				// Return an empty result struct instead of just nil
				return &mcp.ListToolsResult{}, nil
			}
			// Extract tool names just for logging here
			toolNames := make([]string, 0, len(listResult.Tools))
			for _, toolDef := range listResult.Tools {
				toolNames = append(toolNames, toolDef.Name)
			}
			c.log.Printf("Native ListTools for %s returned %d tools: %v", c.serverAddr, len(toolNames), toolNames)
			return listResult, nil // <-- Return the full result struct
		}

		// If we got here, ListTools failed even after potential retry
		c.log.Printf("ERROR: Failed to get tools via native ListTools method for %s: %v", c.serverAddr, err)
		return nil, customErrors.WrapMCPError(err, "tool_discovery_failed", "Failed to get tools via native ListTools method")
	}

	// --- Fallback if client type does not implement ListTools ---
	c.log.Printf("Error: Underlying MCP client (%T) for %s does not implement the toolLister interface. Cannot discover tools.", c.client, c.serverAddr)
	// Return nil struct and error
	return nil, customErrors.NewMCPError("unsupported_operation", fmt.Sprintf("Client type %T does not support tool discovery", c.client))
}

// Close cleans up the MCP client resources.
func (c *Client) Close() {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	c.closeOnce.Do(func() {
		c.log.Printf("Closing MCP client resources for %s...", c.serverAddr)

		// Close the underlying library client if possible
		if closer, ok := c.client.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				// Log underlying close error, especially if it's 'file already closed'
				c.log.Printf("Error closing underlying MCP client for %s: %v", c.serverAddr, err)
			}
		} else {
			c.log.Printf("Underlying client type %T does not implement io.Closer", c.client)
		}

		c.log.Printf("Finished closing MCP client for %s.", c.serverAddr)
	})
}

// PrintEnvironment logs all environment variables for debugging
func (c *Client) PrintEnvironment() {
	c.log.Printf("DEBUGGING: Environment variables for this process:")

	// Print relevant env vars that might affect MCP clients
	relevantVars := []string{
		"TRINO_HOST", "TRINO_PORT", "TRINO_USER", "TRINO_PASSWORD",
		"MCP_DEBUG", "MCP_MODE", "MCP_SERVER_PORT",
	}

	for _, key := range relevantVars {
		value := os.Getenv(key)
		if value != "" {
			// Redact passwords
			if strings.Contains(strings.ToLower(key), "password") {
				c.log.Printf("  %s: <redacted>", key)
			} else {
				c.log.Printf("  %s: %s", key, value)
			}
		} else {
			c.log.Printf("  %s: <not set>", key)
		}
	}

	// For stdio mode, check if command exists
	if strings.Contains(c.serverAddr, "mcp-trino") {
		c.log.Printf("DEBUGGING: Checking mcp-trino command...")
		path, err := exec.LookPath("mcp-trino")
		if err != nil {
			c.log.Printf("  Error: mcp-trino not found in PATH: %v", err)
		} else {
			c.log.Printf("  mcp-trino found at: %s", path)
		}
	}
}

// runNetworkDiagnostics performs basic network checks to diagnose connection issues
//
//nolint:unused // Reserved for future use
func (c *Client) runNetworkDiagnostics(_ context.Context, serverAddr string) error { // nolint:revive // Using underscore for unused parameter
	// Parse URL to extract host and port
	c.log.Printf("Running network diagnostics for %s", serverAddr)

	// Simple check - can we make a HTTP request to the server?
	// This is just a basic check, not a full SSE validation
	c.log.Printf("Attempting HTTP GET to %s (note: this is just a connectivity test, not an SSE test)", serverAddr)

	// This is a simplified version - just log the diagnostic steps
	// In a real implementation, we'd actually make the HTTP request

	c.log.Printf("Diagnostics complete for %s", serverAddr)
	return nil
}
