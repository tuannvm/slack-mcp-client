
Ran tool
The v0.21.0 release of [mcp-go](https://github.com/mark3labs/mcp-go/compare/v0.20.1...v0.21.0) brings in a handful of features and fixes that we can immediately adopt to clean up and harden our Slack MCP client:

1. **Upgrade to v0.21.0**  
   – Bump your `go.mod` to require `github.com/mark3labs/mcp-go v0.21.0`.  
   – Run `go get github.com/mark3labs/mcp-go@v0.21.0 && go mod tidy`.

2. Unified Transport Interface  
   – The official client now exposes a transport‐agnostic interface that works over SSE, HTTP, or stdio.  
   – We should refactor our internal client code to use the new `transport.Interface` instead of separate SSE vs. stdio logic. That will simplify switching modes and make our code more resilient to future additions (e.g. WebSockets).

3. **`omitempty` on Tool Inputs**  
   – They fixed the issue where empty tool inputs came through as `null` by tagging maps with `omitempty`.  
   – Audit our own tool‐input structs (in `internal/llms/…` or wherever you define `ToolInput`) and add ``json:"…,omitempty"`` to any optional fields so downstream clients (e.g. Claude Desktop) don’t silently skip tools.

   Example:  
   ```go
   type HelloToolInput struct {
     Name string `json:"name,omitempty"`
   }
   ```

4. **DefaultArray Support**  
   – v0.21.0 adds `DefaultArray` so you can specify defaults for array parameters.  
   – If any of your tools accept array arguments, declare them with the new helper so you don’t have to write manual nil checks.

5. **Tool Annotation Metadata**  
   – They introduced a way to annotate tools with rich metadata (id, name, description, etc.).  
   – We can decorate our registered tools with these annotations so that any UI (Cursor, Windsurf) can render nicer tool listings automatically.

6. **Improved SSE Lifecycle**  
   – The new client fixes leaks, adds a `cancelSSEStream` helper, and renames `done`→`closed`.  
   – Replace any direct SSE usage in our code with the new client’s stream API to get correct cleanup and timeout handling “for free.”

7. **New Error Helper**  
   – There’s now a `NewToolResultErrorFromErr` constructor that wraps underlying errors cleanly.  
   – Use this in your tool‐invocation code so you surface tool failures with proper context.

Next Steps  
1. Update `go.mod` and imports.  
2. Run `go build` / `go test` and fix any API‐breaking changes (there should be minimal; most of this is additive).  
3. Replace custom transport logic with the unified transport interface.  
4. Add `omitempty` tags and default‐array values where appropriate.  
5. Verify that your Slack bot still passes the CI/CD pipeline (lint, tests, Docker, GoReleaser).  

By pulling in v0.21.0 you’ll get a more robust, cleaner, and feature‐complete foundation—making it easier to maintain and less error‑prone when connecting to multiple MCP servers over different transports.

```
├── internal
│   ├── common
│   │   ├── errors
│   │   │   ├── app_errors.go
│   │   │   └── errors.go
│   │   ├── http
│   │   │   └── client.go
│   │   ├── logging
│   │   │   └── logger.go
│   │   └── types.go
│   ├── config
│   │   └── config.go
│   ├── handlers
│   │   └── llm_mcp_bridge.go
│   │   ├── handler.go
│   │   ├── gateway_handler.go
│   │   ├── registry.go
│   ├── llm
│   │   ├── langchain.go
│   │   ├── openai.go
│   │   ├── provider.go
│   │   └── registry.go
│   ├── mcp
│   │   ├── client.go
│   │   ├── mcp.go
│   │   └── server.go
│   └── slack
│       ├── client.go
│       └── llm_client.go

## Post-Restructuring Improvements

After restructuring the codebase to have a cleaner, more maintainable directory structure, there are still some code duplications that need to be addressed:

### Eliminate Duplication between Slack Client and LLM Implementation

The `internal/slack/client.go` file contains duplicate implementations of OpenAI API structures and direct API calls, when we've already consolidated this functionality in the `internal/llm` package. To fix this:

1. **Remove Duplicated OpenAI Structures**: 
   - Remove `openaiMessage`, `openaiRequest`, `openaiChoice`, and other OpenAI-specific structures from `slack/client.go`
   - Use the consolidated implementations from the `internal/llm` package instead

2. **Leverage the LLM Gateway Handler**:
   - Modify the Slack client to use the `LLMGatewayHandler` from `internal/handlers/gateway_handler.go` for all LLM operations
   - Replace direct API calls to OpenAI with calls through our abstraction layer
   - This centralizes LLM provider management and simplifies future changes

3. **Simplify LLM Interface in Slack Client**:
   - Refactor `callLLM` method in the Slack client to delegate to our gateway handler
   - This eliminates code duplication and ensures consistent handling across providers

### Implementation Plan

1. Create a lightweight LLM client in `internal/slack/llm_client.go` that wraps our gateway handler
2. Update `internal/slack/client.go` to use this client instead of direct API calls
3. Remove all OpenAI-specific code from the Slack client
4. Ensure all LLM operations go through our abstraction layer

This will complete the restructuring by ensuring we have a truly unified approach to LLM operations throughout the codebase.
```
