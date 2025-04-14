package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	sse "github.com/r3labs/sse/v2"
)

// --- JSON-RPC Helper Structs ---

type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC Error %d: %s", e.Code, e.Message)
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"` // Use RawMessage for delayed parsing
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// --- End JSON-RPC Helper Structs ---


// MCPTransport defines the interface for different MCP communication modes
type MCPTransport interface {
	Initialize(ctx context.Context) error
	CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error)
	Close()
}

// StdioTransport implements MCPTransport for stdio communication
type StdioTransport struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	scanner   *bufio.Scanner
	log       *log.Logger
	mu        sync.Mutex // Protects access to stdin/stdout
	address   string     // Command to execute
	isRunning bool
}

func (t *StdioTransport) Initialize(ctx context.Context) error {
	t.log.Println("Initializing stdio transport...")
	// For stdio transport, there's not much to initialize
	return nil
}

func (t *StdioTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	if err := t.startProcess(); err != nil {
		return "", fmt.Errorf("failed to start stdio process: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.isRunning {
		return "", fmt.Errorf("stdio process is not running")
	}

	req := map[string]interface{}{
		"tool_name": toolName,
		"arguments": args,
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal stdio request: %w", err)
	}

	t.log.Printf("Sending to stdio: %s", string(reqBytes))
	_, err = fmt.Fprintln(t.stdin, string(reqBytes)) // Add newline
	if err != nil {
		// Process might have exited
		t.isRunning = false
		return "", fmt.Errorf("failed to write to stdio process stdin: %w", err)
	}

	// Read response line by line
	if t.scanner.Scan() {
		line := t.scanner.Text()
		t.log.Printf("Received from stdio: %s", line)
		// Assuming response is a single JSON line
		var resp map[string]interface{}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			return "", fmt.Errorf("failed to unmarshal response from stdio: %w. Raw: %s", err, line)
		}
		if errorMsg, ok := resp["error"].(string); ok && errorMsg != "" {
			return "", fmt.Errorf("stdio server returned error: %s", errorMsg)
		}
		if result, ok := resp["result"].(string); ok {
			return result, nil
		}
		return "", fmt.Errorf("unexpected response format from stdio: %s", line)
	}

	if err := t.scanner.Err(); err != nil {
		t.isRunning = false
		return "", fmt.Errorf("error reading from stdio process stdout: %w", err)
	}

	// If Scan returned false without an error, it means EOF
	t.isRunning = false
	return "", fmt.Errorf("stdio process closed connection (EOF)")
}

func (t *StdioTransport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cmd != nil && t.cmd.Process != nil {
		t.log.Printf("Closing stdio transport (PID: %d)...", t.cmd.Process.Pid)
		// Closing stdin might signal the process to exit gracefully
		if t.stdin != nil {
			_ = t.stdin.Close()
		}
		// Send SIGTERM signal
		if err := t.cmd.Process.Signal(os.Interrupt); err != nil {
			t.log.Printf("Failed to send SIGINT to stdio process: %v. Attempting SIGKILL...", err)
			_ = t.cmd.Process.Kill()
		} else {
			// Give it a moment to shut down
			go func(p *os.Process) {
				time.Sleep(2 * time.Second)
				// Check if it's still running
				if p.Signal(syscall.Signal(0)) == nil { // Check if process exists
					t.log.Printf("Stdio process did not exit after SIGINT, sending SIGKILL.")
					_ = p.Kill()
				}
			}(t.cmd.Process)
		}
		t.cmd = nil // Prevent further calls
		t.isRunning = false
	}
}

