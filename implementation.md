# Implementation Notes: Slack MCP Client

## Current State (As of 2025-04-14)

1.  **Project Goal:** Build a Slack bot that interacts with external tools and data sources via the Model Context Protocol (MCP), implemented in Go.
2.  **Architecture:** See `README.md` for the high-level design. The core components are the Slack integration, the Go application (`slack-mcp-client`), and external MCP servers.
3.  **Slack Integration:** Basic Slack client setup using `slack-go/slack` is in place (`internal/slack/client.go`), listening for mentions and direct messages.
4.  **MCP Client Configuration:** Uses a flexible configuration approach for multiple MCP servers through `mcp-servers.json` following the schema defined in `mcp-schema.json`.
5.  **LLM Integration:** The `internal/bridge/llm_mcp_bridge.go` component serves as the bridge between LLM models and the MCP client/server interaction.

## Architecture & Implementation

### Core Components

1. **Configuration System**
   - Configuration loaded from environment variables for Slack credentials and LLM settings
   - MCP server configurations from JSON file following the schema with `mcpServers` property
   - Each server defined with `command`, `args`, and optional `env` properties

2. **Slack Client (`internal/slack/client.go`)**
   - Connects to Slack using Socket Mode for secure, firewall-friendly communication
   - Handles app mentions and direct messages
   - Processes user prompts and forwards them to LLM
   - Returns responses and tool results back to Slack channels/DMs

3. **MCP Client (`internal/mcp/client.go`)**
   - Support for multiple transport protocols:
     - **SSE (Server-Sent Events):** For real-time communication
     - **HTTP:** For request-response communication
     - **stdio:** For local development with command-line tools
   - Dynamic initialization with proper command line argument parsing
   - Runtime discovery of available tools from MCP servers

4. **LLM-MCP Bridge (`internal/bridge/llm_mcp_bridge.go`)**
   - Inspects LLM responses and user prompts to detect tool invocation patterns
   - Translates detected patterns into MCP tool calls
   - Processes tool results and formats them for user consumption
   - Supports both structured JSON tool calls and natural language detection

5. **Tool Implementations**
   - **Filesystem operations:** For accessing and managing files
   - **Hello tool:** Simple demonstration tool
   - **Ollama integration:** Connect with Ollama models

### MCP Server Configuration

