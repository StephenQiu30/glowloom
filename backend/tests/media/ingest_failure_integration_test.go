package media_test

import (
	"bytes"
	"crypto/x509"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.temporal.io/sdk/temporal"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func TestMediaIngestRejectsPermanentProviderResults(t *testing.T) {
	database := mediaStoreDB(t)
	_, projectID := mediaStoreProject(t, database)
	var body bytes.Buffer
	if err := png.Encode(&body, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body.Bytes())
	}))
	t.Cleanup(server.Close)
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	// Neither permanent failure is allowed to reach object storage.
	objects, err := objectstorage.Open("http://127.0.0.1:1", "test-media", "test-key", "test-secret", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		kind     string
		maxBytes int64
	}{
		{"oversize", "image", 12},
		{"wrong_output_type", "video", 1024},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opID := seedIngestOperation(t, database, projectID, tc.kind)
			activities, err := mediaflow.NewActivities(database, objects, mediaflow.DownloadPolicy{
				AllowedOrigins: []string{server.URL}, AllowTestLoopbackTLS: true,
				TLSRootCAs: pool, MaxBytes: tc.maxBytes,
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = activities.Ingest(t.Context(), mediaflow.IngestInput{
				OperationID: opID.String(), SeqNo: 1, URL: server.URL + "/result",
			})
			var failure *temporal.ApplicationError
			if !errors.As(err, &failure) || !failure.NonRetryable() || failure.Type() != "provider_result_invalid" {
				t.Fatalf("permanent provider result must stop retrying: %v", err)
			}
		})
	}
}
