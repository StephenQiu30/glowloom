// Package staging adapts the private S3 bucket to bounded receipt recovery.
package staging

import (
	"context"
	"errors"
	"io"

	"github.com/minio/minio-go/v7"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

// Objects never creates a signed URL for staged provider bytes.
type Objects struct{ client *objectstorage.Client }

// NewObjects injects the existing private bucket client.
func NewObjects(client *objectstorage.Client) *Objects { return &Objects{client: client} }

// PutIfAbsent conditionally creates immutable bytes and preserves conflicts.
func (o *Objects) PutIfAbsent(ctx context.Context, key string, reader io.Reader, size int64, mime, digest string) error {
	err := o.client.PutIfAbsent(ctx, key, reader, size, mime, digest)
	if errors.Is(err, objectstorage.ErrObjectExists) {
		return application.ErrProviderStagedObjectExists
	}
	return err
}

// Stat projects metadata and turns missing keys into a recovery-safe sentinel.
func (o *Objects) Stat(ctx context.Context, key string) (application.ProviderStagedObjectInfo, error) {
	info, err := o.client.Stat(ctx, key)
	if err != nil {
		var response minio.ErrorResponse
		if errors.As(err, &response) && (response.Code == "NoSuchKey" || response.Code == "NotFound" || response.StatusCode == 404) {
			return application.ProviderStagedObjectInfo{}, application.ErrProviderStagedObjectMissing
		}
		return application.ProviderStagedObjectInfo{}, err
	}
	return application.ProviderStagedObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}, nil
}

// Open grants private server-side byte access without browser authorization URLs.
func (o *Objects) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return o.client.Get(ctx, key)
}
