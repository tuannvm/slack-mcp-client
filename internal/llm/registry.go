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
	// fallbacks map[string]string // Fallback logic might be simplified or removed depending on requirements
	logger *logging.Logger
	mu     sync.RWMutex
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
	registryLogger.Debug("Available provider factories", "factories", registeredFactories)

	// Iterate through the providers defined in the configuration
	for name, providerConfig := range cfg.LLMProviders {
		registryLogger.Debug("Attempting to initialize provider", "name", name)
		factory, exists := GetProviderFactory(name)
		if !exists {
			registryLogger.Warn("No factory registered for configured provider, skipping.", "provider_name", name)
			continue
		}

		// Create the provider instance using the factory
		// Dereference logger pointer to pass the interface value
		providerInstance, err := factory(providerConfig, *logger)
		if err != nil {
			registryLogger.Error("Failed to create provider instance using factory", "provider_name", name, "error", err)
			continue
		}

		// Store the initialized provider
		r.providers[name] = providerInstance
		initializedProviders++
		registryLogger.Info("Successfully initialized and registered LLM provider", "name", name)
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
			registryLogger.Info("Set primary LLM provider", "name", r.primary)
		} else {
			// Primary provider specified in config was not successfully initialized
			registryLogger.Error("Primary LLM provider specified in config could not be initialized or found", "configured_primary", cfg.LLMProvider)
			// Attempt to set a fallback primary (e.g., the first initialized one)
			if len(r.providers) > 0 {
				for name := range r.providers {
					r.primary = name
					registryLogger.Warn("Falling back to using provider as primary", "name", r.primary)
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
			registryLogger.Info("No primary LLM provider specified in config, defaulting to first initialized", "name", r.primary)
			break
		}
	} else {
		// No primary specified and no providers initialized
		registryLogger.Error("No LLM provider specified in config and none could be initialized.")
		return nil, fmt.Errorf("no LLM provider specified or initialized")
	}

	return r, nil
}

/* // RegisterProvider might be deprecated if initialization is solely config-driven
// RegisterProvider adds a provider to the registry
func (r *ProviderRegistry) RegisterProvider(provider LLMProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()

	info := provider.GetInfo()
	r.providers[info.Name] = provider
	r.logger.Info("Registered LLM provider: %s", info.Name)

	// If this is the first provider or it's langchain, set it as primary
	if len(r.providers) == 1 || info.Name == "langchain-openai" || info.Name == "langchain-ollama" {
		r.primary = info.Name
		r.logger.Info("Set primary provider to: %s", info.Name)
	}
}
*/

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
		r.logger.Warn("Requested provider is not available", "name", info.Name)
		return nil, fmt.Errorf("provider '%s' is not available", info.Name)
	}

	return provider, nil
}

/* // Fallback logic might need rethinking based on config
// SetFallback sets a fallback provider for a primary provider
func (r *ProviderRegistry) SetFallback(primary, fallback string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[primary]; !exists {
		return fmt.Errorf("primary provider '%s' not registered", primary)
	}

	if _, exists := r.providers[fallback]; !exists {
		return fmt.Errorf("fallback provider '%s' not registered", fallback)
	}

	r.fallbacks[primary] = fallback
	r.logger.Info("Set fallback from '%s' to '%s'", primary, fallback)
	return nil
}

// GetProviderWithFallback tries to get the specified provider or falls back if needed
func (r *ProviderRegistry) GetProviderWithFallback(name string) (LLMProvider, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// If no name specified, use the primary provider
	if name == "" {
		name = r.primary
	}

	// Try to get the original provider
	originalName := name
	provider, exists := r.providers[name]

	// Check if provider exists and is available
	if exists && provider.IsAvailable() {
		return provider, name, nil
	}

	// Provider doesn't exist or isn't available, try fallback
	if fallback, hasFallback := r.fallbacks[name]; hasFallback {
		fallbackProvider, fallbackExists := r.providers[fallback]
		if fallbackExists && fallbackProvider.IsAvailable() {
			r.logger.Warn("Using fallback provider '%s' instead of '%s'", fallback, name)
			return fallbackProvider, fallback, nil
		}
	}

	// No fallback or fallback also unavailable, try primary
	if name != r.primary {
		primaryProvider, primaryExists := r.providers[r.primary]
		if primaryExists && primaryProvider.IsAvailable() {
			r.logger.Warn("Using primary provider '%s' instead of '%s'", r.primary, name)
			return primaryProvider, r.primary, nil
		}
	}

	// Still no provider found, try any available provider
	for providerName, candidate := range r.providers {
		if candidate.IsAvailable() {
			r.logger.Warn("Using available provider '%s' instead of '%s'", providerName, name)
			return candidate, providerName, nil
		}
	}

	// No available providers
	if !exists {
		return nil, "", fmt.Errorf("provider '%s' not found and no fallbacks available", originalName)
	}
	return nil, "", fmt.Errorf("provider '%s' is not available and no fallbacks available", originalName)
}
*/

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
	r.logger.Debug("Using provider for completion", "name", info.Name)
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
	r.logger.Debug("Using provider for chat completion", "name", info.Name)
	return provider.GenerateChatCompletion(ctx, messages, options)
}
