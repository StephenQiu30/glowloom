package telemetry

import (
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/felixge/httpsnoop"
	"github.com/google/uuid"
)

var (
	requestIDPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	traceparentPattern = regexp.MustCompile(`(?i)^00-([0-9a-f]{32})-[0-9a-f]{16}-[0-9a-f]{2}$`)
)

func HTTPLoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if logger == nil {
			return next
		}
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			requestID := strings.TrimSpace(request.Header.Get("X-Request-ID"))
			if !requestIDPattern.MatchString(requestID) {
				requestID = uuid.NewString()
				request.Header.Set("X-Request-ID", requestID)
			}
			writer.Header().Set("X-Request-ID", requestID)
			traceID := traceIDFromHeader(request.Header.Get("traceparent"))
			if traceID == "" {
				traceID = strings.ReplaceAll(uuid.NewString(), "-", "")
			}

			captured := httpsnoop.CaptureMetrics(next, writer, request)
			attributes := []slog.Attr{
				slog.String("event", "http_request"),
				slog.String("trace_id", traceID),
				slog.String("request_id", requestID),
				slog.String("method", request.Method),
				slog.String("route", boundedRoute(request.Pattern)),
				slog.Int("status_code", captured.Code),
				slog.Float64("duration_ms", float64(captured.Duration.Microseconds())/1000),
			}
			level := slog.LevelInfo
			if captured.Code >= http.StatusBadRequest {
				attributes = append(attributes, slog.String("error_code", httpErrorCode(captured.Code)))
				level = slog.LevelWarn
			}
			if captured.Code >= http.StatusInternalServerError {
				level = slog.LevelError
			}
			logger.LogAttrs(request.Context(), level, "HTTP request completed", attributes...)
		})
	}
}

func traceIDFromHeader(value string) string {
	match := traceparentPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 2 || match[1] == strings.Repeat("0", 32) {
		return ""
	}
	return strings.ToLower(match[1])
}

func httpErrorCode(status int) string {
	return "http_status_" + strconv.Itoa(status)
}
