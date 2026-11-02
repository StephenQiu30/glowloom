package objectstorage_test

import (
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

func TestStorageStrategyPatternCOSAndMinIO(t *testing.T) {
	cosCalled := false
	sCOS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cosCalled = true
		if !strings.HasPrefix(r.Header.Get("Authorization"), "q-sign-algorithm=sha1") {
			t.Errorf("expected COS signature algorithm, got: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("ETag", `"fixture"`)
		w.Header().Set("X-Cos-Hash-Crc64ecma", fmt.Sprint(crc64.Checksum([]byte("cos-data"), crc64.MakeTable(crc64.ECMA))))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("Content-Length", "8")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Cos-Meta-Sha256", providerSHA)

		switch r.Method {
		case http.MethodPut:
			if r.Header.Get("X-Cos-Forbid-Overwrite") != "true" {
				t.Error("expected X-Cos-Forbid-Overwrite on PutIfAbsent")
			}
			raw, _ := io.ReadAll(r.Body)
			if string(raw) != "cos-data" {
				t.Errorf("expected cos-data, got %s", string(raw))
			}
		case http.MethodGet:
			_, _ = io.WriteString(w, "cos-data")
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer sCOS.Close()

	minioCalled := false
	sMinIO := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		minioCalled = true
		w.Header().Set("ETag", `"fixture"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("Content-Length", "10")
		w.Header().Set("Content-Type", "text/plain")

		switch r.Method {
		case http.MethodPut:
			raw, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(raw), "minio-data") {
				t.Errorf("expected minio-data in payload, got %s", string(raw))
			}
		case http.MethodGet:
			_, _ = io.WriteString(w, "minio-data")
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer sMinIO.Close()

	// 1. Verify COS Strategy via unified Storage interface
	var cosStorage objectstorage.Storage
	clientCOS, err := objectstorage.NewStorage(objectstorage.Options{
		Provider:  "cos",
		Endpoint:  sCOS.URL,
		Bucket:    "fixture-bucket",
		Region:    "ap-guangzhou",
		AccessKey: "fixture-cos-ak",
		SecretKey: "fixture-cos-sk",
	})
	if err != nil {
		t.Fatalf("failed to create COS storage: %v", err)
	}
	cosStorage = clientCOS
	defer cosStorage.Close()

	if err := cosStorage.PutIfAbsent(t.Context(), "file.txt", strings.NewReader("cos-data"), 8, "text/plain", providerSHA); err != nil {
		t.Fatalf("COS PutIfAbsent failed: %v", err)
	}
	cosBody, err := cosStorage.Get(t.Context(), "file.txt")
	if err != nil {
		t.Fatalf("COS Get failed: %v", err)
	}
	cosBytes, err := io.ReadAll(cosBody)
	_ = cosBody.Close()
	if err != nil || string(cosBytes) != "cos-data" {
		t.Fatalf("expected cos-data, got %q, err: %v", cosBytes, err)
	}
	if !cosCalled {
		t.Fatal("expected COS server to be called")
	}

	// 2. Verify MinIO Strategy via unified Storage interface
	var minioStorage objectstorage.Storage
	clientMinIO, err := objectstorage.NewStorage(objectstorage.Options{
		Provider:  "minio",
		Endpoint:  sMinIO.URL,
		Bucket:    "fixture-bucket",
		Region:    "us-east-1",
		AccessKey: "fixture-minio-ak",
		SecretKey: "fixture-minio-sk",
	})
	if err != nil {
		t.Fatalf("failed to create MinIO storage: %v", err)
	}
	minioStorage = clientMinIO
	defer minioStorage.Close()

	if err := minioStorage.PutIfAbsent(t.Context(), "file.txt", strings.NewReader("minio-data"), 10, "text/plain", providerSHA); err != nil {
		t.Fatalf("MinIO PutIfAbsent failed: %v", err)
	}
	minioBody, err := minioStorage.Get(t.Context(), "file.txt")
	if err != nil {
		t.Fatalf("MinIO Get failed: %v", err)
	}
	minioBytes, err := io.ReadAll(minioBody)
	_ = minioBody.Close()
	if err != nil || string(minioBytes) != "minio-data" {
		t.Fatalf("expected minio-data, got %q, err: %v", minioBytes, err)
	}
	if !minioCalled {
		t.Fatal("expected MinIO server to be called")
	}
}

func TestOpenFromConfigSwitchesStrategyWithoutBusinessChanges(t *testing.T) {
	// 1. When GLOWLOOM_OBJECT_STORAGE_PROVIDER is "cos"
	cfgCOS := config.Config{
		ObjectStorageProvider:  "cos",
		ObjectStorageEndpoint:  "http://127.0.0.1:9000",
		ObjectStorageBucket:    "fixture-bucket",
		ObjectStorageRegion:    "ap-guangzhou",
		ObjectStorageAccessKey: "test-ak",
		ObjectStorageSecretKey: "test-sk",
	}
	cosClient, err := objectstorage.OpenFromConfig(cfgCOS)
	if err != nil {
		t.Fatalf("expected COS client creation to succeed: %v", err)
	}
	if cosClient == nil {
		t.Fatal("expected non-nil COS client")
	}
	cosClient.Close()

	// 2. When GLOWLOOM_OBJECT_STORAGE_PROVIDER is "minio"
	cfgMinIO := config.Config{
		ObjectStorageProvider:  "minio",
		ObjectStorageEndpoint:  "http://127.0.0.1:9000",
		ObjectStorageBucket:    "fixture-bucket",
		ObjectStorageRegion:    "us-east-1",
		ObjectStorageAccessKey: "test-ak",
		ObjectStorageSecretKey: "test-sk",
	}
	minioClient, err := objectstorage.OpenFromConfig(cfgMinIO)
	if err != nil {
		t.Fatalf("expected MinIO client creation to succeed: %v", err)
	}
	if minioClient == nil {
		t.Fatal("expected non-nil MinIO client")
	}
	minioClient.Close()

	// 3. When storage configuration is entirely empty
	cfgEmpty := config.Config{}
	emptyClient, err := objectstorage.OpenFromConfig(cfgEmpty)
	if err != nil {
		t.Fatalf("expected empty config to not return error: %v", err)
	}
	if emptyClient != nil {
		t.Fatal("expected nil client for unconfigured storage")
	}
}