func (t *StdioTransport) startProcess() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.isRunning {
		return nil // Already running
	}

	t.log.Printf("Starting stdio process: %s", t.address)
	parts := strings.Fields(t.address)
	if len(parts) == 0 {
		return fmt.Errorf("stdio command is empty")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	// Set environment variables if needed using cmd.Env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command '%s': %w", t.address, err)
	}

	t.cmd = cmd
	t.stdin = stdin
	t.stdout = stdout
	t.scanner = bufio.NewScanner(stdout)
	t.isRunning = true
	t.log.Printf("Stdio process started (PID: %d)", cmd.Process.Pid)

	// Goroutine to log stderr and detect process exit
	go func() {
		stderr, _ := cmd.StderrPipe()
		if stderr != nil {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				t.log.Printf("STDERR: %s", scanner.Text())
			}
		}
		err := cmd.Wait()
		t.mu.Lock()
		t.log.Printf("Stdio process exited (PID: %d). Error: %v", cmd.Process.Pid, err)
		t.isRunning = false
		t.cmd = nil
		t.stdin = nil
		t.stdout = nil
		t.scanner = nil
		t.mu.Unlock()
	}()

	return nil
}

// SSETransport implements MCPTransport for SSE communication
type SSETransport struct {
	client  *sse.Client
	address string
	log     *log.Logger
	mu      sync.Mutex
}

func (t *SSETransport) Initialize(ctx context.Context) error {
	t.log.Println("Initializing SSE transport...")
	// For SSE transport, there's not much to initialize
	return nil
}

func (t *SSETransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	t.mu.Lock() // Protect concurrent calls if the underlying client is not safe
	defer t.mu.Unlock()

	t.log.Printf("SSE CallTool: %s with args %v", toolName, args)

	// The r3labs/sse client handles connection/reconnection internally.
	// We need to subscribe to get events.

	// Channel to receive the result or error
	resultChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Create a context for this specific subscription
	subCtx, cancelSub := context.WithCancel(ctx)
	defer cancelSub() // Ensure cancellation on exit

	go func() {
		err := t.client.SubscribeRawWithContext(subCtx, func(msg *sse.Event) {
			// Assuming the server sends the result as a single JSON message
			t.log.Printf("SSE Received: Event=%s, Data=%s", string(msg.Event), string(msg.Data))

			// Try to parse the data as the expected result format
			var resp map[string]interface{}
			if err := json.Unmarshal(msg.Data, &resp); err != nil {
				// Might be a non-JSON message or status update, log and continue or error
				t.log.Printf("Warning: Failed to unmarshal SSE data: %v. Data: %s", err, string(msg.Data))
				// Depending on protocol, maybe error out:
				// select {
				// case errChan <- fmt.Errorf("failed to unmarshal SSE response: %w", err):
				// default:
				// }
				// cancelSub() // Stop subscription on first error?
				return // Continue listening for other events
			}

			// Check for application-level errors in the response
			if errorMsg, ok := resp["error"].(string); ok && errorMsg != "" {
				t.log.Printf("SSE server returned error: %s", errorMsg)
				select {
				case errChan <- fmt.Errorf("SSE server error: %s", errorMsg):
				default:
				}
				cancelSub() // Stop subscription on receiving error
				return
			}

			// Check for the expected result
			if result, ok := resp["result"].(string); ok {
				t.log.Printf("SSE received successful result.")
				select {
				case resultChan <- result:
				default:
				}
				cancelSub() // Stop subscription on receiving result
				return
			}

			// Handle other event types or unexpected formats if necessary
			t.log.Printf("Received unhandled SSE message format: %s", string(msg.Data))

		})
		if err != nil {
			// Error during initial connection or subscription setup
			select {
			case errChan <- fmt.Errorf("SSE subscription failed: %w", err):
			default:
			}
		} else {
			// Subscription goroutine finished without initial error
			// If we haven't received a result/error by now, it means the context was cancelled
			// or the subscription ended without sending to resultChan/errChan.
			t.log.Println("SSE subscription finished.")
			// Ensure an error is sent if nothing else was
			select {
			case <-resultChan:
			case <-errChan:
			case <-subCtx.Done(): // Check if context caused the finish
				select {
				case errChan <- subCtx.Err():
				default:
				}
			default: // Finished for other reasons (e.g. server disconnect)
				select {
				case errChan <- fmt.Errorf("SSE subscription ended unexpectedly"):
				default:
				}
			}
		}
	}()

	// TODO: Send the actual request to the SSE server.
	// The r3labs/sse client is primarily for LISTENING. How do we SEND the request?
	// The SSE protocol itself doesn't define request/response. Typically, you'd make
	// a separate HTTP POST request to trigger the action, and then listen for results
	// on the SSE stream established by SubscribeRawWithContext.

	// Placeholder: Need to implement the request-sending part (e.g., an HTTP POST)
	// For now, this will likely time out or fail if the server doesn't
	// spontaneously send the result upon connection.
	t.log.Println("Warning: SSE request sending logic not implemented yet.")

	// Wait for the result, error, or context cancellation
	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errChan:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (t *SSETransport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.log.Println("Closing SSE transport...")
	// The r3labs/sse client manages its connection state internally.
	// Calling Close() might not be necessary or available directly depending on usage.
	// If we were managing the subscription context, cancelling it would stop listening.
	// Check library docs for specific cleanup if needed.
}

// HTTPTransport implements MCPTransport for HTTP communication
type HTTPTransport struct {
	client  *http.Client
	address string
	log     *log.Logger
	mu      sync.Mutex
}

func (t *HTTPTransport) Initialize(ctx context.Context) error {
	t.log.Println("Initializing HTTP transport...")
	// For HTTP transport, there's not much to initialize
	return nil
}

func (t *HTTPTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	reqPayload := map[string]interface{}{
		"tool_name": toolName,
		"arguments": args,
	}
	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal HTTP request payload: %w", err)
	}

	// Assuming the server expects POST requests to /call/{toolName} or similar
	// Adjust the URL structure as needed based on the MCP server's HTTP API
	// For now, using a simple /call path
	url := t.address
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	// url += "call/" + toolName // Example: /call/mytool
	url += "call" // Using a generic /call endpoint

	t.log.Printf("HTTP POST to %s with body: %s", url, string(reqBody))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read HTTP response body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP request failed with status %s: %s", httpResp.Status, string(respBodyBytes))
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(respBodyBytes, &resp); err != nil {
		return "", fmt.Errorf("failed to unmarshal HTTP response: %w. Raw: %s", err, string(respBodyBytes))
	}

	if errorMsg, ok := resp["error"].(string); ok && errorMsg != "" {
		return "", fmt.Errorf("HTTP server returned error: %s", errorMsg)
	}

	if result, ok := resp["result"].(string); ok {
		return result, nil
	}

	return "", fmt.Errorf("unexpected response format from HTTP server: %s", string(respBodyBytes))
}

