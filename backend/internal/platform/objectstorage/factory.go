package objectstorage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// Storage defines the unified strategy interface for object storage operations.
// Business callers depend strictly on this interface, allowing the underlying
// storage provider (Tencent Cloud COS, MinIO, etc.) to be switched seamlessly
// via environment variables without affecting business logic.
type Storage interface {
	// Ping checks connectivity and read access to the target bucket.
	Ping(ctx context.Context) error

	// Put writes a complete object with its detected content type.
	Put(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error

	// PutIfAbsent conditionally writes an object if it does not already exist,
	// recording SHA-256 metadata for content verification.
	PutIfAbsent(ctx context.Context, key string, reader io.Reader, size int64, contentType, sha256 string) error

	// Stat retrieves object metadata (size, content type, SHA-256 digest).
	Stat(ctx context.Context, key string) (ObjectInfo, error)

	// Get opens the object for reading. The caller must close the returned body.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Remove deletes an object by its exact key.
	Remove(ctx context.Context, key string) error

	// PresignGet generates a temporary signed GET URL for the specified key with the given TTL.
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)

	// Close releases any idle connections and background resources.
	Close()
}

// Compile-time check that *Client implements Storage.
var _ Storage = (*Client)(nil)

// Options contains the parameters required to initialize a storage strategy.
type Options struct {
	Provider  string // "cos" for Tencent Cloud COS, "minio" for MinIO / S3 compatible
	Endpoint  string // Service endpoint or bucket URL
	Bucket    string // Target bucket name
	Region    string // Provider region (e.g. ap-guangzhou, us-east-1)
	AccessKey string // Access key or Tencent Cloud SecretId
	SecretKey string // Secret key or Tencent Cloud SecretKey
}

// NewStorage is a factory method that creates a concrete Storage strategy based on the given options.
// Following the Strategy Pattern, simply changing Options.Provider selects the corresponding
// storage implementation without modifying any business logic.
func NewStorage(opts Options) (*Client, error) {
	provider := strings.ToLower(strings.TrimSpace(opts.Provider))
	if provider == "" {
		provider = "minio"
	}

	switch provider {
	case "cos":
		return newCOSStrategy(opts)
	case "minio":
		return newMinIOStrategy(opts)
	case "tos":
		return newTOSStrategy(opts)
	default:
		return nil, fmt.Errorf("%w: unsupported object storage provider: %s", ErrUnavailable, opts.Provider)
	}
}

func newCOSStrategy(opts Options) (*Client, error) {
	profile := config.StorageProfile{
		Provider: "cos",
		Mode:     "read_write",
		Endpoint: opts.Endpoint,
		Bucket:   opts.Bucket,
		Region:   opts.Region,
	}
	credentials := config.StorageCredentials{
		AccessKey: opts.AccessKey,
		SecretKey: opts.SecretKey,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	native, err := newCOS(profile, credentials, transport)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, fmt.Errorf("create COS storage strategy: %w", err)
	}
	return &Client{native: native, bucket: opts.Bucket, transport: transport}, nil
}

func newMinIOStrategy(opts Options) (*Client, error) {
	client, err := Open(opts.Endpoint, opts.Bucket, opts.AccessKey, opts.SecretKey, opts.Region)
	if err != nil {
		return nil, fmt.Errorf("create MinIO storage strategy: %w", err)
	}
	return client, nil
}

func newTOSStrategy(opts Options) (*Client, error) {
	profile := config.StorageProfile{
		Provider: "tos",
		Mode:     "read_write",
		Endpoint: opts.Endpoint,
		Bucket:   opts.Bucket,
		Region:   opts.Region,
	}
	credentials := config.StorageCredentials{
		AccessKey: opts.AccessKey,
		SecretKey: opts.SecretKey,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	native, err := newTOS(profile, credentials, transport)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, fmt.Errorf("create TOS storage strategy: %w", err)
	}
	return &Client{native: native, bucket: opts.Bucket, transport: transport}, nil
}
