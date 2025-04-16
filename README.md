# Slack MCP Client in Go

A high-performance Slack bot client that communicates with Model Context Protocol (MCP) servers using multiple transport modes. This project enables AI assistants to seamlessly interact with Slack through standardized MCP tools.

[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/tuannvm/slack-mcp-client/build.yml?branch=main&label=CI%2FCD&logo=github)](https://github.com/tuannvm/slack-mcp-client/actions/workflows/build.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/tuannvm/slack-mcp-client?logo=go)](https://github.com/tuannvm/slack-mcp-client/blob/main/go.mod)
[![Trivy Scan](https://img.shields.io/github/actions/workflow/status/tuannvm/slack-mcp-client/build.yml?branch=main&label=Trivy%20Security%20Scan&logo=aquasec)](https://github.com/tuannvm/slack-mcp-client/actions/workflows/build.yml)
[![Docker Image](https://img.shields.io/github/v/release/tuannvm/slack-mcp-client?sort=semver&label=GHCR&logo=docker)](https://github.com/tuannvm/slack-mcp-client/pkgs/container/slack-mcp-client)
[![GitHub Release](https://img.shields.io/github/v/release/tuannvm/slack-mcp-client?sort=semver)](https://github.com/tuannvm/slack-mcp-client/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Overview

This project implements a Slack bot client that integrates with Model Context Protocol (MCP) servers. It enables AI assistants to access Slack channels and direct messages through standardized MCP tools, creating a powerful bridge between AI capabilities and team communication.

## How It Works

```mermaid
flowchart LR
    User([User]) --> SlackBot
    
    subgraph SlackBotService[Slack Bot Service]
        SlackBot[Slack Bot] <--> MCPClient[MCP Client]
    end
    

    
    MCPClient <--> MCPServer1[MCP Server 1]
    MCPClient <--> MCPServer2[MCP Server 2]
    MCPClient <--> MCPServer3[MCP Server 3]
    
    MCPServer1 <--> Tools1[(Tools)]
    MCPServer2 <--> Tools2[(Tools)]
    MCPServer3 <--> Tools3[(Tools)]
    
    style SlackBotService fill:#F8F9F9,stroke:#333,stroke-width:2px
    style SlackBot fill:#F4D03F,stroke:#333,stroke-width:2px
    style MCPClient fill:#2ECC71,stroke:#333,stroke-width:2px
    style MCPServer1 fill:#E74C3C,stroke:#333,stroke-width:2px
    style MCPServer2 fill:#E74C3C,stroke:#333,stroke-width:2px
    style MCPServer3 fill:#E74C3C,stroke:#333,stroke-width:2px
    style Tools1 fill:#9B59B6,stroke:#333,stroke-width:2px
    style Tools2 fill:#9B59B6,stroke:#333,stroke-width:2px
    style Tools3 fill:#9B59B6,stroke:#333,stroke-width:2px
```

1. **User** interacts only with Slack, sending messages through the Slack interface
2. **Slack API** delivers the message to the Slack Bot Service
3. **Slack Bot Service** is a single process that includes:
   - The Slack Bot component that handles Slack messages
   - The MCP Client component that communicates with MCP servers
4. The **MCP Client** forwards requests to the appropriate MCP Server(s)
5. **MCP Servers** execute their respective tools and return results

## Features

- ✅ **Multi-Mode MCP Client**: 
  - Server-Sent Events (SSE) for real-time communication
  - HTTP transport for JSON-RPC
  - stdio for local development and testing
- ✅ **Slack Integration**: 
  - Uses Socket Mode for secure, firewall-friendly communication
  - Works with both channels and direct messages
- ✅ **Tool Registration**: Dynamically register and call MCP tools
- ✅ **Docker container support**
- ✅ **Compatible with Cursor, Claude Desktop, Windsurf, ChatWise and any MCP-compatible clients**

## Installation

### Homebrew (macOS and Linux)

The easiest way to install slack-mcp-client is using Homebrew:

```bash
# Add the tap repository
brew tap tuannvm/mcp

# Install slack-mcp-client
brew install slack-mcp-client
```

To update to the latest version:

```bash
brew update && brew upgrade slack-mcp-client
```

### Alternative Installation Methods

#### Manual Download

1. Download the appropriate binary for your platform from the [GitHub Releases](https://github.com/tuannvm/slack-mcp-client/releases) page.
2. Place the binary in a directory included in your PATH (e.g., `/usr/local/bin` on Linux/macOS)
3. Make it executable (`chmod +x slack-mcp-client` on Linux/macOS)

#### From Source

```bash
git clone https://github.com/tuannvm/slack-mcp-client.git
cd slack-mcp-client
make build # Binary will be in ./bin/
```

## Downloads

You can download pre-built binaries for your platform:

| Platform | Architecture | Download Link |
|----------|--------------|---------------|
| macOS | x86_64 (Intel) | [Download](https://github.com/tuannvm/slack-mcp-client/releases/latest/download/slack-mcp-client-darwin-amd64) |
| macOS | ARM64 (Apple Silicon) | [Download](https://github.com/tuannvm/slack-mcp-client/releases/latest/download/slack-mcp-client-darwin-arm64) |
| Linux | x86_64 | [Download](https://github.com/tuannvm/slack-mcp-client/releases/latest/download/slack-mcp-client-linux-amd64) |
| Linux | ARM64 | [Download](https://github.com/tuannvm/slack-mcp-client/releases/latest/download/slack-mcp-client-linux-arm64) |
| Windows | x86_64 | [Download](https://github.com/tuannvm/slack-mcp-client/releases/latest/download/slack-mcp-client-windows-amd64.exe) |

Or see all available downloads on the [GitHub Releases](https://github.com/tuannvm/slack-mcp-client/releases) page.

## MCP Integration

This Slack MCP client can be integrated with several AI applications:

### Using Docker Image

To use the Docker image instead of a local binary:

```json
{
  "mcpServers": {
    "slack-mcp-client": {
      "command": "docker",
      "args": ["run", "--rm", "-i", 
        "-e", "SLACK_BOT_TOKEN=your-bot-token", 
        "-e", "SLACK_APP_TOKEN=your-app-token", 
        "-e", "MCP_MODE=sse", 
        "ghcr.io/tuannvm/slack-mcp-client:latest"],
      "env": {}
    }
  }
}
```

> **Note**: When running in Docker, ensure your environment variables are properly passed to the container. This Docker configuration can be used in any of the below applications.

### Cursor

To use with [Cursor](https://cursor.sh/), create or edit `~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "slack-mcp-client": {
      "command": "slack-mcp-client",
      "args": [],
      "env": {
        "SLACK_BOT_TOKEN": "your-bot-token",
        "SLACK_APP_TOKEN": "your-app-token",
        "MCP_MODE": "sse"
      }
    }
  }
}
```

Replace the environment variables with your specific Slack configuration.

### Claude Desktop

To use with [Claude Desktop](https://claude.ai/desktop), edit your Claude configuration file:
- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "slack-mcp-client": {
      "command": "slack-mcp-client",
      "args": [],
      "env": {
        "SLACK_BOT_TOKEN": "your-bot-token",
        "SLACK_APP_TOKEN": "your-app-token",
        "MCP_MODE": "sse"
      }
    }
  }
}
```

After updating the configuration, restart Claude Desktop. You should see the MCP tools available in the tools menu.

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

## Configuration

The client can be configured using the following environment variables:

| Variable | Description | Default |
| ------------------ | ----------------------------- | --------- |
| SLACK_BOT_TOKEN | Bot token for Slack API | (required) |
| SLACK_APP_TOKEN | App-level token for Socket Mode | (required) |
| MCP_MODE | Transport method (sse/http/stdio) | sse |
| MCP_SERVER_LISTEN_ADDRESS | Address for MCP server to listen on | :8081 |
| MCP_TARGET_SERVER_ADDRESS | Target MCP server address | http://127.0.0.1:8081 |
| LOG_LEVEL | Logging level (debug, info, warn, error) | info |

## Transport Modes

The client supports three transport modes that can be configured via the `MCP_MODE` environment variable:

- **SSE (default)**: Uses Server-Sent Events for real-time communication with the MCP server
- **HTTP**: Uses HTTP POST requests with JSON-RPC for communication
- **stdio**: Uses standard input/output for local development and testing

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## CI/CD and Releases

This project uses GitHub Actions for continuous integration and GoReleaser for automated releases.

### Continuous Integration Checks

Our CI pipeline performs the following checks on all PRs and commits to the main branch:

#### Code Quality
- **Linting**: Using golangci-lint to check for common code issues and style violations
- **Go Module Verification**: Ensuring go.mod and go.sum are properly maintained
- **Formatting**: Verifying code is properly formatted with gofmt

#### Security
- **Vulnerability Scanning**: Using govulncheck to check for known vulnerabilities in dependencies
- **Dependency Scanning**: Using Trivy to scan for vulnerabilities in dependencies
- **SBOM Generation**: Creating a Software Bill of Materials for dependency tracking

#### Testing
- **Unit Tests**: Running tests with race detection and code coverage reporting
- **Build Verification**: Ensuring the codebase builds successfully

### Release Process

When changes are merged to the main branch:
1. CI checks are run to validate code quality and security
2. If successful, a new release is automatically created with:
   - Semantic versioning based on commit messages
   - Binary builds for multiple platforms
   - Docker image publishing to GitHub Container Registry
