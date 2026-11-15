package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// ReadFrozenInputDelivery checks the actual Creation task lease before reading its
// immutable source key. It neither authenticates a new user nor admits a command.
func (s *FrozenInputStore) ReadFrozenInputDelivery(ctx context.Context, job taskruntime.Job) (application.FrozenInputEnvelope, error) {
	return s.readFrozenRecoveryTask(ctx, job, application.FrozenInputDeliveryTask)
}

// ReadFrozenInputCancellation reads only a current leased cancellation responsibility.
func (s *FrozenInputStore) ReadFrozenInputCancellation(ctx context.Context, job taskruntime.Job) (application.FrozenInputEnvelope, error) {
	return s.readFrozenRecoveryTask(ctx, job, application.FrozenInputCancellationTask)
}

func (s *FrozenInputStore) readFrozenRecoveryTask(ctx context.Context, job taskruntime.Job, kind string) (application.FrozenInputEnvelope, error) {
	var out application.FrozenInputEnvelope
	if err := s.ready(ctx); err != nil {
		return out, err
	}
	id, err := uuid.Parse(job.BusinessID)
	if err != nil || id == uuid.Nil || id.String() != job.BusinessID || job.Kind != kind {
		return out, taskruntime.ErrLeaseLost
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := taskruntime.Fence(ctx, tx, job); err != nil {
			return err
		}
		row, err := readFrozenRow(tx, id, false)
		if err != nil {
			return err
		}
		out, err = frozenEnvelope(row)
		return err
	})
	return out, err
}
