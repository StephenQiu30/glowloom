package identityruntime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// NewSessionRevocationRecoveryWorker composes only Identity's accepted-request
// recovery. Its caller owns Run, cancellation and joining before database close;
// it is independent of browser verification, token issuance and Redis hints.
func NewSessionRevocationRecoveryWorker(ctx context.Context, db *gorm.DB, writers application.SessionFenceCoordinator, now func() time.Time, concurrency int) (*taskruntime.Worker, error) {
	if db == nil || writers == nil || now == nil || concurrency < 1 || concurrency > 8 {
		return nil, errors.New("session recovery worker requires owner database, coordinator, clock and bounded concurrency")
	}
	store := pgidentity.NewSessionRevocationStore(db)
	bootstrap, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for batch := 0; ; batch++ {
		if batch == 32 {
			return nil, errors.New("session recovery reconciliation budget exhausted")
		}
		count, err := store.EnqueuePendingSessionRevocations(bootstrap, 256, now())
		if err != nil {
			return nil, err
		}
		if count < 256 {
			break
		}
	}
	service := application.NewSessionRevocationRecovery(store, application.NewSessionRevocations(store, writers, now), now)
	handler := func(ctx context.Context, job taskruntime.Job) (time.Time, error) {
		id, err := uuid.Parse(job.BusinessID)
		if err != nil || id == uuid.Nil || job.Kind != application.SessionRevocationRecoveryTask {
			return time.Time{}, errors.New("invalid original session recovery task")
		}
		if err := service.Resume(ctx, id); err != nil {
			return time.Time{}, fmt.Errorf("resume accepted session revocation: %w", err)
		}
		return time.Time{}, nil
	}
	return &taskruntime.Worker{
		Store: taskruntime.NewStore(db), Handlers: map[string]taskruntime.Handler{application.SessionRevocationRecoveryTask: handler},
		Concurrency: concurrency, Lease: 30 * time.Second, Poll: 250 * time.Millisecond,
	}, nil
}
