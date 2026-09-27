package objectstorage_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func TestOpenRejectsMissingAndInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		bucket   string
		access   string
		secret   string
		want     error
	}{
		{name: "missing endpoint", bucket: "lanverse-local", access: "access", secret: "secret", want: objectstorage.ErrEndpointRequired},
		{name: "missing bucket", endpoint: "http://127.0.0.1:9000", access: "access", secret: "secret", want: objectstorage.ErrBucketRequired},
		{name: "missing access key", endpoint: "http://127.0.0.1:9000", bucket: "lanverse-local", secret: "secret", want: objectstorage.ErrCredentialsRequired},
		{name: "missing secret key", endpoint: "http://127.0.0.1:9000", bucket: "lanverse-local", access: "access", want: objectstorage.ErrCredentialsRequired},
		{name: "unsupported scheme", endpoint: "ftp://127.0.0.1:9000", bucket: "lanverse-local", access: "access", secret: "secret", want: objectstorage.ErrInvalidEndpoint},
		{name: "credentials in URL", endpoint: "http://user:password@127.0.0.1:9000", bucket: "lanverse-local", access: "access", secret: "secret", want: objectstorage.ErrInvalidEndpoint},
		{name: "endpoint path", endpoint: "http://127.0.0.1:9000/storage", bucket: "lanverse-local", access: "access", secret: "secret", want: objectstorage.ErrInvalidEndpoint},
		{name: "invalid bucket", endpoint: "http://127.0.0.1:9000", bucket: "BAD_NAME", access: "access", secret: "secret", want: objectstorage.ErrInvalidBucket},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := objectstorage.Open(tt.endpoint, tt.bucket, tt.access, tt.secret, "")
			if !errors.Is(err, tt.want) {
				t.Fatalf("Open() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestPingChecksConfiguredBucketWithoutWriting(t *testing.T) {
	var mu sync.Mutex
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Method)
		mu.Unlock()
		if r.Method != http.MethodHead || r.URL.Path != "/lanverse-local/" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	conn, err := objectstorage.Open(server.URL, "lanverse-local", "access", "secret", "us-east-1")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(methods) != 1 || methods[0] != http.MethodHead {
		t.Fatalf("methods = %v, want one HEAD", methods)
	}
}

func TestPingFailsWhenBucketIsAbsent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	conn, err := objectstorage.Open(server.URL, "lanverse-local", "access", "secret", "us-east-1")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); !errors.Is(err, objectstorage.ErrBucketMissing) {
		t.Fatalf("Ping() error = %v, want ErrBucketMissing", err)
	}
}
