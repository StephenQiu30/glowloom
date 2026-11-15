package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// FrozenInputDeliveryTask is Creation's original immutable-input recovery task.
const FrozenInputDeliveryTask = "creation.frozen_input.deliver"

// FrozenInputDeliveryReceipt confirms only Execution's immutable input acceptance.
// It does not assert a quote, reservation, provider submission or funding outcome.
type FrozenInputDeliveryReceipt struct {
	Key                                             FrozenInputKey
	RequestID, OperationID, SubmissionID, ReceiptID uuid.UUID
}

// Validate binds the closed acknowledgement to the original handoff protocol.
func (r FrozenInputDeliveryReceipt) Validate(input FrozenInputEnvelope) error {
	if r.Key != input.Key() || r.RequestID != input.Input.RequestID || r.OperationID != uuid.NewSHA1(r.Key.HandoffID, []byte("execution.operation")) || r.SubmissionID != uuid.NewSHA1(r.Key.HandoffID, []byte("execution.submission")) || r.ReceiptID != uuid.NewSHA1(r.Key.HandoffID, []byte("execution.frozen_input.acceptance")) {
		return domain.ErrFrozenInputConflict
	}
	return nil
}

// FrozenInputDeliverySource reads only the original task-owned immutable input.
// The store must check its current durable task lease before returning the key.
type FrozenInputDeliverySource interface {
	ReadFrozenInputDelivery(context.Context, taskruntime.Job) (FrozenInputEnvelope, error)
}

// FrozenInputDeliveryTarget acknowledges one original input identity without retry.
type FrozenInputDeliveryTarget interface {
	AcceptFrozenInput(context.Context, FrozenInputKey) (FrozenInputDeliveryReceipt, error)
}

// FrozenInputDelivery is the real durable task handler, not a new scheduler. The
// existing owner worker renews its lease, retries the same task and fences Complete.
type FrozenInputDelivery struct {
	source FrozenInputDeliverySource
	target FrozenInputDeliveryTarget
}

// NewFrozenInputDelivery requires the owner source and authenticated target adapter.
func NewFrozenInputDelivery(source FrozenInputDeliverySource, target FrozenInputDeliveryTarget) *FrozenInputDelivery {
	return &FrozenInputDelivery{source: source, target: target}
}

// Handle sends the same immutable key after a lost ACK or process restart. It
// returns success only for an exact target ACK or a permanently blocked source;
// cancellation retains its separate durable recovery task and original funds.
func (d *FrozenInputDelivery) Handle(ctx context.Context, job taskruntime.Job) (time.Time, error) {
	if d == nil || d.source == nil || d.target == nil {
		return time.Time{}, writefence.ErrUnavailable
	}
	if job.Kind != FrozenInputDeliveryTask {
		return time.Time{}, taskruntime.ErrLeaseLost
	}
	input, err := d.source.ReadFrozenInputDelivery(ctx, job)
	if err != nil {
		return time.Time{}, err
	}
	if input.Input.HandoffID.String() != job.BusinessID {
		return time.Time{}, domain.ErrFrozenInputConflict
	}
	if input.AdoptionBlocked {
		return time.Time{}, nil
	}
	receipt, err := d.target.AcceptFrozenInput(ctx, input.Key())
	if err != nil {
		return time.Time{}, err
	}
	if err := receipt.Validate(input); err != nil {
		return time.Time{}, err
	}
	return time.Time{}, nil
}
