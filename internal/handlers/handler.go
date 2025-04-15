package handlers

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

// ToolHandler defines the interface for all MCP tool handlers
type ToolHandler interface {
	// Handle processes an MCP tool request and returns a result or an error
	Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	
	// GetName returns the name of the tool
	GetName() string
	
	// GetDescription returns a human-readable description of the tool
	GetDescription() string
	
	// GetToolDefinition returns the MCP tool definition
	GetToolDefinition() mcp.Tool
}

// BaseHandler provides common functionality for all handlers
type BaseHandler struct {
	Name        string
	Description string
	Tool        mcp.Tool
	Logger      *logging.Logger
}

// GetName returns the name of the tool
func (h *BaseHandler) GetName() string {
	return h.Name
}

// GetDescription returns a human-readable description of the tool
func (h *BaseHandler) GetDescription() string {
	return h.Description
}

// GetToolDefinition returns the MCP tool definition
func (h *BaseHandler) GetToolDefinition() mcp.Tool {
	return h.Tool
}

// HandlerFuncAdapter converts a simple handler function to a ToolHandler
type HandlerFuncAdapter struct {
	BaseHandler
	HandleFunc func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// NewHandlerFuncAdapter creates a new handler adapter
func NewHandlerFuncAdapter(
	name string,
	description string,
	tool mcp.Tool,
	logger *logging.Logger,
	handleFunc func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error),
) *HandlerFuncAdapter {
	return &HandlerFuncAdapter{
		BaseHandler: BaseHandler{
			Name:        name,
			Description: description,
			Tool:        tool,
			Logger:      logger,
		},
		HandleFunc: handleFunc,
	}
}

// Handle delegates to the handler function
func (h *HandlerFuncAdapter) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return h.HandleFunc(ctx, request)
} 