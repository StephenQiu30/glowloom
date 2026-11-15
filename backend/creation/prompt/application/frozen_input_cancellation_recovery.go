package application

import (
	"context"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// FrozenInputCancellationTask retains the original permanent adoption prohibition.
const FrozenInputCancellationTask = "creation.frozen_input.cancel"

// FrozenInputCancellationRecoveryStore reads the original leased responsibility
// and commits target proof with the source command and task fences in one Tx.
type FrozenInputCancellationRecoveryStore interface {
	ReadFrozenInputCancellation(context.Context, taskruntime.Job) (FrozenInputEnvelope, error)
	CompleteFrozenInputCancellationForTask(context.Context, FrozenInputKey, FrozenInputCancelProof, taskruntime.Job) (FrozenInputEnvelope, error)
}

// FrozenInputCancellationRecovery resumes the existing cancellation command. It
// does not create a new command, infer a refund or bypass the reserved-execution gate.
type FrozenInputCancellationRecovery struct {
	store  FrozenInputCancellationRecoveryStore
	target FrozenInputCancellationTarget
}

// NewFrozenInputCancellationRecovery injects the source owner and authenticated target.
func NewFrozenInputCancellationRecovery(store FrozenInputCancellationRecoveryStore, target FrozenInputCancellationTarget) *FrozenInputCancellationRecovery {
	return &FrozenInputCancellationRecovery{store: store, target: target}
}

// Handle recovers an exact cancellation receipt and leaves unknown/reserved
// responsibility in the existing durable task mechanism instead of reporting success.
func (r *FrozenInputCancellationRecovery) Handle(ctx context.Context, job taskruntime.Job) (time.Time, error) {
	if r == nil || r.store == nil || r.target == nil {
		return time.Time{}, writefence.ErrUnavailable
	}
	if job.Kind != FrozenInputCancellationTask {
		return time.Time{}, taskruntime.ErrLeaseLost
	}
	input, err := r.store.ReadFrozenInputCancellation(ctx, job)
	if err != nil {
		return time.Time{}, err
	}
	if input.Input.HandoffID.String() != job.BusinessID || !input.AdoptionBlocked || input.CancelCommand == nil {
		return time.Time{}, domain.ErrFrozenInputConflict
	}
	if input.CancelProof != nil {
		return time.Time{}, nil
	}
	proof, err := r.target.CancelFrozenInput(ctx, input.Key(), *input.CancelCommand)
	if err != nil {
		return time.Time{}, err
	}
	if _, err := r.store.CompleteFrozenInputCancellationForTask(ctx, input.Key(), proof, job); err != nil {
		return time.Time{}, err
	}
	return time.Time{}, nil
}
