// Package llm provides implementations for language model providers
package llm

import (
	"context"
	"fmt"
	"sync"

	"github.com/tuannvm/slack-mcp-client/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

// ProviderRegistry manages LLM providers and provides fallback functionality
type ProviderRegistry struct {
	providers       map[string]LLMProvider
	fallbacks       map[string]string
	primaryProvider string
	mu              sync.RWMutex
	logger          *logging.Logger
}

// NewProviderRegistry creates a new registry of LLM providers
func NewProviderRegistry(logger *logging.Logger) *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]LLMProvider),
		fallbacks: make(map[string]string),
		logger:    logger.WithName("provider-registry"),
	}
}

// RegisterProvider adds a provider to the registry
func (r *ProviderRegistry) RegisterProvider(provider LLMProvider) {
	info := provider.GetInfo()
	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers[info.Name] = provider
	r.logger.Info("Registered provider: %s", info.Name)

	// If this is the first provider, set it as primary
	if r.primaryProvider == "" {
		r.primaryProvider = info.Name
		r.logger.Info("Set primary provider: %s", info.Name)
	}
}

// SetPrimaryProvider sets the primary provider by name
func (r *ProviderRegistry) SetPrimaryProvider(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; exists {
		r.primaryProvider = name
		r.logger.Info("Set primary provider: %s", name)
		return true
	}

	r.logger.Warn("Cannot set primary provider: %s (not found)", name)
	return false
}

// GetPrimaryProvider returns the name of the primary provider
func (r *ProviderRegistry) GetPrimaryProvider() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.primaryProvider
}

// SetFallback sets a fallback provider for when a provider fails
func (r *ProviderRegistry) SetFallback(providerName, fallbackName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check that both providers exist
	if _, providerExists := r.providers[providerName]; !providerExists {
		r.logger.Warn("Cannot set fallback: provider %s not found", providerName)
		return false
	}

	if _, fallbackExists := r.providers[fallbackName]; !fallbackExists {
		r.logger.Warn("Cannot set fallback: fallback provider %s not found", fallbackName)
		return false
	}

	// Set the fallback
	r.fallbacks[providerName] = fallbackName
	r.logger.Info("Set fallback for %s: %s", providerName, fallbackName)
	return true
}

// GetProvider returns a provider by name, using fallbacks if necessary
func (r *ProviderRegistry) GetProvider(name string) (LLMProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if provider, exists := r.providers[name]; exists {
		return provider, nil
	}

	r.logger.Warn("Provider not found: %s", name)
	return nil, errors.NewLLMError("provider_not_found", fmt.Sprintf("Provider '%s' not found", name))
}

// GenerateCompletion generates a completion using the specified provider or fallbacks
func (r *ProviderRegistry) GenerateCompletion(ctx context.Context, providerName, prompt string, options ProviderOptions) (string, error) {
	r.mu.RLock()
	provider, exists := r.providers[providerName]
	fallbackName, hasFallback := r.fallbacks[providerName]
	r.mu.RUnlock()

	// If the provider doesn't exist, return an error
	if !exists {
		return "", errors.NewLLMError("provider_not_found", fmt.Sprintf("Provider '%s' not found", providerName))
	}

	// Try the primary provider
	result, err := provider.GenerateCompletion(ctx, prompt, options)
	if err == nil {
		return result, nil
	}

	// If there's an error and we have a fallback, try it
	if hasFallback {
		r.logger.Warn("Provider %s failed, trying fallback %s: %v", providerName, fallbackName, err)

		r.mu.RLock()
		fallbackProvider, fallbackExists := r.providers[fallbackName]
		r.mu.RUnlock()

		if fallbackExists {
			return fallbackProvider.GenerateCompletion(ctx, prompt, options)
		}
	}

	// No fallback or fallback also failed, return the original error
	return "", err
}

// GenerateChatCompletion generates a chat completion using the specified provider or fallbacks
func (r *ProviderRegistry) GenerateChatCompletion(ctx context.Context, providerName string, messages []RequestMessage, options ProviderOptions) (string, error) {
	r.mu.RLock()
	provider, exists := r.providers[providerName]
	fallbackName, hasFallback := r.fallbacks[providerName]
	r.mu.RUnlock()

	// If the provider doesn't exist, return an error
	if !exists {
		return "", errors.NewLLMError("provider_not_found", fmt.Sprintf("Provider '%s' not found", providerName))
	}

	// Try the primary provider
	result, err := provider.GenerateChatCompletion(ctx, messages, options)
	if err == nil {
		return result, nil
	}

	// If there's an error and we have a fallback, try it
	if hasFallback {
		r.logger.Warn("Provider %s failed, trying fallback %s: %v", providerName, fallbackName, err)

		r.mu.RLock()
		fallbackProvider, fallbackExists := r.providers[fallbackName]
		r.mu.RUnlock()

		if fallbackExists {
			return fallbackProvider.GenerateChatCompletion(ctx, messages, options)
		}
	}

	// No fallback or fallback also failed, return the original error
	return "", err
}

// ListProviders returns a list of available provider names
func (r *ProviderRegistry) ListProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]string, 0, len(r.providers))
	for name := range r.providers {
		providers = append(providers, name)
	}

	return providers
}
