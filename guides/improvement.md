# LLM Provider Refactoring Plan

This document outlines a plan to refactor the LLM provider integration to make it more extensible and configuration-driven.

## Goals

*   Make adding new `LLMProvider` implementations easier with minimal changes to core components.
*   Centralize LLM provider configuration.
*   Decouple the provider registry and consumers (like the Slack client) from specific provider implementations.
*   Remove the forced "LangChain as gateway" pattern, allowing direct use of any configured provider.

## Proposed Strategy

1.  **Configuration-Driven Initialization:**
    *   Introduce a dedicated `LLMProviders map[string]map[string]interface{}` section in `internal/config/config.go` to hold settings for each potential provider (OpenAI, Ollama, Anthropic, etc.).
    *   The main `LLMProvider` field in the config will specify which configured provider to *use*.
    *   Example `config.yaml` structure:
        ```yaml
        llm_provider: "openai" # Which provider the Slack bot should use

        llm_providers:
          openai:
            # api_key: loaded_from_env_separately
            model: "gpt-4o"
            # other openai specific options...
          ollama:
            endpoint: "http://localhost:11434"
            model: "llama3"
          # anthropic:
          #   # api_key: ...
          #   model: "claude-3-opus"
        ```

2.  **Provider Factory Pattern:**
    *   Define a standard factory function signature: `type ProviderFactory func(config map[string]interface{}, logger *logging.Logger) (LLMProvider, error)` in `internal/llm/provider.go`.
    *   Maintain a package-level map `providerFactories map[string]ProviderFactory` within the `llm` package.
    *   Create a registration function `RegisterProviderFactory(name string, factory ProviderFactory)` in `registry.go` (or `provider.go`) to populate this map.

3.  **Self-Registration using `init()`:**
    *   In each provider implementation file (e.g., `internal/llm/openai.go`), add an `init()` function.
    *   Inside `init()`, call `RegisterProviderFactory("provider-name", NewConcreteProviderFactory)` to register the provider's factory function.

4.  **Registry Initialization Logic:**
    *   Modify `NewProviderRegistry`.
    *   It will iterate through the `cfg.LLMProviders` configuration map.
    *   For each configured provider, it looks up the corresponding factory function in the `providerFactories` map.
    *   It calls the factory, passing the specific configuration block and a logger.
    *   If the factory returns a valid provider, it's added to the registry's `providers` map.

5.  **Simplify Provider Implementations:**
    *   Update provider constructors (e.g., `NewOpenAIProvider`) to accept `map[string]interface{}` and a logger. They should parse this map for their settings instead of reading environment variables directly (except potentially for API keys, which should still come from env for security).
    *   Each provider handles `ProviderOptions` (like `Temperature`) appropriately for its API.

6.  **Simplify Slack Client Usage:**
    *   In `internal/slack/client.go`'s `callLLM` function, read the desired provider name from `c.cfg.LLMProvider`.
    *   Retrieve the provider instance using `c.llmRegistry.GetProvider(providerName)`.
    *   Remove hardcoded "langchain" usage.

7.  **Update `main.go`:**
    *   Ensure the new `LLMProviders` config section is loaded.

8.  **Update Example Configuration:**
    *   Reflect the new `llm_providers` structure in `mcp-servers.json.example` and any other relevant examples.