MCP servers are configured using a JSON file following this structure:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "/Users/tuannvm/Projects",
        "/Users/tuannvm/Downloads"
      ],
      "env": {
        "DEBUG": "mcp:*"
      }
    },
    "github": {
      "command": "github-mcp-server",
      "args": ["stdio"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "your-github-token"
      }
    }
  }
}
```

### Main Process Flow

1. **Start-up Sequence:**
   - Parse command-line arguments
   - Load environment variables and configuration file
   - Initialize MCP clients for each configured server
   - Connect to Slack using Socket Mode

2. **Message Processing:**
   - Receive messages from Slack (mentions or DMs)
   - Forward messages to LLM (Ollama)
   - Process LLM response through LLMMCPBridge
   - If tool invocation is detected:
     - Execute appropriate MCP tool call
     - Format tool results
   - Return final response to Slack

3. **Tool Detection & Execution:**
   - JSON pattern matching for structured tool invocations
   - Regular expression matching for natural language requests
   - Validate against available tools discovered from MCP servers
   - Execute tool call with appropriate parameters
   - Process and format tool results

### Future Improvements

1. **Enhanced Natural Language Tool Detection**
   - Improve regex patterns for more natural interactions
   - Add support for contextual tool detection (understanding from conversation context)

2. **Tool Authentication & Security**
   - Implement proper authentication for secure tool access
   - Add permission controls for sensitive operations

3. **Additional MCP Tool Integrations**
   - Database tools
   - API connectors
   - Document processing tools

4. **UI Enhancements**
   - Rich Slack message formatting for tool results
   - Interactive components (buttons, dropdowns) for tool parameter selection
   - Progress indicators for long-running tool operations

5. **Conversation Context Management**
   - Maintain context between messages for more coherent interactions
   - Allow tools to access previous conversation history when relevant

### Debugging Stdio Transport Issues

*   **Problem:** Significant challenges were encountered with `stdio` transport mode. While `stdio` clients (like `github`, `mcp-trino`) would often successfully complete the initial `Initialize` handshake, subsequent calls like `GetAvailableTools` or `CallTool` would consistently fail with `file already closed` errors.
*   **Initial Investigation:**
    *   Code analysis initially focused on the `GetAvailableTools` implementation in `internal/mcp/client.go`, correcting flawed logic that checked for a non-existent `sendRequest` method on the library's `StdioMCPClient`.
    *   Analysis of the `mcp-go` library's `client/stdio.go` confirmed that `StdioMCPClient` *does* implement the standard `ListTools` method, but doesn't have obvious internal idle timeouts.
*   **Manual Testing:**
    *   Running server commands (e.g., `github-mcp-server stdio`) manually and piping `initialize` followed immediately by `tools/list` JSON-RPC requests worked correctly.
    *   Running the server command manually and letting it sit idle showed it did *not* exit automatically after a short period.
*   **Hypothesis:** The issue wasn't an inherent server crash or timeout, but rather the *delay* between the successful `Initialize` call and the subsequent `GetAvailableTools` call in the original application flow (where all clients were initialized first, then all tools were discovered). This gap seemed to cause the library or the server to close the stdio pipes.
*   **Resolution:** The `main` function in `cmd/slack-mcp-client/main.go` was refactored. Instead of separate loops for initialization and discovery, a single loop now processes each server sequentially: 
    1.  Create client instance.
    2.  Call `Initialize`.
    3.  If initialization succeeds, immediately call `GetAvailableTools`.
*   **Outcome:** This sequential processing eliminated the delay and resolved the `file already closed` errors, allowing `GetAvailableTools` to successfully retrieve tools from the `stdio` servers.

# Slack MCP Client Restructuring

This document outlines the architectural restructuring of the Slack MCP Client for improved maintainability and sustainability.

## Key Changes

### 1. Modular Directory Structure

The codebase has been reorganized with a clear separation of concerns:

```
internal/
├── common/               # Shared utilities
│   ├── errors/           # Error types and handling
│   ├── http/             # HTTP client with retry logic
│   └── logging/          # Structured logging
├── config/               # Configuration loading and validation
├── handlers/             # Tool implementation
│   ├── llm/              # LLM-specific handlers (OpenAI, Ollama)
│   └── system/           # System tool handlers (Hello, etc.)
├── server/               # MCP server implementation
├── slack/                # Slack bot client
├── bridge/               # LLM-MCP Bridge
└── mcp/                  # MCP client functionality
```

### 2. Interface-Based Design

- Introduced a `ToolHandler` interface for all tool implementations
- Created a `BaseHandler` with common functionality
- Implemented a `Registry` for centralized handler management

### 3. Standardized Error Handling

- Created domain-specific error types for better error context
- Implemented consistent error wrapping and propagation
- Added status code to error mapping for HTTP errors

### 4. Robust HTTP Client

- Developed a shared HTTP client with retry functionality
- Implemented exponential backoff with jitter
- Added request/response logging and customizable timeouts

### 5. Structured Logging

- Implemented a hierarchical logger with log levels
- Added context-aware logging with named loggers
- Maintained compatibility with standard log.Logger

### 6. Server Refactoring

- Moved server logic to dedicated package
- Implemented handler registration system
- Improved shutdown handling and error propagation

## Benefits

1. **Maintainability**: Clear separation of concerns makes the codebase easier to understand and maintain
2. **Scalability**: Interface-based design allows for easy addition of new tool handlers
3. **Robustness**: Improved error handling and retry mechanisms increase reliability
4. **Observability**: Structured logging provides better insight into operation
5. **Testability**: Interface-based design makes unit testing easier

## Implementation Details

### Handler Interface

```go
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
```

### Error Handling

```go
// ServiceError represents an error from a specific service
type ServiceError struct {
    Service   string
    Message   string
    Code      string
    Retryable bool
    Cause     error
}
```

### HTTP Client

```go
// DoRequest performs an HTTP request with retries and logging
func (c *Client) DoRequest(ctx context.Context, method, url string, body interface{}, headers map[string]string) ([]byte, int, error) {
    // Implementation with retry logic and error handling
}
```

## Migration Guide

When implementing new features:

1. Create new handlers in the appropriate subdirectory of `internal/handlers/`
2. Implement the `ToolHandler` interface
3. Register the handler in `internal/server/server.go`
4. Use structured error handling with the `errors` package
5. Leverage the common HTTP client for external API calls
6. Use the structured logger for consistent logging

## Next Steps

To complete the restructuring, the following tasks should be undertaken:

1. **Update Main Application**: 
   - Modify `cmd/slack-mcp-client/main.go` to use the new server package
   - Initialize the structured logger

2. **Migrate Configuration Logic**:
   - Move environment variable loading to a central configuration service
   - Implement config validation

3. **Refactor Slack Client**:
   - Use the new structured logger
   - Integrate with the error handling system

4. **Add Comprehensive Tests**:
   - Unit tests for handlers
   - Integration tests for server functionality

5. **Documentation Updates**:
   - Update `README.md` with the new architecture
   - Create API documentation for the new interfaces

6. **Monitoring & Observability**:
   - Add metrics collection
   - Implement trace context propagation

This new architecture provides a solid foundation for future development and ensures the long-term sustainability of the Slack MCP Client.
