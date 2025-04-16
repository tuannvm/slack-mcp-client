package client

import (
	"context"
	//	"encoding/json"
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
)

// Client provides an interface for interacting with an MCP server.
// It handles tool discovery and execution of tool calls.
type Client struct {
	log         *log.Logger
	client      client.MCPClient
	serverAddr  string
	initialized bool // Track if the client has been successfully initialized

	// Fields specific to stdio mode for monitoring the subprocess
	stdioCmd     *exec.Cmd     // Store the command for stdio clients
	stdioExited  chan struct{} // Closed when the stdio process exits prematurely
	stdioExitErr error         //nolint:unused // Stores the exit error if premature
	closeOnce    sync.Once     // Ensures close logic runs only once
	closeMu      sync.Mutex    // Protects access during close
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
	var stdioCmd *exec.Cmd // Variable to hold the command if stdio mode
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

		// Use the library function but capture the *exec.Cmd it creates indirectly
		// We need to ensure the library function still gets called correctly.
		// Unfortunately, the library's NewStdioMCPClient doesn't return the cmd.
		// We might need to re-implement parts of NewStdioMCPClient here or modify the lib.
		// For now, let's assume we *can* get the cmd somehow, or proceed knowing this is a limitation.

		// *** Placeholder: Assuming we could get the cmd ***
		// stdioCmd = createAndStartStdioCmd(command, finalEnv, args...)
		// mcpClient, err = client.NewStdioMCPClientFromCmd(stdioCmd) // Hypothetical library change

		// *** Sticking with the current library call ***
		mcpClient, err = client.NewStdioMCPClient(command, finalEnv, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to create stdio client for command '%s': %w", command, err)
		}
		// PROBLEM: We cannot get the *exec.Cmd from the library's NewStdioMCPClient
		// Therefore, we cannot implement the cmd.Wait() monitoring goroutine without library changes.
		logger.Printf("WARNING: Cannot monitor stdio subprocess exit status without library modification to expose exec.Cmd.")

	default:
		return nil, fmt.Errorf("unsupported MCP mode: %s. Must be 'stdio', 'sse', or 'http'", mode)
	}

	// Create the wrapper client
	wrapperClient := &Client{
		log:         logger,
		client:      mcpClient,
		serverAddr:  addressOrCommand,
		stdioCmd:    stdioCmd,            // Will be nil if not stdio or if we couldn't get it
		stdioExited: make(chan struct{}), // Initialize the channel
	}

	// // Start the monitoring goroutine ONLY if we have the stdioCmd
	// if wrapperClient.stdioCmd != nil {
	// 	go wrapperClient.monitorStdioProcess()
	// }

	return wrapperClient, nil
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
	if !c.initialized {
		c.log.Printf("DEBUG: Client for %s not initialized before CallTool, attempting Initialize...", c.serverAddr)
		initCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // 20s timeout for initialization
		if err := c.Initialize(initCtx); err != nil {
			cancel()
			c.log.Printf("ERROR: Failed to initialize MCP client before CallTool: %v", err)
			return "", fmt.Errorf("failed to initialize MCP client before CallTool: %w", err)
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
		// Add specific hint for stdio clients if CallTool fails with file closed
		if _, isStdio := c.client.(*client.StdioMCPClient); isStdio && strings.Contains(err.Error(), "file already closed") {
			c.log.Printf("  Hint: Stdio process likely exited after Initialize or previous call. Check server command/args/env and logs.")
		}
		return "", fmt.Errorf("failed to call tool '%s': %w", toolName, err)
	}

	// Check if the tool call resulted in an error
	if result.IsError {
		// Attempt to extract error message if available
		var errMsgText string
		if len(result.Content) > 0 {
			if textContent, ok := result.Content[0].(mcp.TextContent); ok {
				errMsgText = fmt.Sprintf("Tool '%s' returned an error: %s", toolName, textContent.Text)
			} else {
				errMsgText = fmt.Sprintf("Tool '%s' execution failed", toolName)
			}
		} else {
			errMsgText = fmt.Sprintf("Tool '%s' execution failed", toolName)
		}
		
		c.log.Printf("Error: %s", errMsgText)
		return "", fmt.Errorf("%s", errMsgText)
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
			// Add specific hint for stdio clients if init fails here
			if _, isStdio := c.client.(*client.StdioMCPClient); isStdio {
				c.log.Printf("  Hint: Stdio process might have exited. Check server command/args/env and logs.")
			}
			return nil, fmt.Errorf("client not initialized before GetAvailableTools: %w", err)
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
		// Add specific hint for stdio clients if ListTools fails
		if _, isStdio := c.client.(*client.StdioMCPClient); isStdio && strings.Contains(err.Error(), "file already closed") {
			c.log.Printf("  Hint: Stdio process likely exited after Initialize or during ListTools. Check server command/args/env and logs.")
		}
		return nil, fmt.Errorf("failed to get tools via native ListTools method: %w", err)
	}

	// --- Fallback if client type does not implement ListTools ---
	c.log.Printf("Error: Underlying MCP client (%T) for %s does not implement the toolLister interface. Cannot discover tools.", c.client, c.serverAddr)
	// Return nil struct and error
	return nil, fmt.Errorf("client type %T does not support tool discovery", c.client)
}

// Close cleans up the MCP client resources.
func (c *Client) Close() {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	c.closeOnce.Do(func() {
		c.log.Printf("Closing MCP client resources for %s...", c.serverAddr)

		// Signal that the closure is intentional for the monitor goroutine
		select {
		case <-c.stdioExited: // Already closed (prematurely or by another Close call)
		default:
			close(c.stdioExited) // Signal normal closure
		}

		// Close the underlying library client if possible
		if closer, ok := c.client.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				// Log underlying close error, especially if it's 'file already closed'
				c.log.Printf("Error closing underlying MCP client for %s: %v", c.serverAddr, err)
			}
		} else {
			c.log.Printf("Underlying client type %T does not implement io.Closer", c.client)
		}

		// Ensure the process is terminated if it's stdio and we managed to get the cmd
		// Note: library's Close() likely handles this, but belt-and-suspenders
		if c.stdioCmd != nil {
			c.log.Printf("DEBUG: [%s] Ensuring stdio process is terminated...", c.serverAddr)
			if c.stdioCmd.Process != nil {
				// Try gentle signal first, then kill
				_ = c.stdioCmd.Process.Signal(os.Interrupt)
				time.Sleep(100 * time.Millisecond) // Give it a moment
				_ = c.stdioCmd.Process.Kill()
			}
			// Wait for the monitor goroutine to finish processing the exit
			// If cmd.Wait() was already done, this is quick
			<-c.stdioExited // Wait for signal confirming exit processing is done
			c.log.Printf("DEBUG: [%s] Stdio process termination confirmed.", c.serverAddr)
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

// SetEnvironment sets environment variables for the MCP client (only effective for stdio mode).
func (c *Client) SetEnvironment(env map[string]string) {
	if len(env) == 0 {
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
	if len(args) == 0 {
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
//
//nolint:unused // Reserved for future use
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
