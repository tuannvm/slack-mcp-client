// Package llm provides implementations for language model providers
package llm

import (
	"context"
	"fmt"
	"sync"

	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/config" // Import config
)

// ProviderRegistry manages all available LLM providers
type ProviderRegistry struct {
	providers map[string]LLMProvider
	primary   string
	logger    *logging.Logger
	mu        sync.RWMutex
}

// NewProviderRegistry creates a new provider registry and initializes providers from config.
func NewProviderRegistry(cfg *config.Config, logger *logging.Logger) (*ProviderRegistry, error) {
	registryLogger := logger.WithName("llm-registry")
	r := &ProviderRegistry{
		providers: make(map[string]LLMProvider),
		logger:    registryLogger,
		mu:        sync.RWMutex{},
	}

	registryLogger.Info("Initializing LLM providers from configuration...")
	initializedProviders := 0
	registeredFactories := ListProviderFactories()
	registryLogger.DebugKV("Available provider factories", "factories", registeredFactories)

	// Iterate through the providers defined in the configuration
	for name, providerConfig := range cfg.LLMProviders {
		registryLogger.DebugKV("Attempting to initialize provider", "name", name)
		factory, exists := GetProviderFactory(name)
		if !exists {
			registryLogger.WarnKV("No factory registered for configured provider, skipping.", "provider_name", name)
			continue
		}

		// Create the provider instance using the factory
		providerInstance, err := factory(providerConfig, logger)
		if err != nil {
			registryLogger.ErrorKV("Failed to create provider instance using factory", "provider_name", name, "error", err)
			continue
		}

		// Store the initialized provider
		r.providers[name] = providerInstance
		initializedProviders++
		registryLogger.InfoKV("Successfully initialized and registered LLM provider", "name", name)
	}

	if initializedProviders == 0 {
		registryLogger.Warn("No LLM providers were successfully initialized from the configuration.")
		// Depending on requirements, this could be an error
		// return nil, fmt.Errorf("no LLM providers initialized")
	}

	// Set the primary provider from the configuration
	if cfg.LLMProvider != "" {
		if _, exists := r.providers[cfg.LLMProvider]; exists {
			r.primary = cfg.LLMProvider
			registryLogger.InfoKV("Set primary LLM provider", "name", r.primary)
		} else {
			// Primary provider specified in config was not successfully initialized
			registryLogger.ErrorKV("Primary LLM provider specified in config could not be initialized or found", "configured_primary", cfg.LLMProvider)
			// Attempt to set a fallback primary (e.g., the first initialized one)
			if len(r.providers) > 0 {
				for name := range r.providers {
					r.primary = name
					registryLogger.WarnKV("Falling back to using provider as primary", "name", r.primary)
					break
				}
			} else {
				// No providers available at all
				registryLogger.Error("No LLM providers available to set as primary.")
				return nil, fmt.Errorf("failed to set a primary LLM provider, none are available")
			}
		}
	} else if len(r.providers) > 0 {
		// No primary provider specified, use the first initialized one
		for name := range r.providers {
			r.primary = name
			registryLogger.InfoKV("No primary LLM provider specified in config, defaulting to first initialized", "name", r.primary)
			break
		}
	} else {
		// No primary specified and no providers initialized
		registryLogger.Error("No LLM provider specified in config and none could be initialized.")
		return nil, fmt.Errorf("no LLM provider specified or initialized")
	}

	return r, nil
}

// GetPrimaryProvider returns the configured primary provider.
func (r *ProviderRegistry) GetPrimaryProvider() (LLMProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.primary == "" {
		return nil, fmt.Errorf("no primary LLM provider configured or available")
	}
	provider, exists := r.providers[r.primary]
	if !exists {
		// This shouldn't happen if initialization logic is correct
		return nil, fmt.Errorf("internal error: primary provider '%s' not found in registry", r.primary)
	}
	return provider, nil
}

// GetProvider returns a provider by name.
// If name is empty, it returns the primary provider.
func (r *ProviderRegistry) GetProvider(name string) (LLMProvider, error) {
	if name == "" {
		return r.GetPrimaryProvider()
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, exists := r.providers[name]
	if !exists {
		return nil, fmt.Errorf("provider '%s' not found in registry", name)
	}

	return provider, nil
}

// GetProviderWithAvailabilityCheck returns the requested provider only if it's available.
// If name is empty, it checks the primary provider.
func (r *ProviderRegistry) GetProviderWithAvailabilityCheck(name string) (LLMProvider, error) {
	provider, err := r.GetProvider(name)
	if err != nil {
		return nil, err // Provider not found
	}

	if !provider.IsAvailable() {
		info := provider.GetInfo()
		r.logger.WarnKV("Requested provider is not available", "name", info.Name)
		return nil, fmt.Errorf("provider '%s' is not available", info.Name)
	}

	return provider, nil
}

// ListProviders returns information about all registered providers
func (r *ProviderRegistry) ListProviders() []ProviderInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []ProviderInfo
	for _, provider := range r.providers {
		// GetInfo now potentially involves an availability check, handle potential updates
		info := provider.GetInfo()
		result = append(result, info)
	}

	return result
}

// GenerateCompletion generates a completion using the specified provider (or primary if empty).
// It checks for provider availability before making the call.
func (r *ProviderRegistry) GenerateCompletion(ctx context.Context, providerName string, prompt string, options ProviderOptions) (string, error) {
	provider, err := r.GetProviderWithAvailabilityCheck(providerName) // Use the availability check method
	if err != nil {
		return "", err
	}

	info := provider.GetInfo()
	r.logger.DebugKV("Using provider for completion", "name", info.Name)
	// Note: GenerateCompletion is deprecated in the interface, but we keep the registry method for now.
	return provider.GenerateCompletion(ctx, prompt, options)
}

// GenerateChatCompletion generates a chat completion using the specified provider (or primary if empty).
// It checks for provider availability before making the call.
func (r *ProviderRegistry) GenerateChatCompletion(ctx context.Context, providerName string, messages []RequestMessage, options ProviderOptions) (string, error) {
	provider, err := r.GetProviderWithAvailabilityCheck(providerName) // Use the availability check method
	if err != nil {
		return "", err
	}

	info := provider.GetInfo()
	r.logger.DebugKV("Using provider for chat completion", "name", info.Name)
	return provider.GenerateChatCompletion(ctx, messages, options)
}
