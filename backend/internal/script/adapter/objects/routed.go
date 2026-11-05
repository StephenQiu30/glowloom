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
}

type locationAccess uint8

const (
	locationRead locationAccess = iota + 1
	locationWrite
	locationRemove
)

// RoutedStorage never derives historical identity from the current default.
type RoutedStorage struct {
	registry  *objectstorage.Registry
	locations RoutedLocations
}

// NewRoutedStorage injects immutable profiles and owning location fences.
func NewRoutedStorage(registry *objectstorage.Registry, locations RoutedLocations) *RoutedStorage {
	return &RoutedStorage{registry: registry, locations: locations}
}

func (s *RoutedStorage) fact(ctx context.Context, f domain.ObjectFact, access locationAccess) (mediadomain.ObjectLocation, error) {
	if s == nil || s.registry == nil || s.locations == nil || f.Validate() != nil || f.LocationID == uuid.Nil {
		return mediadomain.ObjectLocation{}, application.ErrUnavailable
	}
	var l mediadomain.ObjectLocation
	var err error
	switch access {
	case locationWrite:
		l, err = s.locations.ReadWritableWorkLocation(ctx, f.LocationID)
	case locationRemove:
		l, err = s.locations.ReadRemovableWorkLocation(ctx, f.LocationID)
	case locationRead:
		l, err = s.locations.ReadLocation(ctx, f.LocationID)
	default:
		return mediadomain.ObjectLocation{}, application.ErrUnavailable
	}
	if err != nil {
		return l, err
	}
	if l.ID != f.LocationID || l.Ref.Key != f.Key || l.Ref.Validate() != nil || l.ByteSize != nil && *l.ByteSize != f.ByteSize || l.SHA256 != nil && *l.SHA256 != f.SHA256 || l.MIMEType != nil && *l.MIMEType != f.MIME {
		return l, application.ErrObjectMismatch
	}
	return l, nil
}

// GetObject reads the saved physical location.
func (s *RoutedStorage) GetObject(ctx context.Context, f domain.ObjectFact) (io.ReadCloser, error) {
	l, err := s.fact(ctx, f, locationRead)
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
	l, err := s.fact(ctx, f, locationWrite)
	if err != nil {
		return err
	}
	return storageError(s.registry.PutIfAbsent(ctx, objectstorage.Ref{ProfileID: l.Ref.ProfileID, Bucket: l.Ref.Bucket, Key: l.Ref.Key}, r, f.ByteSize, f.MIME, f.SHA256))
}

// RemoveObject checks the owning removal fence immediately before SDK deletion.
func (s *RoutedStorage) RemoveObject(ctx context.Context, f domain.ObjectFact) error {
	l, err := s.fact(ctx, f, locationRemove)
	if err != nil {
		return err
	}
	return storageError(s.registry.Remove(ctx, objectstorage.Ref{ProfileID: l.Ref.ProfileID, Bucket: l.Ref.Bucket, Key: l.Ref.Key}))
}
