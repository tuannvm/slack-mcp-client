// Package system provides system-level tool handlers for basic functionality
package system

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/handlers"
)

// HelloHandler implements the Hello tool
type HelloHandler struct {
	handlers.BaseHandler
}

// NewHelloHandler creates a new HelloHandler
func NewHelloHandler(logger *logging.Logger) *HelloHandler {
	tool := mcp.NewTool(
		"hello",
		mcp.WithDescription("Responds with a greeting"),
		mcp.WithString("name",
			mcp.Description("The name to say hello to (Optional)"),
		),
	)

	return &HelloHandler{
		BaseHandler: handlers.BaseHandler{
			Name:        "hello",
			Description: "Responds with a greeting",
			Tool:        tool,
			Logger:      logger.WithName("hello-tool"),
		},
	}
}

// Handle processes a hello tool request
func (h *HelloHandler) Handle(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.Logger.Debug("Executing hello tool handler with args: %v", request.Params.Arguments)

	name := ""
	if nameArg, ok := request.Params.Arguments["name"]; ok {
		if nameStr, ok := nameArg.(string); ok {
			name = nameStr
		}
	}

	message := fmt.Sprintf("Hello, %s!", name)
	if name == "" {
		message = "Hello there!"
	}

	h.Logger.Debug("Hello tool generated message: %s", message)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: message,
			},
		},
	}, nil
}
