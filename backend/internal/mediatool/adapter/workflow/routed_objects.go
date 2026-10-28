package workflow

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	platform "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// WorkLedger resolves existing identities and fences each bounded external write.
type WorkLedger interface {
	media.StorageWorkAdmission
	media.WorkObjectLocations
	FindWorkLocation(context.Context, uuid.UUID, string) (domain.ObjectLocation, bool, error)
	CancelStorageWork(context.Context, uuid.UUID) error
}

// RoutedObjects creates one frozen storage view for each authorized tool attempt.
type RoutedObjects struct {
	registry *platform.Registry
	ledger   WorkLedger
	legacy   *platform.Client
}

// NewRoutedObjects injects official SDKs and the owning transaction ledger.
func NewRoutedObjects(registry *platform.Registry, ledger WorkLedger, legacy *platform.Client) *RoutedObjects {
	return &RoutedObjects{registry: registry, ledger: ledger, legacy: legacy}
}

// Open creates an isolated view for one authorized durable attempt.
func (f *RoutedObjects) Open(ctx context.Context, owner media.StorageWorkOwner, inputs []domain.MediaAsset) (application.DepthObjects, error) {
	if f == nil || f.registry == nil || f.ledger == nil || owner.Validate() != nil {
		return nil, media.ErrStorageNotReady
	}
	intent, err := f.ledger.ReadStorageWork(ctx, owner)
	if err != nil {
		return nil, err
	}
	if intent.Owner != owner || (intent.State != "open" && intent.State != "cancelled" && intent.State != "accepted") {
		return nil, media.ErrStorageRouteConflict
	}
	sources := make(map[string]*domain.ObjectLocation, len(inputs))
	for _, a := range inputs {
		if a.ObjectKey == "" || a.ProjectID != owner.OwnerID {
			return nil, media.ErrStorageIdentityConflict
		}
		if _, ok := sources[a.ObjectKey]; ok {
			return nil, media.ErrStorageIdentityConflict
		}
		if a.ObjectLocation == nil {
			if f.legacy == nil {
				return nil, media.ErrStorageIdentityConflict
			}
		} else if a.ObjectLocation.Ref.Key != a.ObjectKey || a.ObjectLocation.State != "accepted" || a.ObjectLocation.Ref.Validate() != nil {
			return nil, media.ErrStorageIdentityConflict
		}
		sources[a.ObjectKey] = a.ObjectLocation
	}
	return &workObjects{parent: f, intent: intent, sources: sources}, nil
}

type workObjects struct {
	parent  *RoutedObjects
	intent  media.StorageWorkIntent
	sources map[string]*domain.ObjectLocation
}

func ref(r domain.ObjectRef) platform.Ref {
	return platform.Ref{ProfileID: r.ProfileID, Bucket: r.Bucket, Key: r.Key}
}
func (o *workObjects) output(ctx context.Context, key string) (domain.ObjectLocation, error) {
	if !strings.HasPrefix(key, "projects/"+o.intent.Owner.OwnerID.String()+"/") {
		return domain.ObjectLocation{}, media.ErrStorageIdentityConflict
	}
	location, found, err := o.parent.ledger.FindWorkLocation(ctx, o.intent.ID, key)
	if err != nil {
		return domain.ObjectLocation{}, err
	}
	if found {
		return location, nil
	}
	if o.intent.State != "open" {
		return domain.ObjectLocation{}, media.ErrStorageRouteConflict
	}
	return o.parent.ledger.ReserveWorkLocation(ctx, o.intent.ID, key)
}
func (o *workObjects) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if loc, ok := o.sources[key]; ok {
		if loc == nil {
			return o.parent.legacy.Get(ctx, key)
		}
		return o.parent.registry.Get(ctx, ref(loc.Ref))
	}
	l, e := o.output(ctx, key)
	if e != nil {
		return nil, e
	}
	return o.parent.registry.Get(ctx, ref(l.Ref))
}
func (o *workObjects) Stat(ctx context.Context, key string) (media.ObjectInfo, error) {
	var r domain.ObjectRef
	if l, ok := o.sources[key]; ok {
		if l == nil {
			info, e := o.parent.legacy.Stat(ctx, key)
			return media.ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}, e
		}
		r = l.Ref
	} else {
		l, e := o.output(ctx, key)
		if e != nil {
			return media.ObjectInfo{}, e
		}
		r = l.Ref
	}
	info, e := o.parent.registry.Stat(ctx, ref(r))
	if errors.Is(e, platform.ErrObjectMissing) {
		return media.ObjectInfo{}, application.ErrObjectMissing
	}
	return media.ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}, e
}
func (o *workObjects) PutIfAbsent(ctx context.Context, key string, body io.Reader, size int64, mime, sha string) error {
	if _, ok := o.sources[key]; ok {
		return media.ErrStorageIdentityConflict
	}
	if size < 1 || size > 512<<20 || len(sha) != 64 {
		return media.ErrObjectMismatch
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	loc, e := o.output(ctx, key)
	if e != nil {
		return e
	}
	if loc.ID == uuid.Nil {
		return media.ErrStorageIdentityConflict
	}
	if e = o.parent.ledger.BeginWorkWrite(ctx, o.intent.ID, loc); e != nil {
		return e
	}
	// Native SDKs may close their reader. Ownership of the caller's file remains here.
	e = o.parent.registry.PutIfAbsent(ctx, ref(loc.Ref), io.NopCloser(body), size, mime, sha)
	exists := errors.Is(e, platform.ErrObjectExists)
	if e != nil && !exists {
		return e
	}
	info, e := o.parent.registry.Stat(ctx, ref(loc.Ref))
	if e != nil {
		return e
	}
	if info.Size != size || info.ContentType != mime || info.SHA256 != sha {
		return media.ErrObjectMismatch
	}
	e = o.parent.ledger.ConfirmWorkLocation(ctx, o.intent.ID, loc, media.ObjectInfo{Size: size, ContentType: mime, SHA256: sha})
	if e != nil {
		return e
	}
	if exists {
		return media.ErrObjectAlreadyExists
	}
	return nil
}
func (o *workObjects) Remove(ctx context.Context, key string) error {
	if _, ok := o.sources[key]; ok {
		return media.ErrStorageIdentityConflict
	}
	loc, e := o.output(ctx, key)
	if e != nil {
		return e
	}
	if e = o.parent.ledger.CancelStorageWork(ctx, o.intent.ID); e != nil {
		return e
	}
	yes, e := o.parent.ledger.BeginWorkRemoval(ctx, o.intent.ID, loc)
	if e != nil {
		return e
	}
	if !yes {
		return media.ErrStorageRouteConflict
	}
	if e = o.parent.registry.Remove(ctx, ref(loc.Ref)); e != nil {
		return e
	}
	return o.parent.ledger.ConfirmWorkRemoval(ctx, o.intent.ID, loc)
}
