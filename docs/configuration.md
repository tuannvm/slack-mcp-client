# Configuration Guide

This document provides comprehensive configuration guidance for the Slack MCP Client, covering everything from basic setup to advanced deployment scenarios.

## Overview

The Slack MCP Client has evolved from a simple MCP server connector to a comprehensive LLM orchestration platform. This guide outlines the unified configuration approach that prioritizes simplicity, DRY principles, and excellent user experience.

## Configuration Architecture

### Philosophy: Single File, Logical Sections
**Reject complexity. Embrace simplicity.**

Instead of multiple configuration files that create cognitive overhead, use a **single, well-structured configuration file** with logical sections.

### Unified Configuration Structure

```
config.json                     # Single configuration file
custom-prompt.txt               # Optional custom prompt file
├── examples/
│   ├── config.json.example     # Complete example
│   ├── minimal.json.example    # Minimal setup
│   ├── development.json.example # Development config
│   └── custom-prompt.txt.example # Custom prompt example
├── schema/
│   └── config-schema.json      # Single schema file
└── migrations/
    └── migrate-v1-to-v2.go     # Migration utility
```

## Complete Configuration Reference

Below is the complete configuration schema showing all available options. Fields marked with ⭐ are **required**, fields marked with ⚙️ have **smart defaults**, and fields marked with 🔧 are **optional**.

```json
{
  "version": "2.0",                                    // ⭐ Required
  "slack": {
    "bot_token": "${SLACK_BOT_TOKEN}",                // ⭐ Required
    "app_token": "${SLACK_APP_TOKEN}",                // ⭐ Required
    "use_stdio_client": false                         // ⚙️ Default: false
  },
  "llm": {
    "provider": "openai",                             // ⚙️ Default: "openai"
    "use_native_tools": false,                        // ⚙️ Default: false
    "use_agent": false,                               // ⚙️ Default: false
    "custom_prompt": "You are a helpful assistant.",  // 🔧 Optional
    "custom_prompt_file": "custom-prompt.txt",        // 🔧 Optional
    "replace_tool_prompt": false,                     // ⚙️ Default: false
    "providers": {
      "openai": {
        "model": "gpt-4o",                            // ⚙️ Default: "gpt-4o"
        "api_key": "${OPENAI_API_KEY}",               // ⭐ Required if using OpenAI
        "temperature": 0.7,                           // ⚙️ Default: 0.7
        "max_tokens": 2000                            // 🔧 Optional
      },
      "anthropic": {
        "model": "claude-3-5-sonnet-20241022",        // ⚙️ Default: "claude-3-5-sonnet-20241022"
        "api_key": "${ANTHROPIC_API_KEY}",            // ⭐ Required if using Anthropic
        "temperature": 0.7                            // ⚙️ Default: 0.7
      },
      "ollama": {
        "model": "llama3",                            // ⚙️ Default: "llama3"
        "base_url": "http://localhost:11434",         // ⚙️ Default: "http://localhost:11434"
        "temperature": 0.7                            // ⚙️ Default: 0.7
      }
    }
  },
  "mcp_servers": {
    "server-name": {
      "command": "npx",                               // 🔧 Optional (required if not using url)
      "args": ["-y", "@modelcontextprotocol/server"], // 🔧 Optional
      "url": "http://localhost:3000/sse",             // 🔧 Optional (required if not using command)
      "transport": "stdio",                           // ⚙️ Smart default: "stdio" for command, "sse" for url
      "env": {                                        // 🔧 Optional
        "DEBUG": "true"
      },
      "disabled": false,                              // ⚙️ Default: false
      "initialize_timeout_seconds": 30,               // ⚙️ Default: 30
      "tools": {
        "allow_list": ["tool1", "tool2"],             // 🔧 Optional
        "block_list": ["dangerous_tool"]              // 🔧 Optional
      }
    }
  },
  "rag": {
    "enabled": false,                                 // ⚙️ Default: false
    "provider": "simple",                             // ⚙️ Default: "simple"
    "chunk_size": 1000,                               // ⚙️ Default: 1000
    "providers": {
      "simple": {
        "database_path": "./rag.db"                   // ⚙️ Default: "./rag.db"
      },
      "openai": {
        "index_name": "slack-mcp-rag",                // ⚙️ Default: "slack-mcp-rag"
        "dimensions": 1536                            // ⚙️ Default: 1536
      }
    }
  },
  "monitoring": {
    "enabled": true,                                  // ⚙️ Default: true
    "metrics_port": 8080,                             // ⚙️ Default: 8080
    "logging_level": "info"                           // ⚙️ Default: "info"
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
  "version": "2.0",
  "slack": {
    "bot_token": "${SLACK_BOT_TOKEN}",
    "app_token": "${SLACK_APP_TOKEN}"
  },
  "mcp_servers": {
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
  "version": "2.0",
  "slack": {
    "bot_token": "${SLACK_BOT_TOKEN}",
    "app_token": "${SLACK_APP_TOKEN}"
  },
  "llm": {
    "provider": "openai",
    "use_native_tools": true,
    "custom_prompt": "You are a DevOps assistant."
  },
  "mcp_servers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"],
      "tools": {
        "allow_list": ["read_file", "write_file", "list_directory"]
      }
    },
    "api-server": {
      "url": "https://api.company.com/mcp",
      "transport": "sse",
      "tools": {
        "allow_list": ["weather", "search"]
      }
    }
  },
  "rag": {
    "enabled": true,
    "provider": "openai"
  },
  "monitoring": {
    "enabled": true,
    "metrics_port": 8080,
    "logging_level": "info"
  }
}
```

