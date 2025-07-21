#!/bin/bash

# migrate-config.sh - Convert legacy configuration to new format
# Usage: ./scripts/migrate-config.sh [legacy-config-file] [output-file]

set -e

# Check if jq is available
if ! command -v jq &> /dev/null; then
    echo "Error: jq is required but not installed. Please install jq first."
    echo "  macOS: brew install jq"
    echo "  Ubuntu: sudo apt-get install jq"
    echo "  CentOS: sudo yum install jq"
    exit 1
fi

# Default files
LEGACY_CONFIG="${1:-mcp-servers.json}"
OUTPUT_CONFIG="${2:-config.json}"

# Check if input file exists
if [ ! -f "$LEGACY_CONFIG" ]; then
    echo "Error: Legacy config file '$LEGACY_CONFIG' not found"
    echo "Usage: $0 [legacy-config-file] [output-file]"
    exit 1
fi

# Create backup of legacy config
BACKUP_FILE="${LEGACY_CONFIG}.backup.$(date +%Y%m%d_%H%M%S)"
cp "$LEGACY_CONFIG" "$BACKUP_FILE"
echo "Created backup: $BACKUP_FILE"

echo "Converting $LEGACY_CONFIG to $OUTPUT_CONFIG..."

# Create base new config structure
BASE_CONFIG='{
  "version": "2.0",
  "slack": {
    "botToken": "${SLACK_BOT_TOKEN}",
    "appToken": "${SLACK_APP_TOKEN}"
  },
  "llm": {
    "provider": "openai",
    "providers": {
      "openai": {
        "model": "gpt-4o",
        "apiKey": "${OPENAI_API_KEY}",
        "temperature": 0.7
      },
      "anthropic": {
        "model": "claude-3-5-sonnet-20241022",
        "apiKey": "${ANTHROPIC_API_KEY}",
        "temperature": 0.7
      },
      "ollama": {
        "model": "llama3",
        "baseUrl": "http://localhost:11434",
        "temperature": 0.7
      }
    }
  },
  "mcpServers": {},
  "rag": {
    "provider": "simple",
    "chunkSize": 1000,
    "providers": {
      "simple": {
        "databasePath": "./rag.db"
      },
      "openai": {
        "indexName": "slack-mcp-rag",
        "dimensions": 1536
      }
    }
  },
  "monitoring": {
    "enabled": true,
    "metricsPort": 8080,
    "loggingLevel": "info"
  }
}'

# Function to migrate MCP server config
migrate_mcp_server() {
    local server_config="$1"
    
    # Extract fields and convert mode to transport
    local transport
    if [[ $(echo "$server_config" | jq -r '.mode // empty') != "" ]]; then
        transport=$(echo "$server_config" | jq -r '.mode')
    elif [[ $(echo "$server_config" | jq -r '.command // empty') != "" ]]; then
        transport="stdio"
    elif [[ $(echo "$server_config" | jq -r '.url // empty') != "" ]]; then
        transport="sse"
    else
        transport="stdio"
    fi
    
    # Build new server config
    local new_server='{}'
    
    # Add command if exists
    if [[ $(echo "$server_config" | jq -r '.command // empty') != "" ]]; then
        new_server=$(echo "$new_server" | jq --arg cmd "$(echo "$server_config" | jq -r '.command')" '.command = $cmd')
    fi
    
    # Add args if exists
    if [[ $(echo "$server_config" | jq -r '.args // empty') != "" ]]; then
        new_server=$(echo "$new_server" | jq --argjson args "$(echo "$server_config" | jq '.args')" '.args = $args')
    fi
    
    # Add URL if exists
    if [[ $(echo "$server_config" | jq -r '.url // empty') != "" ]]; then
        new_server=$(echo "$new_server" | jq --arg url "$(echo "$server_config" | jq -r '.url')" '.url = $url')
    fi
    
    # Add transport
    new_server=$(echo "$new_server" | jq --arg transport "$transport" '.transport = $transport')
    
    # Add env if exists
    if [[ $(echo "$server_config" | jq -r '.env // empty') != "" ]]; then
        new_server=$(echo "$new_server" | jq --argjson env "$(echo "$server_config" | jq '.env')" '.env = $env')
    fi
    
    # Add disabled if exists
    if [[ $(echo "$server_config" | jq -r '.disabled // empty') != "" ]]; then
        new_server=$(echo "$new_server" | jq --argjson disabled "$(echo "$server_config" | jq '.disabled')" '.disabled = $disabled')
    fi
    
    # Add timeout if exists
    if [[ $(echo "$server_config" | jq -r '.initialize_timeout_seconds // empty') != "" ]]; then
        new_server=$(echo "$new_server" | jq --argjson timeout "$(echo "$server_config" | jq '.initialize_timeout_seconds')" '.initialize_timeout_seconds = $timeout')
    fi
    
    # Add tools config if allow_list or block_list exists
    if [[ $(echo "$server_config" | jq -r '.allow_list // empty') != "" ]] || [[ $(echo "$server_config" | jq -r '.block_list // empty') != "" ]]; then
        local tools_config='{}'
        
        if [[ $(echo "$server_config" | jq -r '.allow_list // empty') != "" ]]; then
            tools_config=$(echo "$tools_config" | jq --argjson allow_list "$(echo "$server_config" | jq '.allow_list')" '.allow_list = $allow_list')
        fi
        
        if [[ $(echo "$server_config" | jq -r '.block_list // empty') != "" ]]; then
            tools_config=$(echo "$tools_config" | jq --argjson block_list "$(echo "$server_config" | jq '.block_list')" '.block_list = $block_list')
        fi
        
        new_server=$(echo "$new_server" | jq --argjson tools "$tools_config" '.tools = $tools')
    fi
    
    echo "$new_server"
}

