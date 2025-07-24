# Configuration Guide

This document provides comprehensive configuration guidance for the Slack MCP Client, covering everything from basic setup to advanced deployment scenarios.

## Overview

The Slack MCP Client has evolved from a simple MCP server connector to a comprehensive LLM orchestration platform. This guide outlines the unified configuration approach that prioritizes simplicity, DRY principles, and excellent user experience.

### Naming Convention

Configuration files use **camelCase** naming for JSON fields (e.g., `botToken`, `mcpServers`, `useNativeTools`). This follows modern JSON API conventions and provides better IDE support with the included JSON schema.

> **Note**: The application automatically detects and converts legacy snake_case configurations for backward compatibility.

## Configuration Architecture

### Philosophy: Single File, Logical Sections
**Reject complexity. Embrace simplicity.**

Instead of multiple configuration files that create cognitive overhead, use a **single, well-structured configuration file** with logical sections.

### Unified Configuration Structure

```
config.json                     # Single configuration file
custom-prompt.txt               # Optional custom prompt file
├── examples/
│   ├── minimal.json            # Minimal setup example
│   ├── development.json        # Development config example
│   ├── production.json         # Production config example
│   └── custom-prompt.txt       # Custom prompt example
├── schema/
│   └── config-schema.json      # JSON schema for validation
└── scripts/
    └── migrate-config.sh       # Migration utility
```

### JSON Schema Support

The configuration includes comprehensive JSON schema support for enhanced developer experience:

- **Schema Reference**: Include `"$schema": "https://github.com/tuannvm/slack-mcp-client/schema/config-schema.json"` for IDE support
- **Autocomplete**: IDEs provide intelligent autocomplete for configuration fields
- **Validation**: Real-time validation of field types, required fields, and value constraints
- **Documentation**: Inline field descriptions and examples

## Complete Configuration Reference

Below is the complete configuration schema showing all available options. Fields marked with ⭐ are **required**, fields marked with ⚙️ have **smart defaults**, and fields marked with 🔧 are **optional**.

