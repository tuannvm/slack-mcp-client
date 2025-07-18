# Configuration Refactoring Plan

## Executive Summary

The Slack MCP Client has grown from a simple MCP server connector to a comprehensive LLM orchestration platform. The current configuration system, centered around `mcp-servers.json`, is insufficient for the expanded scope. This document outlines a **simplified, user-focused** refactoring plan that prioritizes simplicity, DRY principles, and excellent user experience.

## Current Configuration Issues

### 1. Scope Mismatch
- Current schema only handles MCP servers
- Project now includes LLM providers, RAG systems, monitoring, agents, and more
- Configuration scattered across multiple sources (files, environment variables, defaults)

### 2. Poor Separation of Concerns
- Mixed configuration domains in single config struct
- LLM provider configs embedded in generic maps
- RAG configuration extracted from LLM provider config
- No clear configuration hierarchy

### 3. Maintainability Problems
- Hard to extend for new components
- Complex parsing logic in config.go (243 lines)
- Schema validation only covers MCP servers
- No clear migration path for configuration changes

## Proposed Simplified Architecture

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

### Single Configuration File Format

```json
{
  "version": "2.0",
  "slack": {
    "bot_token": "${SLACK_BOT_TOKEN}",
    "app_token": "${SLACK_APP_TOKEN}",
    "use_stdio_client": false
  },
  "llm": {
    "provider": "openai",
    "use_native_tools": false,
    "use_agent": false,
    "custom_prompt": "You are a helpful AI assistant.",
    "replace_tool_prompt": false,
    "providers": {
      "openai": {
        "model": "gpt-4o",
        "api_key": "${OPENAI_API_KEY}",
        "temperature": 0.7
      },
      "anthropic": {
        "model": "claude-3-5-sonnet-20241022",
        "api_key": "${ANTHROPIC_API_KEY}",
        "temperature": 0.7
      },
      "ollama": {
        "model": "llama3",
        "base_url": "http://localhost:11434",
        "temperature": 0.7
      }
    }
  },
  "mcp_servers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"],
      "disabled": false,
      "tools": {
        "allow_list": [],
        "block_list": []
      }
    },
    "api-server": {
      "transport": "sse",
      "url": "http://localhost:3000/sse",
      "disabled": false,
      "tools": {
        "allow_list": ["weather", "search"]
      }
    }
  },
  "rag": {
    "enabled": false,
    "provider": "simple",
    "chunk_size": 1000,
    "providers": {
      "simple": {
        "database_path": "./rag.db"
      },
      "openai": {
        "index_name": "slack-mcp-rag",
        "dimensions": 1536
      }
    }
  },
  "monitoring": {
    "enabled": true,
    "metrics_port": 8080,
    "logging_level": "info"
  }
}
```

### Minimal Configuration Example

```json
{
  "version": "2.0",
  "slack": {
    "bot_token": "${SLACK_BOT_TOKEN}",
    "app_token": "${SLACK_APP_TOKEN}"
  },
  "llm": {
    "provider": "openai"
  },
  "mcp_servers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"]
    }
  }
}
```

## Implementation Plan

### Phase 1: Simplify Current Config (1 week)

1. **Consolidate Configuration Structure**
   - Move all configuration domains into single file with logical sections
   - Remove redundant configuration patterns
   - Implement smart defaults for all non-essential settings

2. **Basic Validation**
   - Create single JSON schema for entire configuration
   - Implement basic validation with helpful error messages
   - Add `--config-validate` CLI flag for configuration testing

3. **Environment Variable Integration**
   - Use industry standard names without prefix: `SLACK_BOT_TOKEN`, `OPENAI_API_KEY`
   - Simple override pattern: `LLM_PROVIDER` overrides `llm.provider`
   - Maintain backward compatibility with existing environment variables

### Phase 2: Enhance Developer Experience (1 week)

1. **Configuration Validation**
   - Schema-based validation with clear error messages
   - Point users to specific configuration issues
   - Validate required fields and data types

2. **Migration Tools**
   - Automatic migration from current `mcp-servers.json` format
   - `--migrate-config` CLI flag for easy upgrading
   - Backup current configuration before migration

3. **Documentation and Examples**
   - Comprehensive example configurations
   - Clear documentation for each configuration section
   - Troubleshooting guide for common issues

