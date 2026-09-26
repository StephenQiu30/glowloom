// Package otelconn configures trace export for backend processes.
package otelconn

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// ErrInvalidEndpoint means the OTLP/HTTP endpoint is not a plain HTTP(S) origin.
var ErrInvalidEndpoint = errors.New("invalid LV_OTEL_ENDPOINT")

// Open creates a per-process tracer provider. An empty endpoint disables export.
func Open(ctx context.Context, endpoint, serviceName string) (trace.TracerProvider, func(context.Context) error, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return noop.NewTracerProvider(), func(context.Context) error { return nil }, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.ForceQuery {
		return nil, nil, ErrInvalidEndpoint
	}
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(u.String()+"/v1/traces"),
		otlptracehttp.WithTimeout(2*time.Second),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes("", attribute.String("service.name", serviceName))),
	)
	return provider, provider.Shutdown, nil
}