```json
{
  "$schema": "https://github.com/tuannvm/slack-mcp-client/schema/config-schema.json",
  "version": "2.0",                                    // ⭐ Required
  "slack": {
    "botToken": "${SLACK_BOT_TOKEN}",                 // ⭐ Required
    "appToken": "${SLACK_APP_TOKEN}",                 // ⭐ Required
    "messageHistory": 50,                             // ⚙️ Default: 50 messages per channel
    "thinkingMessage": "Thinking..."                  // ⚙️ Default: "Thinking..."
  },
  "llm": {
    "provider": "openai",                             // ⚙️ Default: "openai"
    "useNativeTools": false,                          // ⚙️ Default: false
    "useAgent": false,                                // ⚙️ Default: false
    "customPrompt": "You are a helpful assistant.",   // 🔧 Optional
    "customPromptFile": "custom-prompt.txt",          // 🔧 Optional
    "replaceToolPrompt": false,                       // ⚙️ Default: false
    "maxAgentIterations": 20,                         // ⚙️ Default: 20 (maximum reasoning steps for agent mode)
    "providers": {
      "openai": {
        "model": "gpt-4o",                            // ⚙️ Default: "gpt-4o"
        "apiKey": "${OPENAI_API_KEY}",                // ⭐ Required if using OpenAI
        "temperature": 0.7,                           // ⚙️ Default: 0.7
        "maxTokens": 2000                             // 🔧 Optional
      },
      "anthropic": {
        "model": "claude-3-5-sonnet-20241022",        // ⚙️ Default: "claude-3-5-sonnet-20241022"
        "apiKey": "${ANTHROPIC_API_KEY}",             // ⭐ Required if using Anthropic
        "temperature": 0.7                            // ⚙️ Default: 0.7
      },
      "ollama": {
        "model": "llama3",                            // ⚙️ Default: "llama3"
        "baseUrl": "http://localhost:11434",          // ⚙️ Default: "http://localhost:11434"
        "temperature": 0.7                            // ⚙️ Default: 0.7
      }
    }
  },
  "mcpServers": {
    "server-name": {
      "command": "npx",                               // 🔧 Optional (required if not using url)
      "args": ["-y", "@modelcontextprotocol/server"], // 🔧 Optional
      "url": "http://localhost:3000/sse",             // 🔧 Optional (required if not using command)
      "transport": "stdio",                           // ⚙️ Smart default: "stdio" for command, "sse" for url
      "env": {                                        // 🔧 Optional
        "DEBUG": "true"
      },
      "disabled": false,                              // ⚙️ Default: false
      "initializeTimeoutSeconds": 30,                 // ⚙️ Default: 30
      "tools": {
        "allowList": ["tool1", "tool2"],              // 🔧 Optional
        "blockList": ["dangerous_tool"]               // 🔧 Optional
      }
    }
  },
  "rag": {
    "enabled": false,                                 // ⚙️ Default: false
    "provider": "simple",                             // ⚙️ Default: "simple"
    "chunkSize": 1000,                                // ⚙️ Default: 1000
    "providers": {
      "simple": {
        "databasePath": "./rag.db"                    // ⚙️ Default: "./rag.db"
      },
      "openai": {
        "indexName": "slack-mcp-rag",                 // ⚙️ Default: "slack-mcp-rag"
        "vectorStoreId": "vs_existing_store_id",      // 🔧 Optional: reuse existing vector store
        "dimensions": 1536,                           // ⚙️ Default: 1536
        "similarityMetric": "cosine",                 // 🔧 Optional: cosine, euclidean
        "maxResults": 10                              // ⚙️ Default: 10 search results
      }
    }
  },
  "timeouts": {
    "httpRequestTimeout": "30s",                      // ⚙️ Default: 30s
    "mcpInitTimeout": "30s",                          // ⚙️ Default: 30s
    "toolProcessingTimeout": "3m",                    // ⚙️ Default: 3m
    "bridgeOperationTimeout": "3m",                   // ⚙️ Default: 3m
    "pingTimeout": "5s",                              // ⚙️ Default: 5s
    "responseProcessing": "1m"                        // ⚙️ Default: 1m
  },
  "retry": {
    "maxAttempts": 3,                                 // ⚙️ Default: 3 attempts
    "baseBackoff": "500ms",                           // ⚙️ Default: 500ms
    "maxBackoff": "5s",                               // ⚙️ Default: 5s
    "mcpReconnectAttempts": 5,                        // ⚙️ Default: 5 attempts
    "mcpReconnectBackoff": "1s"                       // ⚙️ Default: 1s
  },
  "monitoring": {
    "enabled": true,                                  // ⚙️ Default: true
    "metricsPort": 8080,                              // ⚙️ Default: 8080
    "loggingLevel": "info"                            // ⚙️ Default: "info"
  }
}
```

**Legend:**
- ⭐ **Required**: Must be provided or application will fail to start
- ⚙️ **Smart Default**: Automatically set if not specified
- 🔧 **Optional**: Can be omitted, no default value

## Configuration Examples by User Type

### 1. Quick Start User (5 minutes)
**Need**: Get up and running in minutes with minimal setup

```json
{
  "$schema": "https://github.com/tuannvm/slack-mcp-client/schema/config-schema.json",
  "version": "2.0",
  "slack": {
    "botToken": "${SLACK_BOT_TOKEN}",
    "appToken": "${SLACK_APP_TOKEN}"
  },
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"]
    }
  }
}
```

