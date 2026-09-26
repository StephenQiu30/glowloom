// Package app is the composition root: it wires routes and runs the process for each role.
package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// ReadyCheck tests whether dependencies needed by the API are available.
type ReadyCheck func(context.Context) error

// NewRouter builds the HTTP router for the api role.
func NewRouter(logger *zap.Logger, ready ReadyCheck, tracerProvider trace.TracerProvider) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(
		gin.Recovery(),
		otelgin.Middleware("lanverse-backend-api",
			otelgin.WithTracerProvider(tracerProvider),
			otelgin.WithMeterProvider(metricnoop.NewMeterProvider()),
			otelgin.WithPropagators(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})),
		),
		requestLogger(logger),
	)

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/readyz", func(c *gin.Context) {
		if ready == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	return router
}

func requestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
			zap.Int("status", c.Writer.Status()),
		}
		if spanContext := trace.SpanFromContext(c.Request.Context()).SpanContext(); spanContext.IsValid() {
			fields = append(fields, zap.String("trace_id", spanContext.TraceID().String()))
		}
		logger.Debug("http request", fields...)
	}
}
