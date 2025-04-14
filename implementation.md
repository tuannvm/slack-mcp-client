# Implementation Notes: Slack MCP Client

## Current State (As of 2025-04-13)

1.  **Project Goal:** Build a Slack bot that interacts with external tools and data sources via the Model Context Protocol (MCP), implemented in Go.
2.  **Architecture:** See `README.md` for the high-level design. The core components are the Slack integration, the Go application (`slack-mcp-client`), and external MCP servers.
3.  **Slack Integration:** Basic Slack client setup using `slack-go/slack` is in place ([`internal/slack_client/client.go`](cci:7://file:///Users/tuannvm/Projects/cli/slack-mcp-client/internal/slack_client/client.go:0:0-0:0)), listening for mentions.
4.  **Internal MCP Server:** An internal MCP server ([`internal/mcp/server.go`](cci:7://file:///Users/tuannvm/Projects/cli/slack-mcp-client/internal/mcp/server.go:0:0-0:0)) is implemented using `mark3labs/mcp-go@v0.20.0`. It serves requests over SSE and currently registers a simple `hello` tool.
5.  **MCP Client (Blocker):** Attempts to use the `mark3labs/mcp-go/client` package ([`internal/mcp/client.go`](cci:7://file:///Users/tuannvm/Projects/cli/slack-mcp-client/internal/mcp/client.go:0:0-0:0)) failed. Investigation revealed that `v0.20.0` **only supports `stdio` client transport**, not SSE or HTTP. We cannot use this library package to connect to our SSE-based MCP server URL (`MCP_TARGET_SERVER_ADDRESS`).

## Chosen Path Forward: Custom SSE Client Implementation

We will implement the necessary SSE client logic directly within [`internal/mcp/client.go`](cci:7://file:///Users/tuannvm/Projects/cli/slack-mcp-client/internal/mcp/client.go:0:0-0:0). This avoids changing the server transport or relying on potentially unavailable library updates.

## Implementation Steps

1.  **Add Dependency:** Introduce an SSE client library. `github.com/r3labs/sse/v2` is a common choice.
    ```bash
    go get github.com/r3labs/sse/v2
    ```
2.  **Refactor `internal/mcp/client.go`:**
    *   Remove imports and usage of `github.com/mark3labs/mcp-go/client`.
    *   Keep imports of `github.com/mark3labs/mcp-go/mcp` for the request/response struct definitions (e.g., `mcp.InitializeRequest`, `mcp.CallToolRequest`, `mcp.TextContent`).
    *   Define the `Client` struct to hold the configuration, logger, and an instance of the chosen SSE client (e.g., `*sse.Client`).
    *   **`NewClient` Function:**
        *   Instantiate the SSE client using the library (e.g., `sse.NewClient(cfg.MCPTargetServerAddress)`).
        *   Add necessary headers (e.g., `Accept: text/event-stream`).
        *   **Crucially:** Implement the MCP `initialize` handshake immediately after connecting:
            *   Generate a unique request ID.
            *   Create an `mcp.InitializeRequest` struct.
            *   Marshal it to JSON.
            *   Send this JSON payload as a **POST request** to the *same* `MCPTargetServerAddress`. The MCP spec often uses POST for sending client->server messages even over an established SSE connection path, or requires a specific mechanism defined by the SSE transport binding if different. We need to verify the exact mechanism expected by `mcp-go`'s SSE server. *Initial assumption: Send via POST.*
            *   Wait for the `mcp.InitializeResult` response. This might come as a standard HTTP response to the POST or as an SSE event. *Need clarification.*
            *   Handle initialization errors.
    *   **`CallHelloTool` (and later `CallTool`) Function:**
        *   Generate a unique request ID.
        *   Create the `mcp.CallToolRequest` struct.
        *   Marshal it to JSON.
        *   Send this JSON payload via POST to the server URL (similar to initialization). *Need clarification on transport mechanism.*
        *   Wait for the `mcp.CallToolResult` response (likely via POST response or SSE event).
        *   Correlate the response using the request ID.
        *   Parse the result, extract `mcp.TextContent` or handle errors.
    *   **SSE Event Handling:** Set up handlers to receive asynchronous messages (events) from the server if the protocol uses SSE for responses or notifications.
    *   **`Close` Function:** Properly close the SSE connection and related resources.
3.  **Update `go.mod`/`go.sum`:** Ensure the new dependency is tracked.
4.  **Testing:** Thoroughly test the connection, initialization, and tool call process against the internal MCP server.

*(Self-correction: The MCP specification and `mcp-go` SSE server implementation need closer examination to determine precisely how client requests like `initialize` and `tools/call` should be sent over the SSE connection path - is it truly a separate POST, or is it framed within the SSE stream itself? The `stdio` example might not be a perfect guide here.)*