func (t *HTTPTransport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.log.Println("Closing HTTP transport...")
	// Standard HTTP client doesn't require explicit closing for connections typically,
	// but can call CloseIdleConnections if needed.
	// t.client.CloseIdleConnections()
}

// Client handles interactions with the MCP server using configurable transport.
type Client struct {
	log        *log.Logger
	transport  MCPTransport
	serverAddr string
}

// NewClient creates a new MCP client handler with the configured transport mode.
func NewClient(mode, address string, logger *log.Logger) (*Client, error) {
	if logger == nil {
		logger = log.New(os.Stderr, "MCP_CLIENT: ", log.LstdFlags|log.Lshortfile)
	}

	logger.Printf("Creating new MCP client: mode=%s, address=%s", mode, address)

	client := &Client{
		log: logger,
	}

	var err error
	var selectedTransport MCPTransport

	switch strings.ToLower(mode) {
	case "stdio":
		// For stdio, 'address' is expected to be the command to execute
		logger.Printf("Initializing Stdio transport with command: %s", address)
		selectedTransport, err = newStdioTransport(address, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize stdio transport: %w", err)
		}

	case "sse":
		// For SSE, 'address' is the base URL of the MCP server
		logger.Printf("Initializing SSE transport with address: %s", address)
		selectedTransport, err = newSSETransport(address, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize SSE transport: %w", err)
		}

	case "http":
		// For HTTP, 'address' is the base URL of the MCP server
		logger.Printf("Initializing HTTP transport with address: %s", address)
		selectedTransport, err = newHTTPTransport(address, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize HTTP transport: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported MCP mode: %s. Must be 'stdio', 'sse', or 'http'", mode)
	}

	client.transport = selectedTransport

	// Initialize the transport (e.g., start process for stdio, setup HTTP client)
	err = client.transport.Initialize(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize transport for mode %s: %w", mode, err)
	}

	logger.Printf("MCP Client initialized successfully with %T transport.", selectedTransport)
	return client, nil
}

// StartListener connects to the MCP server and listens for events.
// This should be run in a goroutine.
// Note: This is primarily used for SSE transport, but could be adapted for other modes.
func (c *Client) StartListener(ctx context.Context) error {
	c.log.Printf("Starting event listener for %s...", c.serverAddr)

	// For SSE transport, we need to start the SSE subscription
	if sseTransport, ok := c.transport.(*SSETransport); ok {
		// SubscribeRaw handles the connection and retries automatically
		err := sseTransport.client.SubscribeRawWithContext(ctx, func(msg *sse.Event) {
			// This function will be called for each message received
			if len(msg.Data) == 0 {
				// Ignore empty messages (e.g., keep-alives)
				return
			}
			c.log.Printf("Received Raw SSE Event: ID=%s, Event=%s, Data=%s", string(msg.ID), string(msg.Event), string(msg.Data))

			// TODO:
			// 1. Parse msg.Data as JSON-RPC response/notification
			// 2. Handle responses (correlate with request ID)
			// 3. Handle notifications (e.g., server-sent tool calls)
		})

		if err != nil {
			c.log.Printf("SSE SubscribeRaw error: %v", err)
			return fmt.Errorf("failed to subscribe to MCP SSE stream: %w", err)
		}

		// If SubscribeRaw exits without error, it might mean the context was cancelled
		c.log.Println("SSE event listener stopped.")
		return nil
	}

	// For other transport types, we might not need an active listener
	c.log.Printf("No listener needed for transport type: %T", c.transport)
	
	// Block until context is done
	<-ctx.Done()
	c.log.Println("Event listener context cancelled.")
	return nil
}

// CallTool delegates the tool call to the configured transport.
func (c *Client) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	if c.transport == nil {
		return "", fmt.Errorf("MCP transport not initialized")
	}
	c.log.Printf("Client delegating CallTool '%s' to %T transport", toolName, c.transport)
	return c.transport.CallTool(ctx, toolName, args)
}

