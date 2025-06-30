# Implementation Notes: Slack MCP Client

## Current State

1. **Project Goal:** Build a Slack bot that interacts with external tools and data sources via the Model Context Protocol (MCP), implemented in Go.
2. **Architecture:** See `README.md` for the high-level design. The core components are the Slack integration, the Go application (`slack-mcp-client`), and external MCP servers.
3. **Slack Integration:** Full-featured Slack client using `slack-go/slack` with Socket Mode for secure communication, supporting mentions, direct messages, and rich Block Kit formatting.
4. **MCP Client Configuration:** Uses a flexible configuration approach for multiple MCP servers through `mcp-servers.json` following the schema defined in `mcp-schema.json`.
5. **LLM Integration:** Multi-provider LLM support through a factory pattern with LangChain as the gateway, supporting OpenAI, Anthropic, and Ollama providers.

## Architecture & Implementation

### Core Components

1. **Configuration System (`internal/config/`)**
   - Configuration loaded from environment variables for Slack credentials and LLM settings
   - MCP server configurations from JSON file following the schema with `mcpServers` property
   - Each server defined with `command`, `args`, `mode`, and optional `env` properties
   - Support for both HTTP/SSE and stdio transport modes
   - LLM provider configuration with factory pattern support

2. **Slack Client (`internal/slack/`)**
   - Connects to Slack using Socket Mode for secure, firewall-friendly communication
   - Handles app mentions and direct messages
   - Processes user prompts and forwards them to LLM providers
   - Advanced message formatting with Block Kit support through `internal/slack/formatter/`
   - Returns responses and tool results back to Slack channels/DMs with rich formatting

3. **MCP Client (`internal/mcp/`)**
   - Support for multiple transport protocols:
     - **HTTP/SSE (Server-Sent Events):** For real-time communication with web-based MCP servers
     - **stdio:** For local development with command-line tools
   - Dynamic initialization with proper command line argument parsing
   - Runtime discovery of available tools from MCP servers
   - Uses mcp-go v0.31.0 with unified transport interface

4. **LLM Provider System (`internal/llm/`)**
   - Factory pattern for provider registration and initialization
   - Registry system for managing multiple LLM providers
   - Configuration-driven provider setup
   - LangChain as the unified gateway for all providers
   - Support for OpenAI, Anthropic, and Ollama providers
   - Availability checking and fallback mechanisms

5. **Handler System (`internal/handlers/`)**
   - Interface-based design for tool handlers
   - LLM-MCP Bridge for detecting tool invocation patterns
   - Registry for centralized handler management
   - Support for both structured JSON tool calls and natural language detection

6. **Common Utilities (`internal/common/`)**
   - Structured logging with hierarchical loggers (`internal/common/logging/`)
   - Standardized error handling (`internal/common/errors/`)
   - HTTP client with retry logic (`internal/common/http/`)
   - Shared types and utilities

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
    },
    "web-server": {
      "mode": "http",
      "url": "http://localhost:8080/mcp",
      "initialize_timeout_seconds": 30
    }
  }
}
```

### LLM Provider Configuration

LLM providers are configured in the main configuration with factory pattern support:

```yaml
llm_provider: "openai" # Which provider to use

llm_providers:
  openai:
    type: "openai"
    model: "gpt-4o"
    # api_key loaded from OPENAI_API_KEY env var
  ollama:
    type: "ollama"
    model: "llama3"
    base_url: "http://localhost:11434"
  anthropic:
    type: "anthropic"
    model: "claude-3-5-sonnet-20241022"
    # api_key loaded from ANTHROPIC_API_KEY env var
```

### Main Process Flow

1. **Start-up Sequence:**
   - Parse command-line arguments with debug flags
   - Load environment variables and configuration file
   - Initialize structured logging system
   - Initialize LLM provider registry with configured providers
   - Initialize MCP clients for each configured server
   - Connect to Slack using Socket Mode

2. **Message Processing:**
   - Receive messages from Slack (mentions or DMs)
   - Forward messages to configured LLM provider through registry
   - Process LLM response through LLM-MCP Bridge
   - If tool invocation is detected:
     - Execute appropriate MCP tool call
     - Format tool results with Slack-compatible formatting
   - Return final response to Slack with Block Kit formatting

3. **Tool Detection & Execution:**
   - JSON pattern matching for structured tool invocations
   - Regular expression matching for natural language requests
   - Validate against available tools discovered from MCP servers
   - Execute tool call with appropriate parameters
   - Process and format tool results with rich Slack formatting

### Slack Message Formatting

The application includes a comprehensive Slack formatting system:

1. **Automatic Format Detection:**
   - Plain text with mrkdwn formatting
   - JSON Block Kit structures
   - Structured data converted to Block Kit

2. **Markdown Support:**
   - Automatic conversion from standard Markdown to Slack mrkdwn
   - Support for bold, italic, strikethrough, code blocks, lists, links
   - Quoted string conversion to inline code blocks

3. **Block Kit Support:**
   - Headers, sections, fields, actions, dividers
   - Automatic field truncation for Slack limits
   - Rich interactive components

### Debugging & Troubleshooting

1. **Logging System:**
   - Structured logging with different levels (Debug, Info, Warn, Error)
   - Component-specific loggers for better tracking
   - Environment variable support for log level configuration

2. **MCP Transport Issues:**
   - Resolved stdio transport issues by sequential processing
   - Proper timeout handling for server initialization
   - Support for both HTTP/SSE and stdio transports

3. **Configuration Validation:**
   - Automatic fallback to available providers
   - Server disable/enable functionality
   - Environment variable overrides

### Future Improvements

1. **Enhanced Tool Discovery**
   - Better caching of discovered tools
   - Dynamic tool refresh capabilities
   - Tool usage analytics

2. **Advanced LLM Features**
   - Function calling support for compatible providers
   - Conversation context management
   - Multi-turn conversation support

3. **Monitoring & Observability**
   - Metrics collection for tool usage
   - Performance monitoring
   - Health check endpoints

4. **Security Enhancements**
   - Tool permission controls
   - User-based access restrictions
   - API key rotation support
