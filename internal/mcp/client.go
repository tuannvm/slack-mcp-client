package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/r3labs/sse/v2"
	"github.com/tuannvm/slack-mcp-client/internal/config"
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


// Client handles interactions with the MCP server using a custom SSE implementation.
type Client struct {
	cfg         *config.Config
	log         *log.Logger
	sseClient   *sse.Client   // Use the r3labs SSE client
	httpClient  *http.Client  // Standard HTTP client for sending requests
	serverAddr  string        // Store the target server address
	// TODO: Add map for request correlation if needed: pendingRequests map[string]chan *mcp.JsonRPCResponse
}

// NewClient creates a new MCP client handler using custom SSE logic.
func NewClient(cfg *config.Config, logger *log.Logger) (*Client, error) {
	logger.Println("Initializing Custom MCP SSE Client...")

	if cfg.MCPTargetServerAddress == "" {
		return nil, errors.New("MCP_TARGET_SERVER_ADDRESS is not set")
	}

	// 1. Initialize the r3labs SSE client (primarily for receiving events)
	// We might not actually connect/subscribe immediately, but instantiate it.
	// Connection might happen implicitly when the server first sends an event,
	// or we might need to manage it more actively depending on the protocol specifics.
	sseCli := sse.NewClient(cfg.MCPTargetServerAddress)
	// Add headers that might be required by the server
	sseCli.Headers["Accept"] = "text/event-stream"
	// TODO: Potentially add authentication headers if needed

	logger.Printf("SSE Client instance configured for target: %s", cfg.MCPTargetServerAddress)

	// 2. Initialize a standard HTTP client (for sending requests via POST)
	httpCli := &http.Client{
		Timeout: 15 * time.Second, // Example timeout
	}
	logger.Println("HTTP Client instance created.")

	c := &Client{
		cfg:        cfg,
		log:        logger,
		sseClient:  sseCli,
		httpClient: httpCli,
		serverAddr: cfg.MCPTargetServerAddress,
	}

	c.log.Printf(
		"MCP Client setup completed. Listener needs to be started separately. Server target: %s",
		c.serverAddr,
	)

	// Return the partially initialized client (listener not started yet)
	return c, nil
}

// StartListener connects to the MCP server's SSE endpoint and listens for events.
// This should be run in a goroutine.
func (c *Client) StartListener(ctx context.Context) error {
	c.log.Printf("Starting SSE event listener for %s...", c.serverAddr)

	// SubscribeRaw handles the connection and retries automatically
	err := c.sseClient.SubscribeRawWithContext(ctx, func(msg *sse.Event) {
		// This function will be called for each message received
		if len(msg.Data) == 0 {
			// Ignore empty messages (e.g., keep-alives)
			// c.log.Println("Received empty SSE event (keep-alive?)")
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

// sendMCPRequest is a helper to send JSON-RPC requests via HTTP POST.
func (c *Client) sendMCPRequest(ctx context.Context, method string, params interface{}) ([]byte, error) {
	requestID := uuid.NewString()
	rpcReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  method,
		Params:  params,
	}

	reqBytes, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request for %s (%s): %w", method, rpcReq.ID, err)
	}

	// --- Construct the target URL (use base server address) ---
	targetURL := c.serverAddr // Send to the root path now

	c.log.Printf("Sending MCP Request ID: %s, Method: %s, Size: %d bytes to %s", rpcReq.ID, method, len(reqBytes), targetURL)

	// Send request to the targetURL
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(reqBytes)) // Use bytes.Reader
	if err != nil {
		return nil, fmt.Errorf("failed to create http request for %s (%s): %w", method, rpcReq.ID, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json") // Expect JSON response for POST

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request failed for %s (%s): %w", method, rpcReq.ID, err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body for %s (%s): %w", method, rpcReq.ID, err)
	}

	c.log.Printf("Received MCP Response ID: %s, Status: %s, Size: %d bytes", rpcReq.ID, httpResp.Status, len(respBody))
	// c.log.Printf("Response Payload: %s", string(respBody)) // Verbose logging if needed

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, fmt.Errorf("http request for %s (%s) failed with status %s: %s", method, rpcReq.ID, httpResp.Status, string(respBody))
	}

	return respBody, nil
}

// CallHelloTool calls the 'hello' tool using the custom SSE client logic.
func (c *Client) CallHelloTool(ctx context.Context, name string) (string, error) {
	c.log.Printf("Calling 'hello' tool (Custom SSE Client)...")

	// 1. Prepare Args map
	args := map[string]any{}
	if name != "" {
		args["name"] = name
	}

	// 2. Create the full mcp.CallToolRequest struct
	toolRequest := mcp.CallToolRequest{}
	toolRequest.Params.Name = "hello" // Set nested Params field
	toolRequest.Params.Arguments = args

	// 3. Send Request
	// Pass the full toolRequest struct as params
	rawResponse, err := c.sendMCPRequest(ctx, "tools/call", toolRequest) // Pass toolRequest here
	if err != nil {
		return "", fmt.Errorf("failed to send 'hello' tool request: %w", err)
	}

	// 3. Parse JSON-RPC Base Response
	var rpcResponse jsonRPCResponse
	if err := json.Unmarshal(rawResponse, &rpcResponse); err != nil {
		return "", fmt.Errorf("failed to parse 'hello' tool JSON-RPC response: %w", err)
	}

	// 4. Check for JSON-RPC Error
	if rpcResponse.Error != nil {
		return "", fmt.Errorf("'hello' tool call failed: %w", rpcResponse.Error)
	}

	// 5. Check for Result field
	if rpcResponse.Result == nil {
		return "", fmt.Errorf("'hello' tool response missing 'result' field")
	}

	// 6. Parse Specific CallToolResult
	var toolResult mcp.CallToolResult
	if err := json.Unmarshal(rpcResponse.Result, &toolResult); err != nil {
		return "", fmt.Errorf("failed to parse 'hello' tool result: %w", err)
	}

	c.log.Printf("Received 'hello' tool result content: %+v", toolResult.Content)

	// 7. Extract Text Content
	for _, contentItem := range toolResult.Content {
		// Attempt to decode the generic map[string]interface{} into TextContent
		var textContent mcp.TextContent
		contentBytes, _ := json.Marshal(contentItem) // Marshal back to bytes
		if err := json.Unmarshal(contentBytes, &textContent); err == nil {
			if textContent.Type == "text" && textContent.Text != "" { // Check type and non-empty
				return textContent.Text, nil
			}
		}
		// Could add handling for other content types here if needed
	}

	return "", fmt.Errorf("no suitable text content found in 'hello' tool result")
}

// Close cleans up the MCP client resources.
func (c *Client) Close() {
	if c.sseClient != nil {
		c.log.Println("Closing SSE client connection (if active)...")
		// The r3labs client doesn't have an explicit Close().
		// Connection is managed via Subscribe/Context cancellation.
		// We might need to cancel a context used in Subscribe if we start it.
	}
	if c.httpClient != nil {
		c.log.Println("Closing idle HTTP client connections...")
		c.httpClient.CloseIdleConnections()
	}
}
