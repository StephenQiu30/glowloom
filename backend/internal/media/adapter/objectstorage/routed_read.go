package objectstorage

import (
	"context"
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