### 2. Production User (15 minutes)
**Need**: Robust configuration with monitoring and security

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
    "customPrompt": "You are a DevOps assistant."
  },
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"],
      "tools": {
        "allowList": ["read_file", "write_file", "list_directory"]
      }
    },
    "api-server": {
      "url": "https://api.company.com/mcp",
      "transport": "sse",
      "tools": {
        "allowList": ["weather", "search"]
      }
    }
  },
  "rag": {
    "enabled": true,
    "provider": "openai"
  },
  "monitoring": {
    "enabled": true,
    "metricsPort": 8080,
    "loggingLevel": "info"
  }
}
```

### 3. Advanced User (30 minutes)
**Need**: Maximum customization and control

```json
{
  "$schema": "https://github.com/tuannvm/slack-mcp-client/schema/config-schema.json",
  "version": "2.0",
  "slack": {
    "botToken": "${SLACK_BOT_TOKEN}",
    "appToken": "${SLACK_APP_TOKEN}"
  },
  "llm": {
    "provider": "anthropic",
    "useNativeTools": true,
    "useAgent": true,
    "customPromptFile": "custom-prompt.txt",
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
        "temperature": 0.5
      },
      "ollama": {
        "model": "llama3.1:8b",
        "baseUrl": "http://localhost:11434",
        "temperature": 0.8
      }
    }
  },
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"],
      "env": {
        "DEBUG": "true"
      },
      "initializeTimeoutSeconds": 60,
      "tools": {
        "blockList": ["delete_file"]
      }
    },
    "kubernetes": {
      "command": "kubectl-mcp-server",
      "args": ["--context", "production"],
      "transport": "stdio",
      "tools": {
        "allowList": ["get_pods", "get_services", "describe_pod"]
      }
    }
  },
  "rag": {
    "enabled": true,
    "provider": "openai",
    "chunkSize": 1500,
    "providers": {
      "openai": {
        "indexName": "company-knowledge-base",
        "dimensions": 1536
      }
    }
  },
  "monitoring": {
    "enabled": true,
    "metricsPort": 8080,
    "loggingLevel": "debug"
  }
}
```

## Environment Variables

### Required Environment Variables

```bash
# Slack configuration
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_APP_TOKEN=xapp-your-app-token

# LLM provider API keys (set based on your provider)
OPENAI_API_KEY=sk-your-openai-key
ANTHROPIC_API_KEY=sk-ant-your-anthropic-key
OLLAMA_BASE_URL=http://localhost:11434
```

### Optional Environment Variable Overrides

```bash
# Application configuration overrides
LLM_PROVIDER=anthropic
MONITORING_ENABLED=true
CUSTOM_PROMPT="You are a DevOps assistant."
```

## Slack App Setup

### Token Types

The application requires two types of tokens:

1. **Bot Token** (`SLACK_BOT_TOKEN`): Starts with `xoxb-`
2. **App-Level Token** (`SLACK_APP_TOKEN`): Starts with `xapp-`

### Required Bot Token Scopes

Add these OAuth scopes to your Bot Token in the "OAuth & Permissions" section:

**Essential Scopes:**
- `app_mentions:read` - Allows the bot to receive mention events
- `chat:write` - Allows the bot to post messages
- `im:history` - Allows the bot to read direct messages
- `im:read` - Allows the bot to receive DM events

**Additional Scopes (for full functionality):**
- `channels:history` - Allows reading public channel history
- `groups:history` - Allows reading private channel history
- `mpim:history` - Allows reading multi-person IM history

### App-Level Token Configuration

1. Go to the "Socket Mode" section in your Slack app settings
2. Enable Socket Mode
3. Create an App-Level Token with the `connections:write` scope
4. Use this token for the `SLACK_APP_TOKEN` environment variable

### Event Subscriptions

In the "Event Subscriptions" section:

1. Turn ON Event Subscriptions
2. Under "Subscribe to bot events", add these event subscriptions:
   - `message.im` - For direct messages to your app
   - `app_mention` - For mentions of your app in channels

### App Home Configuration

In the "App Home" section:

1. Enable the Messages Tab
2. Turn ON "Allow users to send Slash commands and messages from the messages tab"

## Custom Prompt Configuration

### Option 1: Simple Inline Prompt (Most Common)
```json
{
  "llm": {
    "provider": "openai",
    "customPrompt": "You are a helpful DevOps assistant focused on Kubernetes."
  }
}
```

### Option 2: File-Based Prompt (For Long Prompts)
```json
{
  "llm": {
    "provider": "openai",
    "customPromptFile": "custom-prompt.txt"
  }
}
```

**Priority**: `customPromptFile` takes precedence over `customPrompt` if both are set

## Kubernetes Deployment

### Basic Helm Configuration

```yaml
# values.yaml
app:
  config:
    llm:
      provider: "openai"
      customPrompt: "You are a DevOps assistant."

