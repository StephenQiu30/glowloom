package objectstorage_test

import (
	"context"
	"errors"
	"fmt"
	"hash/crc64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func TestProviderInventoryRejectsPartialListing(t *testing.T) {
	for _, provider := range []string{"minio", "cos", "tos"} {
		t.Run(provider, func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				if calls > 1 {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				if provider == "tos" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"IsTruncated":true,"NextContinuationToken":"next","Contents":[{"Key":"project/id/image/a","Size":4}]}`)
					return
				}
				w.Header().Set("Content-Type", "application/xml")
				_, _ = io.WriteString(w, `<ListBucketResult><Name>fixture-bucket</Name><IsTruncated>true</IsTruncated><NextMarker>project/id/image/a</NextMarker><NextContinuationToken>next</NextContinuationToken><Contents><Key>project/id/image/a</Key><Size>4</Size><LastModified>2026-10-15T00:00:00Z</LastModified></Contents></ListBucketResult>`)
			}))
			t.Cleanup(s.Close)
			r := profileRegistry(t, provider, s.URL)
			items, err := r.ListMetadata(t.Context(), "minio-main", "fixture-bucket", "project/id/image/", 100)
			if len(items) != 0 || !errors.Is(err, objectstorage.ErrAccessDenied) {
				t.Fatalf("partial listing exposed: %d, %v", len(items), err)
			}
		})
	}
}

func TestProviderConflictAndMissingClassification(t *testing.T) {
	for _, provider := range []string{"minio", "cos", "tos"} {
		t.Run(provider, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				status, code := http.StatusNotFound, "NoSuchKey"
				if r.Method == http.MethodPut {
					status, code = http.StatusPreconditionFailed, "PreconditionFailed"
					if provider != "minio" {
						status, code = http.StatusConflict, "FileAlreadyExists"
					}
				}
				w.Header().Set("Content-Type", "application/xml")
				if provider == "tos" {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(status)
				if provider == "tos" {
					_, _ = fmt.Fprintf(w, `{"Code":%q,"Message":"fixture server error"}`, code)
				} else {
					_, _ = fmt.Fprintf(w, "<Error><Code>%s</Code></Error>", code)
				}
			}))
			t.Cleanup(s.Close)
			r := profileRegistry(t, provider, s.URL)
			ref := objectstorage.Ref{ProfileID: "minio-main", Bucket: "fixture-bucket", Key: "project/id/image/a"}
			if err := r.PutIfAbsent(t.Context(), ref, strings.NewReader("data"), 4, "text/plain", providerSHA); !errors.Is(err, objectstorage.ErrObjectExists) {
				t.Fatalf("conflict = %v", err)
			}
			body, err := r.Get(t.Context(), ref)
			if err == nil {
				_, err = io.ReadAll(body)
				_ = body.Close()
			}
			if !errors.Is(err, objectstorage.ErrObjectMissing) {
				t.Fatalf("missing = %v", err)
			}
		})
	}
}

func TestProvidersRefreshCredentialAndBoundGrant(t *testing.T) {
	r := profileRegistry(t, "cos", "https://127.0.0.1")
	t.Setenv("TEST_PROVIDER_AK", "rotated-access")
	grant, err := r.PresignGet(t.Context(), objectstorage.Ref{ProfileID: "minio-main", Bucket: "fixture-bucket", Key: "project/id/image/a"}, time.Minute)
	if err != nil || !strings.Contains(grant.URL, "rotated-access") {
		t.Fatal("presign did not refresh the explicitly selected credentials", err)
	}
	if _, err := r.PresignGet(t.Context(), objectstorage.Ref{ProfileID: "minio-main", Bucket: "fixture-bucket", Key: "a"}, 16*time.Minute); !errors.Is(err, objectstorage.ErrInvalidObject) {
		t.Fatal("accepted excessive access grant", err)
	}
}

func TestProviderTemporaryGrantCarriesTokenAndCredentialDeadline(t *testing.T) {
	for _, provider := range []string{"minio", "cos", "tos"} {
		t.Run(provider, func(t *testing.T) {
			r := profileRegistry(t, provider, "https://127.0.0.1")
			expires := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)
			t.Setenv("TEST_PROVIDER_ST", "fixture-session-token")
			t.Setenv("TEST_PROVIDER_EXPIRY", expires.Format(time.RFC3339))
			grant, err := r.PresignGet(t.Context(), objectstorage.Ref{ProfileID: "minio-main", Bucket: "fixture-bucket", Key: "project/id/image/a"}, 5*time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(grant.URL)
			if err != nil {
				t.Fatal("invalid SDK grant")
			}
			token := ""
			for key, values := range u.Query() {
				if strings.HasSuffix(strings.ToLower(key), "security-token") {
					token = values[0]
				}
			}
			if token != "fixture-session-token" {
				t.Fatal("SDK grant omitted temporary credential token")
			}
			if grant.ExpiresAt.After(expires.Add(-29*time.Second)) || time.Until(grant.ExpiresAt) < time.Minute {
				t.Fatal("grant exceeds credential lifetime or was not issued")
			}
			if strings.Contains(fmt.Sprintf("%+v", grant), "fixture-session-token") {
				t.Fatal("formatted access grant exposes URL")
			}
			encoded, err := yaml.Marshal(grant)
			if err != nil || strings.Contains(string(encoded), "fixture-session-token") {
				t.Fatal("YAML access grant exposes URL", err)
			}
		})
	}
}

func TestProviderCOSSDKHostAndObjectPathMatchDeclaredIdentity(t *testing.T) {
	const bucket = "fixture-bucket-123"
	const host = bucket + ".cos.ap-shanghai.myqcloud.com"
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host || r.URL.Path != "/projects/id/image/sample.txt" {
			t.Error("SDK addressed a different physical namespace")
		}
		if r.Header.Get("X-Cos-Forbid-Overwrite") != "true" || r.Header.Get("X-Cos-Meta-Sha256") != providerSHA {
			t.Error("native create lost its immutable metadata")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != "data" {
			t.Error("wrong native upload bytes")
		}
		w.Header().Set("X-Cos-Hash-Crc64ecma", fmt.Sprint(crc64.Checksum(body, crc64.MakeTable(crc64.ECMA))))
		w.Header().Set("ETag", `"fixture"`)
	}))
	t.Cleanup(s.Close)
	transport := s.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = "127.0.0.1"
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != host+":443" {
			return nil, fmt.Errorf("fixture rejected unexpected authority")
		}
		return (&net.Dialer{}).DialContext(ctx, network, s.Listener.Addr().String())
	}
	prior := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = prior; transport.CloseIdleConnections() })
	r := registryForResource(t, "cos", "https://"+host, bucket, "ap-shanghai")
	if err := r.PutIfAbsent(t.Context(), objectstorage.Ref{ProfileID: "minio-main", Bucket: bucket, Key: "projects/id/image/sample.txt"}, strings.NewReader("data"), 4, "text/plain", providerSHA); err != nil {
		t.Fatal(err)
	}
}