// Close cleans up the MCP client resources.
func (c *Client) Close() {
	c.log.Println("Closing MCP client resources...")
	
	// Close the transport
	if c.transport != nil {
		c.transport.Close()
	}
}

// Initialize specific transport implementations

func newStdioTransport(cmdString string, logger *log.Logger) (*StdioTransport, error) {
	t := &StdioTransport{
		log:     logger,
		address: cmdString,
	}
	// We don't start the command here, it's started on the first CallTool
	return t, nil
}

func newSSETransport(baseURL string, logger *log.Logger) (*SSETransport, error) {
	client := sse.NewClient(baseURL)
	// Add configuration like custom HTTP client, headers if needed
	// client.Connection = &http.Client{ Timeout: 30 * time.Second }
	return &SSETransport{
		client:  client,
		address: baseURL,
		log:     logger,
	}, nil
}

func newHTTPTransport(baseURL string, logger *log.Logger) (*HTTPTransport, error) {
	return &HTTPTransport{
		client: &http.Client{
			Timeout: 60 * time.Second, // Configurable timeout
		},
		address: baseURL,
		log:     logger,
	}, nil
}

type mcpMessage struct {
	Version string                 `json:"version"`
	Action  string                 `json:"action"`
	Tool    string                 `json:"tool"`
	Args    map[string]interface{} `json:"args"`
	Error   string                 `json:"error"`
	Result  string                 `json:"result"`
}

type mcpResult struct {
	data []byte
	err  error
}
