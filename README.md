# Slack MCP Client in Go

This project provides a Slack bot client that serves as a bridge between Slack and Model Context Protocol (MCP) servers. By leveraging Slack as the user interface, it allows LLM models to interact with multiple MCP servers using standardized MCP tools.

[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/tuannvm/slack-mcp-client/build.yml?branch=main&label=CI%2FCD&logo=github)](https://github.com/tuannvm/slack-mcp-client/actions/workflows/build.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/tuannvm/slack-mcp-client?logo=go)](https://github.com/tuannvm/slack-mcp-client/blob/main/go.mod)
[![Trivy Scan](https://img.shields.io/github/actions/workflow/status/tuannvm/slack-mcp-client/build.yml?branch=main&label=Trivy%20Security%20Scan&logo=aquasec)](https://github.com/tuannvm/slack-mcp-client/actions/workflows/build.yml)
[![Docker Image](https://img.shields.io/github/v/release/tuannvm/slack-mcp-client?sort=semver&label=GHCR&logo=docker)](https://github.com/tuannvm/slack-mcp-client/pkgs/container/slack-mcp-client)
[![GitHub Release](https://img.shields.io/github/v/release/tuannvm/slack-mcp-client?sort=semver)](https://github.com/tuannvm/slack-mcp-client/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Overview

This project implements a Slack bot client that acts as a bridge between Slack and Model Context Protocol (MCP) servers. It uses Slack as a user interface while enabling LLM models to communicate with various MCP servers through standardized MCP tools.

Important distinction: This client is not designed to interact with the Slack API directly as its primary purpose. However, it can achieve Slack API functionality by connecting to a dedicated Slack MCP server (such as [modelcontextprotocol/servers/slack](https://github.com/modelcontextprotocol/servers/tree/main/src/slack)) alongside other MCP servers.

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
2. **Slack Bot Service** is a single process that includes:
   - The Slack Bot component that handles Slack messages
   - The MCP Client component that communicates with MCP servers
3. The **MCP Client** forwards requests to the appropriate MCP Server(s)
4. **MCP Servers** execute their respective tools and return results

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

## Installation


#### From Source

### Running Locally with Binary

After installing the binary, you can run it locally with the following steps:

1. Set up environment variables:

```bash
# Using environment variables directly
export SLACK_BOT_TOKEN="xoxb-your-bot-token"
export SLACK_APP_TOKEN="xapp-your-app-token"
export OPENAI_API_KEY="sk-your-openai-key"
export OPENAI_MODEL="gpt-4o"
export LOG_LEVEL="info"

# Or create a .env file and source it
cat > .env << EOL
SLACK_BOT_TOKEN="xoxb-your-bot-token"
SLACK_APP_TOKEN="xapp-your-app-token"
OPENAI_API_KEY="sk-your-openai-key"
OPENAI_MODEL="gpt-4o"
LOG_LEVEL="info"
EOL

source .env
```

2. Create an MCP servers configuration file:

```bash
# Create mcp-servers.json in the current directory
cat > mcp-servers.json << EOL
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "$HOME"],
      "env": {}
    }
  }
}
EOL
```

3. Run the application:

```bash
# Run with default settings (looks for mcp-servers.json in current directory)
slack-mcp-client

# Or specify a custom config file location
slack-mcp-client --config /path/to/mcp-servers.json

# Enable debug mode
slack-mcp-client --debug

# Specify OpenAI model
slack-mcp-client --openai-model gpt-4o-mini
```

The application will connect to Slack and start listening for messages. You can check the logs for any errors or connection issues.

### Kubernetes Deployment with Helm

For deploying to Kubernetes, a Helm chart is available in the `helm-chart` directory. This chart provides a flexible way to deploy the slack-mcp-client with proper configuration and secret management.

#### Installing from GitHub Container Registry

The Helm chart is also available directly from GitHub Container Registry, allowing for easier installation without needing to clone the repository:

```bash
# Add the OCI repository to Helm (only needed once)
helm registry login ghcr.io -u USERNAME -p GITHUB_TOKEN

# Pull the Helm chart
helm pull oci://ghcr.io/tuannvm/charts/slack-mcp-client --version 0.1.0

# Or install directly
helm install my-slack-bot oci://ghcr.io/tuannvm/charts/slack-mcp-client --version 0.1.0 -f values.yaml
```

You can check available versions by visiting the GitHub Container Registry in your browser.

#### Prerequisites
- Kubernetes 1.16+
- Helm 3.0+
- Slack Bot and App tokens

#### Basic Installation

```bash
# Create a values file with your configuration
cat > values.yaml << EOL
secret:
  create: true

env:
  SLACK_BOT_TOKEN: "xoxb-your-bot-token"
  SLACK_APP_TOKEN: "xapp-your-app-token"
  OPENAI_API_KEY: "sk-your-openai-key"
  OPENAI_MODEL: "gpt-4o"
  LOG_LEVEL: "info"

# Optional: Configure MCP servers
configMap:
  create: true
EOL

# Install the chart
helm install my-slack-bot ./helm-chart/slack-mcp-client -f values.yaml
```

#### Configuration Options

The Helm chart supports various configuration options including:
- Setting resource limits and requests
- Configuring MCP servers via ConfigMap
- Managing sensitive data via Kubernetes secrets
- Customizing deployment parameters

For more details, see the [Helm chart README](helm-chart/slack-mcp-client/README.md).

#### Using the Docker Image from GHCR

The Helm chart uses the Docker image from GitHub Container Registry (GHCR) by default. You can specify a particular version or use the latest tag:

```yaml
# In your values.yaml
image:
  repository: ghcr.io/tuannvm/slack-mcp-client
  tag: "latest"  # Or use a specific version like "1.0.0"
  pullPolicy: IfNotPresent
```

To manually pull the image:

```bash
# Pull the latest image
docker pull ghcr.io/tuannvm/slack-mcp-client:latest

# Or pull a specific version
docker pull ghcr.io/tuannvm/slack-mcp-client:1.0.0
```

If you're using private images, you can configure image pull secrets in your values:

```yaml
imagePullSecrets:
  - name: my-ghcr-secret
```

### Docker Compose for Local Testing

For local testing and development, you can use Docker Compose to easily run the slack-mcp-client along with additional MCP servers.

#### Setup

1. Create a `.env` file with your credentials:

```bash
# Create .env file from example
cp .env.example .env
# Edit the file with your credentials
nano .env
```

2. Create a `mcp-servers.json` file (or use the example):

```bash
# Create mcp-servers.json from example
cp mcp-servers.json.example mcp-servers.json
# Edit if needed
nano mcp-servers.json
```

3. Start the services:

```bash
# Start services in detached mode
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

#### Docker Compose Configuration

The included `docker-compose.yml` provides:

- Environment variables loaded from `.env` file
- Volume mounting for MCP server configuration
- Examples of connecting to additional MCP servers (commented out)

```yaml
version: '3.8'

services:
  slack-mcp-client:
    image: ghcr.io/tuannvm/slack-mcp-client:latest
    container_name: slack-mcp-client
    environment:
      - SLACK_BOT_TOKEN=${SLACK_BOT_TOKEN}
      - SLACK_APP_TOKEN=${SLACK_APP_TOKEN}
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - OPENAI_MODEL=${OPENAI_MODEL:-gpt-4o}
    volumes:
      - ./mcp-servers.json:/app/mcp-servers.json:ro
```

You can easily extend this setup to include additional MCP servers in the same network.

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

For detailed instructions on Slack app configuration, token setup, required permissions, and troubleshooting common issues, see the [Slack Configuration Guide](slack.md).

## Configuration

The client can be configured using the following environment variables:

| Variable        | Description                              | Default    |
| --------------- | ---------------------------------------- | ---------- |
| SLACK_BOT_TOKEN | Bot token for Slack API                  | (required) |
| SLACK_APP_TOKEN | App-level token for Socket Mode          | (required) |
| OPENAI_API_KEY  | API key for OpenAI authentication        | (required) |
| OPENAI_MODEL    | OpenAI model to use                      | gpt-4o     |
| LOG_LEVEL       | Logging level (debug, info, warn, error) | info       |

## Transport Modes

The client supports three transport modes:

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
   - Helm chart publishing to GitHub Container Registry
