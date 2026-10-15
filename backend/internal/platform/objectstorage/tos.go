package objectstorage

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos/enum"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

type tosBackend struct {
	sdk    *tos.ClientV2
	bucket string
}

func newTOS(p config.StorageProfile, c config.StorageCredentials, transport http.RoundTripper) (backend, error) {
	cred := tos.NewStaticCredentials(c.AccessKey, c.SecretKey)
	cred.WithSecurityToken(c.SessionToken)
	sdk, err := tos.NewClientV2(p.Endpoint, tos.WithRegion(p.Region), tos.WithCredentials(cred), tos.WithHTTPTransport(transport), tos.WithMaxRetryCount(0), tos.WithDisableTrailerHeader(true))
	if err != nil {
		return nil, err
	}
	return tosBackend{sdk: sdk, bucket: p.Bucket}, nil
}
func (b tosBackend) put(ctx context.Context, key string, reader io.Reader, size int64, contentType, sha string) error {
	_, err := b.sdk.PutObjectV2(ctx, &tos.PutObjectV2Input{PutObjectBasicInput: tos.PutObjectBasicInput{Bucket: b.bucket, Key: key, ContentLength: size, ContentType: contentType, Meta: map[string]string{"sha256": sha}, ForbidOverwrite: true}, Content: reader})
	return err
}
func (b tosBackend) stat(ctx context.Context, key string) (ObjectInfo, error) {
	info, err := b.sdk.HeadObjectV2(ctx, &tos.HeadObjectV2Input{Bucket: b.bucket, Key: key})
	if err != nil {
		return ObjectInfo{}, err
	}
	sha := ""
	if info.Meta != nil {
		sha, _ = info.Meta.Get("sha256")
	}
	return ObjectInfo{Size: info.ContentLength, ContentType: info.ContentType, SHA256: sha}, nil
}
func (b tosBackend) get(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := b.sdk.GetObjectV2(ctx, &tos.GetObjectV2Input{Bucket: b.bucket, Key: key})
	if err != nil {
		return nil, err
	}
	return resp.Content, nil
}
func (b tosBackend) remove(ctx context.Context, key string) error {
	_, err := b.sdk.DeleteObjectV2(ctx, &tos.DeleteObjectV2Input{Bucket: b.bucket, Key: key})
	return err
}
func (b tosBackend) sign(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	grant, err := b.sdk.PreSignedURL(&tos.PreSignedURLInput{HTTPMethod: enum.HttpMethodGet, Bucket: b.bucket, Key: key, Expires: int64(ttl / time.Second)})
	if err != nil {
		return "", err
	}
	return grant.SignedUrl, nil
}
func (b tosBackend) list(ctx context.Context, prefix string, maxItems int) ([]ObjectMetadata, error) {
	var out []ObjectMetadata
	token := ""
	seen := make(map[string]bool)
	for {
		page, err := b.sdk.ListObjectsType2(ctx, &tos.ListObjectsType2Input{Bucket: b.bucket, Prefix: prefix, ContinuationToken: token, MaxKeys: min(maxItems-len(out)+1, 1000), ListOnlyOnce: true})
		if err != nil {
			return nil, err
		}
		for _, item := range page.Contents {
			out = append(out, ObjectMetadata{Key: item.Key, Size: item.Size})
		}
		if len(out) > maxItems {
			return nil, ErrInventoryLimit
		}
		if !page.IsTruncated {
			return out, nil
		}
		if page.NextContinuationToken == "" || seen[page.NextContinuationToken] {
			return nil, ErrInvalidObject
		}
		token = page.NextContinuationToken
		seen[token] = true
	}
}
