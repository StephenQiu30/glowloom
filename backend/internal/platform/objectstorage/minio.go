package objectstorage

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func newBackend(p config.StorageProfile, c config.StorageCredentials, transport http.RoundTripper) (backend, error) {
	switch p.Provider {
	case "minio":
		u, err := url.Parse(p.Endpoint)
		if err != nil {
			return nil, ErrInvalidEndpoint
		}
		lookup := minio.BucketLookupPath
		if p.AddressingStyle == "virtual" {
			lookup = minio.BucketLookupDNS
		}
		sdk, err := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(c.AccessKey, c.SecretKey, c.SessionToken), Secure: u.Scheme == "https", Region: p.Region, Transport: transport, BucketLookup: lookup})
		if err != nil {
			return nil, err
		}
		return minioBackend{client: &Client{sdk: sdk, bucket: p.Bucket}}, nil
	case "cos":
		return newCOS(p, c, transport)
	case "tos":
		return newTOS(p, c, transport)
	default:
		return nil, ErrUnsupportedCapability
	}
}

type minioBackend struct{ client *Client }

func (b minioBackend) put(ctx context.Context, key string, reader io.Reader, size int64, contentType, sha string) error {
	opts := minio.PutObjectOptions{ContentType: contentType, UserMetadata: map[string]string{"sha256": sha}, DisableMultipart: true}
	opts.SetMatchETagExcept("*")
	_, err := b.client.sdk.PutObject(ctx, b.client.bucket, key, reader, size, opts)
	return err
}
func (b minioBackend) stat(ctx context.Context, key string) (ObjectInfo, error) {
	return b.client.Stat(ctx, key)
}
func (b minioBackend) get(ctx context.Context, key string) (io.ReadCloser, error) {
	return b.client.Get(ctx, key)
}
func (b minioBackend) remove(ctx context.Context, key string) error { return b.client.Remove(ctx, key) }
func (b minioBackend) sign(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return b.client.PresignGet(ctx, key, ttl)
}
func (b minioBackend) list(ctx context.Context, prefix string, maxItems int) ([]ObjectMetadata, error) {
	return b.client.ListMetadata(ctx, prefix, maxItems)
}
