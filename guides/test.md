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
