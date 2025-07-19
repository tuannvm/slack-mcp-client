# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Slack MCP (Model Context Protocol) Client written in Go that serves as a bridge between Slack and multiple MCP servers. It allows LLM models to interact with various tools through a unified Slack interface.

## Core Architecture

### Main Components
- **Slack Bot Client**: Handles Slack integration via Socket Mode
- **MCP Client Manager**: Manages connections to multiple MCP servers (HTTP/SSE, stdio)
- **LLM Provider Registry**: Supports OpenAI, Anthropic, Ollama via LangChain gateway
- **Tool Discovery System**: Dynamically registers tools from all connected MCP servers
- **RAG System**: Retrieval-Augmented Generation with JSON and OpenAI vector stores
- **Agent Mode**: LangChain-powered conversational agents with tool orchestration
- **Monitoring**: Prometheus metrics for tool usage and LLM token tracking

### Key Packages
- `cmd/main.go`: Application entry point and initialization
- `internal/config/`: Configuration management with environment variable overrides
- `internal/slack/`: Slack client implementation and message formatting
- `internal/mcp/`: MCP client implementations (SSE, HTTP, stdio)
- `internal/llm/`: LLM provider factories and LangChain integration
- `internal/rag/`: RAG providers and tool implementations
- `internal/handlers/`: Tool handlers and LLM-MCP bridge
- `internal/monitoring/`: Prometheus metrics

## Build and Development Commands

### Essential Commands
```bash
# Build the application
make build

# Run the application
make run

# Run tests
make test

# Run linting and formatting
make lint

# Check all (format, lint, vet, test)
make check

# Clean build artifacts
make clean
```

### Testing Commands
```bash
# Run all tests with verbose output
go test -v ./...

# Run tests with race detection
go test -race ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
```

### Docker Commands
```bash
# Build Docker image
make docker-build

# Run with Docker Compose
docker-compose up -d
```

## Configuration

### Environment Variables Required
- `SLACK_BOT_TOKEN`: Bot token for Slack API
- `SLACK_APP_TOKEN`: App-level token for Socket Mode
- `OPENAI_API_KEY`: OpenAI API key (default provider)
- `LLM_PROVIDER`: Provider selection (openai, anthropic, ollama)

### Configuration Files
- `mcp-servers.json`: MCP server definitions and tool configurations
- `.env`: Environment variables (optional)
- Config supports both legacy format and new `mcpServers` format

### RAG Configuration
RAG can be enabled via LLM provider config with `rag_enabled: true`. Supports JSON-based storage and OpenAI vector stores.

## Development Patterns

### MCP Server Integration
1. MCP servers are configured in `mcp-servers.json` with command/args or URL
2. Clients support stdio, HTTP, and SSE transport modes
3. Tool discovery happens at startup with allow/block lists
4. Failed servers are logged but don't crash the application

### LLM Provider Pattern
1. Factory pattern in `internal/llm/` for provider creation
2. LangChain gateway provides unified interface
3. Environment variables override config file settings
4. Supports native tools vs system prompt-based tools

### Error Handling
1. Domain-specific errors in `internal/common/errors/`
2. Graceful degradation when MCP servers fail
3. Comprehensive logging with structured fields
4. Circuit breaker pattern for failed connections

### Testing Strategy
1. Unit tests for core business logic
2. Integration tests for MCP client functionality
3. Mock interfaces for external dependencies
4. Test coverage tracking in CI/CD

## Agent Mode vs Standard Mode

### Standard Mode (Default)
- Single-prompt interactions with tool descriptions in system prompt
- Direct JSON tool call parsing and execution
- Predictable token usage and conversation flow

### Agent Mode
- Multi-turn conversational interactions via LangChain agents
- Context-aware tool usage decisions
- Better user context integration and reasoning capabilities
- Enable with `use_agent: true` in config

## Monitoring and Observability

### Prometheus Metrics
- Tool invocation counters with error rates
- LLM token usage histograms by model and type
- Metrics endpoint at `:8080/metrics` (configurable)

### Logging
- Structured logging with configurable levels
- Component-specific loggers for MCP servers
- Debug mode for detailed MCP communication

## Common Development Tasks

### Adding New MCP Server
1. Add server config to `mcp-servers.json`
2. Test connection with `--debug` flag
3. Verify tool discovery in logs
4. Add to allow/block lists if needed

### Adding New LLM Provider
1. Create factory in `internal/llm/`
2. Implement LangChain integration
3. Add environment variable handling in `config.go`
4. Update provider constants

### Debugging MCP Issues
1. Enable `--mcpdebug` for MCP client logs
2. Check server initialization timeouts
3. Verify NPM packages for JavaScript servers
4. Test stdio vs HTTP transport modes