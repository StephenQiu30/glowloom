package managed

import (
	"context"
	"errors"
	"reflect"
	"slices"

	"github.com/google/uuid"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/execution/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// IngestStore is the operation-owned authority and frozen result boundary.
type IngestStore interface {
	LoadWorkflowOperation(context.Context, uuid.UUID) (application.WorkflowOperation, error)
	ReadManagedResult(context.Context, application.ProviderDispatchIdentity) (*application.ManagedResult, error)
	IngestManagedText(context.Context, application.ProviderDispatchIdentity) error
	AuthorizeWorker(context.Context, uuid.UUID, bool) error
}

// ArtifactIngest consumes immutable supplier evidence without exposing Media storage.
type ArtifactIngest interface {
	IngestArtifacts(context.Context, application.ProviderDispatchIdentity, application.ManagedResult, *application.WorkflowControlDelivery) error
}

// IngestActivities retains worker and original-result checks before owner delegation.
type IngestActivities struct {
	store   IngestStore
	results *application.ManagedResults
	media   ArtifactIngest
}

// NewIngestActivities injects an explicit owner port; missing binary delivery fails closed.
func NewIngestActivities(store IngestStore, results *application.ManagedResults, media ArtifactIngest) *IngestActivities {
	return &IngestActivities{store: store, results: results, media: media}
}

// Ingest retries transfer and registration without access to a supplier Key.
func (a *IngestActivities) Ingest(ctx context.Context, operationID uuid.UUID) (resultErr error) {
	store := a.store
	if store == nil {
		return application.ErrExecutionAuthorityRevoked
	}
	defer func() {
		if resultErr != nil {
			if err := store.AuthorizeWorker(ctx, operationID, true); errors.Is(err, application.ErrExecutionAuthorityRevoked) {
				resultErr = err
			}
		}
	}()
	loaded, err := store.LoadWorkflowOperation(ctx, operationID)
	if err != nil {
		return err
	}
	if loaded.Operation.ModelProfileVersionID == nil || loaded.Operation.PriceRuleVersionID == nil || loaded.Operation.Status != operationdomain.StatusIngesting || catalogdomain.Moderation(loaded.Provider.Moderation) != catalogdomain.ModerationProvider {
		return application.ErrInvalidProviderCall
	}
	identity := application.ProviderDispatchIdentity{ProjectID: loaded.Operation.ProjectID, OperationID: operationID, ModelProfileVersionID: *loaded.Operation.ModelProfileVersionID, PriceRuleVersionID: *loaded.Operation.PriceRuleVersionID, Action: "submit", Attempt: 1, RequestKey: loaded.ProviderRequestKey}
	return a.ingestResult(ctx, identity, nil)
}

type originalDeliveryStore interface {
	OriginalDeliveryIdentity(context.Context, application.WorkflowControlDelivery) (application.ProviderDispatchIdentity, error)
	IngestOriginalText(context.Context, application.ProviderDispatchIdentity, application.WorkflowControlDelivery) error
}

// IngestOriginal archives an already paid result under its exact accepted requester.
// It never enters a new provider dispatch or settlement workflow.
func (a *IngestActivities) IngestOriginal(ctx context.Context, d application.WorkflowControlDelivery) error {
	store, ok := a.store.(originalDeliveryStore)
	if !ok {
		return application.ErrInvalidProviderCall
	}
	identity, err := store.OriginalDeliveryIdentity(ctx, d)
	if err != nil {
		return err
	}
	return a.ingestResult(ctx, identity, &d)
}

func (a *IngestActivities) ingestResult(ctx context.Context, identity application.ProviderDispatchIdentity, d *application.WorkflowControlDelivery) error {
	result, err := a.store.ReadManagedResult(ctx, identity)
	if err != nil {
		return err
	}
	if result == nil || result.State != application.ManagedResultSucceeded {
		return application.ErrProviderCallConflict
	}
	if d != nil {
		if a.results == nil {
			return application.ErrProviderCallConflict
		}
		original, err := a.results.Verify(ctx, identity)
		if err != nil {
			return err
		}
		if original == nil || original.State != application.ManagedResultSucceeded || original.Text != result.Text || original.TaskID != result.TaskID || !reflect.DeepEqual(original.Subtitle, result.Subtitle) || !slices.Equal(original.Artifacts, result.Artifacts) {
			return application.ErrProviderCallConflict
		}
	}
	if result.Text != "" {
		if d != nil {
			return a.store.(originalDeliveryStore).IngestOriginalText(ctx, identity, *d)
		}
		return a.store.IngestManagedText(ctx, identity)
	}
	if a.media == nil {
		return writefence.ErrUnavailable
	}
	return a.media.IngestArtifacts(ctx, identity, *result, d)
}
