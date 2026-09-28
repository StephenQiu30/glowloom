package workflow

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

// BatchLaunchInput identifies one child before the provider/project slot is reserved.
type BatchLaunchInput struct {
	BatchID     string `json:"batch_id"`
	OperationID string `json:"operation_id"`
}

// BatchStore is the parent's narrow durable boundary.
type BatchStore interface {
	LoadWorkflowBatch(context.Context, uuid.UUID) (application.WorkflowBatch, error)
	MarkWorkflowBatchRunning(context.Context, uuid.UUID) error
	MarkWorkflowBatchCancelRequested(context.Context, uuid.UUID) error
	AcquireBatchLaunch(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	PauseWorkflowBatch(context.Context, uuid.UUID, string) error
	ResumeWorkflowBatch(context.Context, uuid.UUID) error
	FinishWorkflowBatch(context.Context, uuid.UUID) error
}

// BatchQueuedCanceler closes a child before any provider call and releases its reservation.
type BatchQueuedCanceler interface {
	CancelQueuedBatchOperation(context.Context, uuid.UUID, uuid.UUID) (bool, error)
}

// OperationFinalizer exposes both terminal settlement and queued batch cancellation
// to the flow worker while each Activity receives only the method it needs.
type OperationFinalizer interface {
	Finalizer
	BatchQueuedCanceler
}

// BatchActivities adapt the persistent batch state to Temporal's flow queue.
type BatchActivities struct {
	store    BatchStore
	canceler BatchQueuedCanceler
}

// NewBatchActivities injects the durable batch store and atomic child canceler.
func NewBatchActivities(store BatchStore, canceler BatchQueuedCanceler) *BatchActivities {
	return &BatchActivities{store: store, canceler: canceler}
}

// LoadBatch reloads the selected children for a parent workflow run.
func (a *BatchActivities) LoadBatch(ctx context.Context, batchID string) (application.WorkflowBatch, error) {
	id, err := parseBatchWorkflowID(batchID)
	if err != nil {
		return application.WorkflowBatch{}, err
	}
	loaded, err := a.store.LoadWorkflowBatch(ctx, id)
	return loaded, permanentActivityError(err)
}

// MarkBatchRunning records the confirmed parent transition idempotently.
func (a *BatchActivities) MarkBatchRunning(ctx context.Context, batchID string) error {
	id, err := parseBatchWorkflowID(batchID)
	if err != nil {
		return err
	}
	return permanentActivityError(a.store.MarkWorkflowBatchRunning(ctx, id))
}

// MarkBatchCancelRequested prevents further child admission before cancellation.
func (a *BatchActivities) MarkBatchCancelRequested(ctx context.Context, batchID string) error {
	id, err := parseBatchWorkflowID(batchID)
	if err != nil {
		return err
	}
	return permanentActivityError(a.store.MarkWorkflowBatchCancelRequested(ctx, id))
}

// CancelQueuedBatchChild settles one child only while no provider call exists.
func (a *BatchActivities) CancelQueuedBatchChild(ctx context.Context, input BatchLaunchInput) (bool, error) {
	batchID, err := parseBatchWorkflowID(input.BatchID)
	if err != nil {
		return false, err
	}
	operationID, err := parseBatchWorkflowID(input.OperationID)
	if err != nil {
		return false, err
	}
	cancelled, err := a.canceler.CancelQueuedBatchOperation(ctx, batchID, operationID)
	return cancelled, permanentActivityError(err)
}

// AcquireBatchLaunch reserves one durable child concurrency slot.
func (a *BatchActivities) AcquireBatchLaunch(ctx context.Context, input BatchLaunchInput) (bool, error) {
	batchID, err := parseBatchWorkflowID(input.BatchID)
	if err != nil {
		return false, err
	}
	operationID, err := parseBatchWorkflowID(input.OperationID)
	if err != nil {
		return false, err
	}
	acquired, err := a.store.AcquireBatchLaunch(ctx, batchID, operationID)
	return acquired, permanentActivityError(err)
}

// PauseBatch persists the provider breaker before another child can enter.
func (a *BatchActivities) PauseBatch(ctx context.Context, batchID, reason string) error {
	id, err := parseBatchWorkflowID(batchID)
	if err != nil {
		return err
	}
	return permanentActivityError(a.store.PauseWorkflowBatch(ctx, id, reason))
}

// ResumeBatch clears the admission guard after the parent receives resume.
func (a *BatchActivities) ResumeBatch(ctx context.Context, batchID string) error {
	id, err := parseBatchWorkflowID(batchID)
	if err != nil {
		return err
	}
	return permanentActivityError(a.store.ResumeWorkflowBatch(ctx, id))
}

// FinishBatch writes the terminal summary and its Outbox event once.
func (a *BatchActivities) FinishBatch(ctx context.Context, batchID string) error {
	id, err := parseBatchWorkflowID(batchID)
	if err != nil {
		return err
	}
	return permanentActivityError(a.store.FinishWorkflowBatch(ctx, id))
}

func parseBatchWorkflowID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil || id.String() != value {
		return uuid.Nil, ErrInvalidOperationInput
	}
	return id, nil
}

