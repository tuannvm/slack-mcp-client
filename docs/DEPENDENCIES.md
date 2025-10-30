# Dependencies

This document tracks major dependencies and their versions for the Slack MCP Client.

## Core Dependencies

### LangChain Go

**Current Version**: v0.1.14 (Upgraded: 2025-10-29)

**Purpose**: LLM integration, agent framework, and tool orchestration

**Key Features Used**:
- Agent framework (ConversationalAgent, Executor)
- LLM providers (OpenAI, Anthropic, Ollama)
- Tool abstraction and callback handlers
- RAG components (document loaders, text splitters)

**Recent Updates**:
- **v0.1.14 (2025-10-29)**: Major stability and performance improvements
  - Fixed memory and goroutine leaks in streaming for OpenAI, Anthropic, Ollama
  - Enhanced agent parsing for multi-line tool calls
  - Improved error handling and API key sanitization
  - Panic prevention in streaming edge cases
  - See [v0.1.14 Upgrade Report](./v0.1.14-upgrade-plan.md) for details

**Documentation**: [github.com/tmc/langchaingo](https://github.com/tmc/langchaingo)

---

### Slack Go SDK

**Package**: `github.com/slack-go/slack`

**Purpose**: Slack API integration and Socket Mode communication

**Key Features Used**:
- Socket Mode for real-time messaging
- Block Kit message formatting
- User context and thread management
- Rich message formatting

**Documentation**: [github.com/slack-go/slack](https://github.com/slack-go/slack)

---

### Model Context Protocol (MCP)

**Purpose**: Standardized protocol for AI model-tool communication

**Transports Supported**:
- HTTP with JSON-RPC 2.0
- Server-Sent Events (SSE) with automatic retry
- stdio for local development

**Specification**: MCP 2025-06-18

**Documentation**: [modelcontextprotocol.io](https://modelcontextprotocol.io)

---

## Monitoring & Observability

### Prometheus

**Package**: `github.com/prometheus/client_golang`

**Purpose**: Metrics collection and monitoring

**Metrics Provided**:
- Tool invocation counters with error tracking
- LLM token usage histograms by model and type
- Endpoint: `/metrics` on configurable port (default: 8080)

---

### OpenTelemetry

**Packages**:
- `go.opentelemetry.io/otel`
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace`

**Purpose**: Distributed tracing for LLM operations and tool calls

**Providers Supported**:
- Simple OTLP for basic tracing
- Langfuse for advanced LLM observability

---

## Development Dependencies

### Testing

- `github.com/stretchr/testify` - Testing utilities and assertions

### Build & Release

- GoReleaser - Automated release management
- GitHub Actions - CI/CD pipeline
- Trivy - Security scanning
- golangci-lint - Code quality checks

---

## Dependency Management

### Upgrade Policy

1. **Security fixes**: Upgrade immediately
2. **Bug fixes**: Upgrade within 1 week if affecting us
3. **New features**: Upgrade when needed
4. **Major versions**: Plan carefully, expect breaking changes

### Monitoring

- Subscribe to release notifications for critical dependencies
- Quarterly review of outdated dependencies: `go list -u -m all`
- Security scanning in CI/CD pipeline

### Upgrade Process

Follow the [Upgrade Template](./UPGRADE_TEMPLATE.md) for consistent upgrade documentation:

1. Research release notes and breaking changes
2. Test in development environment
3. Document changes in upgrade report
4. Update this dependencies file
5. Deploy to staging, then production

---

## Version History

### langchaingo

| Version | Date | Changes | Report |
|---------|------|---------|--------|
| v0.1.14 | 2025-10-29 | Streaming fixes, agent improvements, security enhancements | [Report](./v0.1.14-upgrade-plan.md) |
| v0.1.13 | Previous | Initial version in use | - |

---

## Transitive Dependencies

Major transitive dependencies automatically managed by `go.mod`:

- `golang.org/x/net` - Network primitives
- `golang.org/x/sys` - System calls
- `golang.org/x/crypto` - Cryptography
- `google.golang.org/grpc` - gRPC for some MCP transports
- `google.golang.org/api` - Google Cloud APIs (for Vertex AI)

Run `go mod graph` to see the complete dependency tree.

---

## Security

### Vulnerability Scanning

Automated security scanning in CI/CD:
- **govulncheck**: Checks for known vulnerabilities in Go dependencies
- **Trivy**: Comprehensive dependency and container scanning
- **SBOM Generation**: Software Bill of Materials for tracking

### Reporting

To report security vulnerabilities, see [SECURITY.md](../SECURITY.md).

---

## License Compliance

All dependencies are vetted for license compatibility:
- Primary dependencies use permissive licenses (MIT, Apache 2.0, BSD)
- Full license information available in `go.mod` and vendored dependencies

Run `go-licenses csv .` to generate a complete license report.

---

## See Also

- [Upgrade Template](./UPGRADE_TEMPLATE.md) - Template for documenting dependency upgrades
- [v0.1.14 Upgrade Report](./v0.1.14-upgrade-plan.md) - Recent langchaingo upgrade
- [Implementation Notes](./implementation.md) - Technical architecture details
- [Configuration Guide](./configuration.md) - Dependency configuration
