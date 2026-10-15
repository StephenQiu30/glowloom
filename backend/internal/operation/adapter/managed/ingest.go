package managed

import (
	"context"
	"errors"
	"io"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediapg "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	operationpg "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// IngestActivities use the existing media pipeline on its media queue.
type IngestActivities struct {
	db      *gorm.DB
	objects *objectstorage.Client
	results *application.ManagedResults
}

// NewIngestActivities shares the private immutable staging store with the worker.
func NewIngestActivities(db *gorm.DB, objects *objectstorage.Client, results *application.ManagedResults) *IngestActivities {
	return &IngestActivities{db: db, objects: objects, results: results}
}

// Ingest retries transfer and registration without access to a supplier Key.
func (a *IngestActivities) Ingest(ctx context.Context, operationID uuid.UUID) error {
	store := operationpg.NewStore(a.db)
	loaded, err := store.LoadWorkflowOperation(ctx, operationID)
	if err != nil {
		return err
	}
	if loaded.Operation.ModelProfileVersionID == nil || loaded.Operation.PriceRuleVersionID == nil || loaded.Operation.Status != "ingesting" || loaded.Provider.Moderation != "provider" {
		return application.ErrInvalidProviderCall
	}
	identity := application.ProviderDispatchIdentity{ProjectID: loaded.Operation.ProjectID, OperationID: operationID, ModelProfileVersionID: *loaded.Operation.ModelProfileVersionID, PriceRuleVersionID: *loaded.Operation.PriceRuleVersionID, Action: "submit", Attempt: 1, RequestKey: loaded.ProviderRequestKey}
	result, err := store.ReadManagedResult(ctx, identity)
	if err != nil {
		return err
	}
	if result == nil || result.State != "succeeded" {
		return application.ErrProviderCallConflict
	}
	if result.Text != "" {
		return store.IngestManagedText(ctx, identity)
	}
	repo := mediapg.NewIngestRepository(a.db)
	service := mediaapp.NewIngestService(repo, artifactDownload{results: a.results, identity: identity}, mediaflow.FFProber{}, mediaflow.FFRenderer{}, mediaObjects{client: a.objects})
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
	return nil
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

type mediaObjects struct{ client *objectstorage.Client }

func (s mediaObjects) PutIfAbsent(ctx context.Context, key string, r io.Reader, size int64, mime, digest string) error {
	err := s.client.PutIfAbsent(ctx, key, r, size, mime, digest)
	if errors.Is(err, objectstorage.ErrObjectExists) {
		return mediaapp.ErrObjectAlreadyExists
	}
	return err
}
func (s mediaObjects) Stat(ctx context.Context, key string) (mediaapp.ObjectInfo, error) {
	info, err := s.client.Stat(ctx, key)
	return mediaapp.ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}, err
}
