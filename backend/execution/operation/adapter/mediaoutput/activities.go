package mediaoutput

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
)

// IngestRepository returns only Execution's original durable result and delivery intent.
type IngestRepository interface {
	ReadOutputManifest(context.Context, uuid.UUID) (application.ManagedResult, error)
	PrepareManagedOutputDelivery(context.Context, application.ManagedResult, application.ManagedArtifact) (application.OutputDeliveryRequest, error)
}

// Artifacts opens the exact original staged artifact; implementations may stream
// from a private Media staging port without exposing physical routes to this worker.
type Artifacts interface {
	OpenManagedArtifact(context.Context, application.ProviderDispatchIdentity, application.ManagedArtifact) (io.ReadCloser, error)
}

// OriginalArchive retains the separately authorized terminal recovery use case.
// It grants no new execution, output adoption or charge.
type OriginalArchive interface {
	IngestOriginal(context.Context, application.WorkflowControlDelivery) error
}

// Activities is the actual existing operation.ingest worker's private owner adapter.
type Activities struct {
	repo      IngestRepository
	artifacts Artifacts
	delivery  *application.OutputDelivery
	original  OriginalArchive
}

// NewActivities requires every existing responsibility explicitly, including the
// separately authorized original-archive path. A missing owner port fails closed.
func NewActivities(repo IngestRepository, artifacts Artifacts, delivery *application.OutputDelivery, original OriginalArchive) (*Activities, error) {
	if repo == nil || artifacts == nil || delivery == nil || original == nil {
		return nil, errors.New("incomplete output ingest dependencies")
	}
	return &Activities{repo: repo, artifacts: artifacts, delivery: delivery, original: original}, nil
}

// RegisterGeneratedMedia installs the actual admitted media-ingest task without
// adding a second scheduler or silently replacing an existing task handler.
func RegisterGeneratedMedia(handlers map[string]taskruntime.Handler, activities *Activities) error {
	if handlers == nil || activities == nil || handlers["operation.ingest"] != nil {
		return errors.New("invalid generated media handler registration")
	}
	handlers["operation.ingest"] = func(ctx context.Context, job taskruntime.Job) (time.Time, error) {
		id, err := uuid.Parse(job.BusinessID)
		if err != nil || id.String() != job.BusinessID || job.Kind != "operation.ingest" {
			return time.Time{}, application.ErrInvalidProviderCall
		}
		return time.Time{}, activities.Ingest(ctx, id)
	}
	return nil
}

// Ingest publishes each original sequence, then lets the local delivery writer
// atomically bind the whole manifest and settle only its already established cost.
func (a *Activities) Ingest(ctx context.Context, operation uuid.UUID) error {
	if a == nil || operation == uuid.Nil {
		return application.ErrInvalidProviderCall
	}
	result, err := a.repo.ReadOutputManifest(ctx, operation)
	if err != nil {
		return err
	}
	if result.State != application.ManagedResultSucceeded || result.Text != "" || len(result.Artifacts) == 0 {
		return application.ErrProviderCallConflict
	}
	for _, artifact := range result.Artifacts {
		req, err := a.repo.PrepareManagedOutputDelivery(ctx, result, artifact)
		if err != nil {
			return err
		}
		if err := a.delivery.Deliver(ctx, req, func(ctx context.Context) (io.ReadCloser, error) {
			return a.artifacts.OpenManagedArtifact(ctx, result.Identity, artifact)
		}); err != nil {
			return err
		}
	}
	return nil
}

// IngestOriginal uses its existing accepted recovery authority and never enters
// the output-delivery settlement path for another charge.
func (a *Activities) IngestOriginal(ctx context.Context, delivery application.WorkflowControlDelivery) error {
	if a == nil || a.original == nil {
		return application.ErrInvalidWorkflowControl
	}
	return a.original.IngestOriginal(ctx, delivery)
}
