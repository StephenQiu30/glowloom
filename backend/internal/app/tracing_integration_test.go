package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

func TestInitializeAPIExportsTraceWithHostServices(t *testing.T) {
	if os.Getenv("LV_TEST_API_OTEL") != "1" {
		t.Skip("set LV_TEST_API_OTEL=1 and LV_ENV_FILE to verify the host services and OTLP export")
	}
	requests := make(chan struct{}, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/traces" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(receiver.Close)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load local configuration: %v", err)
	}
	cfg.OTelEndpoint = receiver.URL
	server, cleanup, err := initializeAPI(t.Context(), cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("initialize API: %v", err)
	}
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			cleanup()
		}
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil)
	server.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("readyz status = %d, want 200", recorder.Code)
	}
	cleanup()
	cleaned = true
	select {
	case <-requests:
	default:
		t.Fatal("API did not export the request trace")
	}
}
