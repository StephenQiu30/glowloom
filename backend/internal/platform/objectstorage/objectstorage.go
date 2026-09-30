// Package objectstorage owns the S3-compatible object storage client used by backend roles.
package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/s3utils"
)

var (
	// ErrEndpointRequired means no object storage endpoint is configured.
	ErrEndpointRequired = errors.New("LV_OBJECT_STORAGE_ENDPOINT is required")
	// ErrInvalidEndpoint means the endpoint is not a plain HTTP(S) origin.
	ErrInvalidEndpoint = errors.New("invalid LV_OBJECT_STORAGE_ENDPOINT")
	// ErrBucketRequired means no target bucket is configured.
	ErrBucketRequired = errors.New("LV_OBJECT_STORAGE_BUCKET is required")
	// ErrInvalidBucket means the configured bucket name is not accepted by S3.
	ErrInvalidBucket = errors.New("invalid LV_OBJECT_STORAGE_BUCKET")
	// ErrCredentialsRequired means the client has no access or secret key.
	ErrCredentialsRequired = errors.New("LV_OBJECT_STORAGE_ACCESS_KEY and LV_OBJECT_STORAGE_SECRET_KEY are required")
	// ErrBucketMissing means the configured target bucket does not exist.
	ErrBucketMissing = errors.New("configured object storage bucket does not exist")
	// ErrInvalidObject means an object write or stat lacks a safe key or valid metadata.
	ErrInvalidObject = errors.New("invalid object storage object")
	// ErrObjectExists means conditional creation found an object at the key.
	ErrObjectExists = errors.New("object storage key already exists")
)

// Client provides access to the configured private object storage bucket.
type Client struct {
	sdk    *minio.Client
	bucket string
}

// ObjectInfo contains only the metadata needed to verify a stored result.
type ObjectInfo struct {
	Size        int64
	ContentType string
	SHA256      string
}

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
	if err := s3utils.CheckValidBucketNameStrict(bucket); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidBucket, err)
	}
	if strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return nil, ErrCredentialsRequired
	}
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.ForceQuery {
		return nil, ErrInvalidEndpoint
	}
	client, err := minio.New(u.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: u.Scheme == "https",
		Region: strings.TrimSpace(region),
	})
	if err != nil {
		return nil, fmt.Errorf("create object storage client: %w", err)
	}
	return &Client{sdk: client, bucket: bucket}, nil
}

// Ping checks read-only access to the configured bucket.
func (c *Client) Ping(ctx context.Context) error {
	exists, err := c.sdk.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("check object storage bucket: %w", err)
	}
	if !exists {
		return ErrBucketMissing
	}
	return nil
}

// Put writes a complete object with its detected content type. Callers own
// bounded reading and may retry the same key after an interrupted upload.
func (c *Client) Put(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	if c == nil || c.sdk == nil {
		return ErrInvalidObject
	}
	if !validKey(key) || reader == nil || size < 0 || strings.TrimSpace(contentType) == "" {
		return ErrInvalidObject
	}
	_, err := c.sdk.PutObject(ctx, c.bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("write media object: %w", err)
	}
	return nil
}

// PutIfAbsent creates an immutable result object and records the checked SHA.
// A retry must inspect the existing object's metadata before reusing it.
func (c *Client) PutIfAbsent(ctx context.Context, key string, reader io.Reader, size int64, contentType, sha256 string) error {
	if c == nil || c.sdk == nil || !validKey(key) || reader == nil || size < 0 ||
		strings.TrimSpace(contentType) == "" || len(sha256) != 64 {
		return ErrInvalidObject
	}
	opts := minio.PutObjectOptions{
		ContentType:  contentType,
		UserMetadata: map[string]string{"sha256": sha256},
	}
	opts.SetMatchETagExcept("*")
	_, err := c.sdk.PutObject(ctx, c.bucket, key, reader, size, opts)
	if err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusPreconditionFailed {
			return ErrObjectExists
		}
		return fmt.Errorf("create media object: %w", err)
	}
	return nil
}

// Stat reports the persisted object's size and content type.
func (c *Client) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	if c == nil || c.sdk == nil || !validKey(key) {
		return ObjectInfo{}, ErrInvalidObject
	}
	info, err := c.sdk.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("stat media object: %w", err)
	}
	sha := ""
	for key, value := range info.UserMetadata {
		if strings.EqualFold(key, "sha256") || strings.EqualFold(key, "X-Amz-Meta-Sha256") {
			sha = value
			break
		}
	}
	return ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: sha}, nil
}

// Remove deletes one exact object key. Callers must first establish ownership
// of that key, for example when cleaning up a test-created or purged asset.
func (c *Client) Remove(ctx context.Context, key string) error {
	if c == nil || c.sdk == nil || !validKey(key) {
		return ErrInvalidObject
	}
	if err := c.sdk.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("remove media object: %w", err)
	}
	return nil
}

// PresignGet grants bounded access to one safe, server-selected object key.
func (c *Client) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if c == nil || c.sdk == nil || !validKey(key) || ttl < time.Second || ttl > 15*time.Minute {
		return "", ErrInvalidObject
	}
	signed, err := c.sdk.PresignedGetObject(ctx, c.bucket, key, ttl, url.Values{})
	if err != nil {
		return "", fmt.Errorf("sign media preview: %w", err)
	}
	return signed.String(), nil
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
