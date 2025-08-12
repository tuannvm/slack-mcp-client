package observability

import (
	"context"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"go.opentelemetry.io/otel/trace"
	"time"
)

type TracingProvider string

const (
	ProviderSimple   TracingProvider = "simple-otel"
	ProviderLangfuse TracingProvider = "langfuse-otel"
	ProviderDisabled TracingProvider = "disabled"
)

const TracerName = "slack-mcp-client"

type TracingHandler interface {
	// Core span operations
	StartTrace(ctx context.Context, name string, input string, metadata map[string]string) (context.Context, trace.Span)
	StartSpan(ctx context.Context, name string, spanType string, input string, metadata map[string]string) (context.Context, trace.Span)
	StartLLMSpan(ctx context.Context, name string, model string, input string, parameters map[string]interface{}) (context.Context, trace.Span)

	// Span attribute setters
	SetOutput(span trace.Span, output string)
	SetTokenUsage(span trace.Span, promptTokens, completionTokens, reasoningTokens, totalTokens int)
	SetDuration(span trace.Span, duration time.Duration)

	// Status and error handling
	RecordError(span trace.Span, err error, level string)
	RecordSuccess(span trace.Span, message string)

	// Provider info
	GetProvider() TracingProvider
	IsEnabled() bool
}

// noOpHandler is an embedded struct that provides no-op implementations
// Any provider can embed this to get default no-op behavior
type noOpHandler struct{}

func (n noOpHandler) StartTrace(ctx context.Context, name string, input string, metadata map[string]string) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}

func (n noOpHandler) StartSpan(ctx context.Context, name string, spanType string, input string, metadata map[string]string) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}

func (n noOpHandler) StartLLMSpan(ctx context.Context, name string, model string, input string, parameters map[string]interface{}) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}

func (n noOpHandler) SetOutput(span trace.Span, output string) {}

func (n noOpHandler) SetTokenUsage(span trace.Span, promptTokens, completionTokens, reasoningTokens, totalTokens int) {
}

func (n noOpHandler) SetDuration(span trace.Span, duration time.Duration) {}

func (n noOpHandler) RecordError(span trace.Span, err error, level string) {}

func (n noOpHandler) RecordSuccess(span trace.Span, message string) {}

func (n noOpHandler) GetProvider() TracingProvider {
	return ProviderDisabled
}

func (n noOpHandler) IsEnabled() bool {
	return false
}

// disabledHandler is the concrete implementation for disabled tracing
type disabledHandler struct {
	noOpHandler
}

// NewTracingHandler creates a new tracing handler based on config
func NewTracingHandler(cfg *config.Config, logger *logging.Logger) TracingHandler {
	// Check if observability is disabled
	if cfg == nil || !cfg.Observability.Enabled {
		logger.Info("Observability disabled")
		return &disabledHandler{}
	}

	// Create the appropriate provider with failure handling
	switch cfg.Observability.Provider {
	case "langfuse-otel":
		provider := NewLangfuseProvider(cfg, logger)
		if !provider.IsEnabled() {
			logger.Warn("Langfuse provider failed to initialize, falling back to disabled")
			return &disabledHandler{}
		}
		logger.InfoKV("Tracing provider initialized", "type", "langfuse-otel", "enabled", true)
		return provider
	case "simple-otel":
		provider := NewSimpleProvider(cfg, logger)
		if !provider.IsEnabled() {
			logger.Warn("Simple provider failed to initialize, falling back to disabled")
			return &disabledHandler{}
		}
		logger.InfoKV("Tracing provider initialized", "type", "simple-otel", "enabled", true)
		return provider
	default:
		logger.WarnKV("Unknown provider, defaulting to simple-otel", "provider", cfg.Observability.Provider)
		provider := NewSimpleProvider(cfg, logger)
		if !provider.IsEnabled() {
			logger.Warn("Simple provider failed to initialize, falling back to disabled")
			return &disabledHandler{}
		}
		logger.InfoKV("Tracing provider initialized", "type", "simple-otel", "enabled", true)
		return provider
	}
}
