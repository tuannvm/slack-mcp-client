package observability

import (
	"context"
	"time"
	"go.opentelemetry.io/otel/trace"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/config"
)

type TracingProvider string

const (
	ProviderSimple   TracingProvider = "simple-otel"
	ProviderLangfuse  TracingProvider = "langfuse-otel"
	ProviderDisabled  TracingProvider = "disabled"
)

// Provider defines the interface that all tracing providers must implement
type Provider interface {
    // Core span operations
    StartTrace(ctx context.Context, name string, input string, metadata map[string]string) (context.Context, trace.Span)
    StartSpan(ctx context.Context, name string, spanType string, input string, metadata map[string]string) (context.Context, trace.Span)
    StartLLMSpan(ctx context.Context, name string, model string, input string, parameters map[string]interface{}) (context.Context, trace.Span)

    // Span attribute setters
    SetOutput(span trace.Span, output string)
    SetTokenUsage(span trace.Span, promptTokens, completionTokens, totalTokens int)
    SetDuration(span trace.Span, duration time.Duration)

    // Status and error handling
    RecordError(span trace.Span, err error, level string)
    RecordSuccess(span trace.Span, message string)

    // Provider info
    GetProviderType() TracingProvider
    IsEnabled() bool
}

// TracingHandler is the main handler that delegates to specific providers
type TracingHandler struct {
    provider Provider
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

func (n noOpHandler) SetTokenUsage(span trace.Span, promptTokens, completionTokens, totalTokens int) {}

func (n noOpHandler) SetDuration(span trace.Span, duration time.Duration) {}

func (n noOpHandler) RecordError(span trace.Span, err error, level string) {}

func (n noOpHandler) RecordSuccess(span trace.Span, message string) {}

func (n noOpHandler) GetProviderType() TracingProvider {
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
func NewTracingHandler(cfg *config.Config, logger *logging.Logger) *TracingHandler {

    // Determine which provider to use
    if cfg == nil || !cfg.Observability.Enabled {
        logger.Info("Observability disabled")
        return &TracingHandler{provider: &disabledHandler{}}
    }
    // Create the appropriate provider
    switch cfg.Observability.Provider {
    case "langfuse-otel":
        logger.InfoKV("Tracing provider initialized", "type", "langfuse-otel", "enabled", true)
        return &TracingHandler{provider: NewLangfuseProvider(cfg, logger)}
    case "simple-otel":
        logger.InfoKV("Tracing provider initialized", "type", "simple-otel", "enabled", true)
        return &TracingHandler{provider: NewSimpleProvider(cfg, logger)}
    default:
        logger.WarnKV("Unknown provider, defaulting to simple-otel", "provider", cfg.Observability.Provider)
        logger.InfoKV("Tracing provider initialized", "type", "simple-otel", "enabled", true)
        return &TracingHandler{provider: NewSimpleProvider(cfg, logger)}
    }
}

// Delegate all methods to the underlying provider
func (h *TracingHandler) StartTrace(ctx context.Context, name string, input string, metadata map[string]string) (context.Context, trace.Span) {
    return h.provider.StartTrace(ctx, name, input, metadata)
}

func (h *TracingHandler) StartSpan(ctx context.Context, name string, spanType string, input string, metadata map[string]string) (context.Context, trace.Span) {
    return h.provider.StartSpan(ctx, name, spanType, input, metadata)
}

func (h *TracingHandler) StartLLMSpan(ctx context.Context, name string, model string, input string, parameters map[string]interface{}) (context.Context, trace.Span) {
    return h.provider.StartLLMSpan(ctx, name, model, input, parameters)
}

func (h *TracingHandler) SetOutput(span trace.Span, output string) {
    h.provider.SetOutput(span, output)
}

func (h *TracingHandler) SetTokenUsage(span trace.Span, promptTokens, completionTokens, totalTokens int) {
    h.provider.SetTokenUsage(span, promptTokens, completionTokens, totalTokens)
}

func (h *TracingHandler) SetDuration(span trace.Span, duration time.Duration) {
    h.provider.SetDuration(span, duration)
}

func (h *TracingHandler) RecordError(span trace.Span, err error, level string) {
    h.provider.RecordError(span, err, level)
}

func (h *TracingHandler) RecordSuccess(span trace.Span, message string) {
    h.provider.RecordSuccess(span, message)
}

func (h *TracingHandler) IsEnabled() bool {
    return h.provider.IsEnabled()
}

func (h *TracingHandler) GetProvider() TracingProvider {
    return h.provider.GetProviderType()
}