package objects

import (
	"context"
	"io"

	"github.com/google/uuid"

	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

// RoutedLocations is the media owner's physical proof and write/removal fence.
type RoutedLocations interface {
	ReadLocation(context.Context, uuid.UUID) (mediadomain.ObjectLocation, error)
	ReadWritableWorkLocation(context.Context, uuid.UUID) (mediadomain.ObjectLocation, error)
	ReadRemovableWorkLocation(context.Context, uuid.UUID) (mediadomain.ObjectLocation, error)
	ResolveLegacyRef(context.Context, string) (mediadomain.ObjectRef, error)
}

// RoutedStorage never derives historical identity from the current default.
type RoutedStorage struct {
	registry  *objectstorage.Registry
	locations RoutedLocations
}

// NewRoutedStorage injects immutable profiles and owning location fences.
func NewRoutedStorage(registry *objectstorage.Registry, locations RoutedLocations) *RoutedStorage {
	return &RoutedStorage{registry: registry, locations: locations}
}

func (s *RoutedStorage) fact(ctx context.Context, f domain.ObjectFact, mode string) (mediadomain.ObjectLocation, error) {
	if s == nil || s.registry == nil || s.locations == nil || f.Validate() != nil || f.LocationID == uuid.Nil {
		return mediadomain.ObjectLocation{}, application.ErrUnavailable
	}
	var l mediadomain.ObjectLocation
	var err error
	switch mode {
	case "write":
		l, err = s.locations.ReadWritableWorkLocation(ctx, f.LocationID)
	case "remove":
		l, err = s.locations.ReadRemovableWorkLocation(ctx, f.LocationID)
	default:
		l, err = s.locations.ReadLocation(ctx, f.LocationID)
	}
	if err != nil {
		return l, err
	}
	if l.ID != f.LocationID || l.Ref.Key != f.Key || l.Ref.Validate() != nil || l.ByteSize != nil && *l.ByteSize != f.ByteSize || l.SHA256 != nil && *l.SHA256 != f.SHA256 || l.MIMEType != nil && *l.MIMEType != f.MIME {
		return l, application.ErrObjectMismatch
	}
	return l, nil
}

// GetObject reads a saved location or an independently proved legacy identity.
func (s *RoutedStorage) GetObject(ctx context.Context, f domain.ObjectFact) (io.ReadCloser, error) {
	if f.LocationID == uuid.Nil {
		return s.Get(ctx, f.Key)
	}
	l, err := s.fact(ctx, f, "read")
	if err != nil {
		return nil, err
	}
	r, err := s.registry.Get(ctx, objectstorage.Ref{ProfileID: l.Ref.ProfileID, Bucket: l.Ref.Bucket, Key: l.Ref.Key})
	if err != nil {
		return nil, storageError(err)
	}
	return &privateReader{ReadCloser: r}, nil
}

// PutObjectIfAbsent checks the durable write fence before SDK creation.
func (s *RoutedStorage) PutObjectIfAbsent(ctx context.Context, f domain.ObjectFact, r io.Reader) error {
	l, err := s.fact(ctx, f, "write")
	if err != nil {
		return err
	}
	return storageError(s.registry.PutIfAbsent(ctx, objectstorage.Ref{ProfileID: l.Ref.ProfileID, Bucket: l.Ref.Bucket, Key: l.Ref.Key}, r, f.ByteSize, f.MIME, f.SHA256))
}

// RemoveObject checks the owning removal fence immediately before SDK deletion.
func (s *RoutedStorage) RemoveObject(ctx context.Context, f domain.ObjectFact) error {
	l, err := s.fact(ctx, f, "remove")
	if err != nil {
		return err
	}
	return storageError(s.registry.Remove(ctx, objectstorage.Ref{ProfileID: l.Ref.ProfileID, Bucket: l.Ref.Bucket, Key: l.Ref.Key}))
}

// Get supports only an independently proved original legacy identity.
func (s *RoutedStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if s == nil || s.registry == nil || s.locations == nil {
		return nil, application.ErrUnavailable
	}
	ref, err := s.locations.ResolveLegacyRef(ctx, key)
	if err != nil {
		return nil, err
	}
	r, err := s.registry.Get(ctx, objectstorage.Ref{ProfileID: ref.ProfileID, Bucket: ref.Bucket, Key: ref.Key})
	if err != nil {
		return nil, storageError(err)
	}
	return &privateReader{ReadCloser: r}, nil
}

// PutIfAbsent rejects writes without physical ownership.
func (*RoutedStorage) PutIfAbsent(context.Context, string, io.Reader, int64, string, string) error {
	return application.ErrUnavailable
}

// Remove rejects deletion without physical ownership.
func (*RoutedStorage) Remove(context.Context, string) error { return application.ErrUnavailable }
