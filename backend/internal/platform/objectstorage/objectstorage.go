// Package objectstorage owns the S3-compatible object storage client used by backend roles.
package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

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
)

// Client provides access to the configured private object storage bucket.
type Client struct {
	sdk    *minio.Client
	bucket string
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
