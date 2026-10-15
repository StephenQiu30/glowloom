package objectstorage

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	platform "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// UploadLocationStore owns immutable intent, checked facts and cleanup fencing.
type UploadLocationStore interface {
	ReserveUploadLocation(context.Context, uuid.UUID, domain.ObjectRef) (domain.ObjectLocation, error)
	ConfirmUploadLocation(context.Context, domain.ObjectLocation, application.ObjectInfo) error
	BeginUploadRemoval(context.Context, domain.ObjectLocation) (bool, error)
	ConfirmUploadRemoval(context.Context, domain.ObjectLocation) error
}

// RoutedUploadObjects combines SDK I/O with durable upload ownership and cleanup.
type RoutedUploadObjects struct {
	registry  *platform.Registry
	locations UploadLocationStore
}

// NewRoutedUploadObjects injects physical profiles and the location ledger.
func NewRoutedUploadObjects(registry *platform.Registry, locations UploadLocationStore) *RoutedUploadObjects {
	return &RoutedUploadObjects{registry: registry, locations: locations}
}

func transportRef(ref domain.ObjectRef) platform.Ref {
	return platform.Ref{ProfileID: ref.ProfileID, Bucket: ref.Bucket, Key: ref.Key}
}

// CreateUploadObject reserves ownership, conditionally writes and verifies provider metadata.
func (s *RoutedUploadObjects) CreateUploadObject(ctx context.Context, target application.UploadTarget, key string, file *application.Downloaded) (domain.ObjectLocation, error) {
	if s == nil || s.registry == nil || s.locations == nil {
		return domain.ObjectLocation{}, application.ErrUnavailable
	}
	if file == nil || file.File == nil || file.Size < 1 || len(file.SHA256) != 64 {
		return domain.ObjectLocation{}, application.ErrInvalidUpload
	}
	ref := domain.ObjectRef{ProfileID: target.ProfileID, Bucket: target.Bucket, Key: key}
	location, err := s.locations.ReserveUploadLocation(ctx, target.SessionID, ref)
	if err != nil {
		return domain.ObjectLocation{}, err
	}
	if _, err := file.File.Seek(0, io.SeekStart); err != nil {
		return location, err
	}
	err = s.registry.PutIfAbsent(ctx, transportRef(ref), file.File, file.Size, file.MIMEType, file.SHA256)
	if err != nil && !errors.Is(err, platform.ErrObjectExists) {
		location.State = "unknown"
		return location, err
	}
	info, err := s.registry.Stat(ctx, transportRef(ref))
	if err != nil {
		location.State = "unknown"
		return location, err
	}
	if info.Size != file.Size || info.ContentType != file.MIMEType || info.SHA256 != file.SHA256 {
		location.State = "unknown"
		return location, application.ErrObjectMismatch
	}
	if err := s.locations.ConfirmUploadLocation(ctx, location, application.ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}); err != nil {
		location.State = "unknown"
		return location, err
	}
	location.State = "present"
	location.ByteSize = &file.Size
	location.SHA256 = &file.SHA256
	location.MIMEType = &file.MIMEType
	return location, nil
}

// RemoveUploadObject fences acceptance before deleting a known unaccepted object.
func (s *RoutedUploadObjects) RemoveUploadObject(ctx context.Context, location domain.ObjectLocation) error {
	if s == nil || s.registry == nil || s.locations == nil {
		return application.ErrUnavailable
	}
	remove, err := s.locations.BeginUploadRemoval(ctx, location)
	if err != nil || !remove {
		return err
	}
	if err := s.registry.Remove(ctx, transportRef(location.Ref)); err != nil {
		return err
	}
	return s.locations.ConfirmUploadRemoval(ctx, location)
}
