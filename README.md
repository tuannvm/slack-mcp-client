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

![Image](https://github.com/user-attachments/assets/48a587e4-7895-4a6f-9745-61b21894c34c)

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
  - Rich message formatting with Markdown and Block Kit
  - Automatic conversion of quoted strings to code blocks for better readability
- ✅ **Multi-Provider LLM Support**:
  - OpenAI (GPT-4, GPT-4o, etc.)
  - Anthropic (Claude 3.5 Sonnet, etc.) 
  - Ollama (Local LLMs like Llama, Mistral, etc.)
  - Factory pattern for easy provider switching
  - LangChain gateway for unified API
- ✅ **Custom Prompt Engineering**: 
  - System Prompts - Define custom AI assistant behavior and personality
- ✅ **RAG (Retrieval-Augmented Generation)**: 
  - LangChain Go Compatible - Drop-in replacement for standard vector stores
  - Document Processing - PDF ingestion with intelligent chunking
  - CLI Tools - Command-line utilities for document management
  - Extensible Design - Easy to add SQLite, Redis, or vector embeddings
- ✅ **Tool Registration**: Dynamically register and call MCP tools
- ✅ **Configuration Management**:
  - JSON-based MCP server configuration
  - Environment variable support
  - Multiple transport modes (HTTP/SSE, stdio)
- ✅ **Production Ready**:
  - Docker container support
  - Kubernetes Helm charts
  - Comprehensive logging and error handling
  - 88%+ test coverage

## Installation

### From Binary Release

Download the latest binary from the [GitHub releases page](https://github.com/tuannvm/slack-mcp-client/releases/latest) or install using Go:

```bash
# Install latest version using Go
go install github.com/tuannvm/slack-mcp-client@latest

# Or build from source
git clone https://github.com/tuannvm/slack-mcp-client.git
cd slack-mcp-client
make build
# Binary will be in ./bin/slack-mcp-client
```

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

### RAG Setup and Usage

The client includes an improved RAG (Retrieval-Augmented Generation) system that's compatible with LangChain Go and provides professional-grade performance:

#### Quick Start with RAG

1. **Enable RAG in your configuration:**

```json
{
  "llm_provider": "openai",
  "llm_providers": {
    "openai": {
      "type": "openai", 
      "model": "gpt-4o",
      "rag_config": {
        "enabled": true,
        "provider": "json",
        "database_path": "./knowledge.json",
        "chunk_size": 1000,
        "chunk_overlap": 200,
        "max_results": 10
      }
    }
  }
}
```

2. **Ingest documents using CLI:**

```bash
# Ingest PDF files from a directory
slack-mcp-client --rag-ingest ./company-docs --rag-db ./knowledge.json

# Test search functionality
slack-mcp-client --rag-search "vacation policy" --rag-db ./knowledge.json

# Get database statistics
slack-mcp-client --rag-stats --rag-db ./knowledge.json
```

3. **Use in Slack:**

Once configured, the LLM can automatically search your knowledge base:

**User**: "What's our vacation policy?"

**AI**: "Let me search our knowledge base for vacation policy information..."
*(Automatically searches RAG database)*

**AI**: "Based on our company policy documents, you get 15 days of vacation..."

#### RAG Features

- **🎯 Smart Search**: Advanced relevance scoring with word frequency, filename boosting, and phrase matching
- **🔗 LangChain Compatible**: Drop-in replacement for standard vector stores
- **📈 Extensible**: Easy to add vector embeddings and other backends

### Custom Prompts and Assistants

The client supports advanced prompt engineering capabilities for creating specialized AI assistants:

#### System Prompts

Create custom AI personalities and behaviors:

```bash
# Create a custom system prompt file
cat > sales-assistant.txt << EOL
You are SalesGPT, a helpful sales assistant specializing in B2B software sales.

Your expertise includes:
- Lead qualification and discovery
- Solution positioning and value propositions  
- Objection handling and negotiation
- CRM best practices and sales processes

Always:
- Ask qualifying questions to understand prospect needs
- Provide specific, actionable sales advice
- Reference industry best practices
- Maintain a professional yet friendly tone

When discussing pricing, always emphasize value over cost.
EOL

# Use the custom prompt
slack-mcp-client --system-prompt ./sales-assistant.txt
```

#### Configuration-Based Prompts

Define prompts in your configuration:

```json
{
  "llm_provider": "openai",
  "llm_providers": {
    "openai": {
      "type": "openai",
      "model": "gpt-4o",
      "system_prompt": "You are a helpful DevOps assistant specializing in Kubernetes and cloud infrastructure.",
      "conversation_starters": [
        "Help me debug a Kubernetes pod issue",
        "Explain best practices for CI/CD pipelines", 
        "Review my Dockerfile for optimization"
      ]
    }
  }
}
```

#### Assistant Roles

Create specialized assistants for different use cases:

- **DevOps Assistant**: Kubernetes, Docker, CI/CD expertise
- **Sales Assistant**: Lead qualification, objection handling
- **HR Assistant**: Policy questions, onboarding guidance
- **Support Assistant**: Customer issue resolution
- **Code Review Assistant**: Security, performance, best practices

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
   - `users:read`
   - `users.profile:read`
4. Enable Event Subscriptions and subscribe to:
   - `app_mention`
   - `message.im`
5. Install the app to your workspace

For detailed instructions on Slack app configuration, token setup, required permissions, and troubleshooting common issues, see the [Slack Configuration Guide](docs/configuration.md).

## LLM Integration

The client supports multiple LLM providers through a flexible integration system:

### LangChain Gateway

The LangChain gateway enables seamless integration with various LLM providers:

- **OpenAI**: Native support for GPT models (default)
- **Ollama**: Local LLM support for models like Llama, Mistral, etc.
- **Extensible**: Can be extended to support other LangChain-compatible providers

### LLM-MCP Bridge

The custom LLM-MCP bridge layer enables any LLM to use MCP tools without requiring native function-calling capabilities:

- **Universal Compatibility**: Works with any LLM, including those without function-calling
- **Pattern Recognition**: Detects when a user prompt or LLM response should trigger a tool call
- **Natural Language Support**: Understands both structured JSON tool calls and natural language requests

### Configuration

LLM providers can be configured via environment variables or command-line flags:

```bash
# Set OpenAI as the provider (default)
export LLM_PROVIDER="openai"
export OPENAI_MODEL="gpt-4o"

# Use Anthropic
export LLM_PROVIDER="anthropic"
export ANTHROPIC_API_KEY="your-anthropic-api-key"
export ANTHROPIC_MODEL="claude-3-5-sonnet-20241022"

# Or use Ollama
export LLM_PROVIDER="ollama"
export LANGCHAIN_OLLAMA_URL="http://localhost:11434"
export LANGCHAIN_OLLAMA_MODEL="llama3"
```

### Switching Between Providers

You can easily switch between providers by changing the `LLM_PROVIDER` environment variable:

```bash
# Use OpenAI
export LLM_PROVIDER=openai

# Use Anthropic
export LLM_PROVIDER=anthropic

# Use Ollama (local)
export LLM_PROVIDER=ollama
```

## Configuration

The client uses two main configuration approaches:

### Environment Variables

Configure LLM providers and Slack integration using environment variables:

| Variable              | Description                                  | Default    |
| --------------------- | -------------------------------------------- | ---------- |
| SLACK_BOT_TOKEN       | Bot token for Slack API                      | (required) |
| SLACK_APP_TOKEN       | App-level token for Socket Mode              | (required) |
| OPENAI_API_KEY        | API key for OpenAI authentication            | (required) |
| OPENAI_MODEL          | OpenAI model to use                          | gpt-4o     |
| ANTHROPIC_API_KEY     | API key for Anthropic authentication         | (required for Anthropic) |
| ANTHROPIC_MODEL       | Anthropic model to use                       | claude-3-5-sonnet-20241022 |
| LOG_LEVEL             | Logging level (debug, info, warn, error)     | info       |
| LLM_PROVIDER          | LLM provider to use (openai, anthropic, ollama) | openai     |
| LANGCHAIN_OLLAMA_URL  | URL for Ollama when using LangChain          | http://localhost:11434 |
| LANGCHAIN_OLLAMA_MODEL| Model name for Ollama when using LangChain   | llama3     |

### MCP Server Configuration

MCP servers are configured via a JSON configuration file (default: `mcp-servers.json`):

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/directory"],
      "env": {}
    },
    "github": {
      "command": "github-mcp-server",
      "args": ["stdio"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "your-token"
      }
    },
    "web-api": {
      "mode": "http",
      "url": "http://localhost:8080/mcp",
      "initialize_timeout_seconds": 30
    }
  }
}
```

For detailed configuration options, see the [Implementation Notes](docs/implementation.md).

## Slack-Formatted Output

The client includes a comprehensive Slack-formatted output system that enhances message display in Slack:

- **Automatic Format Detection**: Automatically detects message type (plain text, markdown, JSON Block Kit, structured data) and applies appropriate formatting
- **Markdown Formatting**: Supports Slack's mrkdwn syntax with automatic conversion from standard Markdown
  - Converts `**bold**` to `*bold*` for proper Slack bold formatting
  - Preserves inline code, block quotes, lists, and other formatting elements
- **Quoted String Enhancement**: Automatically converts double-quoted strings to inline code blocks for better visualization
  - Example: `"namespace-name"` becomes `` `namespace-name` `` in Slack
  - Improves readability of IDs, timestamps, and other quoted values
- **Block Kit Integration**: Converts structured data to Block Kit layouts for better visual presentation
  - Automatically validates against Slack API limits
  - Falls back to plain text if Block Kit validation fails

For more details, see the [Slack Formatting Guide](docs/format.md).

## Transport Modes

The client supports three transport modes:

- **SSE (default)**: Uses Server-Sent Events for real-time communication with the MCP server
- **HTTP**: Uses HTTP POST requests with JSON-RPC for communication
- **stdio**: Uses standard input/output for local development and testing

## Documentation

Comprehensive documentation is available in the `docs/` directory:

### Configuration & Setup
- **[Slack Configuration Guide](docs/configuration.md)** - Complete guide for setting up your Slack app, including required permissions, tokens, and troubleshooting common issues

### Development & Implementation
- **[Implementation Notes](docs/implementation.md)** - Detailed technical documentation covering the current architecture, core components, and implementation details
- **[Requirements Specification](docs/requirements.md)** - Comprehensive requirements documentation including implemented features, quality requirements, and future enhancements

### User Guides
- **[Slack Formatting Guide](docs/format.md)** - Complete guide to message formatting including Markdown-to-Slack conversion, Block Kit layouts, and automatic format detection
- **[RAG Implementation Guide](docs/rag-json.md)** - Detailed guide for the improved RAG system with LangChain Go compatibility and performance optimizations
- **[RAG SQLite Implementation](docs/rag-sqlite.md)** - Implementation plan for native Go SQLite integration with ChatGPT-like upload experience
- **[Testing Guide](docs/test.md)** - Comprehensive testing documentation covering unit tests, integration tests, manual testing procedures, and debugging

### Quick Links
- **Setup**: Start with the [Slack Configuration Guide](docs/configuration.md) for initial setup
- **RAG**: Check the [RAG Implementation Guide](docs/rag-json.md) for document knowledge base integration
- **Formatting**: See the [Slack Formatting Guide](docs/format.md) for message formatting capabilities
- **RAG SQLite**: See the [RAG SQLite Implementation](docs/rag-sqlite.md) for native Go implementation with modern upload UX
- **Development**: Check the [Implementation Notes](docs/implementation.md) for technical details
- **Testing**: Use the [Testing Guide](docs/test.md) for testing procedures and debugging

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