### Phase 3: Polish and Advanced Features (1 week)

1. **Optional Advanced Features**
   - Hot configuration reloading (if needed)
   - Configuration drift detection
   - Environment-specific configuration overrides

2. **Developer Tools**
   - Configuration generation wizard
   - Configuration diff tools
   - Integration with popular IDEs

3. **Code Refactoring Tasks**
   - Rename `Mode` field to `Transport` in Go structs (internal/config/config.go)
   - Update MCP client code to use `transport` instead of `mode` (internal/mcp/client.go)
   - Update all references in cmd/main.go from `serverConf.Mode` to `serverConf.Transport`
   - Ensure consistent naming across entire codebase

## Benefits of Simplified Architecture

### 1. Reduced Cognitive Load
- **Single file to understand**: No need to juggle multiple configuration files
- **Logical sections**: Clear separation of concerns within one file
- **Progressive disclosure**: Start with minimal config, add complexity as needed

### 2. DRY Principles
- **No duplication**: API keys defined once, referenced where needed
- **Consistent patterns**: Environment variable handling follows same pattern
- **Shared defaults**: Common settings defined once

### 3. Excellent User Experience
- **Fast onboarding**: New users need to understand one file
- **Easy debugging**: All configuration in one place
- **Simple deployment**: Single configuration artifact
- **Clear error messages**: Validation points to specific issues

### 4. Maintainability
- **Reduced complexity**: Single schema, single validation path
- **Clear ownership**: Each section has clear responsibility
- **Easy testing**: Configuration validation in one place

## Migration Strategy

### 1. Backward Compatibility
- Support current `mcp-servers.json` format during transition
- Automatic detection and migration of legacy format
- Deprecation warnings with clear migration path

### 2. Migration Tools
- `--migrate-config` CLI command for automatic migration
- Backup existing configuration before migration
- Dry-run mode to preview changes

### 3. Rollout Plan
- Feature flag for new configuration system
- Gradual rollout with easy rollback
- Clear documentation and migration guide

### 4. Helm Chart Integration
- Support custom prompt files via ConfigMap
- Allow different prompts per deployment environment
- Enable prompt customization without code changes
- **Security-first approach**: Secrets for tokens/API keys, ConfigMaps for non-sensitive data

## Configuration Best Practices

### 1. Environment Variable Strategy
```bash
# Industry standard names - no prefix needed
SLACK_BOT_TOKEN=xoxb-your-token
SLACK_APP_TOKEN=xapp-your-token
OPENAI_API_KEY=sk-your-key
ANTHROPIC_API_KEY=sk-ant-your-key
OLLAMA_BASE_URL=http://localhost:11434

# Application configuration
LLM_PROVIDER=anthropic
MONITORING_ENABLED=true
CUSTOM_PROMPT="You are a DevOps assistant."
```

### 2. Custom Prompt Support (Flexible Options)

**Option 1: Simple Inline Prompt (Most Common)**
```json
{
  "llm": {
    "provider": "openai",
    "custom_prompt": "You are a helpful DevOps assistant focused on Kubernetes."
  }
}
```

**Option 2: File-Based Prompt (For Long Prompts)**
```json
{
  "llm": {
    "provider": "openai",
    "custom_prompt_file": "custom-prompt.txt"
  }
}
```

**Priority: `custom_prompt_file` takes precedence over `custom_prompt` if both are set**

**Helm Chart Integration - Simple Inline:**
```yaml
# values.yaml - Simple approach (90% of use cases)
app:
  customPrompt: "You are a DevOps assistant specialized in Kubernetes operations."
  
configMap:
  create: true
  data:
    config.json: |
      {
        "version": "2.0",
        "llm": {
          "provider": "openai",
          "custom_prompt": {{ .Values.app.customPrompt | quote }}
        }
      }
```

**Helm Chart Integration - File-Based:**
```yaml
# values.yaml - File-based for complex prompts
app:
  customPrompt: |
    You are a specialized AI assistant for DevOps and Infrastructure.
    
    Your primary focus areas:
    - Kubernetes operations and troubleshooting
    - CI/CD pipeline optimization
    - Infrastructure as Code
    - Security best practices
    
    Use tools to gather information before making recommendations.
  
configMap:
  create: true
  data:
    config.json: |
      {
        "version": "2.0",
        "llm": {
          "provider": "openai",
          "custom_prompt_file": "custom-prompt.txt"
        }
      }
    custom-prompt.txt: |
{{ .Values.app.customPrompt | indent 6 }}
```

