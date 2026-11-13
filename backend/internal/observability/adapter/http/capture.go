// Package http exposes safe request diagnostics over authenticated HTTP.
package http

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/observability/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

const requestTraceIDKey = "diagnostic_trace_id"

// SnapshotTraceID retains the active server trace before tracing restores the request context.
// Register it inside the tracing middleware and inside outer request completion.
func SnapshotTraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if span := trace.SpanContextFromContext(c.Request.Context()); span.IsValid() {
			c.Set(requestTraceIDKey, span.TraceID().String())
		}
		c.Next()
	}
}

// Recorder accepts only the whitelisted completion projection.
type Recorder interface{ Append(domain.DiagnosticRecord) }

// RequestCompletion preserves structured logging and replaces raw Gin recovery output.
func RequestCompletion(logger *zap.Logger, recorder Recorder) gin.HandlerFunc {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(c *gin.Context) {
		started := time.Now()
		id, e := uuid.Parse(c.GetHeader("X-Request-Id"))
		if e != nil || id == uuid.Nil {
			id = uuid.New()
		}
		c.Set("request_id", id.String())
		c.Header("X-Request-Id", id.String())
		defer func() {
			if recover() != nil {
				c.Set("request_panic", true)
				if !c.Writer.Written() {
					httpapi.WriteProblem(c, 500, "internal_error", nil)
				}
				c.Abort()
			}
			route := c.FullPath()
			if route == "/healthz" || route == "/readyz" || strings.HasPrefix(route, "/api/platform/diagnostics") {
				return
			}
			if route == "" {
				route = "unmatched"
			}
			r := domain.DiagnosticRecord{Panic: c.GetBool("request_panic"), Timestamp: time.Now().UTC(), Method: c.Request.Method, Route: route, Status: c.Writer.Status(), ElapsedMS: float64(time.Since(started).Microseconds()) / 1000, RequestID: c.GetString("request_id")}
			if id, err := trace.TraceIDFromHex(c.GetString(requestTraceIDKey)); err == nil && id.IsValid() {
				r.TraceID = id.String()
			} else if span := trace.SpanContextFromContext(c.Request.Context()); span.IsValid() {
				r.TraceID = span.TraceID().String()
			}
			switch r.Method {
			case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE":
			default:
				r.Method = "OTHER"
			}
			if recorder != nil {
				appendRecord(recorder, r)
			}
			fields := []zap.Field{zap.String("method", r.Method), zap.String("path", r.Route), zap.Int("status", r.Status), zap.Float64("elapsed_ms", r.ElapsedMS), zap.String("request_id", r.RequestID), zap.Bool("panic", r.Panic)}
			if r.TraceID != "" {
				fields = append(fields, zap.String("trace_id", r.TraceID))
			}
			logger.Debug("http request", fields...)
		}()
		c.Next()
	}
}

// appendRecord isolates diagnostic failure without serializing its panic or error.
func appendRecord(recorder Recorder, record domain.DiagnosticRecord) {
	defer func() { _ = recover() }()
	recorder.Append(record)
}
