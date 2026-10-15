package objectstorage

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	platform "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// RoutedReadObjects adapts exact persisted locations to the official SDK registry.
// It has no active-route dependency and cannot reinterpret a historical key.
type RoutedReadObjects struct{ registry *platform.Registry }

// NewRoutedReadObjects injects the immutable profile registry.
func NewRoutedReadObjects(registry *platform.Registry) *RoutedReadObjects {
	return &RoutedReadObjects{registry: registry}
}

// Get opens bytes from exactly the saved profile and bucket.
func (s *RoutedReadObjects) Get(ctx context.Context, ref domain.ObjectRef) (io.ReadCloser, error) {
	if s == nil || s.registry == nil {
		return nil, application.ErrUnavailable
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	return s.registry.Get(ctx, transportRef(ref))
}

// PresignGet preserves the SDK grant and its actual expiry.
func (s *RoutedReadObjects) PresignGet(ctx context.Context, ref domain.ObjectRef, ttl time.Duration) (application.AccessGrant, error) {
	if s == nil || s.registry == nil {
		return application.AccessGrant{}, application.ErrUnavailable
	}
	if err := ref.Validate(); err != nil {
		return application.AccessGrant{}, err
	}
	grant, err := s.registry.PresignGet(ctx, transportRef(ref), ttl)
	if err != nil {
		return application.AccessGrant{}, err
	}
	return application.AccessGrant{URL: grant.URL, ExpiresAt: grant.ExpiresAt}, nil
}

// Stat projects metadata for a single explicitly selected physical reference.
func (s *RoutedReadObjects) Stat(ctx context.Context, ref domain.ObjectRef) (application.ObjectInfo, error) {
	if s == nil || s.registry == nil {
		return application.ObjectInfo{}, application.ErrUnavailable
	}
	if err := ref.Validate(); err != nil {
		return application.ObjectInfo{}, err
	}
	info, err := s.registry.Stat(ctx, transportRef(ref))
	if errors.Is(err, platform.ErrObjectMissing) {
		return application.ObjectInfo{}, errors.Join(application.ErrObjectMissing, err)
	}
	if err != nil {
		return application.ObjectInfo{}, err
	}
	return application.ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}, nil
}

// Exists treats only a confirmed missing object as absent.
func (s *RoutedReadObjects) Exists(ctx context.Context, ref domain.ObjectRef) (bool, error) {
	_, err := s.Stat(ctx, ref)
	if errors.Is(err, platform.ErrObjectMissing) {
		return false, nil
	}
	return err == nil, err
}

// PutIfAbsent bounds relay execution and preserves the platform's atomic create.
func (s *RoutedReadObjects) PutIfAbsent(ctx context.Context, ref domain.ObjectRef, reader io.Reader, size int64, mime, sha string) error {
	if s == nil || s.registry == nil {
		return application.ErrUnavailable
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	err := s.registry.PutIfAbsent(ctx, transportRef(ref), reader, size, mime, sha)
	if errors.Is(err, platform.ErrObjectExists) {
		return errors.Join(application.ErrObjectAlreadyExists, err)
	}
	return err
}

// Remove deletes exactly the location fenced by the owning worker repository.
func (s *RoutedReadObjects) Remove(ctx context.Context, ref domain.ObjectRef) error {
	if s == nil || s.registry == nil {
		return application.ErrUnavailable
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	return s.registry.Remove(ctx, transportRef(ref))
}
