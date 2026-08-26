package bootstrap

import (
	"encoding/json"
	"net/http"

	"github.com/rs/cors"

	publicopenapi "github.com/StephenQiu30/lanverse/backend/api/openapi"
)

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Reason  string `json:"reason,omitempty"`
}

func NewAPIHandler(runtime RuntimeOptions) http.Handler {
	runtime = runtime.normalized()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writeHealthResponse(writer, http.StatusOK, healthResponse{Status: "ok", Service: "lanverse-api"})
	})
	mux.HandleFunc("GET /readyz", readinessHandler(runtime))
	mux.HandleFunc("GET /version", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, runtime.Build)
	})
	mux.HandleFunc("GET /openapi.json", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(publicopenapi.Document())
	})
	mux.Handle("GET /metrics", runtime.Metrics.Handler())
	if runtime.RegisterRoutes != nil {
		runtime.RegisterRoutes(mux)
	}
	handler := runtime.Metrics.Middleware(mux)
	if len(runtime.AllowedOrigins) > 0 {
		handler = cors.New(cors.Options{
			AllowedOrigins:   runtime.AllowedOrigins,
			AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete, http.MethodOptions},
			AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Request-ID"},
			AllowCredentials: true,
			MaxAge:           600,
		}).Handler(handler)
	}
	return handler
}

func readinessHandler(runtime RuntimeOptions) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if runtime.Ready != nil {
			if err := runtime.Ready(request.Context()); err != nil {
				writeNotReady(writer, "dependency_unavailable")
				return
			}
		}
		writeHealthResponse(writer, http.StatusOK, healthResponse{Status: "ready", Service: "lanverse-api"})
	}
}

func writeNotReady(writer http.ResponseWriter, reason string) {
	writeHealthResponse(writer, http.StatusServiceUnavailable, healthResponse{Status: "not_ready", Service: "lanverse-api", Reason: reason})
}

func writeHealthResponse(writer http.ResponseWriter, status int, response healthResponse) {
	writeJSON(writer, status, response)
}

func writeJSON(writer http.ResponseWriter, status int, response any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(response)
}
