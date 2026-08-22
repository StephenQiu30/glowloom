package observability

import (
	"context"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func Tracer(name string) trace.Tracer { return otel.Tracer(name) }

func Start(ctx context.Context, tracer trace.Tracer, operation string) (context.Context, trace.Span) {
	return tracer.Start(ctx, operation)
}

func LogOperation(ctx context.Context, logger *slog.Logger, operation string, started time.Time, err error) {
	attrs := []any{"operation", operation, "duration_ms", time.Since(started).Milliseconds()}
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		attrs = append(attrs, "trace_id", span.SpanContext().TraceID().String(), "span_id", span.SpanContext().SpanID().String())
	}
	if err != nil {
		attrs = append(attrs, "error", err)
		logger.ErrorContext(ctx, "operation failed", attrs...)
		return
	}
	logger.InfoContext(ctx, "operation completed", attrs...)
}

func ServiceName() string {
	if value := os.Getenv("OTEL_SERVICE_NAME"); value != "" {
		return value
	}
	return "lanverse-backend"
}
