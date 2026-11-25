package objectstorage

import (
	"context"
	"errors"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	platform "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// RoutedStorageUsageObjects accounts for every admitted physical profile.
type RoutedStorageUsageObjects struct{ registry *platform.Registry }

// NewRoutedStorageUsageObjects injects the admitted profile registry.
func NewRoutedStorageUsageObjects(registry *platform.Registry) *RoutedStorageUsageObjects {
	return &RoutedStorageUsageObjects{registry: registry}
}

// ListMetadata returns a complete bounded physical inventory.
func (s *RoutedStorageUsageObjects) ListMetadata(ctx context.Context, p application.StorageUsagePrefix, limit int) ([]application.StorageUsageObject, error) {
	if s == nil || s.registry == nil {
		return nil, application.ErrUnavailable
	}
	listed, err := s.registry.ListMetadata(ctx, p.ProfileID, p.Bucket, p.Prefix, limit)
	if err != nil {
		return nil, err
	}
	result := make([]application.StorageUsageObject, 0, len(listed))
	for _, item := range listed {
		result = append(result, application.StorageUsageObject{Ref: domain.ObjectRef{ProfileID: item.Ref.ProfileID, Bucket: item.Ref.Bucket, Key: item.Ref.Key}, ByteSize: item.Size})
	}
	return result, nil
}

// Stat distinguishes confirmed absence from failed authorization or transport.
func (s *RoutedStorageUsageObjects) Stat(ctx context.Context, ref domain.ObjectRef) (int64, bool, error) {
	if s == nil || s.registry == nil {
		return 0, false, application.ErrUnavailable
	}
	if err := ref.Validate(); err != nil {
		return 0, false, err
	}
	info, err := s.registry.Stat(ctx, transportRef(ref))
	if errors.Is(err, platform.ErrObjectMissing) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return info.Size, true, nil
}