// BatchWorkflow launches only selected, reserved child operations. The database
// Activity enforces capacity across parent workflows; workflow state tracks only
// this run's child futures and reloads durable status after ContinueAsNew.
func BatchWorkflow(ctx workflow.Context, input BatchInput) error {
	batchID, err := parseBatchWorkflowID(input.BatchID)
	if err != nil {
		return err
	}
	flowCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		TaskQueue: "flow", StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, MaximumInterval: time.Minute},
	})
	loaded, err := loadBatch(flowCtx, batchID)
	if err != nil {
		return err
	}
	if loaded.Batch.Status == domain.BatchStatusFinished || loaded.Batch.Status == domain.BatchStatusCancelled {
		return nil
	}
	if loaded.Batch.Status == domain.BatchStatusConfirmed {
		if err := workflow.ExecuteActivity(flowCtx, "flow.MarkBatchRunning", input.BatchID).Get(flowCtx, nil); err != nil {
			return err
		}
	}
	started := make(map[uuid.UUID]bool)
	acknowledged := make(map[string]bool, len(input.AcknowledgedTerminalIDs))
	for _, id := range input.AcknowledgedTerminalIDs {
		acknowledged[id] = true
	}
	cancelSignalled := make(map[string]bool, len(input.CancelSignalledIDs))
	for _, id := range input.CancelSignalledIDs {
		cancelSignalled[id] = true
	}
	startedCount, completedCount, inFlight := 0, 0, 0
	historyVersion := workflow.GetVersion(ctx, "batch-history-rollover", workflow.DefaultVersion, 1)
	childErrors := make(map[uuid.UUID]error)
	for {
		loaded, err = loadBatch(flowCtx, batchID)
		if err != nil {
			return err
		}
		if loaded.Batch.Status == domain.BatchStatusFinished || loaded.Batch.Status == domain.BatchStatusCancelled {
			return nil
		}
		for _, item := range loaded.Items {
			if childErr := childErrors[item.OperationID]; childErr != nil {
				if item.Status.IsTerminal() {
					delete(childErrors, item.OperationID)
				} else {
					return fmt.Errorf("batch child workflow: %w", childErr)
				}
			}
		}
		var cancelSignal struct{}
		if workflow.GetSignalChannel(ctx, "cancel").ReceiveAsync(&cancelSignal) && !loaded.CancelRequested {
			if err := workflow.ExecuteActivity(flowCtx, "flow.MarkBatchCancelRequested", input.BatchID).Get(flowCtx, nil); err != nil {
				return err
			}
			continue
		}
		if loaded.CancelRequested {
			cancelledAny := false
			for _, item := range loaded.Items {
				if item.Status.IsTerminal() {
					continue
				}
				if item.Status == domain.StatusConfirmed || item.Status == domain.StatusSubmitting ||
					item.Status == domain.StatusCancelling {
					var cancelled bool
					if err := workflow.ExecuteActivity(flowCtx, "flow.CancelQueuedBatchChild", BatchLaunchInput{
						BatchID: input.BatchID, OperationID: item.OperationID.String(),
					}).Get(flowCtx, &cancelled); err != nil {
						return err
					}
					if cancelled {
						cancelledAny = true
						continue
					}
				}
				childID := item.OperationID.String()
				if cancelSignalled[childID] {
					continue
				}
				if err := workflow.SignalExternalWorkflow(ctx, "operation/"+childID, "", "cancel", nil).Get(ctx, nil); err != nil {
					latest, loadErr := loadBatch(flowCtx, batchID)
					if loadErr != nil {
						return loadErr
					}
					if batchItemTerminal(latest.Items, item.OperationID) {
						continue
					}
					return fmt.Errorf("signal batch child cancellation %s: %w", childID, err)
				}
				cancelSignalled[childID] = true
				input.CancelSignalledIDs = append(input.CancelSignalledIDs, childID)
			}
			if cancelledAny {
				continue
			}
		}
		if !loaded.CancelRequested && loaded.PausedReason == "" && hasNonterminalBatchItem(loaded.Items) {
			if code := batchFailureStreakSince(loaded.Items, acknowledged); code != "" {
				if err := workflow.ExecuteActivity(flowCtx, "flow.PauseBatch", input.BatchID, code).Get(flowCtx, nil); err != nil {
					return err
				}
				continue
			}
		}
		if loaded.PausedReason != "" && !loaded.CancelRequested {
			var signal struct{}
			if workflow.GetSignalChannel(ctx, "resume").ReceiveAsync(&signal) {
				if err := workflow.ExecuteActivity(flowCtx, "flow.ResumeBatch", input.BatchID).Get(flowCtx, nil); err != nil {
					return err
				}
				for _, item := range loaded.Items {
					if item.Status.IsTerminal() {
						id := item.OperationID.String()
						if !acknowledged[id] {
							acknowledged[id] = true
							input.AcknowledgedTerminalIDs = append(input.AcknowledgedTerminalIDs, id)
						}
					}
				}
				continue
			}
		}
		remaining, launched := 0, 0
		for _, item := range loaded.Items {
			if item.Status.IsTerminal() {
				continue
			}
			remaining++
			if item.Status != domain.StatusConfirmed || started[item.OperationID] ||
				loaded.PausedReason != "" || loaded.CancelRequested || startedCount >= 100 {
				continue
			}
			var admitted bool
			if err := workflow.ExecuteActivity(flowCtx, "flow.AcquireBatchLaunch", BatchLaunchInput{
				BatchID: input.BatchID, OperationID: item.OperationID.String(),
			}).Get(flowCtx, &admitted); err != nil {
				return err
			}
			if !admitted {
				continue
			}
			childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
				WorkflowID: "operation/" + item.OperationID.String(), TaskQueue: "flow",
				ParentClosePolicy:     enums.PARENT_CLOSE_POLICY_ABANDON,
				WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
			})
			child := workflow.ExecuteChildWorkflow(childCtx, OperationWorkflow, OperationInput{
				OperationID: item.OperationID.String(), BatchID: input.BatchID,
			})
			var execution workflow.Execution
			if err := child.GetChildWorkflowExecution().Get(ctx, &execution); err != nil {
				if !alreadyStartedChild(err) {
					return fmt.Errorf("start batch child %s: %w", item.OperationID, err)
				}
			} else {
				inFlight++
				operationID := item.OperationID
				workflow.Go(ctx, func(childCtx workflow.Context) {
					if err := child.Get(childCtx, nil); err != nil && !alreadyStartedChild(err) {
						childErrors[operationID] = err
					}
					inFlight--
					completedCount++
				})
			}
			started[item.OperationID] = true
			startedCount++
			launched++
		}
		if remaining == 0 && inFlight == 0 {
			return workflow.ExecuteActivity(flowCtx, "flow.FinishBatch", input.BatchID).Get(flowCtx, nil)
		}
		info := workflow.GetInfo(ctx)
		rollHistory := historyVersion >= 1 && (info.GetContinueAsNewSuggested() ||
			info.GetCurrentHistoryLength() >= 10_000 || info.GetCurrentHistorySize() >= 8*1024*1024)
		if (startedCount >= 100 || rollHistory) && remaining > 0 {
			return workflow.NewContinueAsNewError(ctx, BatchWorkflow, input)
		}
		if launched > 0 {
			continue
		}
		if completedCount > 0 {
			completedCount = 0
			continue
		}
		if _, err := workflow.AwaitWithTimeout(ctx, 5*time.Second, func() bool {
			return completedCount > 0 || len(childErrors) > 0 || (remaining == 0 && inFlight == 0)
		}); err != nil {
			return err
		}
		completedCount = 0
	}
}

