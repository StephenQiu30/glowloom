// Package objectstorage owns the Tencent Cloud COS client used by backend roles.
package objectstorage

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

var (
	// ErrUnavailable rejects absent object storage dependencies without panicking.
	ErrUnavailable = errors.New("object storage unavailable")
	// ErrEndpointRequired means no object storage endpoint is configured.
	ErrEndpointRequired = errors.New("COS bucket origin is required")
	// ErrInvalidEndpoint means the origin does not match the configured COS bucket.
	ErrInvalidEndpoint = errors.New("invalid COS bucket origin")
	// ErrBucketRequired means no target bucket is configured.
	ErrBucketRequired = errors.New("COS bucket name is required")
	// ErrInvalidBucket means the configured bucket name is not accepted by COS.
	ErrInvalidBucket = errors.New("invalid COS bucket name")
	// ErrCredentialsRequired means the client has no access or secret key.
	ErrCredentialsRequired = errors.New("COS SecretId and SecretKey are required")
	// ErrBucketMissing means the configured target bucket does not exist.
	ErrBucketMissing = errors.New("configured object storage bucket does not exist")
	// ErrInvalidObject means an object write or stat lacks a safe key or valid metadata.
	ErrInvalidObject = errors.New("invalid object storage object")
	// ErrObjectExists means conditional creation found an object at the key.
	ErrObjectExists = errors.New("object storage key already exists")
)

// Client provides access to the configured private object storage bucket.
type Client struct {
	native    cosBackend
	transport *http.Transport
}

// ObjectInfo contains only the metadata needed to verify a stored result.
type ObjectInfo struct {
	Size        int64
	ContentType string
	SHA256      string
}

var bucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

// Open validates configuration and creates a client without making a network request.
func Open(endpoint, bucket, accessKey, secretKey, region string) (*Client, error) {
	endpoint = strings.TrimSpace(endpoint)
	bucket = strings.TrimSpace(bucket)
	if endpoint == "" {
		return nil, ErrEndpointRequired
	}
	if bucket == "" {
		return nil, ErrBucketRequired
	}
	if !bucketName.MatchString(bucket) || strings.Contains(bucket, "..") || net.ParseIP(bucket) != nil {
		return nil, ErrInvalidBucket
	}
	if strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return nil, ErrCredentialsRequired
	}
	if err := config.ValidateCOSOrigin(endpoint, bucket, strings.TrimSpace(region), true); err != nil {
		return nil, ErrInvalidEndpoint
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	native, err := newCOS(config.StorageProfile{Endpoint: endpoint}, config.StorageCredentials{AccessKey: accessKey, SecretKey: secretKey}, transport)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	return &Client{native: native, transport: transport}, nil
}

// Ping checks read-only access to the configured bucket.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.native.sdk == nil {
		return ErrUnavailable
	}
	return storageError("health_check", "direct", c.native.ping(ctx))
}

// Put writes a complete object with its detected content type. Callers own
// bounded reading and may retry the same key after an interrupted upload.
func (c *Client) Put(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	if c == nil || c.native.sdk == nil {
		return ErrUnavailable
	}
	if !validKey(key) || reader == nil || size < 0 || strings.TrimSpace(contentType) == "" {
		return ErrInvalidObject
	}
	return storageError("put", "storage", c.native.put(ctx, key, reader, size, contentType, ""))
}

// PutIfAbsent creates an immutable result object and records the checked SHA.
// A retry must inspect the existing object's metadata before reusing it.
func (c *Client) PutIfAbsent(ctx context.Context, key string, reader io.Reader, size int64, contentType, sha256 string) error {
	if c == nil || c.native.sdk == nil {
		return ErrUnavailable
	}
	if !validKey(key) || reader == nil || size < 0 ||
		strings.TrimSpace(contentType) == "" || len(sha256) != 64 {
		return ErrInvalidObject
	}
	return storageError("put", "storage", c.native.put(ctx, key, reader, size, contentType, sha256))
}

// Stat reports the persisted object's size and content type.
func (c *Client) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	if c == nil || c.native.sdk == nil {
		return ObjectInfo{}, ErrUnavailable
	}
	if !validKey(key) {
		return ObjectInfo{}, ErrInvalidObject
	}
	info, err := c.native.stat(ctx, key)
	return info, storageError("stat", "direct", err)
}

// Get opens exact private bytes for a server-side owner. Callers must bound
// reads, verify their expected metadata/digest, and close the returned object.
func (c *Client) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if c == nil || c.native.sdk == nil {
		return nil, ErrUnavailable
	}
	if !validKey(key) {
		return nil, ErrInvalidObject
	}
	body, err := c.native.get(ctx, key)
	if err != nil {
		return nil, storageError("get", "direct", err)
	}
	return &checkedReader{ReadCloser: body, profile: "direct"}, nil
}

// Remove deletes one exact object key. Callers must first establish ownership
// of that key, for example when cleaning up a test-created or purged asset.
func (c *Client) Remove(ctx context.Context, key string) error {
	if c == nil || c.native.sdk == nil {
		return ErrUnavailable
	}
	if !validKey(key) {
		return ErrInvalidObject
	}
	return storageError("remove", "storage", c.native.remove(ctx, key))
}

// PresignGet grants bounded access to one safe, server-selected object key.
func (c *Client) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if c == nil || c.native.sdk == nil {
		return "", ErrUnavailable
	}
	if !validKey(key) || ttl < time.Second || ttl > 15*time.Minute {
		return "", ErrInvalidObject
	}
	grant, err := c.native.sign(ctx, key, ttl)
	return grant, storageError("sign", "direct", err)
}

func validKey(key string) bool {
	if key == "" || strings.HasPrefix(key, "/") || strings.HasSuffix(key, "/") || strings.Contains(key, "\\") {
		return false
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

// Close releases only the native provider connection owned by this client.
func (c *Client) Close() {
	if c != nil && c.transport != nil {
		c.transport.CloseIdleConnections()
	}
}
