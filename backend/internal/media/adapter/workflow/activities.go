package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// IngestInput is the JSON request used by OperationWorkflow.
type IngestInput = application.IngestInput

// IngestOutput is the durable candidate returned to OperationWorkflow.
type IngestOutput = application.IngestOutput

// RecordModerationInput is the media owner's final moderation command.
type RecordModerationInput = application.RecordModerationInput

// RecordModerationOutput is the media owner's accepted moderation result.
type RecordModerationOutput = application.RecordModerationOutput

// Activities adapts the media application service to the media task queue.
type Activities struct {
	ingest *application.IngestService
	staged *application.IngestService
	repo   *pgmedia.IngestRepository
}

// NewActivities wires the database, private bucket, and explicit result URL
// allowlist. It does not make an outbound request during construction.
func NewActivities(db *gorm.DB, objects *objectstorage.Client, policy DownloadPolicy) (*Activities, error) {
	return newScopedActivities(db, objects, policy, nil)
}

// NewScopedActivities injects durable authority for URL ingest without a staged source.
func NewScopedActivities(db *gorm.DB, objects *objectstorage.Client, policy DownloadPolicy, scopes pgmedia.ScopeAccessFactory) (*Activities, error) {
	return newScopedActivities(db, objects, policy, scopes)
}

func newScopedActivities(db *gorm.DB, objects *objectstorage.Client, policy DownloadPolicy, scopes pgmedia.ScopeAccessFactory) (*Activities, error) {
	if db == nil || objects == nil {
		return nil, fmt.Errorf("configure media activity: %w", ErrResultUnavailable)
	}
	downloader, err := NewSecureDownloader(policy)
	if err != nil {
		return nil, err
	}
	repo := pgmedia.NewScopedIngestRepository(db, scopes)
	service := application.NewIngestService(repo, downloader, FFProber{}, FFRenderer{}, objectStore{client: objects})
	return &Activities{ingest: service, repo: repo}, nil
}

// NewActivitiesWithStaging accepts both existing result URLs and private receipts.
// Both inputs use the same verification, rendering, and candidate repository.
func NewActivitiesWithStaging(db *gorm.DB, objects *objectstorage.Client, policy DownloadPolicy, source application.StagedSource) (*Activities, error) {
	return NewScopedActivitiesWithStaging(db, objects, policy, source, nil)
}

// NewScopedActivitiesWithStaging explicitly binds independent worker authorization.
func NewScopedActivitiesWithStaging(db *gorm.DB, objects *objectstorage.Client, policy DownloadPolicy, source application.StagedSource, scopes pgmedia.ScopeAccessFactory) (*Activities, error) {
	if source == nil {
		return nil, fmt.Errorf("configure staged media activity: %w", application.ErrInvalidIngest)
	}
	activities, err := newScopedActivities(db, objects, policy, scopes)
	if err != nil {
		return nil, err
	}
	activities.staged = application.NewStagedIngestService(activities.repo, source, FFProber{}, FFRenderer{}, objectStore{client: objects})
	return activities, nil
}

// Ingest downloads, validates, stores, and records one provider result.
func (a *Activities) Ingest(ctx context.Context, input IngestInput) (IngestOutput, error) {
	if a == nil || a.ingest == nil {
		return IngestOutput{}, application.ErrStorageNotReady
	}
	service := a.ingest
	if input.Receipt != nil && a.staged != nil {
		service = a.staged
	}
	return service.Ingest(ctx, input)
}

// IngestFailureCode classifies permanent media failures for operation settlement.
// Empty means the durable step can retry.
func IngestFailureCode(err error) string {
	switch {
	case errors.Is(err, ErrUnsafeResultURL), errors.Is(err, ErrResultTooLarge), errors.Is(err, application.ErrInvalidIngest):
		return "provider_result_invalid"
	case errors.Is(err, ErrResultExpired):
		return "result_expired"
	case errors.Is(err, ErrUnsupportedMedia):
		return "unsupported_media"
	default:
		return ""
	}
}

// RecordModeration applies a final moderation decision to the candidate and
// its asset in one database transaction.
func (a *Activities) RecordModeration(ctx context.Context, input RecordModerationInput) (RecordModerationOutput, error) {
	opID, opErr := uuid.Parse(input.OperationID)
	outputID, outputErr := uuid.Parse(input.OutputID)
	if opErr != nil || outputErr != nil {
		return RecordModerationOutput{}, pgmedia.ErrOutputConflict
	}
	status := input.Status
	if err := a.repo.RecordModeration(ctx, opID, outputID, status, input.Reason); err != nil {
		return RecordModerationOutput{}, err
	}
	return RecordModerationOutput{Status: input.Status}, nil
}

type objectStore struct{ client *objectstorage.Client }

func (s objectStore) PutIfAbsent(ctx context.Context, key string, reader io.Reader, size int64, contentType, sha256 string) error {
	err := s.client.PutIfAbsent(ctx, key, reader, size, contentType, sha256)
	if errors.Is(err, objectstorage.ErrObjectExists) {
		return application.ErrObjectAlreadyExists
	}
	return err
}

func (s objectStore) Stat(ctx context.Context, key string) (application.ObjectInfo, error) {
	info, err := s.client.Stat(ctx, key)
	if err != nil {
		return application.ObjectInfo{}, err
	}
	return application.ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}, nil
}

// NewRoutedScopedActivitiesWithStaging uses operation-owned locations for both URL and receipt inputs.
func NewRoutedScopedActivitiesWithStaging(db *gorm.DB, registry *objectstorage.Registry, policy DownloadPolicy, source application.StagedSource, scopes pgmedia.ScopeAccessFactory) (*Activities, error) {
	if db == nil || registry == nil || source == nil {
		return nil, fmt.Errorf("configure routed media activities: %w", application.ErrUnavailable)
	}
	downloader, err := NewSecureDownloader(policy)
	if err != nil {
		return nil, err
	}
	repo := pgmedia.NewScopedIngestRepository(db, scopes)
	routes := pgmedia.NewStorageRoutes(db)
	objects := mediaobjects.NewGeneratedObjects(registry, routes)
	return &Activities{repo: repo, ingest: application.NewRoutedIngestService(repo, downloader, FFProber{}, FFRenderer{}, objects, routes), staged: application.NewRoutedStagedIngestService(repo, source, FFProber{}, FFRenderer{}, objects, routes)}, nil
}

// NewUnavailableActivities keeps moderation recovery available while storage is unconfigured.
func NewUnavailableActivities(db *gorm.DB, scopes pgmedia.ScopeAccessFactory) *Activities {
	return &Activities{repo: pgmedia.NewScopedIngestRepository(db, scopes)}
}
