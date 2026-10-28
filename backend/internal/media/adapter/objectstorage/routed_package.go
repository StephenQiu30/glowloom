package objectstorage

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	platform "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// PackageLocations receives fixed admission IDs; it never selects an active route.
type PackageLocations interface {
	ReadLocation(context.Context, uuid.UUID) (domain.ObjectLocation, error)
	ResolveLegacyRef(context.Context, string) (domain.ObjectRef, error)
	BeginWorkWrite(context.Context, uuid.UUID, domain.ObjectLocation) error
	ConfirmWorkLocation(context.Context, uuid.UUID, domain.ObjectLocation, application.ObjectInfo) error
	CancelStorageWork(context.Context, uuid.UUID) error
	BeginWorkRemoval(context.Context, uuid.UUID, domain.ObjectLocation) (bool, error)
	ConfirmWorkRemoval(context.Context, uuid.UUID, domain.ObjectLocation) error
}

// RoutedPackageObjects performs SDK operations only under saved package ownership.
type RoutedPackageObjects struct {
	objects   *RoutedReadObjects
	locations PackageLocations
}

// NewRoutedPackageObjects injects fixed locations and their work fences.
func NewRoutedPackageObjects(registry *platform.Registry, locations PackageLocations) *RoutedPackageObjects {
	return &RoutedPackageObjects{objects: NewRoutedReadObjects(registry), locations: locations}
}
func (p *RoutedPackageObjects) location(ctx context.Context, o application.PackageObject) (domain.ObjectLocation, error) {
	if p == nil || p.locations == nil {
		return domain.ObjectLocation{}, application.ErrStorageNotReady
	}
	if o.Location == nil {
		ref, err := p.locations.ResolveLegacyRef(ctx, o.Key)
		return domain.ObjectLocation{Ref: ref}, err
	}
	l, err := p.locations.ReadLocation(ctx, o.Location.ID)
	if err != nil {
		return l, err
	}
	if l.Ref != o.Location.Ref || l.Ref.Key != o.Key {
		return domain.ObjectLocation{}, application.ErrObjectMismatch
	}
	return l, nil
}

// GetPackage opens the saved location or an independently proved legacy source.
func (p *RoutedPackageObjects) GetPackage(ctx context.Context, o application.PackageObject) (io.ReadCloser, error) {
	l, e := p.location(ctx, o)
	if e != nil {
		return nil, e
	}
	return p.objects.Get(ctx, l.Ref)
}

// ExistsPackage distinguishes confirmed absence from storage failures.
func (p *RoutedPackageObjects) ExistsPackage(ctx context.Context, o application.PackageObject) (bool, error) {
	l, e := p.location(ctx, o)
	if e != nil {
		return false, e
	}
	return p.objects.Exists(ctx, l.Ref)
}

// PutPackage records a bounded write fence before conditional SDK creation.
func (p *RoutedPackageObjects) PutPackage(ctx context.Context, o application.PackageObject, r io.Reader) error {
	if o.Location == nil || o.StorageIntentID == uuid.Nil {
		return application.ErrStorageNotReady
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	l, err := p.location(ctx, o)
	if err != nil {
		return err
	}
	if err := p.locations.BeginWorkWrite(ctx, o.StorageIntentID, l); err != nil {
		return err
	}
	return p.objects.PutIfAbsent(ctx, l.Ref, r, o.ByteSize, o.MIMEType, o.SHA256)
}

// ConfirmPackageObject records the caller's complete byte verification.
func (p *RoutedPackageObjects) ConfirmPackageObject(ctx context.Context, o application.PackageObject) error {
	if o.Location == nil || o.StorageIntentID == uuid.Nil {
		return application.ErrStorageNotReady
	}
	l, e := p.location(ctx, o)
	if e != nil {
		return e
	}
	return p.locations.ConfirmWorkLocation(ctx, o.StorageIntentID, l, application.ObjectInfo{Size: o.ByteSize, SHA256: o.SHA256, ContentType: o.MIMEType})
}

// RemovePackage requires cancelled ownership and a drained write namespace.
func (p *RoutedPackageObjects) RemovePackage(ctx context.Context, o application.PackageObject) error {
	if o.Location == nil || o.StorageIntentID == uuid.Nil {
		return application.ErrStorageNotReady
	}
	l, e := p.location(ctx, o)
	if e != nil {
		return e
	}
	if l.State == "deleted" {
		return nil
	}
	if e = p.locations.CancelStorageWork(ctx, o.StorageIntentID); e != nil {
		return e
	}
	ready, e := p.locations.BeginWorkRemoval(ctx, o.StorageIntentID, l)
	if e != nil {
		return e
	}
	if !ready {
		return application.ErrStorageNotReady
	}
	e = p.objects.Remove(ctx, l.Ref)
	if errors.Is(e, application.ErrObjectMissing) {
		return nil
	}
	return e
}

// ConfirmPackageRemoval records absence after the owning removal intent.
func (p *RoutedPackageObjects) ConfirmPackageRemoval(ctx context.Context, o application.PackageObject) error {
	l, e := p.location(ctx, o)
	if e != nil {
		return e
	}
	if l.State == "deleted" {
		return nil
	}
	return p.locations.ConfirmWorkRemoval(ctx, o.StorageIntentID, l)
}

// Get rejects reads without an owning package object.
func (*RoutedPackageObjects) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, application.ErrStorageNotReady
}

// Exists rejects bare-key existence checks.
func (*RoutedPackageObjects) Exists(context.Context, string) (bool, error) {
	return false, application.ErrStorageNotReady
}

// Remove rejects deletion without a saved location and work owner.
func (*RoutedPackageObjects) Remove(context.Context, string) error {
	return application.ErrStorageNotReady
}

// PutIfAbsent rejects writes without an admitted package object.
func (*RoutedPackageObjects) PutIfAbsent(context.Context, string, io.Reader, int64, string, string) error {
	return application.ErrStorageNotReady
}
