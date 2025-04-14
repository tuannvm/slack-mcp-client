# Slack MCP Client

A flexible Slack bot client that communicates with Model Context Protocol (MCP) servers using multiple transport modes (SSE, HTTP, stdio). This bot acts as an intelligent assistant within Slack channels and direct messages, utilizing the MCP protocol to invoke external tools.

## Features

- **Multi-Mode MCP Client**: Support for multiple communication protocols:
  - **SSE (Server-Sent Events)**: Real-time event streaming from MCP servers
  - **HTTP**: Standard JSON-RPC over HTTP
  - **stdio**: Simple input/output for local development and testing

- **Slack Integration**: Connects to Slack using Socket Mode for secure, firewall-friendly communication

- **Tool Registration**: Register and call MCP tools like the sample "hello" tool

- **Extensible Architecture**: Easy to add new tools and transport modes

## Architecture

```
+----------------+     +----------------+     +----------------+
|                |     |                |     |                |
|   Slack API    |<--->|   Slack Bot    |<--->|   MCP Client   |
|                |     |                |     |                |
+----------------+     +----------------+     +-------+--------+
                                                     |
                                                     v
                                              +------+-------+
                                              |              |
                                              |  MCP Server  |
                                              |              |
                                              +--------------+
```

## Prerequisites

- Go 1.19 or higher
- A Slack workspace with admin permissions
- A Slack app with Socket Mode enabled

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/tuannvm/slack-mcp-client.git
   cd slack-mcp-client
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

## Configuration

1. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

2. Edit the `.env` file with your Slack API tokens and MCP configuration:
   ```
   # Slack Configuration
   SLACK_BOT_TOKEN="xoxb-your-bot-token"
   SLACK_APP_TOKEN="xapp-your-app-level-token"

   # MCP Configuration
   MCP_SERVER_LISTEN_ADDRESS=":8081"
   MCP_TARGET_SERVER_ADDRESS="http://127.0.0.1:8081"
   MCP_MODE="sse"  # Options: sse, http, stdio
   ```

## Slack App Setup

1. Create a new Slack app at https://api.slack.com/apps
2. Enable Socket Mode and generate an app-level token
3. Add the following Bot Token Scopes:
   - `app_mentions:read`
   - `chat:write`
   - `im:history`
   - `im:read`
   - `im:write`
4. Enable Event Subscriptions and subscribe to:
   - `app_mention`
   - `message.im`
5. Install the app to your workspace

## Running the Application

```bash
go run ./cmd/slack-mcp-client
```

Or build and run the binary:

```bash
go build -o slack-mcp-client ./cmd/slack-mcp-client
./slack-mcp-client
```

## Transport Modes

The client supports three transport modes that can be configured via the `MCP_MODE` environment variable:

- **SSE (default)**: Uses Server-Sent Events for real-time communication with the MCP server
- **HTTP**: Uses HTTP POST requests with JSON-RPC for communication
- **stdio**: Uses standard input/output for local development and testing

## Adding New Tools

To add a new MCP tool:

1. Create a new tool definition in `internal/tools/`
2. Register the tool in `internal/mcp/server.go`
3. Implement the tool call logic in the appropriate transport implementation

Example of a simple tool definition:

```go
package tools

// HelloToolInput defines the input parameters for the hello tool
type HelloToolInput struct {
	Name string `json:"name"`
}

// HelloToolOutput defines the output structure for the hello tool
type HelloToolOutput struct {
	Message string `json:"message"`
}
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
