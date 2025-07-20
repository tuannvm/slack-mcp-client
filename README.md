# Slack MCP Client

**A production-ready bridge between Slack and AI models with full MCP compatibility.**

This client enables AI models (OpenAI, Anthropic, Ollama) to interact with real tools and systems through Slack conversations. Built on the industry-standard Model Context Protocol (MCP), it provides secure access to filesystems, databases, Kubernetes clusters, Git repositories, and custom tools.

[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/tuannvm/slack-mcp-client/build.yml?branch=main&label=CI%2FCD&logo=github)](https://github.com/tuannvm/slack-mcp-client/actions/workflows/build.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/tuannvm/slack-mcp-client?logo=go)](https://github.com/tuannvm/slack-mcp-client/blob/main/go.mod)
[![Trivy Scan](https://img.shields.io/github/actions/workflow/status/tuannvm/slack-mcp-client/build.yml?branch=main&label=Trivy%20Security%20Scan&logo=aquasec)](https://github.com/tuannvm/slack-mcp-client/actions/workflows/build.yml)
[![Docker Image](https://img.shields.io/github/v/release/tuannvm/slack-mcp-client?sort=semver&label=GHCR&logo=docker)](https://github.com/tuannvm/slack-mcp-client/pkgs/container/slack-mcp-client)
[![GitHub Release](https://img.shields.io/github/v/release/tuannvm/slack-mcp-client?sort=semver)](https://github.com/tuannvm/slack-mcp-client/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> **Compatible with MCP Specification 2025-06-18** - Compliant with the latest Model Context Protocol standards

## Key Features

- **Universal MCP Compatibility** - Supports all transport methods (HTTP, SSE, stdio)
- **Multi-Provider LLM Support** - OpenAI GPT-4o, Anthropic Claude, Ollama local models
- **Agent Mode** - Multi-step reasoning with LangChain for complex workflows
- **RAG Integration** - Knowledge base with semantic search capabilities
- **Production Ready** - Comprehensive configuration, monitoring, and security

## Use Cases

- **DevOps Teams** - Infrastructure automation and monitoring through Slack
- **Development Teams** - Code review, Git operations, and file management
- **Support Teams** - Database queries, system status checks, and troubleshooting
- **General Use** - AI assistance with actual tools and system integration

## MCP Compatibility

**Compliant with the official Model Context Protocol (2025-06-18 specification):**

- **All Transport Methods** - HTTP, SSE, and stdio protocols
- **JSON-RPC 2.0** - Standard communication protocol
- **Official MCP Servers** - Compatible with all [modelcontextprotocol/servers](https://github.com/modelcontextprotocol/servers)
- **Custom MCP Servers** - Works with any MCP-compliant server
- **Security Standards** - Implements user consent, data privacy, and tool safety requirements

## How It Works

![Image](https://github.com/user-attachments/assets/48a587e4-7895-4a6f-9745-61b21894c34c)

```mermaid
flowchart LR
    User([👤 User]) --> Slack{🔗 Slack Interface}

    subgraph Infrastructure[Observability]
        Config[📋 Unified Config<br/>JSON Schema]
        Monitoring[📊 Monitoring<br/>Prometheus Metrics]
        Logging[📝 Structured Logging<br/>Debug & Analytics]
    end
    
    subgraph Core[Features]
        Slack --> Bridge[🌉 LLM-MCP Bridge<br/>Orchestration Layer]
        
        subgraph LLM[🤖 AI Processing]
            Bridge --> LLMRegistry[LLM Provider Registry]
            LLMRegistry --> OpenAI[OpenAI<br/>GPT-4o]
            LLMRegistry --> Anthropic[Anthropic<br/>Claude]
            LLMRegistry --> Ollama[Ollama<br/>Local Models]
            
            Bridge --> Agent{🎯 Agent Mode?}
            Agent -->|Yes| LangChain[🔄 LangChain Agent<br/>Multi-step Reasoning]
            Agent -->|No| Standard[⚡ Standard Mode<br/>Single Response]
        end
        
        subgraph Knowledge[📚 Knowledge & Memory]
            Bridge --> RAG[🧠 RAG System]
            RAG --> SimpleRAG[📄 JSON Store<br/>Simple Documents]
            RAG --> VectorRAG[🔍 OpenAI Vector Store<br/>Semantic Search]
        end
        
        subgraph Tools[🛠️ MCP Mode]
            Bridge --> MCPManager[MCP Client]
            MCPManager --> FileSystem[📁 Filesystem MCP Server<br/>Read/Write Files]
            MCPManager --> Git[🌿 Git MCP Server <br/>Repository Tools]
            MCPManager --> Kubernetes[☸️ Kubernetes MCP Server<br/>Cluster Management]
        end
    end
    
    
    Config -.-> Core
    Core -.-> Monitoring
    Core -.-> Logging
    
    style Core fill:#F8F9FA,stroke:#6C757D,stroke-width:3px
    style LLM fill:#E3F2FD,stroke:#1976D2,stroke-width:2px
    style Knowledge fill:#E8F5E8,stroke:#388E3C,stroke-width:2px
    style Tools fill:#FFF3E0,stroke:#F57C00,stroke-width:2px
    style Infrastructure fill:#F3E5F5,stroke:#7B1FA2,stroke-width:2px
    
    style User fill:#4CAF50,stroke:#2E7D32,stroke-width:2px,color:#fff
    style Slack fill:#4A90E2,stroke:#1565C0,stroke-width:2px,color:#fff
    style Bridge fill:#FF9800,stroke:#E65100,stroke-width:2px,color:#fff
    style LangChain fill:#9C27B0,stroke:#4A148C,stroke-width:2px,color:#fff
    style RAG fill:#2196F3,stroke:#0D47A1,stroke-width:2px,color:#fff
```

1. **User** interacts through Slack, sending messages that trigger intelligent AI workflows
2. **LLM-MCP Bridge** serves as the intelligent orchestration layer that:
   - Routes requests to appropriate LLM providers (OpenAI, Anthropic, Ollama)
   - Chooses between Agent Mode (multi-step reasoning) or Standard Mode (single response)
   - Integrates RAG system for knowledge retrieval and context enhancement
   - Manages tool discovery and execution across multiple MCP servers
3. **Knowledge & Memory** system provides contextual intelligence:
   - Simple JSON store for lightweight document storage
   - OpenAI Vector Store for semantic search and enterprise-grade RAG
4. **Tool Ecosystem** connects to diverse external systems:
   - Filesystem operations for file management
   - Git integration for repository interactions  
   - Kubernetes cluster management and monitoring
   - Custom tools via HTTP, SSE, or stdio protocols
5. **Infrastructure** ensures production-ready deployment:
   - Unified JSON configuration with environment variable support
   - Prometheus metrics for observability and monitoring
   - Structured logging for debugging and analytics

## Features

- ✅ **Multi-Mode MCP Client**: 
  - Server-Sent Events (SSE) for real-time communication with automatic retry
  - HTTP transport for JSON-RPC
  - stdio for local development and testing
- ✅ **Slack Integration**: 
  - Uses Socket Mode for secure, firewall-friendly communication
  - Works with both channels and direct messages
  - Rich message formatting with Markdown and Block Kit
  - Customizable bot behavior and message history
- ✅ **Multi-Provider LLM Support**:
  - OpenAI (GPT-4, GPT-4o, etc.)
  - Anthropic (Claude 3.5 Sonnet, etc.) 
  - Ollama (Local LLMs like Llama, Mistral, etc.)
  - Native tool calling and agent mode support
  - LangChain gateway for unified API
- ✅ **Agent Mode**:
  - Autonomous AI agents powered by LangChain
  - Multi-step reasoning and tool orchestration
  - Configurable agent iterations and behavior
  - Streaming responses with real-time updates
  - Advanced prompt engineering capabilities
- ✅ **RAG (Retrieval-Augmented Generation)**: 
  - Multiple providers: Simple JSON storage, OpenAI Vector Store
  - Reusable vector stores with `vectorStoreId` support
  - Configurable search parameters and similarity metrics
  - PDF ingestion with intelligent chunking
  - CLI tools for document management
- ✅ **Unified Configuration**:
  - Single JSON configuration file with JSON schema validation
  - Comprehensive timeout and retry configuration
  - Environment variable substitution and overrides
  - All underlying package options exposed
  - Smart defaults with full customization capability
- ✅ **Production Ready**:
  - Docker container support with GHCR publishing
  - Kubernetes Helm charts with OCI registry
  - Comprehensive logging and error handling
  - 88%+ test coverage with security scanning
- ✅ **Monitoring & Observability**:
  - Prometheus metrics integration
  - Tool invocation tracking with error rates
  - LLM token usage monitoring by model and type
  - Configurable metrics endpoint and logging levels

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

2. Create a unified configuration file:

```bash
# Create config.json with the new unified configuration format
cat > config.json << EOL
{
  "\$schema": "https://github.com/tuannvm/slack-mcp-client/schema/config-schema.json",
  "version": "2.0",
  "slack": {
    "botToken": "\${SLACK_BOT_TOKEN}",
    "appToken": "\${SLACK_APP_TOKEN}"
  },
  "llm": {
    "provider": "openai",
    "useNativeTools": true,
    "providers": {
      "openai": {
        "model": "gpt-4o",
        "apiKey": "\${OPENAI_API_KEY}",
        "temperature": 0.7
      }
    }
  },
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "\$HOME"]
    }
  },
  "monitoring": {
    "enabled": true,
    "metricsPort": 8080,
    "loggingLevel": "info"
  }
}
EOL
```

3. Run the application:

```bash
# Run with unified configuration (looks for config.json in current directory)
slack-mcp-client --config config.json

# Enable debug mode with structured logging
slack-mcp-client --config config.json --debug

# Validate configuration before running
slack-mcp-client --config-validate --config config.json

# Configure metrics port via config file or flag
slack-mcp-client --config config.json --metrics-port 9090
```

### Migrating from Legacy Configuration

If you have an existing `mcp-servers.json` file from a previous version, you can migrate to the new unified configuration format:

```bash
# Automatic migration (recommended)
slack-mcp-client --migrate-config --config legacy-mcp-servers.json --output config.json

# Manual migration: Use examples as templates
cp examples/minimal.json config.json
# Edit config.json with your specific settings

# Validate the new configuration
slack-mcp-client --config-validate --config config.json
```

The new configuration format provides:
- **Single File**: All settings in one `config.json` file
- **JSON Schema**: IDE support with autocomplete and validation
- **Environment Variables**: Use `${VAR_NAME}` syntax for secrets
- **Smart Defaults**: Minimal configuration required for basic usage
- **Comprehensive Options**: All underlying package settings exposed

The application will connect to Slack and start listening for messages. You can check the logs for any errors or connection issues.

### RAG Setup and Usage

The client includes an improved RAG (Retrieval-Augmented Generation) system that's compatible with LangChain Go and provides professional-grade performance:

#### Quick Start with RAG

1. **Enable RAG in your configuration:**

```json
{
  "$schema": "https://github.com/tuannvm/slack-mcp-client/schema/config-schema.json",
  "version": "2.0",
  "slack": {
    "botToken": "${SLACK_BOT_TOKEN}",
    "appToken": "${SLACK_APP_TOKEN}"
  },
  "llm": {
    "provider": "openai",
    "useNativeTools": true,
    "providers": {
      "openai": {
        "model": "gpt-4o",
        "apiKey": "${OPENAI_API_KEY}"
      }
    }
  },
  "rag": {
    "enabled": true,
    "provider": "simple",
    "chunkSize": 1000,
    "providers": {
      "simple": {
        "databasePath": "./knowledge.json"
      },
      "openai": {
        "indexName": "my-knowledge-base",
        "vectorStoreId": "vs_existing_store_id",
        "dimensions": 1536,
        "maxResults": 10
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
  "$schema": "https://github.com/tuannvm/slack-mcp-client/schema/config-schema.json",
  "version": "2.0",
  "slack": {
    "botToken": "${SLACK_BOT_TOKEN}",
    "appToken": "${SLACK_APP_TOKEN}"
  },
  "llm": {
    "provider": "openai",
    "useNativeTools": true,
    "customPrompt": "You are a helpful DevOps assistant specializing in Kubernetes and cloud infrastructure.",
    "providers": {
      "openai": {
        "model": "gpt-4o",
        "apiKey": "${OPENAI_API_KEY}",
        "temperature": 0.7
      }
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

### Agent Mode

Agent Mode enables more interactive and context-aware conversations using LangChain's agent framework. Instead of single-prompt interactions, agents can engage in multi-step reasoning, use tools more strategically, and maintain better context throughout conversations.

#### How Agent Mode Works

Agent Mode uses LangChain's conversational agent framework to provide:

1. **Interactive Conversations**: Maintains context across multiple exchanges
2. **Strategic Tool Usage**: Agents decide when and how to use available tools
3. **Multi-Step Reasoning**: Can break down complex problems into manageable steps
4. **Streaming Responses**: Provides real-time updates during processing
5. **User Context Integration**: Incorporates user information for personalized responses

#### Agent Mode Configuration

Enable Agent Mode in your configuration file:

```json
{
  "$schema": "https://github.com/tuannvm/slack-mcp-client/schema/config-schema.json",
  "version": "2.0",
  "slack": {
    "botToken": "${SLACK_BOT_TOKEN}",
    "appToken": "${SLACK_APP_TOKEN}"
  },
  "llm": {
    "provider": "openai",
    "useNativeTools": true,
    "useAgent": true,
    "customPrompt": "You are a DevOps expert specializing in Kubernetes and cloud infrastructure. Always think through problems step by step.",
    "maxAgentIterations": 25,
    "providers": {
      "openai": {
        "model": "gpt-4o",
        "apiKey": "${OPENAI_API_KEY}",
        "temperature": 0.7
      }
    }
  },
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/project"]
    },
    "github": {
      "command": "github-mcp-server",
      "args": ["stdio"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_TOKEN}"
      }
    }
  }
}
```

#### Configuration Options

- **`llm.useAgent`**: Enable agent mode (default: false)
- **`llm.useNativeTools`**: Use native LangChain tools vs system prompt-based tools (default: false)
- **`llm.customPrompt`**: System prompt for agent behavior
- **`llm.maxAgentIterations`**: Maximum agent reasoning steps (default: 20)

#### Agent vs Standard Mode

**Standard Mode**:
- Single-prompt interactions
- Tools described in system prompt as JSON schemas
- Direct tool call parsing and execution
- More predictable token usage
- Simpler conversation flow

**Agent Mode**:
- Multi-turn conversational interactions
- Context-aware tool usage decisions
- Better user context integration
- More natural conversation flow
- Enhanced reasoning capabilities

#### Agent Mode Examples

**Interactive Development Consultation**:
```
User: "I need help optimizing my React app performance"

Agent Response:
🤖 I'd be happy to help optimize your React app performance! Let me understand your current setup better.

[Agent maintains conversation context and asks relevant follow-up questions]
Agent: "What specific performance issues are you experiencing? Are you seeing slow renders, large bundle sizes, or something else?"

User: "The app takes too long to load initially"

Agent: "Let me check your current bundle setup and suggest optimizations..."
[Agent uses filesystem tools to analyze the project structure and provides targeted advice]
```

**Contextual Problem Solving**:
```
User: "Can you help me with my deployment pipeline?"

Agent Response:
🤖 I'll help you with your deployment pipeline. Since I know you're working on a React project, let me check your current CI/CD setup.

[Agent leverages previous conversation context and user information to provide personalized assistance]
[Agent strategically uses relevant tools based on the conversation flow]
```

#### Agent Mode Best Practices

1. **System Prompts**: Design clear, specific system prompts that guide the agent's behavior
2. **Tool Selection**: Provide relevant tools for the agent's domain
3. **Context Management**: Agents maintain better context across conversations
4. **User Personalization**: Leverage user context integration for personalized responses
5. **Tool Strategy**: Choose between native tools or system prompt-based tools based on your needs

#### Limitations and Considerations

- **OpenAI Agent**: Native OpenAI agent in langchaingo has known issues, uses conversational agent as workaround
- **LangChain Dependency**: Agent mode requires LangChain provider
- **Permissions**: May require additional Slack permissions for user information retrieval
- **Performance**: Agent mode may have different performance characteristics than standard mode

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
3. Check `Allow users to send Slash commands and messages from the chat tab` in App Home page, to enable direct message to Slack app.
![image](https://github.com/wyangsun/wyangsun.github.io/blob/master/slackapp.png)
4. Add the following Bot Token Scopes:
   - `app_mentions:read`
   - `chat:write`
   - `im:history`
   - `im:read`
   - `im:write`
   - `users:read`
   - `users.profile:read`
5. Enable Event Subscriptions and subscribe to:
   - `app_mention`
   - `message.im`
6. Install the app to your workspace

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

### Monitoring Configuration

The client includes Prometheus metrics support for monitoring tool usage and performance:

- **Metrics Endpoint**: Accessible at `/metrics` on the configured port
- **Default Port**: 8080 (configurable via `--metrics-port` flag)
- **Metrics Available**:
  - `slackmcp_tool_invocations_total`: Counter for tool invocations with labels for tool name, server, and error status
  - `slackmcp_llm_tokens`: Histogram for LLM token usage by type and model

Example metrics access:
```bash
# Access metrics endpoint
curl http://localhost:8080/metrics

# Run with custom metrics port
slack-mcp-client --metrics-port 9090
```

### Unified Configuration Format

All configuration is now managed through a single `config.json` file with comprehensive options:

```json
{
  "$schema": "https://github.com/tuannvm/slack-mcp-client/schema/config-schema.json",
  "version": "2.0",
  "slack": {
    "botToken": "${SLACK_BOT_TOKEN}",
    "appToken": "${SLACK_APP_TOKEN}",
    "messageHistory": 50,
    "thinkingMessage": "Processing..."
  },
  "llm": {
    "provider": "openai",
    "useNativeTools": true,
    "useAgent": false,
    "customPrompt": "You are a helpful assistant.",
    "maxAgentIterations": 20,
    "providers": {
      "openai": {
        "model": "gpt-4o",
        "apiKey": "${OPENAI_API_KEY}",
        "temperature": 0.7,
        "maxTokens": 2000
      },
      "anthropic": {
        "model": "claude-3-5-sonnet-20241022",
        "apiKey": "${ANTHROPIC_API_KEY}",
        "temperature": 0.7
      }
    }
  },
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"],
      "initializeTimeoutSeconds": 30,
      "tools": {
        "allowList": ["read_file", "write_file", "list_directory"],
        "blockList": ["delete_file"]
      }
    },
    "web-api": {
      "url": "http://localhost:8080/mcp",
      "transport": "sse",
      "initializeTimeoutSeconds": 30
    }
  },
  "rag": {
    "enabled": true,
    "provider": "openai",
    "chunkSize": 1000,
    "providers": {
      "openai": {
        "vectorStoreId": "vs_existing_store_id",
        "dimensions": 1536,
        "maxResults": 10
      }
    }
  },
  "timeouts": {
    "httpRequestTimeout": "30s",
    "toolProcessingTimeout": "3m",
    "mcpInitTimeout": "30s"
  },
  "retry": {
    "maxAttempts": 3,
    "baseBackoff": "500ms",
    "maxBackoff": "5s"
  },
  "monitoring": {
    "enabled": true,
    "metricsPort": 8080,
    "loggingLevel": "info"
  }
}
```

For detailed configuration options and migration guides, see the [Configuration Guide](docs/configuration.md).

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

- **SSE (default)**: Uses Server-Sent Events for real-time communication with the MCP server, includes automatic retry logic for enhanced reliability
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
- **Agent Mode**: See the Agent Mode section above for autonomous AI agents with tool chaining
- **RAG**: Check the [RAG Implementation Guide](docs/rag-json.md) for document knowledge base integration
- **Formatting**: See the [Slack Formatting Guide](docs/format.md) for message formatting capabilities
- **RAG SQLite**: See the [RAG SQLite Implementation](docs/rag-sqlite.md) for native Go implementation with modern upload UX
- **Development**: Check the [Implementation Notes](docs/implementation.md) for technical details
- **Testing**: Use the [Testing Guide](docs/test.md) for testing procedures and debugging
- **Monitoring**: See the metrics configuration section above for Prometheus integration

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
