package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

const (
	maxReconnectAttempts = 5
	baseBackoffDuration  = time.Second
)

type SSEMCPClientWithRetry struct {
	*client.Client

	serverAddr string
	headers    http.Header
	log        *logging.Logger

	ctx    context.Context
	cancel context.CancelFunc

	mutex sync.RWMutex

	reconnectMu           sync.Mutex
	isReconnectInProgress bool
	reconnectErr          error
	reconnectDoneCh       chan struct{}
}

func NewSSEMCPClientWithRetry(serverAddr string, hdr http.Header, log *logging.Logger) (*SSEMCPClientWithRetry, error) {
	// Convert http.Header to map[string]string for the client library
	headerMap := make(map[string]string)
	for key, values := range hdr {
		if len(values) > 0 {
			headerMap[key] = values[0] // Use the first value for each header
		}
	}
	
	sseClient, err := client.NewSSEMCPClient(serverAddr, client.WithHeaders(headerMap))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &SSEMCPClientWithRetry{
		Client:     sseClient,
		serverAddr: serverAddr,
		headers:    hdr,
		log:        log,
		ctx:        ctx,
		cancel:     cancel,
	}

	return c, nil
}

func (c *SSEMCPClientWithRetry) Start(ctx context.Context) error {
	return c.Client.Start(ctx)
}

func (c *SSEMCPClientWithRetry) CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := c.callTool(ctx, request)
	if err == nil {
		return result, nil
	}

	var terr *transport.Error
	if !errors.As(err, &terr) {
		return nil, err
	}

	c.log.ErrorKV("Tool call failed, attempting reconnect", "error", err)

	err = c.sharedReconnect(ctx)
	if err != nil {
		return nil, fmt.Errorf("tool call failed after reconnect attempt: %w", err)
	}

	result, err = c.callTool(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("tool call failed after reconnect: %w", err)
	}

	return result, nil
}

func (c *SSEMCPClientWithRetry) Close() error {
	c.cancel()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.Client != nil {
		return c.Client.Close()
	}
	return nil
}

func (c *SSEMCPClientWithRetry) callTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.Client == nil {
		return nil, fmt.Errorf("client not connected")
	}
	return c.Client.CallTool(ctx, request)
}

func (c *SSEMCPClientWithRetry) connect() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.Client != nil {
		if err := c.Client.Close(); err != nil {
			c.log.WarnKV("Failed to close old client during reconnect", "error", err)
		}
	}

	// Convert http.Header to map[string]string for the client library
	headerMap := make(map[string]string)
	for key, values := range c.headers {
		if len(values) > 0 {
			headerMap[key] = values[0] // Use the first value for each header
		}
	}

	sseClient, err := client.NewSSEMCPClient(c.serverAddr, client.WithHeaders(headerMap))
	if err != nil {
		return err
	}

	if err = sseClient.Start(c.ctx); err != nil {
		_ = sseClient.Close()
		return err
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	if _, err = sseClient.Initialize(c.ctx, initReq); err != nil {
		_ = sseClient.Close()
		return err
	}

	c.Client = sseClient
	return nil
}

// sharedReconnect ensures only one reconnect attempt runs, others wait for it.
func (c *SSEMCPClientWithRetry) sharedReconnect(ctx context.Context) error {
	c.reconnectMu.Lock()
	if c.isReconnectInProgress {
		ready := c.reconnectDoneCh
		c.reconnectMu.Unlock()

		select {
		case <-ready:
			return c.reconnectErr
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	c.isReconnectInProgress = true
	c.reconnectDoneCh = make(chan struct{})
	c.reconnectMu.Unlock()

	go func() {
		var success bool
		var err error

	reconnectLoop:
		for attempt := 1; attempt <= maxReconnectAttempts; attempt++ {
			err = c.connect()
			if err == nil {
				success = true
				break
			}

			c.log.InfoKV("Reconnect failed", "attempt", attempt, "error", err)

			select {
			case <-time.After(time.Duration(attempt) * baseBackoffDuration):
			case <-c.ctx.Done():
				err = c.ctx.Err()
				break reconnectLoop
			}
		}

		c.reconnectMu.Lock()
		defer c.reconnectMu.Unlock()

		if success {
			c.log.Info("Reconnected successfully")
			c.reconnectErr = nil
		} else {
			c.log.Error("All reconnect attempts failed — client is still disconnected")
			c.reconnectErr = err
		}
		close(c.reconnectDoneCh)
		c.isReconnectInProgress = false
	}()

	select {
	case <-c.reconnectDoneCh:
		c.reconnectMu.Lock()
		defer c.reconnectMu.Unlock()
		return c.reconnectErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