**Kubernetes Security Best Practice:**
```yaml
# values.yaml - Production-ready approach
app:
  config:
    llm:
      provider: "openai"
      customPrompt: "You are a DevOps assistant."

# Secrets configuration - Keys match environment variables exactly
secrets:
  create: true
  data:
    SLACK_BOT_TOKEN: ""     # Set via external secret manager
    SLACK_APP_TOKEN: ""     # Set via external secret manager  
    OPENAI_API_KEY: ""      # Set via external secret manager
    ANTHROPIC_API_KEY: ""   # Set via external secret manager
  validation:
    required: ["SLACK_BOT_TOKEN", "SLACK_APP_TOKEN"]
    patterns:
      SLACK_BOT_TOKEN: "^xoxb-"
      SLACK_APP_TOKEN: "^xapp-"
      OPENAI_API_KEY: "^sk-"
      ANTHROPIC_API_KEY: "^sk-ant-"

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
          "custom_prompt": {{ .Values.app.config.llm.customPrompt | quote }},
          "providers": {
            "openai": {
              "model": "gpt-4o",
              "api_key": "${OPENAI_API_KEY}"
            },
            "anthropic": {
              "model": "claude-3-5-sonnet-20241022",
              "api_key": "${ANTHROPIC_API_KEY}"
            }
          }
        }
      }
```

### 3. Kubernetes Security Model

**Key Mapping: Config File ↔ Kubernetes Secret**
```yaml
# values.yaml - Secret keys match environment variables exactly
secrets:
  create: true
  data:
    SLACK_BOT_TOKEN: ""      # Maps to ${SLACK_BOT_TOKEN} in config
    SLACK_APP_TOKEN: ""      # Maps to ${SLACK_APP_TOKEN} in config
    OPENAI_API_KEY: ""       # Maps to ${OPENAI_API_KEY} in config
    ANTHROPIC_API_KEY: ""    # Maps to ${ANTHROPIC_API_KEY} in config

# templates/secret.yaml - Loop through secrets
{{- if .Values.secrets.create }}
apiVersion: v1
kind: Secret
metadata:
  name: {{ include "slack-mcp-client.fullname" . }}-secrets
type: Opaque
data:
  {{- range $key, $value := .Values.secrets.data }}
  {{- if $value }}
  {{ $key }}: {{ $value | b64enc }}
  {{- end }}
  {{- end }}
{{- end }}

# templates/deployment.yaml - Environment variables from secrets
env:
  {{- range $key, $value := .Values.secrets.data }}
  - name: {{ $key }}                    # Environment variable name
    valueFrom:
      secretKeyRef:
        name: {{ include "slack-mcp-client.fullname" . }}-secrets
        key: {{ $key }}                 # Secret key (same as env var name)
        optional: {{ if or (eq $key "SLACK_BOT_TOKEN") (eq $key "SLACK_APP_TOKEN") }}false{{ else }}true{{ end }}
  {{- end }}
```

**Important**: The secret key names in Kubernetes must match the environment variable names used in the config file's `${VARIABLE_NAME}` placeholders.

### 4. Smart Defaults Strategy
- **Transport inference**: `command` → `stdio`, `url` → `sse`
- **Timeout defaults**: 30 seconds for MCP server initialization
- **Progressive enhancement**: Start minimal, add complexity as needed
- **Runtime validation**: Validate after environment substitution and defaults applied

```go
// Example default value implementation
func (c *Config) ApplyDefaults() {
    // LLM defaults
    if c.LLM.Provider == "" {
        c.LLM.Provider = "openai"
    }
    
    // Monitoring defaults
    if c.Monitoring.MetricsPort == 0 {
        c.Monitoring.MetricsPort = 8080
    }
    if c.Monitoring.LoggingLevel == "" {
        c.Monitoring.LoggingLevel = "info"
    }
    
    // MCP server defaults applied via GetTransport(), GetInitializeTimeout()
}
```