# Start with base config
NEW_CONFIG="$BASE_CONFIG"

# Check if it's legacy mcp-servers.json format
if jq -e '.mcpServers' "$LEGACY_CONFIG" > /dev/null 2>&1; then
    echo "Detected legacy mcp-servers.json format"
    
    # Migrate each MCP server
    for server_name in $(jq -r '.mcpServers | keys[]' "$LEGACY_CONFIG"); do
        echo "Migrating MCP server: $server_name"
        server_config=$(jq ".mcpServers[\"$server_name\"]" "$LEGACY_CONFIG")
        new_server=$(migrate_mcp_server "$server_config")
        NEW_CONFIG=$(echo "$NEW_CONFIG" | jq --arg name "$server_name" --argjson server "$new_server" '.mcpServers[$name] = $server')
    done
    
elif jq -e '.servers' "$LEGACY_CONFIG" > /dev/null 2>&1; then
    echo "Detected legacy config.json format"
    
    # Migrate Slack configuration
    if jq -e '.slack_bot_token' "$LEGACY_CONFIG" > /dev/null 2>&1; then
        bot_token=$(jq -r '.slack_bot_token' "$LEGACY_CONFIG")
        if [[ "$bot_token" != "null" && "$bot_token" != "" ]]; then
            NEW_CONFIG=$(echo "$NEW_CONFIG" | jq --arg token "$bot_token" '.slack.bot_token = $token')
        fi
    fi
    
    if jq -e '.slack_app_token' "$LEGACY_CONFIG" > /dev/null 2>&1; then
        app_token=$(jq -r '.slack_app_token' "$LEGACY_CONFIG")
        if [[ "$app_token" != "null" && "$app_token" != "" ]]; then
            NEW_CONFIG=$(echo "$NEW_CONFIG" | jq --arg token "$app_token" '.slack.app_token = $token')
        fi
    fi
    
    # Migrate LLM configuration
    if jq -e '.llm_provider' "$LEGACY_CONFIG" > /dev/null 2>&1; then
        provider=$(jq -r '.llm_provider' "$LEGACY_CONFIG")
        if [[ "$provider" != "null" && "$provider" != "" ]]; then
            NEW_CONFIG=$(echo "$NEW_CONFIG" | jq --arg provider "$provider" '.llm.provider = $provider')
        fi
    fi
    
    if jq -e '.use_native_tools' "$LEGACY_CONFIG" > /dev/null 2>&1; then
        use_native=$(jq -r '.use_native_tools' "$LEGACY_CONFIG")
        if [[ "$use_native" != "null" ]]; then
            NEW_CONFIG=$(echo "$NEW_CONFIG" | jq --argjson use_native "$use_native" '.llm.use_native_tools = $use_native')
        fi
    fi
    
    if jq -e '.use_agent' "$LEGACY_CONFIG" > /dev/null 2>&1; then
        use_agent=$(jq -r '.use_agent' "$LEGACY_CONFIG")
        if [[ "$use_agent" != "null" ]]; then
            NEW_CONFIG=$(echo "$NEW_CONFIG" | jq --argjson use_agent "$use_agent" '.llm.use_agent = $use_agent')
        fi
    fi
    
    if jq -e '.custom_prompt' "$LEGACY_CONFIG" > /dev/null 2>&1; then
        custom_prompt=$(jq -r '.custom_prompt' "$LEGACY_CONFIG")
        if [[ "$custom_prompt" != "null" && "$custom_prompt" != "" ]]; then
            NEW_CONFIG=$(echo "$NEW_CONFIG" | jq --arg prompt "$custom_prompt" '.llm.custom_prompt = $prompt')
        fi
    fi
    
    if jq -e '.replace_tool_prompt' "$LEGACY_CONFIG" > /dev/null 2>&1; then
        replace_prompt=$(jq -r '.replace_tool_prompt' "$LEGACY_CONFIG")
        if [[ "$replace_prompt" != "null" ]]; then
            NEW_CONFIG=$(echo "$NEW_CONFIG" | jq --argjson replace "$replace_prompt" '.llm.replace_tool_prompt = $replace')
        fi
    fi
    
    # Migrate LLM providers
    if jq -e '.llm_providers' "$LEGACY_CONFIG" > /dev/null 2>&1; then
        for provider_name in $(jq -r '.llm_providers | keys[]' "$LEGACY_CONFIG"); do
            echo "Migrating LLM provider: $provider_name"
            provider_config=$(jq ".llm_providers[\"$provider_name\"]" "$LEGACY_CONFIG")
            
            # Create new provider config
            new_provider='{}'
            
            for field in model api_key base_url temperature max_tokens; do
                if jq -e ".$field" <<< "$provider_config" > /dev/null 2>&1; then
                    value=$(jq -r ".$field" <<< "$provider_config")
                    if [[ "$value" != "null" && "$value" != "" ]]; then
                        if [[ "$field" == "temperature" || "$field" == "max_tokens" ]]; then
                            new_provider=$(echo "$new_provider" | jq --argjson val "$value" ".$field = \$val")
                        else
                            new_provider=$(echo "$new_provider" | jq --arg val "$value" ".$field = \$val")
                        fi
                    fi
                fi
            done
            
            NEW_CONFIG=$(echo "$NEW_CONFIG" | jq --arg name "$provider_name" --argjson provider "$new_provider" '.llm.providers[$name] = $provider')
        done
    fi
    
    # Migrate MCP servers
    if jq -e '.servers' "$LEGACY_CONFIG" > /dev/null 2>&1; then
        for server_name in $(jq -r '.servers | keys[]' "$LEGACY_CONFIG"); do
            echo "Migrating MCP server: $server_name"
            server_config=$(jq ".servers[\"$server_name\"]" "$LEGACY_CONFIG")
            new_server=$(migrate_mcp_server "$server_config")
            NEW_CONFIG=$(echo "$NEW_CONFIG" | jq --arg name "$server_name" --argjson server "$new_server" '.mcpServers[$name] = $server')
        done
    fi
    
else
    echo "Error: Could not detect legacy config format"
    echo "Expected either 'mcpServers' or 'servers' field in JSON"
    exit 1
fi

# Write new config
echo "$NEW_CONFIG" | jq '.' > "$OUTPUT_CONFIG"

echo ""
echo "Migration completed successfully!"
echo "- Legacy config backed up to: $BACKUP_FILE"
echo "- New config written to: $OUTPUT_CONFIG"
echo ""
echo "Please review the new configuration file and update environment variables as needed:"
echo "- SLACK_BOT_TOKEN"
echo "- SLACK_APP_TOKEN"
echo "- OPENAI_API_KEY (if using OpenAI)"
echo "- ANTHROPIC_API_KEY (if using Anthropic)"
echo "- OLLAMA_BASE_URL (if using Ollama)"
echo ""
echo "You can now use the new configuration with:"
echo "  ./slack-mcp-client -config $OUTPUT_CONFIG"
