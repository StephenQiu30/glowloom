package operation

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/managed"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/media/adapter/objectstorage"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

type originalDeliveryChecker interface {
	CheckOriginalDelivery(*gorm.DB, application.WorkflowControlDelivery) error
}

type legacyArtifactIngest struct {
	db       *gorm.DB
	registry *objectstorage.Registry
	results  *application.ManagedResults
	scopes   mediapg.ScopeAccessFactory
	store    managed.IngestStore
}

// NewRoutedIngestActivities assembles the real legacy Media transaction boundary.
// Native Execution must inject its owned publication worker instead of this bridge.
func NewRoutedIngestActivities(db *gorm.DB, registry *objectstorage.Registry, results *application.ManagedResults, scopes mediapg.ScopeAccessFactory, store managed.IngestStore) *managed.IngestActivities {
	return managed.NewIngestActivities(store, results, &legacyArtifactIngest{db: db, registry: registry, results: results, scopes: scopes, store: store})
}

func (a *legacyArtifactIngest) IngestArtifacts(ctx context.Context, identity application.ProviderDispatchIdentity, result application.ManagedResult, d *application.WorkflowControlDelivery) error {
	if a.results == nil || a.registry == nil {
		return mediaapp.ErrStorageNotReady
	}
	repo := mediapg.NewScopedIngestRepository(a.db, a.scopes)
	if d != nil {
		checker, ok := a.store.(originalDeliveryChecker)
		if !ok {
			return application.ErrInvalidProviderCall
		}
		repo = repo.ForOriginalDelivery(func(tx *gorm.DB, id uuid.UUID) error {
			if id != d.TargetID {
				return application.ErrInvalidWorkflowControl
			}
			return checker.CheckOriginalDelivery(tx, *d)
		})
	}
	service := mediaapp.NewRoutedIngestService(repo, managedArtifactDownload{results: a.results, identity: identity}, mediaflow.FFProber{}, mediaflow.FFRenderer{}, mediaobjects.NewGeneratedObjects(a.registry, mediapg.NewStorageRoutes(a.db)), mediapg.NewStorageRoutes(a.db))
	for _, artifact := range result.Artifacts {
		output, err := service.Ingest(ctx, mediaapp.IngestInput{OperationID: identity.OperationID.String(), SeqNo: artifact.Sequence, URL: strconv.Itoa(artifact.Sequence)})
		if err != nil {
			return err
		}
		outputID, err := uuid.Parse(output.OutputID)
		if err != nil {
			return application.ErrProviderCallConflict
		}
		if err := repo.RecordModeration(ctx, identity.OperationID, outputID, mediadomain.ModerationPassed, ""); err != nil {
			return err
		}
	}
	return repo.CompleteGeneratedWork(ctx, identity.OperationID, len(result.Artifacts))
}

type managedArtifactDownload struct {
	results  *application.ManagedResults
	identity application.ProviderDispatchIdentity
}

func (d managedArtifactDownload) Download(ctx context.Context, value string) (*mediaapp.Downloaded, error) {
	seq, err := strconv.Atoi(value)
	if err != nil || seq < 1 || seq > 8 {
		return nil, application.ErrInvalidProviderCall
	}
	artifact, data, err := d.results.ReadArtifact(ctx, d.identity, seq)
	if err != nil {
		return nil, err
	}
	return generatedTempArtifact(data, artifact.MIME)
}
