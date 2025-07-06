// Package rag provides a factory for creating RAG providers
package rag

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// RAGFactory creates RAG providers based on configuration
type RAGFactory struct {
	defaultProvider string
	databasePath    string
}

// NewRAGFactory creates a new RAG factory
func NewRAGFactory(defaultProvider, databasePath string) *RAGFactory {
	if defaultProvider == "" {
		defaultProvider = "simple"
	}
	if databasePath == "" {
		databasePath = "./knowledge.json"
	}
	
	return &RAGFactory{
		defaultProvider: defaultProvider,
		databasePath:    databasePath,
	}
}

// CreateProvider creates a RAG provider based on the given type
func (f *RAGFactory) CreateProvider(providerType string, config map[string]interface{}) (RAGProvider, error) {
	if providerType == "" {
		providerType = f.defaultProvider
	}

	switch providerType {
	case "simple":
		// SimpleRAG doesn't fully implement RAGProvider, so wrap it
		return NewLangChainCompatibleRAG(ProviderConfig{
			DatabasePath: f.databasePath,
		})
		
	case "openai":
		// Create OpenAI provider
		vectorProvider, err := CreateVectorProvider(ProviderConfig{
			Provider: "openai",
			Options:  config,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenAI provider: %w", err)
		}
		
		// Initialize the provider
		if err := vectorProvider.Initialize(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to initialize OpenAI provider: %w", err)
		}
		
		// Wrap in adapter for backward compatibility
		adapter, err := NewVectorProviderAdapter(vectorProvider)
		if err != nil {
			vectorProvider.Close()
			return nil, fmt.Errorf("failed to create adapter: %w", err)
		}
		
		return adapter, nil
		
	default:
		// Try to create a custom vector provider
		vectorProvider, err := CreateVectorProvider(ProviderConfig{
			Provider: providerType,
			Options:  config,
		})
		if err != nil {
			return nil, fmt.Errorf("unknown RAG provider: %s", providerType)
		}
		
		// Initialize the provider
		if err := vectorProvider.Initialize(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to initialize %s provider: %w", providerType, err)
		}
		
		// Wrap in adapter
		adapter, err := NewVectorProviderAdapter(vectorProvider)
		if err != nil {
			vectorProvider.Close()
			return nil, fmt.Errorf("failed to create adapter: %w", err)
		}
		
		return adapter, nil
	}
}

// CreateVectorProviderFromConfig creates a vector provider from configuration
func (f *RAGFactory) CreateVectorProviderFromConfig(config map[string]interface{}) (VectorProvider, error) {
	// Extract provider type from config
	providerType, ok := config["provider"].(string)
	if !ok {
		providerType = f.defaultProvider
	}
	
	// Handle special cases
	if providerType == "simple" {
		return NewSimpleRAGAdapter(f.databasePath), nil
	}
	
	// Extract provider-specific config
	providerConfig := make(map[string]interface{})
	if pc, ok := config[providerType].(map[string]interface{}); ok {
		providerConfig = pc
	}
	
	// Create the vector provider
	provider, err := CreateVectorProvider(ProviderConfig{
		Provider: providerType,
		Options:  providerConfig,
	})
	if err != nil {
		return nil, err
	}
	
	// Initialize the provider
	if err := provider.Initialize(context.Background()); err != nil {
		provider.Close()
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
	}
	
	// Extract database path
	if dbPath, ok := llmProviderConfig["rag_database"].(string); ok {
		ragConfig["database_path"] = dbPath
	}
	
	// Extract provider-specific configs
	// For OpenAI
	if openaiConfig, ok := llmProviderConfig["rag_openai"].(map[string]interface{}); ok {
		ragConfig["openai"] = openaiConfig
	} else {
		// Try to construct OpenAI config from environment or LLM config
		openaiConfig := make(map[string]interface{})
		
		// Use API key from environment or LLM config
		if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
			openaiConfig["api_key"] = apiKey
		} else if apiKey, ok := llmProviderConfig["api_key"].(string); ok {
			openaiConfig["api_key"] = apiKey
		}
		
		// Add other OpenAI-specific settings if present
		if assistantID, ok := llmProviderConfig["rag_assistant_id"].(string); ok {
			openaiConfig["assistant_id"] = assistantID
		}
		if vectorStoreID, ok := llmProviderConfig["rag_vector_store_id"].(string); ok {
			openaiConfig["vector_store_id"] = vectorStoreID
		}
		
		if len(openaiConfig) > 0 {
			ragConfig["openai"] = openaiConfig
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
		// If using OpenAI LLM, default to OpenAI RAG
		return "openai"
	default:
		// Default to simple for all other cases
		return "simple"
	}
}