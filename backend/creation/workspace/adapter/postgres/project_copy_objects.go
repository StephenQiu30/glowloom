package postgres

import (
	"context"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

// ProjectCopyMediaObjects binds each media checkpoint to one claimed job and phase.
// It stores no context and performs no background work.
type ProjectCopyMediaObjects struct {
	store       *ProjectCopyStore
	job, worker uuid.UUID
	stage       string
}

// NewProjectCopyMediaObjects injects the coordinator fence for transfer or cleanup.
func NewProjectCopyMediaObjects(store *ProjectCopyStore, job, worker uuid.UUID, cleanup bool) *ProjectCopyMediaObjects {
	stage := "media"
	if cleanup {
		stage = "cleanup"
	}
	return &ProjectCopyMediaObjects{store: store, job: job, worker: worker, stage: stage}
}

func (r *ProjectCopyMediaObjects) use(ctx context.Context, actor mediaapp.Actor, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot, apply func(application.ProjectCopyMediaOwner) error) error {
	if r == nil || r.store == nil || apply == nil {
		return workspacesource.ErrProjectDependencyUnavailable
	}
	return r.store.UseAttempt(ctx, identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}, r.job, r.worker, r.stage, func(owners application.ProjectCopyOwners, job domain.ProjectCopyJob) error {
		if binding != copyMediaBinding(job) || snapshot.ID != job.Manifest.MediaSnapshotID || snapshot.ManifestSHA256 != job.Manifest.MediaSHA256 || snapshot.Assets != job.Manifest.Assets || snapshot.Renditions != job.Manifest.Renditions {
			return domain.ErrInvalidProjectCopy
		}
		return apply(owners.Media)
	})
}

// Objects reads only intents on the current job's immutable snapshot.
func (r *ProjectCopyMediaObjects) Objects(ctx context.Context, actor mediaapp.Actor, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot) ([]mediaapp.ProjectCopyObject, error) {
	var objects []mediaapp.ProjectCopyObject
	err := r.use(ctx, actor, binding, snapshot, func(owner application.ProjectCopyMediaOwner) error {
		var err error
		objects, err = owner.Objects(ctx, actor, binding, snapshot)
		return err
	})
	return objects, err
}

// RecordObjectDigest commits source evidence only while the media attempt remains live.
func (r *ProjectCopyMediaObjects) RecordObjectDigest(ctx context.Context, actor mediaapp.Actor, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot, asset uuid.UUID, kind, digest string, size int64) error {
	return r.use(ctx, actor, binding, snapshot, func(owner application.ProjectCopyMediaOwner) error {
		return owner.RecordObjectDigest(ctx, actor, binding, snapshot, asset, kind, digest, size)
	})
}

// ConfirmObject records an exact target readback behind the same worker fence.
func (r *ProjectCopyMediaObjects) ConfirmObject(ctx context.Context, actor mediaapp.Actor, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot, asset uuid.UUID, kind, digest string, size int64) error {
	return r.use(ctx, actor, binding, snapshot, func(owner application.ProjectCopyMediaOwner) error {
		return owner.ConfirmObject(ctx, actor, binding, snapshot, asset, kind, digest, size)
	})
}

// AuthorizeObjectRemoval refuses cleanup from a stale or non-cancelled worker.
func (r *ProjectCopyMediaObjects) AuthorizeObjectRemoval(ctx context.Context, actor mediaapp.Actor, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot, asset uuid.UUID, kind string) error {
	return r.use(ctx, actor, binding, snapshot, func(owner application.ProjectCopyMediaOwner) error {
		return owner.AuthorizeObjectRemoval(ctx, actor, binding, snapshot, asset, kind)
	})
}

// ConfirmObjectRemoved commits actual byte absence under the cleanup fence.
func (r *ProjectCopyMediaObjects) ConfirmObjectRemoved(ctx context.Context, actor mediaapp.Actor, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot, asset uuid.UUID, kind string) error {
	return r.use(ctx, actor, binding, snapshot, func(owner application.ProjectCopyMediaOwner) error {
		return owner.ConfirmObjectRemoved(ctx, actor, binding, snapshot, asset, kind)
	})
}

var _ mediaapp.ProjectCopyObjectRepository = (*ProjectCopyMediaObjects)(nil)

// BeginObjectWrite fences the durable write intent before the external request.
func (r *ProjectCopyMediaObjects) BeginObjectWrite(ctx context.Context, actor mediaapp.Actor, binding mediaapp.ProjectCopyBinding, snapshot mediaapp.ProjectCopySnapshot, asset uuid.UUID, kind string) error {
	return r.use(ctx, actor, binding, snapshot, func(owner application.ProjectCopyMediaOwner) error {
		return owner.BeginObjectWrite(ctx, actor, binding, snapshot, asset, kind)
	})
}
