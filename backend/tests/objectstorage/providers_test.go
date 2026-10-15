package objectstorage_test

import (
	"context"
	"errors"
	"fmt"
	"hash/crc64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

const providerSHA = "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"

func profileRegistry(t *testing.T, provider, endpoint string) *objectstorage.Registry {
	t.Helper()
	return registryForResource(t, provider, endpoint, "fixture-bucket", "us-east-1")
}

func registryForResource(t *testing.T, provider, endpoint, bucket, region string) *objectstorage.Registry {
	t.Helper()
	t.Setenv("TEST_PROVIDER_AK", "fixture-access")
	t.Setenv("TEST_PROVIDER_SK", "fixture-secret")
	t.Setenv("TEST_PROVIDER_ST", "")
	t.Setenv("TEST_PROVIDER_EXPIRY", "")
	file := filepath.Join(t.TempDir(), "storage.yaml")
	raw := fmt.Sprintf("object_storage:\n  schema_version: 1\n  revision: test-v1\n  profiles:\n    minio-main:\n      provider: %s\n      endpoint: %s\n      bucket: fixture-bucket\n      region: us-east-1\n      credentials:\n        source: env\n        access_key_env: TEST_PROVIDER_AK\n        secret_key_env: TEST_PROVIDER_SK\n", provider, endpoint)
	raw = strings.Replace(raw, "bucket: fixture-bucket", "bucket: "+bucket, 1)
	raw = strings.Replace(raw, "region: us-east-1", "region: "+region, 1)
	raw += "        session_token_env: TEST_PROVIDER_ST\n        expires_at_env: TEST_PROVIDER_EXPIRY\n"
	if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadObjectStorage(file, "local")
	if err != nil {
		t.Fatal(err)
	}
	r, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r
}

func TestProvidersOfficialSDKContract(t *testing.T) {
	for _, provider := range []string{"minio", "cos", "tos"} {
		t.Run(provider, func(t *testing.T) {
			meta := map[string]string{"minio": "X-Amz-Meta-Sha256", "cos": "X-Cos-Meta-Sha256", "tos": "X-Tos-Meta-Sha256"}[provider]
			condition := map[string]string{"minio": "If-None-Match", "cos": "X-Cos-Forbid-Overwrite", "tos": "X-Tos-Forbid-Overwrite"}[provider]
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") == "" {
					t.Error("official SDK did not sign request")
				}
				w.Header().Set("ETag", `"not-a-digest"`)
				w.Header().Set("X-Amz-Bucket-Region", "us-east-1")
				w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
				w.Header().Set("X-Cos-Hash-Crc64ecma", fmt.Sprint(crc64.Checksum([]byte("data"), crc64.MakeTable(crc64.ECMA))))
				w.Header().Set("X-Tos-Hash-Crc64ecma", fmt.Sprint(crc64.Checksum([]byte("data"), crc64.MakeTable(crc64.ECMA))))
				switch r.Method {
				case http.MethodPut:
					body, _ := io.ReadAll(r.Body)
					if (r.Header.Get("X-Amz-Decoded-Content-Length") != "4" || !strings.Contains(string(body), "\r\ndata\r\n")) && (string(body) != "data" || r.ContentLength != 4) {
						t.Errorf("wrong body or length: %d/%d", len(body), r.ContentLength)
					}
					if r.Header.Get(meta) != providerSHA || r.Header.Get("Content-Type") != "text/plain" || r.Header.Get(condition) == "" {
						t.Error("immutable metadata/condition missing")
					}
				case http.MethodHead:
					w.Header().Set("Content-Length", "4")
					w.Header().Set("Content-Type", "text/plain")
					w.Header().Set(meta, providerSHA)
					w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
				case http.MethodGet:
					_, _ = io.WriteString(w, "data")
				case http.MethodDelete:
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected method %s", r.Method)
				}
			}))
			t.Cleanup(server.Close)
			registry := profileRegistry(t, provider, server.URL)
			ref := objectstorage.Ref{ProfileID: "minio-main", Bucket: "fixture-bucket", Key: "project/id/image/sample.txt"}
			if err := registry.PutIfAbsent(t.Context(), ref, strings.NewReader("data"), 4, "text/plain", providerSHA); err != nil {
				t.Fatal(fixtureError(err))
			}
			info, err := registry.Stat(t.Context(), ref)
			if err != nil || info.Size != 4 || info.ContentType != "text/plain" || info.SHA256 != providerSHA {
				t.Fatalf("Stat = %+v, %v", info, err)
			}
			body, err := registry.Get(t.Context(), ref)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(body)
			_ = body.Close()
			if err != nil || string(data) != "data" {
				t.Fatalf("Get = %q, %v", data, err)
			}
			grant, err := registry.PresignGet(t.Context(), ref, time.Minute)
			if err != nil || grant.URL == "" || time.Until(grant.ExpiresAt) < 55*time.Second {
				t.Fatalf("presign missing bounded grant: %v", err)
			}
			if err := registry.Remove(t.Context(), ref); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProvidersRejectWrongProfileBucket(t *testing.T) {
	r := profileRegistry(t, "minio", "https://unused.example.test")
	for _, ref := range []objectstorage.Ref{{ProfileID: "unknown", Bucket: "fixture-bucket", Key: "key"}, {ProfileID: "minio-main", Bucket: "other-bucket", Key: "key"}, {ProfileID: "minio-main", Bucket: "fixture-bucket", Key: "a/../b"}, {ProfileID: "minio-main", Bucket: "fixture-bucket", Key: "a\x00b"}} {
		if _, err := r.Stat(t.Context(), ref); !errors.Is(err, objectstorage.ErrInvalidObject) {
			t.Fatalf("unsafe ref accepted: %v", err)
		}
	}
}

func TestProviderErrorMappingDoesNotTreat403AsMissing(t *testing.T) {
	for _, provider := range []string{"minio", "cos", "tos"} {
		t.Run(provider, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
			t.Cleanup(s.Close)
			r := profileRegistry(t, provider, s.URL)
			ref := objectstorage.Ref{ProfileID: "minio-main", Bucket: "fixture-bucket", Key: "private/key"}
			_, err := r.Stat(t.Context(), ref)
			if !errors.Is(err, objectstorage.ErrAccessDenied) || errors.Is(err, objectstorage.ErrObjectMissing) {
				t.Fatalf("403 classification = %v", err)
			}
			if strings.Contains(fmt.Sprintf("%+v", err), "fixture-secret") || strings.Contains(err.Error(), s.URL) {
				t.Fatal("SDK diagnostic leaked access data")
			}
		})
	}
}

