package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

// Activity names are fixed cross-language Temporal contracts.
const (
	IngestActivityName           = "media.Ingest"
	RecordModerationActivityName = "media.RecordModeration"
)

// IngestInput is the JSON request used by OperationWorkflow.
type IngestInput = application.IngestInput

// IngestOutput is the durable candidate returned to OperationWorkflow.
type IngestOutput = application.IngestOutput

// RecordModerationInput records one final review decision for a generated
// candidate. A failed moderation call leaves both rows pending for retry.
type RecordModerationInput struct {
	OperationID string `json:"operation_id"`
	OutputID    string `json:"output_id"`
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
}

// RecordModerationOutput confirms the persisted moderation decision.
type RecordModerationOutput struct {
	Status string `json:"status"`
}

// Activities adapts the media application service to the media task queue.
type Activities struct {
	ingest *application.IngestService
	repo   *pgmedia.IngestRepository
}

// NewActivities wires the database, private bucket, and explicit result URL
// allowlist. It does not make an outbound request during construction.
func NewActivities(db *gorm.DB, objects *objectstorage.Client, policy DownloadPolicy) (*Activities, error) {
	if db == nil || objects == nil {
		return nil, fmt.Errorf("configure media activity: %w", ErrResultUnavailable)
	}
	downloader, err := NewSecureDownloader(policy)
	if err != nil {
		return nil, err
	}
	repo := pgmedia.NewIngestRepository(db)
	service := application.NewIngestService(repo, downloader, FFProber{}, FFRenderer{}, objectStore{client: objects})
	return &Activities{ingest: service, repo: repo}, nil
}

// Register adds only the two media activities; the caller chooses the media queue.
func Register(w worker.Worker, activities *Activities) {
	w.RegisterActivityWithOptions(activities.Ingest, activity.RegisterOptions{Name: IngestActivityName})
	w.RegisterActivityWithOptions(activities.RecordModeration, activity.RegisterOptions{Name: RecordModerationActivityName})
}

// Ingest downloads, validates, stores, and records one provider result.
func (a *Activities) Ingest(ctx context.Context, input IngestInput) (IngestOutput, error) {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				activity.RecordHeartbeat(ctx)
			}
		}
	}()
	defer func() { close(stop); <-done }()
	output, err := a.ingest.Ingest(ctx, input)
	if err != nil {
		return IngestOutput{}, classifyIngestError(err)
	}
	return output, nil
}

func classifyIngestError(err error) error {
	if errors.Is(err, ErrUnsafeResultURL) {
		return temporal.NewNonRetryableApplicationError("provider result URL rejected", "provider_result_invalid", err)
	}
	if errors.Is(err, ErrResultExpired) {
		return temporal.NewNonRetryableApplicationError("provider result URL unavailable", "result_expired", err)
	}
	if errors.Is(err, ErrUnsupportedMedia) {
		return temporal.NewNonRetryableApplicationError("unsupported media result", "unsupported_media", err)
	}
	return err
}

// RecordModeration applies a final moderation decision to the candidate and
// its asset in one database transaction.
func (a *Activities) RecordModeration(ctx context.Context, input RecordModerationInput) (RecordModerationOutput, error) {
	opID, opErr := uuid.Parse(input.OperationID)
	outputID, outputErr := uuid.Parse(input.OutputID)
	if opErr != nil || outputErr != nil {
		return RecordModerationOutput{}, pgmedia.ErrOutputConflict
	}
	status := domain.ModerationStatus(input.Status)
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
