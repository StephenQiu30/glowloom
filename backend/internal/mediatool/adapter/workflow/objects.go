package workflow

import (
	"context"
	"errors"
	"io"

	"github.com/minio/minio-go/v7"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

// Objects maps exact private absence and conditional creation into worker errors.
type Objects struct{ client *objectstorage.Client }

// NewObjects injects the configured private bucket client.
func NewObjects(client *objectstorage.Client) *Objects { return &Objects{client: client} }

// Get returns an owned stream; the worker checks bounded bytes and their digest.
func (o *Objects) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return o.client.Get(ctx, key)
}

// Stat preserves infrastructure failures separately from an absent owned key.
func (o *Objects) Stat(ctx context.Context, key string) (mediaapp.ObjectInfo, error) {
	info, err := o.client.Stat(ctx, key)
	if err != nil {
		var response minio.ErrorResponse
		if errors.As(err, &response) && (response.Code == "NoSuchKey" || response.Code == "NotFound" || response.StatusCode == 404) {
			return mediaapp.ObjectInfo{}, application.ErrObjectMissing
		}
		return mediaapp.ObjectInfo{}, err
	}
	return mediaapp.ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}, nil
}

// PutIfAbsent never overwrites an earlier attempt's already-created result.
func (o *Objects) PutIfAbsent(ctx context.Context, key string, reader io.Reader, size int64, mime, sha string) error {
	err := o.client.PutIfAbsent(ctx, key, reader, size, mime, sha)
	if errors.Is(err, objectstorage.ErrObjectExists) {
		return mediaapp.ErrObjectAlreadyExists
	}
	return err
}