secrets:
  create: true
  data:
    SLACK_BOT_TOKEN: ""     # Set via external secret manager
    SLACK_APP_TOKEN: ""     # Set via external secret manager  
    OPENAI_API_KEY: ""      # Set via external secret manager
    ANTHROPIC_API_KEY: ""   # Set via external secret manager

configMap:
  create: true
  data:
    config.json: |
      {
        "$schema": "https://github.com/tuannvm/slack-mcp-client/schema/config-schema.json",
        "version": "2.0",
        "slack": {
          "botToken": "${SLACK_BOT_TOKEN}",
          "appToken": "${SLACK_APP_TOKEN}"
        },
        "llm": {
          "provider": {{ .Values.app.config.llm.provider | quote }},
          "customPrompt": {{ .Values.app.config.llm.customPrompt | quote }}
        }
      }
```

### Security Best Practices

- Use **Secrets** for tokens and API keys
- Use **ConfigMaps** for non-sensitive configuration
- Secret key names must match environment variable names in config file
- Consider using external secret management (AWS Secrets Manager, Vault, etc.)

## Configuration Validation

### Runtime Validation
The application validates configuration after loading environment variables and applying defaults:

```bash
# Test configuration
./slack-mcp-client --config-validate

# Migrate from legacy format
./slack-mcp-client --migrate-config
```

### Common Validation Errors

**Missing Required Fields:**
```json
{
  "error": "Configuration validation failed",
  "details": "SLACK_BOT_TOKEN environment variable not set",
  "suggestion": "Set SLACK_BOT_TOKEN environment variable with your Slack bot token"
}
```

**Invalid Provider Configuration:**
```json
{
  "error": "Configuration validation failed",
  "details": "LLM provider 'openai' not configured",
  "suggestion": "Add OpenAI configuration to llm.providers.openai section"
}
```

## Migration from Legacy Format

The application supports both automatic detection and manual migration from legacy formats:

### Automatic Detection (Recommended)
Legacy configurations are automatically detected and converted at runtime:
- **Legacy `mcp-servers.json`**: Automatically detected by presence of `mcpServers` field without `version`, `slack`, or `llm` fields
- **Snake_case format**: Legacy snake_case field names are automatically converted during loading
- **No action required**: Existing configurations continue to work without changes

### Manual Migration
For permanent migration to the new format:

1. **Automatic Migration**: Run `./slack-mcp-client --migrate-config --config legacy-config.json`
2. **Manual Migration**: Use the provided examples as templates
3. **Validation**: Test with `--config-validate` before deployment

### Migration Benefits
- **IDE Support**: JSON schema provides autocomplete and validation
- **Modern Format**: camelCase follows current JSON API conventions
- **Better Documentation**: Inline field descriptions via schema

## Troubleshooting

### Common Issues

**"Sending messages to this app has been turned off"**
- Check App Home settings
- Verify Event Subscriptions are configured
- Ensure app is installed with required scopes

**Configuration validation failures**
- Check environment variables are set
- Validate JSON syntax
- Ensure required fields are present

**MCP server connection issues**
- Check server commands and arguments
- Verify network connectivity for URL-based servers
- Review server logs for initialization errors

### Debug Mode

Enable debug logging for detailed troubleshooting:

```json
{
  "monitoring": {
    "enabled": true,
    "loggingLevel": "debug"
  }
}
```

## Best Practices

1. **Start Simple**: Begin with minimal configuration, add complexity as needed
2. **Use Environment Variables**: Never hardcode secrets in configuration files
3. **Validate Early**: Use `--config-validate` to catch issues before deployment
4. **Monitor Usage**: Enable monitoring to track performance and costs
5. **Version Control**: Keep configuration examples in version control
6. **Document Changes**: Update configuration documentation when adding new features

## Advanced Configuration

For AI/ML-specific configuration options including enhanced LLM providers, advanced RAG settings, and production AI features, see the [AI Configuration Guide](./configuration-ai.md).

## Support

For configuration issues:
1. Check the troubleshooting section above
2. Validate your configuration with `--config-validate`
3. Review application logs for specific error messages
4. Consult the [AI Configuration Guide](./configuration-ai.md) for advanced features
