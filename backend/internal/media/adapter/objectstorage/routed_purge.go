package objectstorage

import (
	"context"
	"io"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	platform "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// PurgeLocations resolves only an explicitly retained legacy binding.
type PurgeLocations interface {
	ResolveLegacyRef(context.Context, string) (domain.ObjectRef, error)
}

// RoutedPurgeObjects receives locations from the currently authorized purge lease.
type RoutedPurgeObjects struct {
	objects   *RoutedReadObjects
	locations PurgeLocations
}

// NewRoutedPurgeObjects injects the registry and explicit legacy proof resolver.
func NewRoutedPurgeObjects(registry *platform.Registry, locations PurgeLocations) *RoutedPurgeObjects {
	return &RoutedPurgeObjects{objects: NewRoutedReadObjects(registry), locations: locations}
}
func (p *RoutedPurgeObjects) ref(ctx context.Context, o application.PurgeObject) (domain.ObjectRef, error) {
	if o.Location != nil {
		if o.Location.Ref.Key != o.ObjectKey || o.Location.Ref.Validate() != nil {
			return domain.ObjectRef{}, application.ErrObjectMismatch
		}
		return o.Location.Ref, nil
	}
	if p == nil || p.locations == nil {
		return domain.ObjectRef{}, application.ErrStorageNotReady
	}
	return p.locations.ResolveLegacyRef(ctx, o.ObjectKey)
}

// GetPurge reads the physical object selected by the purge lease.
func (p *RoutedPurgeObjects) GetPurge(ctx context.Context, o application.PurgeObject) (io.ReadCloser, error) {
	r, e := p.ref(ctx, o)
	if e != nil {
		return nil, e
	}
	return p.objects.Get(ctx, r)
}

// ExistsPurge checks exact physical absence without default routing.
func (p *RoutedPurgeObjects) ExistsPurge(ctx context.Context, o application.PurgeObject) (bool, error) {
	r, e := p.ref(ctx, o)
	if e != nil {
		return false, e
	}
	return p.objects.Exists(ctx, r)
}

// RemovePurge deletes the location already verified by its owning purge intent.
func (p *RoutedPurgeObjects) RemovePurge(ctx context.Context, o application.PurgeObject) error {
	r, e := p.ref(ctx, o)
	if e != nil {
		return e
	}
	return p.objects.Remove(ctx, r)
}

// Get rejects reads without a frozen purge object.
func (*RoutedPurgeObjects) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, application.ErrStorageNotReady
}

// Exists rejects bare-key existence checks.
func (*RoutedPurgeObjects) Exists(context.Context, string) (bool, error) {
	return false, application.ErrStorageNotReady
}

// Remove rejects deletion without a frozen physical object.
func (*RoutedPurgeObjects) Remove(context.Context, string) error {
	return application.ErrStorageNotReady
}
