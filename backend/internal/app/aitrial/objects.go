package aitrial

import (
	"context"
	"io"

	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trialtask"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// Objects uses the exact destination frozen in development_trial input/state.
// It never selects a default profile for an existing experiment result.
type Objects struct{ registry *objectstorage.Registry }

// NewObjects injects the registry used for the trial's frozen physical destination.
func NewObjects(registry *objectstorage.Registry) *Objects { return &Objects{registry: registry} }
func transport(r mediadomain.ObjectRef) objectstorage.Ref {
	return objectstorage.Ref{ProfileID: r.ProfileID, Bucket: r.Bucket, Key: r.Key}
}

// CheckWrite checks write permission on the exact frozen object reference.
func (o *Objects) CheckWrite(ctx context.Context, r mediadomain.ObjectRef) error {
	if o == nil || o.registry == nil || r.Validate() != nil {
		return objectstorage.ErrUnavailable
	}
	return o.registry.CheckWrite(ctx, transport(r))
}

// PutIfAbsent conditionally writes the exact frozen physical object.
func (o *Objects) PutIfAbsent(ctx context.Context, r mediadomain.ObjectRef, body io.Reader, size int64, mime, sha string) error {
	if err := o.CheckWrite(ctx, r); err != nil {
		return err
	}
	return o.registry.PutIfAbsent(ctx, transport(r), body, size, mime, sha)
}

// Stat reads metadata from the exact frozen physical object.
func (o *Objects) Stat(ctx context.Context, r mediadomain.ObjectRef) (objectstorage.ObjectInfo, error) {
	if o == nil || o.registry == nil || r.Validate() != nil {
		return objectstorage.ObjectInfo{}, objectstorage.ErrUnavailable
	}
	return o.registry.Stat(ctx, transport(r))
}

// Get opens the exact frozen physical object for reading.
func (o *Objects) Get(ctx context.Context, r mediadomain.ObjectRef) (io.ReadCloser, error) {
	if o == nil || o.registry == nil || r.Validate() != nil {
		return nil, objectstorage.ErrUnavailable
	}
	return o.registry.Get(ctx, transport(r))
}

// Sessions pins a destination in the same transaction as the original durable
// trial input and queue reference, before any provider or CLI generation call.
func Sessions(db *gorm.DB, registry *objectstorage.Registry) *trialtask.Sessions {
	objects := NewObjects(registry)
	return trialtask.NewSessions(db).WithOutputAdmission(func(ctx context.Context, tx *gorm.DB, input application.TrialInput) (mediadomain.ObjectRef, error) {
		route, err := pgmedia.NewStorageRoutes(tx).ReadConfigured(ctx)
		if err != nil {
			return mediadomain.ObjectRef{}, err
		}
		ref := mediadomain.ObjectRef{ProfileID: route.ProfileID, Bucket: route.Bucket, Key: application.TrialObjectKey(input, 1)}
		if err := objects.CheckWrite(ctx, ref); err != nil {
			return ref, err
		}
		return ref, nil
	})
}
