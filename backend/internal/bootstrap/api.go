package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
)

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Reason  string `json:"reason,omitempty"`
}

func NewAPIHandler(
	legacyAPIURL *url.URL,
	upstreamClient *http.Client,
	runtime RuntimeOptions,
) http.Handler {
	runtime = runtime.normalized()
	proxy := httputil.NewSingleHostReverseProxy(legacyAPIURL)
	proxy.Transport = upstreamClient.Transport
	proxy.ErrorHandler = func(writer http.ResponseWriter, _ *http.Request, _ error) {
		writeHealthResponse(
			writer,
			http.StatusBadGateway,
			healthResponse{
				Status:  "upstream_unavailable",
				Service: "lanverse-api",
			},
		)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writeHealthResponse(
			writer,
			http.StatusOK,
			healthResponse{Status: "ok", Service: "lanverse-api"},
		)
	})
	mux.HandleFunc("GET /readyz", readinessHandler(legacyAPIURL, upstreamClient))
	mux.HandleFunc("GET /version", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, runtime.Build)
	})
	mux.Handle("GET /metrics", runtime.Metrics.Handler())
	mux.Handle("/", proxy)
	return runtime.Metrics.Middleware(mux)
}

func readinessHandler(
	legacyAPIURL *url.URL,
	upstreamClient *http.Client,
) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		readinessURL := legacyAPIURL.ResolveReference(&url.URL{Path: "/readyz"})
		upstreamRequest, err := http.NewRequestWithContext(
			request.Context(),
			http.MethodGet,
			readinessURL.String(),
			nil,
		)
		if err != nil {
			writeNotReady(writer, "legacy_runtime_request_invalid")
			return
		}

		response, err := upstreamClient.Do(upstreamRequest)
		if err != nil {
			writeNotReady(writer, "legacy_runtime_unavailable")
			return
		}
		defer response.Body.Close()
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			writeNotReady(writer, "legacy_runtime_not_ready")
			return
		}

		writeHealthResponse(
			writer,
			http.StatusOK,
			healthResponse{Status: "ready", Service: "lanverse-api"},
		)
	}
}

func writeNotReady(writer http.ResponseWriter, reason string) {
	writeHealthResponse(
		writer,
		http.StatusServiceUnavailable,
		healthResponse{
			Status:  "not_ready",
			Service: "lanverse-api",
			Reason:  reason,
		},
	)
}

func writeHealthResponse(writer http.ResponseWriter, status int, response healthResponse) {
	writeJSON(writer, status, response)
}

func writeJSON(writer http.ResponseWriter, status int, response any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(response)
}
