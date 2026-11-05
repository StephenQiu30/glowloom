package managed

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// IngestStore is the operation-owned authority and frozen result boundary.
type IngestStore interface {
	LoadWorkflowOperation(context.Context, uuid.UUID) (application.WorkflowOperation, error)
	ReadManagedResult(context.Context, application.ProviderDispatchIdentity) (*application.ManagedResult, error)
	IngestManagedText(context.Context, application.ProviderDispatchIdentity) error
	AuthorizeWorker(context.Context, uuid.UUID, bool) error
}

// IngestActivities use the existing media pipeline on its media queue.
type IngestActivities struct {
	store    IngestStore
	scopes   mediapg.ScopeAccessFactory
	db       *gorm.DB
	results  *application.ManagedResults
	registry *objectstorage.Registry
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
	CheckOriginalDelivery(*gorm.DB, application.WorkflowControlDelivery) error
	IngestOriginalText(context.Context, application.ProviderDispatchIdentity, application.WorkflowControlDelivery) error
}

// IngestOriginal archives one previously paid result on behalf of the accepted
// requester; this activity never enters the execution or settlement workflow.
func (a *IngestActivities) IngestOriginal(ctx context.Context, d application.WorkflowControlDelivery) error {
	store, ok := a.store.(originalDeliveryStore)
	if !ok {
		return application.ErrInvalidProviderCall
	}
	i, err := store.OriginalDeliveryIdentity(ctx, d)
	if err != nil {
		return err
	}
	return a.ingestResult(ctx, i, &d)
}

func (a *IngestActivities) ingestResult(ctx context.Context, identity application.ProviderDispatchIdentity, d *application.WorkflowControlDelivery) error {
	store := a.store
	operationID := identity.OperationID
	result, err := store.ReadManagedResult(ctx, identity)
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
			return store.(originalDeliveryStore).IngestOriginalText(ctx, identity, *d)
		}
		return store.IngestManagedText(ctx, identity)
	}
	if a.results == nil || a.registry == nil {
		return mediaapp.ErrStorageNotReady
	}
	repo := mediapg.NewScopedIngestRepository(a.db, a.scopes)
	if d != nil {
		repo = repo.ForOriginalDelivery(func(tx *gorm.DB, id uuid.UUID) error {
			if id != d.TargetID {
				return application.ErrInvalidWorkflowControl
			}
			return store.(originalDeliveryStore).CheckOriginalDelivery(tx, *d)
		})
	}
	service := mediaapp.NewRoutedIngestService(repo, artifactDownload{results: a.results, identity: identity}, mediaflow.FFProber{}, mediaflow.FFRenderer{}, mediaobjects.NewGeneratedObjects(a.registry, mediapg.NewStorageRoutes(a.db)), mediapg.NewStorageRoutes(a.db))
	for _, artifact := range result.Artifacts {
		output, err := service.Ingest(ctx, mediaapp.IngestInput{OperationID: operationID.String(), SeqNo: artifact.Sequence, URL: strconv.Itoa(artifact.Sequence)})
		if err != nil {
			return err
		}
		outputID, err := uuid.Parse(output.OutputID)
		if err != nil {
			return application.ErrProviderCallConflict
		}
		if err := repo.RecordModeration(ctx, operationID, outputID, mediadomain.ModerationPassed, ""); err != nil {
			return err
		}
	}
	return repo.CompleteGeneratedWork(ctx, operationID, len(result.Artifacts))
}

type artifactDownload struct {
	results  *application.ManagedResults
	identity application.ProviderDispatchIdentity
}

func (d artifactDownload) Download(ctx context.Context, value string) (*mediaapp.Downloaded, error) {
	seq, err := strconv.Atoi(value)
	if err != nil || seq < 1 || seq > 8 {
		return nil, application.ErrInvalidProviderCall
	}
	artifact, data, err := d.results.ReadArtifact(ctx, d.identity, seq)
	if err != nil {
		return nil, err
	}
	return tempArtifact(data, artifact.MIME)
}

// NewRoutedIngestActivities keeps originals and previews on the admitted physical route.
func NewRoutedIngestActivities(db *gorm.DB, registry *objectstorage.Registry, results *application.ManagedResults, scopes mediapg.ScopeAccessFactory, store IngestStore) *IngestActivities {
	return &IngestActivities{db: db, registry: registry, results: results, scopes: scopes, store: store}
}
