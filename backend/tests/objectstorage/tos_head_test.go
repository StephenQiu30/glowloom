package objectstorage_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func TestTOSHeadEmptyErrorBodyUsesOfficialExistenceStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   error
	}{{"missing", 404, objectstorage.ErrObjectMissing}, {"denied", 403, objectstorage.ErrAccessDenied}, {"unavailable", 503, objectstorage.ErrOutcomeUnknown}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "HEAD" {
					t.Error("must remain HEAD")
				}
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(server.Close)
			registry := profileRegistry(t, "tos", server.URL)
			_, err := registry.Stat(t.Context(), objectstorage.Ref{ProfileID: "minio-main", Bucket: "fixture-bucket", Key: "project/id/image/a"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("HEAD %d classification %v", tc.status, err)
			}
		})
	}
}