func batchItemTerminal(items []application.WorkflowBatchItem, operationID uuid.UUID) bool {
	for _, item := range items {
		if item.OperationID == operationID {
			return item.Status.IsTerminal()
		}
	}
	return false
}

func alreadyStartedChild(err error) bool {
	var serverError *serviceerror.WorkflowExecutionAlreadyStarted
	return temporal.IsWorkflowExecutionAlreadyStartedError(err) || errors.As(err, &serverError)
}

// BatchFailureStreak returns a normalized provider error after five latest
// settled children fail with that code. Nonterminal children are ignored.
func BatchFailureStreak(items []application.WorkflowBatchItem) string {
	return batchFailureStreakSince(items, nil)
}

func batchFailureStreakSince(items []application.WorkflowBatchItem, acknowledged map[string]bool) string {
	terminal := make([]application.WorkflowBatchItem, 0, len(items))
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

func hasNonterminalBatchItem(items []application.WorkflowBatchItem) bool {
	for _, item := range items {
		if !item.Status.IsTerminal() {
			return true
		}
	}
	return false
}

func loadBatch(ctx workflow.Context, batchID uuid.UUID) (application.WorkflowBatch, error) {
	var loaded application.WorkflowBatch
	if err := workflow.ExecuteActivity(ctx, "flow.LoadBatch", batchID.String()).Get(ctx, &loaded); err != nil {
		return application.WorkflowBatch{}, err
	}
	if loaded.Batch.ID != batchID || loaded.Batch.Validate() != nil ||
		len(loaded.Items) != int(loaded.Batch.TotalCount) {
		return application.WorkflowBatch{}, ErrInvalidOperationInput
	}
	return loaded, nil
}

// RegisterBatch installs the parent only after its durable finish, cancellation,
// and recovery paths are implemented and verified together.
func RegisterBatch(w worker.Worker, activities *BatchActivities) {
	w.RegisterWorkflowWithOptions(BatchWorkflow, workflow.RegisterOptions{Name: "BatchWorkflow"})
	w.RegisterActivityWithOptions(activities.LoadBatch, activity.RegisterOptions{Name: "flow.LoadBatch"})
	w.RegisterActivityWithOptions(activities.MarkBatchRunning, activity.RegisterOptions{Name: "flow.MarkBatchRunning"})
	w.RegisterActivityWithOptions(activities.MarkBatchCancelRequested, activity.RegisterOptions{Name: "flow.MarkBatchCancelRequested"})
	w.RegisterActivityWithOptions(activities.CancelQueuedBatchChild, activity.RegisterOptions{Name: "flow.CancelQueuedBatchChild"})
	w.RegisterActivityWithOptions(activities.AcquireBatchLaunch, activity.RegisterOptions{Name: "flow.AcquireBatchLaunch"})
	w.RegisterActivityWithOptions(activities.PauseBatch, activity.RegisterOptions{Name: "flow.PauseBatch"})
	w.RegisterActivityWithOptions(activities.ResumeBatch, activity.RegisterOptions{Name: "flow.ResumeBatch"})
	w.RegisterActivityWithOptions(activities.FinishBatch, activity.RegisterOptions{Name: "flow.FinishBatch"})
}
