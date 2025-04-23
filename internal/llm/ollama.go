// Package llm provides implementations for language model providers
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

const (
	ollamaProviderName   = "ollama"
	defaultOllamaBaseURL = "http://localhost:11434"
	defaultOllamaModel   = "llama3"
	defaultOllamaTimeout = 2 * time.Minute
)

// OllamaProvider implements the LLMProvider interface for Ollama
type OllamaProvider struct {
	baseURL      string
	defaultModel string
	logger       *logging.Logger
	httpClient   *http.Client
	timeout      time.Duration
}

// ollamaResponse represents the structure of a response from the Ollama API
type ollamaResponse struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Response  string    `json:"response"`
	Done      bool      `json:"done"`
	// ... other fields like context, total_duration, etc. are omitted for simplicity
}

// ollamaRequest represents the structure of a request to the Ollama API
type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	// Options can be added here if needed (e.g., temperature)
}

// init registers the Ollama provider factory.
func init() {
	err := RegisterProviderFactory(ollamaProviderName, NewOllamaProviderFactory)
	if err != nil {
		panic(fmt.Sprintf("Failed to register ollama provider factory: %v", err))
	}
}

// NewOllamaProviderFactory creates an Ollama provider instance from configuration.
func NewOllamaProviderFactory(config map[string]interface{}, logger logging.Logger) (LLMProvider, error) {
	baseURL, _ := config["base_url"].(string)
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
		logger.Info("Ollama base_url not specified, defaulting to", "url", baseURL)
	}

	modelName, _ := config["model"].(string)
	if modelName == "" {
		modelName = defaultOllamaModel
		logger.Info("Ollama model not specified, defaulting to", "model", modelName)
	}

	timeout := defaultOllamaTimeout
	if timeoutStr, ok := config["timeout"].(string); ok {
		parsedDuration, err := time.ParseDuration(timeoutStr)
		if err == nil {
			timeout = parsedDuration
		} else {
			logger.Warn("Invalid ollama timeout format in config, using default", "value", timeoutStr, "default", defaultOllamaTimeout, "error", err)
		}
	}

	// Corrected logging: Use key-value pairs directly with the logger instance.
	providerLogger := logger.WithName("ollama-provider")
	providerLogger.Info("Creating Ollama provider", "model", modelName, "url", baseURL)

	return &OllamaProvider{
		baseURL:      strings.TrimSuffix(baseURL, "/"), // Ensure no trailing slash
		defaultModel: modelName,
		logger:       providerLogger,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}, nil
}

// GetInfo returns information about the provider.
func (p *OllamaProvider) GetInfo() ProviderInfo {
	return ProviderInfo{
		Name:        ollamaProviderName,
		DisplayName: fmt.Sprintf("Ollama (%s)", p.defaultModel),
		Description: fmt.Sprintf("Connects to an Ollama instance at %s", p.baseURL),
		Configured:  true, // Assumed configured if created via factory
		Available:   p.IsAvailable(),
		Configuration: map[string]string{
			"Base URL": p.baseURL,
			"Model":    p.defaultModel,
			"Timeout":  p.timeout.String(),
		},
	}
}

// IsAvailable checks if the Ollama server is reachable.
func (p *OllamaProvider) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // Short timeout for availability check
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL, nil)
	if err != nil {
		p.logger.Warn("Failed to create availability check request", "error", err)
		return false
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Warn("Availability check request failed", "error", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		p.logger.Debug("Ollama instance is available")
		return true
	} else {
		p.logger.Warn("Ollama instance returned non-OK status", "status", resp.StatusCode)
		return false
	}
}

// GenerateCompletion generates a text completion using Ollama.
func (p *OllamaProvider) GenerateCompletion(ctx context.Context, prompt string, options ProviderOptions) (string, error) {
	model := options.Model
	if model == "" {
		model = p.defaultModel
	}

	requestPayload := ollamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false, // We want the full response
	}

	requestBody, err := json.Marshal(requestPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ollama request: %w", err)
	}

	apiURL := p.baseURL + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to create ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	p.logger.Debug("Sending request to Ollama", "url", apiURL, "model", model)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorBody bytes.Buffer
		_, _ = errorBody.ReadFrom(resp.Body)
		return "", fmt.Errorf("ollama API returned error status %d: %s", resp.StatusCode, errorBody.String())
	}

	var ollamaResponse ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResponse); err != nil {
		return "", fmt.Errorf("failed to decode ollama response: %w", err)
	}

	p.logger.Debug("Received response from Ollama", "model", ollamaResponse.Model)
	return ollamaResponse.Response, nil
}

// GenerateChatCompletion is not natively supported by the basic Ollama /api/generate endpoint.
// This could be implemented using /api/chat if needed, but for now, it delegates to GenerateCompletion.
func (p *OllamaProvider) GenerateChatCompletion(ctx context.Context, messages []RequestMessage, options ProviderOptions) (string, error) {
	p.logger.Warn("GenerateChatCompletion called on Ollama provider, using simple prompt concatenation.")
	// Simple concatenation of messages for now.
	// TODO: Implement using the /api/chat endpoint for proper chat handling.
	var promptBuilder strings.Builder
	for _, msg := range messages {
		promptBuilder.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
	}
	return p.GenerateCompletion(ctx, promptBuilder.String(), options)
}
