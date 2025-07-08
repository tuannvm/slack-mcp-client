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

var (
	maxReconnectAttempts = 5
	baseBackoffDuration  = time.Second
)

type SSEMCPClientWithRetry struct {
	*client.Client

	serverAddr string
	log        *logging.Logger

	ctx    context.Context
	cancel context.CancelFunc

	mutex         sync.RWMutex
	reconnectCh   chan struct{}
	reconnectDone chan struct{}
}

func NewSSEMCPClientWithRetry(serverAddr string, log *logging.Logger) (*SSEMCPClientWithRetry, error) {
	sseClient, err := client.NewSSEMCPClient(serverAddr)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &SSEMCPClientWithRetry{
		Client:        sseClient,
		serverAddr:    serverAddr,
		log:           log,
		ctx:           ctx,
		cancel:        cancel,
		reconnectCh:   make(chan struct{}, 1),
		reconnectDone: nil,
	}

	go c.reconnectLoop()
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

	select {
	case <-c.triggerReconnect():
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	result, err = c.callTool(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("tool call failed after reconnect: %w", err)
	}

	return result, nil
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
		c.Client.Close()
	}

	sseClient, err := client.NewSSEMCPClient(c.serverAddr)
	if err != nil {
		return err
	}

	if err = sseClient.Start(context.Background()); err != nil {
		return err
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	if _, err = sseClient.Initialize(c.ctx, initReq); err != nil {
		return err
	}

	c.Client = sseClient
	return nil
}

// triggerReconnect schedules a reconnect if not already in progress and returns a channel that will be closed when it's done.
func (c *SSEMCPClientWithRetry) triggerReconnect() <-chan struct{} {
	select {
	case c.reconnectCh <- struct{}{}:
		done := make(chan struct{})
		c.reconnectDone = done
		return done
	default:
		if c.reconnectDone == nil {
			// edge case
			noop := make(chan struct{})
			close(noop)
			return noop
		}
		return c.reconnectDone
	}
}

func (c *SSEMCPClientWithRetry) reconnectLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.reconnectCh:
			for attempt := 1; attempt <= maxReconnectAttempts; attempt++ {
				if err := c.connect(); err != nil {
					c.log.InfoKV("Reconnect failed", "attempt", attempt, "error", err)

					select {
					case <-time.After(time.Duration(attempt) * baseBackoffDuration):
					case <-c.ctx.Done():
						return
					}
				} else {
					c.log.Info("Reconnected successfully")
					break
				}
			}

			if c.reconnectDone != nil {
				close(c.reconnectDone)
				c.reconnectDone = nil
			}
		}
	}
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
