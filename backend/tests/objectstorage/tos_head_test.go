package objectstorage_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func TestTOSHeadRequiresObjectAbsenceEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		ec     string
		want   error
	}{{"unclassified 404", 404, "", objectstorage.ErrOutcomeUnknown}, {"confirmed missing", 404, "0017-00000003", objectstorage.ErrObjectMissing}, {"denied", 403, "", objectstorage.ErrAccessDenied}, {"unavailable", 503, "", objectstorage.ErrOutcomeUnknown}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "HEAD" {
					t.Error("must remain HEAD")
				}
				if tc.ec != "" {
					w.Header().Set("x-tos-ec", tc.ec)
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
