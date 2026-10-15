package staging

import (
	"context"
	"errors"
	"io"

	"strings"
	"time"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	platform "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// OperationWorkStore returns durable output targets and records write ownership.
type OperationWorkStore interface {
	mediaapp.StorageWorkAdmission
	mediaapp.WorkObjectLocations
}

// RoutedFactory binds provider staging to the target saved during quote admission.
type RoutedFactory struct {
	registry *platform.Registry
	store    OperationWorkStore
}

// NewRoutedFactory explicitly injects immutable profiles and admitted work.
func NewRoutedFactory(registry *platform.Registry, store OperationWorkStore) *RoutedFactory {
	return &RoutedFactory{registry: registry, store: store}
}

// PinOperationStorage belongs to the operation quote transaction; the caller
// constructs this adapter over its transaction-bound repository.
func (f *RoutedFactory) PinOperationStorage(ctx context.Context, project, id uuid.UUID) error {
	if f == nil || f.store == nil {
		return mediaapp.ErrUnavailable
	}
	_, err := f.store.PinStorageWork(ctx, mediaapp.StorageWorkOwner{Kind: "operation", OwnerID: project, WorkID: id})
	return err
}

// ForProviderDispatch reads the saved target, never pins the active route late.
func (f *RoutedFactory) ForProviderDispatch(ctx context.Context, identity application.ProviderDispatchIdentity) (application.ProviderStagingObjects, error) {
	if f == nil || f.store == nil || f.registry == nil {
		return nil, mediaapp.ErrUnavailable
	}
	if identity.Validate() != nil {
		return nil, application.ErrInvalidProviderCall
	}
	intent, err := f.store.ReadStorageWork(ctx, mediaapp.StorageWorkOwner{Kind: "operation", OwnerID: identity.ProjectID, WorkID: identity.OperationID})
	if err != nil {
		return nil, err
	}
	prefix := application.ProviderStagePrefix(identity) + "/"
	return &routedObjects{registry: f.registry, store: f.store, intent: intent, prefix: prefix}, nil
}

type routedObjects struct {
	registry *platform.Registry
	store    mediaapp.WorkObjectLocations
	intent   mediaapp.StorageWorkIntent
	prefix   string
}

func (o *routedObjects) ref(key string) (platform.Ref, error) {
	ref := mediadomain.ObjectRef{ProfileID: o.intent.ProfileID, Bucket: o.intent.Bucket, Key: key}
	if !strings.HasPrefix(key, o.prefix) || ref.Validate() != nil {
		return platform.Ref{}, application.ErrInvalidProviderCall
	}
	return platform.Ref{ProfileID: ref.ProfileID, Bucket: ref.Bucket, Key: key}, nil
}

func (o *routedObjects) PutIfAbsent(ctx context.Context, key string, reader io.Reader, size int64, mime, sha string) error {
	ref, err := o.ref(key)
	if err != nil {
		return err
	}
	location, err := o.store.ReserveWorkLocation(ctx, o.intent.ID, key)
	if err != nil {
		return err
	}
	if location.Ref != (mediadomain.ObjectRef{ProfileID: ref.ProfileID, Bucket: ref.Bucket, Key: ref.Key}) {
		return mediaapp.ErrStorageIdentityConflict
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	err = o.registry.PutIfAbsent(ctx, ref, reader, size, mime, sha)
	if err != nil && !errors.Is(err, platform.ErrObjectExists) {
		return err
	}
	info, statErr := o.registry.Stat(ctx, ref)
	if statErr != nil {
		return statErr
	}
	if info.Size != size || info.ContentType != mime || info.SHA256 != sha {
		return mediaapp.ErrObjectMismatch
	}
	if err := o.store.ConfirmWorkLocation(ctx, o.intent.ID, location, mediaapp.ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}); err != nil {
		return err
	}
	if errors.Is(err, platform.ErrObjectExists) {
		return application.ErrProviderStagedObjectExists
	}
	return nil
}
func (o *routedObjects) Stat(ctx context.Context, key string) (application.ProviderStagedObjectInfo, error) {
	ref, err := o.ref(key)
	if err != nil {
		return application.ProviderStagedObjectInfo{}, err
	}
	info, err := o.registry.Stat(ctx, ref)
	if errors.Is(err, platform.ErrObjectMissing) {
		return application.ProviderStagedObjectInfo{}, errors.Join(application.ErrProviderStagedObjectMissing, err)
	}
	return application.ProviderStagedObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}, err
}
func (o *routedObjects) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	ref, err := o.ref(key)
	if err != nil {
		return nil, err
	}
	return o.registry.Get(ctx, ref)
}
