package observability

import (
    "context"
    "fmt"
    "os"
    "time"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
    "github.com/tuannvm/slack-mcp-client/internal/common/logging"
    "github.com/tuannvm/slack-mcp-client/internal/config"
)

const TracerName = "slack-mcp-client"

// SimpleProvider provides basic OpenTelemetry tracing
type SimpleProvider struct {
    tracer  trace.Tracer
    logger  *logging.Logger
    config  *config.ObservabilityConfig
    enabled bool
}

// NewSimpleProvider creates a new simple OpenTelemetry provider
func NewSimpleProvider(cfg *config.Config, logger *logging.Logger) *SimpleProvider {
    return &SimpleProvider{
        tracer:  otel.Tracer(TracerName),
        logger:  logger,
        config:  &cfg.Observability,
        enabled: cfg.Observability.Enabled,
	}
}

func (p *SimpleProvider) StartTrace(ctx context.Context, name string, input string, metadata map[string]string) (context.Context, trace.Span) {
    spanCtx, span := p.tracer.Start(ctx, name)

    // Apply basic attributes
    span.SetAttributes(
        attribute.String("service.name", p.getServiceName()),
        attribute.String("service.version", p.getServiceVersion()),
        attribute.String("environment", p.getEnvironment()),
        attribute.String("trace.name", name),
        attribute.String("input.value", input),
        attribute.Int("input.length", len(input)),
    )

    // Add metadata as regular attributes
    for key, value := range metadata {
        span.SetAttributes(
            attribute.String(key, value),
        )
    }

    return spanCtx, span
}

func (p *SimpleProvider) StartSpan(ctx context.Context, name string, spanType string, input string, metadata map[string]string) (context.Context, trace.Span) {
    spanCtx, span := p.tracer.Start(ctx, name)

    // Apply basic attributes
    if spanType != "" {
        span.SetAttributes(
            attribute.String("span.type", spanType),
        )
    }

    if input != "" {
        span.SetAttributes(
            attribute.String("input.value", input),
            attribute.Int("input.length", len(input)),
        )
    }

    // Add metadata as regular attributes
    for key, value := range metadata {
        span.SetAttributes(
            attribute.String(key, value),
        )
    }

    return spanCtx, span
}

func (p *SimpleProvider) StartLLMSpan(ctx context.Context, name string, model string, input string, parameters map[string]interface{}) (context.Context, trace.Span) {
    spanCtx, span := p.tracer.Start(ctx, name)

    // Apply LLM-specific attributes
    span.SetAttributes(
        attribute.String("llm.operation_type", "generation"),
        attribute.String("llm.model_name", model),
        attribute.String("model", model),
        attribute.String("input.value", input),
        attribute.Int("input.length", len(input)),
    )

    // Add parameters as individual attributes
    for key, value := range parameters {
        switch v := value.(type) {
        case string:
            span.SetAttributes(attribute.String(fmt.Sprintf("llm.parameter.%s", key), v))
        case int:
            span.SetAttributes(attribute.Int(fmt.Sprintf("llm.parameter.%s", key), v))
        case float64:
            span.SetAttributes(attribute.Float64(fmt.Sprintf("llm.parameter.%s", key), v))
        case bool:
            span.SetAttributes(attribute.Bool(fmt.Sprintf("llm.parameter.%s", key), v))
        }
    }

    return spanCtx, span
}

func (p *SimpleProvider) SetOutput(span trace.Span, output string) {
    span.SetAttributes(
        attribute.String("output.value", output),
        attribute.Int("output.length", len(output)),
    )
}

func (p *SimpleProvider) SetTokenUsage(span trace.Span, promptTokens, completionTokens, totalTokens int) {
    span.SetAttributes(
        attribute.Int("llm.usage.prompt_tokens", promptTokens),
        attribute.Int("llm.usage.completion_tokens", completionTokens),
        attribute.Int("llm.usage.total_tokens", totalTokens),
        attribute.Int("tokens.prompt", promptTokens),
        attribute.Int("tokens.completion", completionTokens),
        attribute.Int("tokens.total", totalTokens),
    )
}

func (p *SimpleProvider) SetDuration(span trace.Span, duration time.Duration) {
    span.SetAttributes(
        attribute.Float64("duration.seconds", duration.Seconds()),
        attribute.Int64("duration.milliseconds", duration.Milliseconds()),
    )
}

func (p *SimpleProvider) RecordError(span trace.Span, err error, level string) {
    if err == nil {
        return
    }

    span.SetAttributes(
        attribute.String("error.type", "error"),
        attribute.String("error.message", err.Error()),
    )

    if level != "" {
        span.SetAttributes(
            attribute.String("error.level", level),
        )
    }
	span.RecordError(err)
    span.SetStatus(codes.Error, err.Error())
}

func (p *SimpleProvider) RecordSuccess(span trace.Span, message string) {
    span.SetAttributes(
        attribute.String("status", "success"),
    )
    span.SetStatus(codes.Ok, message)
}

func (p *SimpleProvider) GetProviderType() TracingProvider {
    return ProviderSimple
}

func (p *SimpleProvider) IsEnabled() bool {
    return p.enabled
}

// Helper methods
func (p *SimpleProvider) getServiceName() string {
    if p.config != nil && p.config.ServiceName != "" {
        return p.config.ServiceName
    }
    return "slack-mcp-client"
}

func (p *SimpleProvider) getServiceVersion() string {
    if p.config != nil && p.config.ServiceVersion != "" {
        return p.config.ServiceVersion
    }
    return "1.0.0"
}

func (p *SimpleProvider) getEnvironment() string {
    if env := os.Getenv("ENVIRONMENT"); env != "" {
        return env
    }
    if env := os.Getenv("ENV"); env != "" {
        return env
    }
    return "development"
}