package identity_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/tests/storagefixture"
)

// reviewStorage preserves the controlled rendering/revocation points while
// exercising the production registry and real PostgreSQL write-location ledger.
func reviewStorage(t *testing.T, tx *gorm.DB, objects *reviewObjects) (*objectstorage.Registry, *pgmedia.StorageRoutes) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/identity-fixture/")
		w.Header().Set("ETag", `"review-fixture"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		switch r.Method {
		case http.MethodPut:
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read fixture body", 400)
				return
			}
			// Native SDK streaming framing surrounds these known controlled bytes.
			var content []byte
			for _, candidate := range []string{"preview-one", "preview-two", "output", "pcm"} {
				if bytes.Contains(raw, []byte(candidate)) {
					content = []byte(candidate)
					break
				}
			}
			if content == nil {
				http.Error(w, "unexpected fixture output", 400)
				return
			}
			if err := objects.PutIfAbsent(r.Context(), key, bytes.NewReader(content), int64(len(content)), r.Header.Get("Content-Type"), r.Header.Get("X-Amz-Meta-Sha256")); err != nil {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
		case http.MethodHead, http.MethodGet:
			info, err := objects.Stat(r.Context(), key)
			if err != nil {
				http.Error(w, `<Error><Code>NoSuchKey</Code></Error>`, 404)
				return
			}
			w.Header().Set("Content-Type", info.ContentType)
			w.Header().Set("Content-Length", fmt.Sprint(info.Size))
			w.Header().Set("X-Amz-Meta-Sha256", info.SHA256)
			if r.Method == http.MethodGet {
				body, err := objects.Get(r.Context(), key)
				if err != nil {
					http.Error(w, "fixture read", 500)
					return
				}
				defer func() { _ = body.Close() }()
				_, _ = io.Copy(w, body)
			}
		default:
			http.Error(w, "unsupported fixture method", 400)
		}
	}))
	t.Cleanup(server.Close)
	cfg := storagefixture.Configuration(t, server.URL, "identity-fixture")
	profile := cfg.Profiles[cfg.DefaultWriteProfile]
	delete(cfg.Profiles, cfg.DefaultWriteProfile)
	cfg.DefaultWriteProfile = "identity-fixture"
	cfg.Profiles[cfg.DefaultWriteProfile] = profile
	registry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	routes := pgmedia.NewStorageRoutes(tx)
	if err := routes.RegisterProfiles(t.Context(), []media.StorageProfile{{ID: "identity-fixture", Provider: "minio", Endpoint: server.URL, SigningEndpoint: server.URL, Region: profile.Region, Bucket: profile.Bucket, AddressingStyle: "path"}}); err != nil {
		t.Fatal("register exact review SDK fixture profile", err)
	}
	seedIdentityStorageRoute(t, tx)
	return registry, routes
}