### 3. Advanced User (30 minutes)
**Need**: Maximum customization and control

```json
{
  "version": "2.0",
  "slack": {
    "bot_token": "${SLACK_BOT_TOKEN}",
    "app_token": "${SLACK_APP_TOKEN}"
  },
  "llm": {
    "provider": "anthropic",
    "use_native_tools": true,
    "use_agent": true,
    "custom_prompt_file": "custom-prompt.txt",
    "providers": {
      "openai": {
        "model": "gpt-4o",
        "api_key": "${OPENAI_API_KEY}",
        "temperature": 0.7,
        "max_tokens": 2000
      },
      "anthropic": {
        "model": "claude-3-5-sonnet-20241022",
        "api_key": "${ANTHROPIC_API_KEY}",
        "temperature": 0.5
      },
      "ollama": {
        "model": "llama3.1:8b",
        "base_url": "http://localhost:11434",
        "temperature": 0.8
      }
    }
  },
  "mcp_servers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"],
      "env": {
        "DEBUG": "true"
      },
      "initialize_timeout_seconds": 60,
      "tools": {
        "block_list": ["delete_file"]
      }
    },
    "kubernetes": {
      "command": "kubectl-mcp-server",
      "args": ["--context", "production"],
      "transport": "stdio",
      "tools": {
        "allow_list": ["get_pods", "get_services", "describe_pod"]
      }
    }
  },
  "rag": {
    "enabled": true,
    "provider": "openai",
    "chunk_size": 1500,
    "providers": {
      "openai": {
        "index_name": "company-knowledge-base",
        "dimensions": 1536
      }
    }
  },
  "monitoring": {
    "enabled": true,
    "metrics_port": 8080,
    "logging_level": "debug"
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
    "custom_prompt": "You are a helpful DevOps assistant focused on Kubernetes."
  }
}
```

### Option 2: File-Based Prompt (For Long Prompts)
```json
{
  "llm": {
    "provider": "openai",
    "custom_prompt_file": "custom-prompt.txt"
  }
}
```

**Priority**: `custom_prompt_file` takes precedence over `custom_prompt` if both are set

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
        "version": "2.0",
        "slack": {
          "bot_token": "${SLACK_BOT_TOKEN}",
          "app_token": "${SLACK_APP_TOKEN}"
        },
        "llm": {
          "provider": {{ .Values.app.config.llm.provider | quote }},
          "custom_prompt": {{ .Values.app.config.llm.customPrompt | quote }}
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

If you're upgrading from the legacy `mcp-servers.json` format:

1. **Automatic Migration**: Run `./slack-mcp-client --migrate-config`
2. **Manual Migration**: Use the provided examples as templates
3. **Validation**: Test with `--config-validate` before deployment

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
    "logging_level": "debug"
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