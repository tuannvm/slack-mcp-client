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
