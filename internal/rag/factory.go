// Package rag provides a simplified factory for creating vector providers
package rag

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// CreateVectorProvider creates a vector provider instance based on configuration
func CreateVectorProvider(providerType string, config map[string]interface{}) (VectorProvider, error) {
	factory, exists := providerRegistry[providerType]
	if !exists {
		return nil, &ProviderNotFoundError{Provider: providerType}
	}
	return factory(config)
}

// CreateProviderFromConfig creates a vector provider from configuration map
func CreateProviderFromConfig(config map[string]interface{}) (VectorProvider, error) {
	// Extract provider type from config
	providerType, ok := config["provider"].(string)
	if !ok {
		providerType = "simple" // Default to simple provider
	}

	// Create the vector provider
	provider, err := CreateVectorProvider(providerType, config)
	if err != nil {
		return nil, err
	}

	// Initialize the provider
	if err := provider.Initialize(context.Background()); err != nil {
		if closeErr := provider.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close provider: %v\n", closeErr)
		}
		return nil, fmt.Errorf("failed to initialize provider: %w", err)
	}

	return provider, nil
}

// ExtractRAGConfig extracts RAG configuration from LLM provider config
func ExtractRAGConfig(llmProviderConfig map[string]interface{}) map[string]interface{} {
	ragConfig := make(map[string]interface{})

	// Check if RAG is enabled
	if enabled, ok := llmProviderConfig["rag_enabled"].(bool); ok && !enabled {
		return nil
	}

	// Extract RAG provider
	if provider, ok := llmProviderConfig["rag_provider"].(string); ok {
		ragConfig["provider"] = provider
	} else {
		ragConfig["provider"] = "simple" // Default
	}

	// Extract database path (for simple provider)
	if dbPath, ok := llmProviderConfig["rag_database"].(string); ok {
		ragConfig["database_path"] = dbPath
	}

	// Extract rag_config section if present
	if ragConfigSection, ok := llmProviderConfig["rag_config"].(map[string]interface{}); ok {
		// Copy all rag_config values to main config
		for key, value := range ragConfigSection {
			ragConfig[key] = value
		}
	}

	// Add OpenAI API key from environment if using OpenAI provider
	if provider, ok := ragConfig["provider"].(string); ok && provider == "openai" {
		if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
			ragConfig["api_key"] = apiKey
		} else if apiKey, ok := llmProviderConfig["api_key"].(string); ok {
			ragConfig["api_key"] = apiKey
		}
	}

	return ragConfig
}

// GetProviderFromFlags determines the provider from command-line flags
func GetProviderFromFlags(ragProvider string, llmProvider string) string {
	// If explicit RAG provider is specified, use it
	if ragProvider != "" {
		return ragProvider
	}

	// Otherwise, infer from LLM provider for convenience
	switch strings.ToLower(llmProvider) {
	case "openai":
		return "openai"
	default:
		return "simple"
	}
}