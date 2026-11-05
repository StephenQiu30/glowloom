package task

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// BatchStore owns durable batch launch and per-item admission facts.
type BatchStore interface {
	LoadWorkflowBatch(context.Context, uuid.UUID) (app.WorkflowBatch, error)
	MarkWorkflowBatchRunning(context.Context, uuid.UUID) error
	MarkWorkflowBatchCancelRequested(context.Context, uuid.UUID) error
	AcquireBatchLaunch(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	PauseWorkflowBatch(context.Context, uuid.UUID, string) error
	ResumeWorkflowBatch(context.Context, uuid.UUID) error
	FinishWorkflowBatch(context.Context, uuid.UUID) error
	OriginalDeliveryRecoveryRequired(context.Context, app.WorkflowControlDelivery) (bool, error)
}

func (f flow) batch(ctx context.Context, j taskruntime.Job) (time.Time, error) {
	id, err := uuid.Parse(j.BusinessID)
	if err != nil {
		return time.Time{}, ErrInvalidOperationInput
	}
	b, err := f.store.LoadWorkflowBatch(ctx, id)
	if err != nil {
		return time.Time{}, err
	}
	next := time.Now().Add(2 * time.Second)
	switch b.Batch.Status {
	case domain.BatchStatusFinished, domain.BatchStatusCancelled:
		return time.Time{}, nil
	case domain.BatchStatusConfirmed:
		return next, f.store.MarkWorkflowBatchRunning(ctx, id)
	}
	remaining := false
	for _, i := range b.Items {
		if !i.Status.IsTerminal() {
			remaining = true
			break
		}
	}
	if !remaining {
		return time.Time{}, f.store.FinishWorkflowBatch(ctx, id)
	}
	if b.PausedReason != "" && !b.CancelRequested {
		return next, nil
	}
	if !b.CancelRequested {
		if code := BatchFailureStreak(b.Items); code != "" {
			return next, f.store.PauseWorkflowBatch(ctx, id, code)
		}
	}
	for _, i := range b.Items {
		if i.Status.IsTerminal() {
			continue
		}
		if b.CancelRequested {
			done, err := f.finalizer.CancelQueuedBatchOperation(ctx, id, i.OperationID)
			if err != nil {
				return next, err
			}
			if done {
				continue
			}
		}
		if i.Status == domain.StatusConfirmed {
			ok, err := f.store.AcquireBatchLaunch(ctx, id, i.OperationID)
			if err != nil {
				return next, err
			}
			if !ok {
				continue
			}
		}
		if err := f.store.ScheduleOperationStep(ctx, "operation.run", i.OperationID); err != nil {
			return next, err
		}
	}
	return next, nil
}
func (f flow) control(ctx context.Context, j taskruntime.Job, d app.WorkflowControlDelivery) error {
	if j.BusinessID != d.EventID.String() {
		return app.ErrInvalidWorkflowControl
	}
	allowed, err := f.store.CanDeliverWorkflowControl(ctx, d)
	if errors.Is(err, app.ErrExecutionAuthorityRevoked) {
		return nil
	}
	if err != nil || !allowed {
		return err
	}
	if d.TargetType == "batch" {
		if d.Action == "cancel" {
			return f.store.MarkWorkflowBatchCancelRequested(ctx, d.TargetID)
		}
		return f.store.ResumeWorkflowBatch(ctx, d.TargetID)
	}
	if d.Action == "recover" {
		required, err := f.store.OriginalDeliveryRecoveryRequired(ctx, d)
		if err != nil {
			return err
		}
		if required {
			return f.store.ScheduleOriginalDelivery(ctx, d)
		}
	}
	return f.store.ScheduleOperationStep(ctx, "operation.run", d.TargetID)
}

// BatchFailureStreak returns the consecutive batch admission failure count.
func BatchFailureStreak(items []app.WorkflowBatchItem) string {
	return batchFailureStreakSince(items, nil)
}

func batchFailureStreakSince(items []app.WorkflowBatchItem, acknowledged map[string]bool) string {
	terminal := make([]app.WorkflowBatchItem, 0, len(items))
	for _, item := range items {
		if item.Status.IsTerminal() && !item.FinishedAt.IsZero() && !acknowledged[item.OperationID.String()] {
			terminal = append(terminal, item)
		}
	}
	sort.Slice(terminal, func(i, j int) bool {
		if terminal[i].FinishedAt.Equal(terminal[j].FinishedAt) {
			return terminal[i].OperationID.String() < terminal[j].OperationID.String()
		}
		return terminal[i].FinishedAt.Before(terminal[j].FinishedAt)
	})
	var code string
	consecutive := 0
	for _, item := range terminal {
		if item.Status != domain.StatusFailed || !strings.HasPrefix(item.FailureCode, "provider:") {
			code, consecutive = "", 0
			continue
		}
		if item.FailureCode == code {
			consecutive++
		} else {
			code, consecutive = item.FailureCode, 1
		}
	}
	if consecutive >= 5 {
		return code
	}
	return ""
}
