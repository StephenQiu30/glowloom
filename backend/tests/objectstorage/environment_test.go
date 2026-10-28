package objectstorage_test

import (
	"errors"
	"fmt"
	"hash/crc64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func TestUniformEnvironmentUsesNativeTOSProtocolAndNoImplicitLegacy(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !strings.HasPrefix(r.Header.Get("Authorization"), "TOS4-HMAC-SHA256 ") || r.Header.Get("X-Amz-Date") != "" {
			t.Error("configured TOS used a non-native S3 protocol")
		}
		w.Header().Set("ETag", `"fixture"`)
		w.Header().Set("X-Tos-Hash-Crc64ecma", fmt.Sprint(crc64.Checksum([]byte("data"), crc64.MakeTable(crc64.ECMA))))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		if r.Method == http.MethodPut {
			if r.Header.Get("X-Tos-Forbid-Overwrite") == "" || r.Header.Get("X-Tos-Meta-Sha256") != providerSHA {
				t.Error("native immutable output metadata missing")
			}
			raw, _ := io.ReadAll(r.Body)
			if string(raw) != "data" {
				t.Error("native body changed")
			}
			return
		}
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, "data")
			return
		}
		w.Header().Set("Content-Length", "4")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Tos-Meta-Sha256", providerSHA)
	}))
	defer s.Close()
	t.Setenv("GLOWLOOM_ENV_FILE", "")
	t.Setenv("GLOWLOOM_ENV", "local")
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_CONFIG", "")
	for key, value := range map[string]string{"PROVIDER": "tos", "ENDPOINT": s.URL, "BUCKET": "fixture-bucket", "REGION": "cn-beijing", "ACCESS_KEY": "fixture-access", "SECRET_KEY": "fixture-secret", "PROFILE_ID": "tos-main", "REVISION": "native-env-v1"} {
		t.Setenv("GLOWLOOM_OBJECT_STORAGE_"+key, value)
	}
	for _, key := range []string{"PROVIDER", "ENDPOINT", "BUCKET", "REGION", "ACCESS_KEY", "SECRET_KEY", "PROFILE_ID"} {
		t.Setenv("GLOWLOOM_OBJECT_STORAGE_LEGACY_"+key, "")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	r, err := objectstorage.OpenProfiles(*cfg.ObjectStorage)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if calls != 0 {
		t.Fatal("configuration construction contacted storage")
	}
	ref := objectstorage.Ref{ProfileID: "tos-main", Bucket: "fixture-bucket", Key: "personal/org/actor/audio/voice.mp3"}
	if err := r.PutIfAbsent(t.Context(), ref, strings.NewReader("data"), 4, "text/plain", providerSHA); err != nil {
		t.Fatal(err)
	}
	info, err := r.Stat(t.Context(), ref)
	if err != nil || info.Size != 4 || info.SHA256 != providerSHA {
		t.Fatal("TOS exact profile metadata", err)
	}
	body, err := r.Get(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || string(raw) != "data" {
		t.Fatal("TOS actual bytes", err)
	}
	if legacy, err := objectstorage.OpenLegacy(cfg); err != nil || legacy != nil {
		t.Fatal("new profile became an old-key reader", err)
	}
}
func TestNilClientInterfacesRejectAllObjectIO(t *testing.T) {
	var c *objectstorage.Client
	ctx := t.Context()
	for _, err := range []error{c.Ping(ctx), c.Put(ctx, "a", strings.NewReader("a"), 1, "text/plain"), c.PutIfAbsent(ctx, "a", strings.NewReader("a"), 1, "text/plain", providerSHA), c.Remove(ctx, "a")} {
		if !errors.Is(err, objectstorage.ErrUnavailable) {
			t.Fatal("nil object dependency not unavailable", err)
		}
	}
	if _, err := c.Get(ctx, "a"); !errors.Is(err, objectstorage.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := c.Stat(ctx, "a"); !errors.Is(err, objectstorage.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := c.PresignGet(ctx, "a", time.Minute); !errors.Is(err, objectstorage.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := c.Exists(ctx, "a"); !errors.Is(err, objectstorage.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := c.ListMetadata(ctx, "project/id/image/", 10); !errors.Is(err, objectstorage.ErrUnavailable) {
		t.Fatal(err)
	}
}
