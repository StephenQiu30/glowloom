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

// CopyWorkLocations resolves only owner-admitted exact output identities.
type CopyWorkLocations interface {
	ReadWorkLocationByRef(context.Context, domain.ObjectRef) (domain.ObjectLocation, error)
	ReadWritableWorkLocation(context.Context, uuid.UUID) (domain.ObjectLocation, error)
	ReadRemovableWorkLocation(context.Context, uuid.UUID) (domain.ObjectLocation, error)
}

// RoutedCopyObjects joins a frozen reference with its durable physical fence.
type RoutedCopyObjects struct {
	*RoutedReadObjects
	registry  *platform.Registry
	locations CopyWorkLocations
}

// NewRoutedCopyObjects never selects the current route for an existing copy.
func NewRoutedCopyObjects(registry *platform.Registry, locations CopyWorkLocations) *RoutedCopyObjects {
	return &RoutedCopyObjects{RoutedReadObjects: NewRoutedReadObjects(registry), registry: registry, locations: locations}
}
func (o *RoutedCopyObjects) allowed(ctx context.Context, ref domain.ObjectRef, remove bool) error {
	if o == nil || o.registry == nil || o.locations == nil {
		return application.ErrUnavailable
	}
	l, err := o.locations.ReadWorkLocationByRef(ctx, ref)
	if err != nil {
		return err
	}
	var checked domain.ObjectLocation
	if remove {
		checked, err = o.locations.ReadRemovableWorkLocation(ctx, l.ID)
	} else {
		checked, err = o.locations.ReadWritableWorkLocation(ctx, l.ID)
	}
	if err != nil {
		return err
	}
	if checked.ID != l.ID || checked.Ref != ref {
		return application.ErrStorageIdentityConflict
	}
	return nil
}

// PutIfAbsent cannot write after cancellation, expiry or acceptance.
func (o *RoutedCopyObjects) PutIfAbsent(ctx context.Context, ref domain.ObjectRef, r io.Reader, size int64, mime, sha string) error {
	if err := o.allowed(ctx, ref, false); err != nil {
		return err
	}
	err := o.registry.PutIfAbsent(ctx, transportRef(ref), r, size, mime, sha)
	if errors.Is(err, platform.ErrObjectExists) {
		return application.ErrObjectAlreadyExists
	}
	return err
}

// Remove requires a current durable owner retirement; metadata is not proof of absence.
func (o *RoutedCopyObjects) Remove(ctx context.Context, ref domain.ObjectRef) error {
	if err := o.allowed(ctx, ref, true); err != nil {
		return err
	}
	return o.registry.Remove(ctx, transportRef(ref))
}
