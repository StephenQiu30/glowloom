package objectstorage

import (
	"context"
	"errors"

	"github.com/minio/minio-go/v7"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	platformobjects "github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

// StorageUsageObjects preserves actual metadata and explicit absence from S3.
type StorageUsageObjects struct{ client *platformobjects.Client }

// NewStorageUsageObjects injects the configured private bucket reader.
func NewStorageUsageObjects(client *platformobjects.Client) *StorageUsageObjects {
	return &StorageUsageObjects{client: client}
}

// ListMetadata returns every actual object within the bounded owning prefix.
func (s *StorageUsageObjects) ListMetadata(ctx context.Context, prefix string, limit int) ([]application.StorageUsageObject, error) {
	if s == nil || s.client == nil {
		return nil, application.ErrUnavailable
	}
	listed, err := s.client.ListMetadata(ctx, prefix, limit)
	if err != nil {
		return nil, err
	}
	result := make([]application.StorageUsageObject, 0, len(listed))
	for _, object := range listed {
		result = append(result, application.StorageUsageObject{Key: object.Key, ByteSize: object.Size})
	}
	return result, nil
}

// Stat distinguishes an actual missing key from transport and authorization errors.
func (s *StorageUsageObjects) Stat(ctx context.Context, key string) (int64, bool, error) {
	if s == nil || s.client == nil {
		return 0, false, application.ErrUnavailable
	}
	info, err := s.client.Stat(ctx, key)
	if err != nil {
		var objectError minio.ErrorResponse
		if errors.As(err, &objectError) && (objectError.Code == "NoSuchKey" || objectError.Code == "NoSuchObject") && objectError.StatusCode == 404 {
			return 0, false, nil
		}
		return 0, false, err
	}
	return info.Size, true, nil
}
