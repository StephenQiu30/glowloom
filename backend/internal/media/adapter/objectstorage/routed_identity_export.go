package objectstorage

import (
	"context"
	"io"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	platform "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// RoutedIdentityExportLocations owns export admission and cleanup fencing.
type RoutedIdentityExportLocations interface {
	BeginExportObjectRemoval(context.Context, identityapp.DirectoryExportJob) (identityapp.ExportObjectLocation, bool, error)
	ConfirmExportObjectRemoval(context.Context, identityapp.DirectoryExportJob, identityapp.ExportObjectLocation) error
}

// RoutedIdentityExportObjects keeps short-lived identity files on the same
// profile registry and object-location ledger as media outputs.
type RoutedIdentityExportObjects struct {
	registry  *platform.Registry
	locations RoutedIdentityExportLocations
}

// NewRoutedIdentityExportObjects injects the registry and export cleanup owner.
func NewRoutedIdentityExportObjects(registry *platform.Registry, locations RoutedIdentityExportLocations) *RoutedIdentityExportObjects {
	return &RoutedIdentityExportObjects{registry: registry, locations: locations}
}

func exportTransportRef(location identityapp.ExportObjectLocation) platform.Ref {
	return platform.Ref{ProfileID: location.ProfileID, Bucket: location.Bucket, Key: location.Key}
}

func validExportLocation(location identityapp.ExportObjectLocation) bool {
	return location.ID != uuid.Nil && (domain.ObjectRef{ProfileID: location.ProfileID, Bucket: location.Bucket, Key: location.Key}).Validate() == nil
}

// PutIfAbsent conditionally writes the export at its exact frozen location.
func (s *RoutedIdentityExportObjects) PutIfAbsent(ctx context.Context, location identityapp.ExportObjectLocation, reader io.Reader, size int64, mime, sha string) error {
	if s == nil || s.registry == nil || !validExportLocation(location) {
		return identityapp.ErrExportUnavailable
	}
	return s.registry.PutIfAbsent(ctx, exportTransportRef(location), reader, size, mime, sha)
}

// Stat reads metadata from the exact frozen export location.
func (s *RoutedIdentityExportObjects) Stat(ctx context.Context, location identityapp.ExportObjectLocation) (identityapp.ExportObjectInfo, error) {
	if s == nil || s.registry == nil || !validExportLocation(location) {
		return identityapp.ExportObjectInfo{}, identityapp.ErrExportUnavailable
	}
	info, err := s.registry.Stat(ctx, exportTransportRef(location))
	if err != nil {
		return identityapp.ExportObjectInfo{}, err
	}
	return identityapp.ExportObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}, nil
}

// Get opens the exact frozen export location for reading.
func (s *RoutedIdentityExportObjects) Get(ctx context.Context, location identityapp.ExportObjectLocation) (io.ReadCloser, error) {
	if s == nil || s.registry == nil || !validExportLocation(location) {
		return nil, identityapp.ErrExportUnavailable
	}
	return s.registry.Get(ctx, exportTransportRef(location))
}

// Remove removes an export object only after its owning cleanup fence permits it.
func (s *RoutedIdentityExportObjects) Remove(ctx context.Context, job identityapp.DirectoryExportJob) error {
	if s == nil || s.registry == nil || s.locations == nil {
		return identityapp.ErrExportUnavailable
	}
	location, allowed, err := s.locations.BeginExportObjectRemoval(ctx, job)
	if err != nil {
		return err
	}
	if !allowed {
		return identityapp.ErrExportUnavailable
	}
	if !validExportLocation(location) {
		return application.ErrStorageRouteConflict
	}
	if err := s.registry.Remove(ctx, exportTransportRef(location)); err != nil {
		return err
	}
	return s.locations.ConfirmExportObjectRemoval(ctx, job, location)
}

var _ identityapp.RoutedExportObjects = (*RoutedIdentityExportObjects)(nil)
