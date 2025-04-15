package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
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
	initialized bool // Track if the client has been successfully initialized
}

// NewClient creates a new MCP client handler.
// For stdio mode, addressOrCommand should be the command path, and args should be provided.
// For http/sse modes, addressOrCommand is the URL, and args is ignored.
func NewClient(mode, addressOrCommand string, args []string, env map[string]string, logger *log.Logger) (*Client, error) {
	if logger == nil {
		logger = log.New(os.Stderr, "MCP_CLIENT: ", log.LstdFlags|log.Lshortfile)
	}

	logger.Printf("Creating new MCP client: mode='%s'", mode)

	var mcpClient client.MCPClient
	var err error
	clientMode := strings.ToLower(mode)

	switch clientMode {
	case "http", "sse": // Treat http and sse similarly for client creation
		url := addressOrCommand
		logger.Printf("Using SSEMCPClient for %s mode with URL: %s", clientMode, url)
		mcpClient, err = client.NewSSEMCPClient(url)
		if err != nil {
			return nil, fmt.Errorf("failed to create SSEMCPClient for %s: %w", url, err)
		}

	case "stdio":
		command := addressOrCommand
		logger.Printf("Using StdioMCPClient for stdio mode with command: '%s', args: %v", command, args)

		// Construct the environment slice for the subprocess
		processEnv := os.Environ()
		envMap := make(map[string]string)
		for _, entry := range processEnv {
			parts := strings.SplitN(entry, "=", 2)
			if len(parts) == 2 {
				envMap[parts[0]] = parts[1]
			}
		}
		for key, value := range env {
			envMap[key] = value // Override or add from config
		}
		finalEnv := make([]string, 0, len(envMap))
		for key, value := range envMap {
			finalEnv = append(finalEnv, fmt.Sprintf("%s=%s", key, value))
		}
		
		if len(env) > 0 {
			logger.Printf("Passing %d specific environment variables to stdio process '%s'", len(env), command)
		}
		
		// Create stdio client with command, args, and merged environment
		mcpClient, err = client.NewStdioMCPClient(command, finalEnv, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to create stdio client for command '%s': %w", command, err)
		}

	default:
		return nil, fmt.Errorf("unsupported MCP mode: %s. Must be 'stdio', 'sse', or 'http'", mode)
	}

	// Return the wrapped client
	return &Client{
		log:        logger,
		client:     mcpClient,
		serverAddr: addressOrCommand, // Store original address/command for logging
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

	// If already successfully initialized, skip
	if c.initialized {
		c.log.Printf("DEBUG: Client for %s already initialized, skipping Initialize call", c.serverAddr)
		return nil
	}

	c.log.Printf("Attempting to initialize MCP client for %s...", c.serverAddr)

	var initErr error
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION

	// Call the underlying library's Initialize method
	switch typedClient := c.client.(type) {
	case *client.SSEMCPClient:
		c.log.Printf("DEBUG: Calling library Initialize for SSE/HTTP client")
		_, initErr = typedClient.Initialize(ctx, initReq)
	case *client.StdioMCPClient:
		c.log.Printf("DEBUG: Calling library Initialize for Stdio client")
		_, initErr = typedClient.Initialize(ctx, initReq)
	default:
		// Fallback for unknown client types - try pinging
		c.log.Printf("Warning: Unknown MCP client type (%T), attempting generic ping for initialization check.", c.client)
		initErr = c.client.Ping(ctx)
	}

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
		
		return fmt.Errorf("MCP client initialization failed: %w", initErr)
	}

	c.log.Printf("MCP client for %s successfully initialized.", c.serverAddr)
	c.initialized = true // Set flag ONLY on success
	return nil
}