### 5. Runtime Validation Strategy
```go
// ValidateAfterDefaults validates configuration after defaults and env substitution
func (c *Config) ValidateAfterDefaults() error {
    // Validate required fields after environment substitution
    if c.Slack.BotToken == "" || strings.HasPrefix(c.Slack.BotToken, "${") {
        return fmt.Errorf("SLACK_BOT_TOKEN environment variable not set")
    }
    if c.Slack.AppToken == "" || strings.HasPrefix(c.Slack.AppToken, "${") {
        return fmt.Errorf("SLACK_APP_TOKEN environment variable not set")
    }
    
    // Validate LLM provider exists
    if _, exists := c.LLM.Providers[c.LLM.Provider]; !exists {
        return fmt.Errorf("LLM provider '%s' not configured", c.LLM.Provider)
    }
    
    return nil
}
```

**Clear Error Messages:**
```json
{
  "error": "Configuration validation failed",
  "details": "SLACK_BOT_TOKEN environment variable not set",
  "suggestion": "Set SLACK_BOT_TOKEN environment variable with your Slack bot token"
}
```

### 6. Go-Idiomatic Configuration Composition
```go
// Config represents the main application configuration
type Config struct {
    Version        string                        `json:"version"`
    UseStdioClient *bool                         `json:"use_stdio_client,omitempty"`
    Slack          SlackConfig                   `json:"slack"`
    LLM            LLMConfig                     `json:"llm"`
    MCPServers     map[string]MCPServerConfig   `json:"mcp_servers"`
    RAG            RAGConfig                     `json:"rag,omitempty"`
    Monitoring     MonitoringConfig             `json:"monitoring,omitempty"`
}

// SlackConfig contains Slack-specific configuration
type SlackConfig struct {
    BotToken       string `json:"bot_token"`  // Validated after env substitution
    AppToken       string `json:"app_token"`  // Validated after env substitution
    UseStdioClient bool   `json:"use_stdio_client,omitempty"`
}

// LLMConfig contains LLM provider configuration
type LLMConfig struct {
    Provider          string                       `json:"provider"`  // Validated at runtime
    UseNativeTools    bool                         `json:"use_native_tools,omitempty"`
    UseAgent          bool                         `json:"use_agent,omitempty"`
    CustomPrompt      string                       `json:"custom_prompt,omitempty"`
    CustomPromptFile  string                       `json:"custom_prompt_file,omitempty"`
    ReplaceToolPrompt bool                         `json:"replace_tool_prompt,omitempty"`
    Providers         map[string]LLMProviderConfig `json:"providers"`  // Validated at runtime
}

// LLMProviderConfig contains provider-specific settings
type LLMProviderConfig struct {
    Model       string  `json:"model"`                  // Let provider APIs handle validation
    APIKey      string  `json:"api_key,omitempty"`
    BaseURL     string  `json:"base_url,omitempty"`
    Temperature float64 `json:"temperature,omitempty"`  // Let provider APIs handle validation
    MaxTokens   int     `json:"max_tokens,omitempty"`   // Let provider APIs handle validation
}

// MCPServerConfig contains MCP server configuration
type MCPServerConfig struct {
    Command                  string            `json:"command,omitempty"`
    Args                     []string          `json:"args,omitempty"`
    URL                      string            `json:"url,omitempty"`
    Transport                string            `json:"transport,omitempty"`  // "stdio", "sse", "http" - inferred if not specified
    Env                      map[string]string `json:"env,omitempty"`
    Disabled                 bool              `json:"disabled,omitempty"`
    InitializeTimeoutSeconds *int              `json:"initialize_timeout_seconds,omitempty"`
    Tools                    MCPToolsConfig    `json:"tools,omitempty"`
}

// GetTransport returns the transport type, inferring from other fields if not explicitly set
func (mcp *MCPServerConfig) GetTransport() string {
    if mcp.Transport != "" {
        return mcp.Transport
    }
    if mcp.Command != "" {
        return "stdio"  // Default: if command is specified, use stdio
    }
    if mcp.URL != "" {
        return "sse"    // Default: if URL is specified, use sse
    }
    return "stdio"      // Fallback default
}

// GetInitializeTimeout returns the timeout with default fallback
func (mcp *MCPServerConfig) GetInitializeTimeout() int {
    if mcp.InitializeTimeoutSeconds != nil {
        return *mcp.InitializeTimeoutSeconds
    }
    return 30 // Default timeout: 30 seconds
}

// MCPToolsConfig contains tool filtering configuration
type MCPToolsConfig struct {
    AllowList []string `json:"allow_list,omitempty"`
    BlockList []string `json:"block_list,omitempty"`
}

// RAGConfig contains RAG system configuration
type RAGConfig struct {
    Enabled      bool                           `json:"enabled,omitempty"`
    Provider     string                         `json:"provider,omitempty"`
    ChunkSize    int                            `json:"chunk_size,omitempty"`
    Providers    map[string]RAGProviderConfig   `json:"providers,omitempty"`
}

// RAGProviderConfig contains RAG provider-specific settings
type RAGProviderConfig struct {
    DatabasePath     string `json:"database_path,omitempty"`
    IndexName        string `json:"index_name,omitempty"`
    Dimensions       int    `json:"dimensions,omitempty"`
    SimilarityMetric string `json:"similarity_metric,omitempty"`
}

// MonitoringConfig contains monitoring and observability settings
type MonitoringConfig struct {
    Enabled      bool   `json:"enabled,omitempty"`
    MetricsPort  int    `json:"metrics_port,omitempty"`
    LoggingLevel string `json:"logging_level,omitempty"`
}
```

