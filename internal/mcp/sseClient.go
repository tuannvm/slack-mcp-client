package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
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
	log        *logging.Logger

	ctx    context.Context
	cancel context.CancelFunc

	mutex sync.RWMutex

	reconnectMu           sync.Mutex
	isReconnectInProgress bool
	reconnectErr          error
	reconnectDoneCh       chan struct{}
}

func NewSSEMCPClientWithRetry(serverAddr string, log *logging.Logger) (*SSEMCPClientWithRetry, error) {
	sseClient, err := client.NewSSEMCPClient(serverAddr)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &SSEMCPClientWithRetry{
		Client:     sseClient,
		serverAddr: serverAddr,
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

	if !strings.Contains(err.Error(), "transport error") {
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

	sseClient, err := client.NewSSEMCPClient(c.serverAddr)
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
				break
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