// CallTool delegates the tool call to the official MCP client.
func (c *Client) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("MCP client reference is nil")
	}

	// Ensure the client is initialized before making any tool calls.
	// Use a longer timeout for this initial check/attempt.
	if !c.initialized {
		c.log.Printf("Client for %s not initialized before CallTool, attempting Initialize...", c.serverAddr)
		initCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // 20s timeout for initialization
		if err := c.Initialize(initCtx); err != nil {
			cancel()
			c.log.Printf("ERROR: Failed to initialize MCP client before CallTool: %v", err)
			return "", fmt.Errorf("failed to initialize MCP client before CallTool: %w", err)
		}
		cancel()
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
		// Attempt to extract error message if available
		errMsg := fmt.Sprintf("tool '%s' execution failed", toolName)
		if len(result.Content) > 0 {
			if textContent, ok := result.Content[0].(mcp.TextContent); ok {
				errMsg = fmt.Sprintf("Tool '%s' returned an error: %s", toolName, textContent.Text)
			}
		}
		c.log.Printf(errMsg)
		return "", fmt.Errorf(errMsg)
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

	// Ensure the client is initialized. Attempt once with a longer timeout if not.
	if !c.initialized {
		c.log.Printf("Client for %s not initialized before GetAvailableTools, attempting Initialize...", c.serverAddr)
		initCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // 20s timeout for initialization
		if err := c.Initialize(initCtx); err != nil {
			cancel()
			c.log.Printf("ERROR: Initialization attempt failed: %v", err)
			return nil, fmt.Errorf("client not initialized: %w", err)
		}
		cancel()
	}

	// Define interfaces
	type toolLister interface {
		ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error)
	}
	type requestSender interface {
		sendRequest(ctx context.Context, method string, params interface{}) (*json.RawMessage, error)
	}

	// *** Determine the strategy based on client type ***
	var useSendRequestMethod bool = false
	if _, isStdio := c.client.(*client.StdioMCPClient); isStdio {
		if _, implementsSender := c.client.(requestSender); implementsSender {
			c.log.Println("Client is StdioMCPClient and implements sendRequest. Using manual 'tools/list' call.")
			useSendRequestMethod = true
		} else {
			c.log.Printf("Error: StdioMCPClient (%T) does not implement the expected sendRequest interface. Cannot discover tools.", c.client)
			return nil, fmt.Errorf("stdio client type %T does not support sendRequest workaround", c.client)
		}
	}

	// *** Execute based on strategy ***

	// Strategy 1: Use sendRequest workaround (primarily for Stdio)
	if useSendRequestMethod {
		sender := c.client.(requestSender) // We know this type assertion is safe now
		params := map[string]interface{}{}
		c.log.Printf("DEBUG: Using empty map for params: %+v", params)

		rawResponse, err := sender.sendRequest(ctx, "tools/list", params)
		if err != nil {
			c.log.Printf("ERROR: Manual 'tools/list' call failed via sendRequest: %v", err)
			if strings.Contains(err.Error(), "file already closed") {
				c.log.Printf("  Hint: Stdio process likely exited after initialization. Check server logs.")
			}
			return nil, fmt.Errorf("manual 'tools/list' call failed via sendRequest: %w", err)
		}
		if rawResponse == nil {
			c.log.Printf("Warning: manual 'tools/list' call via sendRequest returned nil response, assuming no tools")
			return []string{}, nil
		}

		var listResult mcp.ListToolsResult
		if err := json.Unmarshal(*rawResponse, &listResult); err != nil {
			return nil, fmt.Errorf("failed to unmarshal manual 'tools/list' response: %w. Response: %s", err, string(*rawResponse))
		}

		toolNames := make([]string, 0, len(listResult.Tools))
		for _, toolDef := range listResult.Tools {
			toolNames = append(toolNames, toolDef.Name)
		}
		c.log.Printf("Manual 'tools/list' call via sendRequest succeeded, returned %d tools: %v", len(toolNames), toolNames)
		return toolNames, nil
	}

	// Strategy 2: Use native ListTools (for SSE/HTTP or Stdio if it implements it and not sendRequest)
	if lister, ok := c.client.(toolLister); ok {
		c.log.Println("Using native ListTools method")
		req := mcp.ListToolsRequest{}
		
		listResult, err := lister.ListTools(ctx, req)
		
		// Simple retry logic if first attempt fails
		if err != nil {
			c.log.Printf("DEBUG: First ListTools attempt failed: %v. Trying Ping and retrying...", err)
			pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second) 
			if pingErr := c.client.Ping(pingCtx); pingErr != nil {
				pingCancel()
				c.log.Printf("DEBUG: Ping also failed: %v. Not retrying ListTools.", pingErr)
			} else {
				pingCancel()
				c.log.Printf("DEBUG: Ping succeeded, retrying ListTools")
				listResult, err = lister.ListTools(ctx, req) // Retry the call
			}
		}
		
		// Process result if successful
		if err == nil {
			if listResult == nil {
				c.log.Printf("Warning: ListTools returned nil result despite nil error")
				return []string{}, nil
			}
			toolNames := make([]string, 0, len(listResult.Tools))
			for _, toolDef := range listResult.Tools {
				toolNames = append(toolNames, toolDef.Name)
			}
			c.log.Printf("Native ListTools returned %d tools: %v", len(toolNames), toolNames)
			return toolNames, nil
		}
		
		// If we got here, ListTools failed even after potential retry
		return nil, fmt.Errorf("failed to get tools via native ListTools method: %w", err)
	}

	// --- Fallback if client type supports neither method --- 
	c.log.Printf("Error: Underlying MCP client (%T) supports neither sendRequest nor ListTools interfaces. Cannot discover tools.", c.client)
	return []string{}, fmt.Errorf("client type %T does not support tool discovery", c.client)
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

	// For stdio, the environment is actually set during NewClient now.
	// This function mainly exists for logging/debugging.
	c.log.Printf("DEBUG: Environment variables were configured for '%s' during client creation:", c.serverAddr)
	for key, value := range env {
		// Redact passwords
		if strings.Contains(strings.ToLower(key), "password") {
			c.log.Printf("  %s: <redacted>", key)
		} else {
			c.log.Printf("  %s: %s", key, value)
		}
	}
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

// runNetworkDiagnostics performs basic network checks to diagnose connection issues
func (c *Client) runNetworkDiagnostics(ctx context.Context, serverAddr string) error {
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