## User Segmentation and Onboarding

### User Personas and Configuration Needs

**1. Quick Start User (30% of users)**
- **Need**: Get up and running in minutes
- **Configuration**: Minimal setup with smart defaults
- **Example**: Developer trying the tool for the first time

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

**2. Production User (40% of users)**
- **Need**: Robust configuration with monitoring and security
- **Configuration**: Full featured with explicit settings
- **Example**: DevOps engineer deploying to production

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

**3. Advanced User (30% of users)**
- **Need**: Maximum customization and control
- **Configuration**: Complex multi-provider setup with custom prompts
- **Example**: AI researcher or enterprise platform team

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
    },
    "api-server": {
      "url": "https://api.company.com/mcp",
      "transport": "sse",
      "tools": {
        "allow_list": ["weather", "search", "analytics"]
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

### Complete Configuration Reference

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

### Persona-Specific Quickstart Guides

**Quick Start User Guide (5 minutes)**
1. Set environment variables: `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`
2. Create minimal config.json (3 lines of actual config)
3. Run the application
4. Test with filesystem tools

**Production User Guide (15 minutes)**
1. Set all required environment variables
2. Configure LLM provider with explicit settings
3. Set up monitoring and logging
4. Configure MCP servers with tool filtering
5. Deploy with Helm chart

**Advanced User Guide (30 minutes)**
1. Set up multiple LLM providers
2. Configure custom prompts and agent mode
3. Set up RAG system with vector store
4. Configure complex MCP server setups
5. Set up advanced monitoring and debugging

## Success Metrics

1. **Simplicity**
   - Single configuration file
   - Reduce config.go from 243 lines to <100 lines
   - 90% of users need only minimal configuration

2. **User Experience**
   - Time to first working configuration <5 minutes
   - Clear error messages resolve 95% of configuration issues
   - Zero-config defaults work for development

3. **Maintainability**
   - Configuration bugs reduced by 80%
   - New feature configuration takes <1 hour to implement
   - Migration from legacy format takes <1 minute

4. **Developer Experience**
   - Single schema file provides IDE support
   - Configuration validation catches errors before runtime
   - Clear documentation with working examples

## Conclusion

This simplified refactoring plan **rejects over-engineering** in favor of **user-focused simplicity**. By using a single, well-structured configuration file with logical sections, we achieve:

- **Reduced cognitive load** for users
- **DRY principles** without duplication
- **Excellent user experience** with fast onboarding
- **Easy maintenance** with clear ownership
- **Future extensibility** without complexity

The key insight is that **simplicity is the ultimate sophistication**. Users want to configure their application quickly and correctly, not manage multiple configuration files with complex interdependencies.

This approach follows the Unix philosophy: do one thing well. For a CLI tool, that means making configuration as simple and intuitive as possible while maintaining all the necessary functionality.