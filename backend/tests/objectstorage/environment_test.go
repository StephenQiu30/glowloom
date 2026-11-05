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

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func TestUniformEnvironmentUsesNativeCOSProtocolAndNoImplicitLegacy(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !strings.HasPrefix(r.Header.Get("Authorization"), "q-sign-algorithm=sha1&") || r.Header.Get("X-Amz-Date") != "" {
			t.Error("configured SDK did not use the COS protocol")
		}
		w.Header().Set("ETag", `"fixture"`)
		w.Header().Set("X-Cos-Hash-Crc64ecma", fmt.Sprint(crc64.Checksum([]byte("data"), crc64.MakeTable(crc64.ECMA))))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		if r.Method == http.MethodPut {
			if r.Header.Get("X-Cos-Forbid-Overwrite") == "" || r.Header.Get("X-Cos-Meta-Sha256") != providerSHA {
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
		w.Header().Set("X-Cos-Meta-Sha256", providerSHA)
	}))
	defer s.Close()
	r := profileRegistry(t, s.URL)
	defer r.Close()
	if calls != 0 {
		t.Fatal("configuration construction contacted storage")
	}
	ref := objectstorage.Ref{ProfileID: "cos-main", Bucket: "fixture-bucket", Key: "personal/org/actor/audio/voice.mp3"}
	if err := r.PutIfAbsent(t.Context(), ref, strings.NewReader("data"), 4, "text/plain", providerSHA); err != nil {
		t.Fatal(err)
	}
	info, err := r.Stat(t.Context(), ref)
	if err != nil || info.Size != 4 || info.SHA256 != providerSHA {
		t.Fatal("COS exact profile metadata", err)
	}
	body, err := r.Get(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || string(raw) != "data" {
		t.Fatal("COS fixture bytes", err)
	}
}

func TestNativeCOSRoutesStayExplicit(t *testing.T) {
	r := profileRegistry(t, "http://127.0.0.1:9000")
	ref := objectstorage.Ref{ProfileID: "cos-main", Bucket: "fixture-bucket", Key: "personal/org/actor/audio/voice.mp3"}
	if err := r.CheckWrite(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	ref.ProfileID = "unconfigured-cos"
	if err := r.CheckWrite(t.Context(), ref); !errors.Is(err, objectstorage.ErrInvalidObject) {
		t.Fatal("registry inferred a fallback route", err)
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
