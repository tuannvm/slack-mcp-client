package tools

import (
	"context"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
)

// HelloToolInput defines the input structure for the Hello tool.
// This might not be strictly necessary with the new handler approach,
// but we keep it for clarity/potential future use.
type HelloToolInput struct {
	Name string `json:"name"`
}

// HelloToolOutput defines the output structure for the Hello tool.
// This is superseded by returning *mcp.CallToolResult directly.
// type HelloToolOutput struct {
// 	Message string `json:"message"`
// }

// handleHelloTool is the handler function for the 'hello' tool.
func HandleHelloTool(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	logger.Printf("Executing hello tool handler with args: %v", request.Params.Arguments)

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

	logger.Printf("Hello tool generated message: %s", message)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: message,
			},
		},
	}, nil
}
