// Package mcp provides client and server functionality for Model Context Protocol.
package mcp

import (
	"github.com/tuannvm/slack-mcp-client/internal/mcp/client"
	"github.com/tuannvm/slack-mcp-client/internal/mcp/server"
)

// For backwards compatibility, we're re-exporting the Server type from internal/mcp/server
// This allows existing code to continue using mcp.Server without changes
type Server = server.Server

// NewServer creates a new MCP server using the implementation in internal/mcp/server
var NewServer = server.NewServer

// For backwards compatibility, we're re-exporting the Client type from internal/mcp/client
// This allows existing code to continue using mcp.Client without changes
type Client = client.Client

// NewClient creates a new MCP client using the implementation in internal/mcp/client
var NewClient = client.NewClient