func TestProviderGetMapsDeferredReadErrors(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `<Error><Code>NoSuchKey</Code></Error>`)
	}))
	t.Cleanup(s.Close)
	r := profileRegistry(t, "minio", s.URL)
	ref := objectstorage.Ref{ProfileID: "minio-main", Bucket: "fixture-bucket", Key: "private/key"}
	b, err := r.Get(t.Context(), ref)
	if err == nil {
		defer func() { _ = b.Close() }()
		_, err = io.ReadAll(b)
	}
	if !errors.Is(err, objectstorage.ErrObjectMissing) {
		t.Fatalf("deferred missing = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.Get(ctx, ref); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
}

func TestProviderZeroLengthControlObject(t *testing.T) {
	for _, provider := range []string{"minio", "cos", "tos"} {
		t.Run(provider, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, _ := io.ReadAll(r.Body)
				if len(data) != 0 && r.Header.Get("X-Amz-Decoded-Content-Length") != "0" {
					t.Errorf("length = %d", r.ContentLength)
				}
				w.Header().Set("ETag", `"empty"`)
			}))
			t.Cleanup(s.Close)
			r := profileRegistry(t, provider, s.URL)
			if err := r.PutIfAbsent(t.Context(), objectstorage.Ref{ProfileID: "minio-main", Bucket: "fixture-bucket", Key: "intent/control"}, strings.NewReader(""), 0, "application/octet-stream", strings.Repeat("0", 64)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Only used against local fixtures; never log an unwrapped provider error in a service.
func fixtureError(err error) error {
	if e, ok := err.(interface{ Unwrap() []error }); ok {
		children := e.Unwrap()
		if len(children) > 1 {
			return children[len(children)-1]
		}
	}
	return err
}
