package telemetry

import (
	"net/http"
	"strings"

	"github.com/felixge/httpsnoop"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type HTTPMetrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func NewHTTPMetrics() *HTTPMetrics {
	registry := prometheus.NewRegistry()
	requests := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "lanverse",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "HTTP requests by method, bounded route and status class.",
		},
		[]string{"method", "route", "status_class"},
	)
	duration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "lanverse",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration by method, bounded route and status class.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "route", "status_class"},
	)
	registry.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		requests,
		duration,
	)
	return &HTTPMetrics{
		registry: registry,
		requests: requests,
		duration: duration,
	}
}

func (metrics *HTTPMetrics) Handler() http.Handler {
	return promhttp.HandlerFor(metrics.registry, promhttp.HandlerOpts{})
}

func (metrics *HTTPMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		captured := httpsnoop.CaptureMetrics(next, writer, request)
		route := boundedRoute(request.Pattern)
		statusClass := httpStatusClass(captured.Code)
		metrics.requests.WithLabelValues(request.Method, route, statusClass).Inc()
		metrics.duration.WithLabelValues(request.Method, route, statusClass).Observe(
			captured.Duration.Seconds(),
		)
	})
}

func boundedRoute(pattern string) string {
	if pattern == "" {
		return "unmatched"
	}
	if _, route, found := strings.Cut(pattern, " "); found {
		return route
	}
	return pattern
}

func httpStatusClass(status int) string {
	if status < 100 || status > 599 {
		return "unknown"
	}
	return string(rune('0'+status/100)) + "xx"
}
