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

	obshttp "github.com/StephenQiu30/glowloom-server/backend/internal/observability/adapter/http"
)

// ReadyCheck tests whether dependencies needed by the API are available.
type ReadyCheck func(context.Context) error

// NewRouter builds the HTTP router for the api role.
func NewRouter(logger *zap.Logger, ready ReadyCheck, tracerProvider trace.TracerProvider, recorders ...obshttp.Recorder) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	var recorder obshttp.Recorder
	if len(recorders) > 0 {
		recorder = recorders[0]
	}
	router.Use(
		obshttp.RequestCompletion(logger, recorder),
		otelgin.Middleware("glowloom-backend-api",
			otelgin.WithTracerProvider(tracerProvider),
			otelgin.WithFilter(func(r *http.Request) bool { return r.URL.Path != "/api/media-export-launch" }),
			otelgin.WithMeterProvider(metricnoop.NewMeterProvider()),
			otelgin.WithPropagators(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})),
		),
		obshttp.SnapshotTraceID(),
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
