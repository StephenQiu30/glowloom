package workflow

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// Transfer activities separate physical work from orchestration loss fencing.
const (
	TransferActivity          = "media.ProcessTransfer"
	TransferInterruptActivity = "media.InterruptTransfer"
)

// TransferWorkflowConfig is internal queue configuration, never a public command.
type TransferWorkflowConfig struct {
	ActivityTaskQueue string
}

// TransferWorkflow runs one saved attempt without automatic object write retries.
func TransferWorkflow(ctx workflow.Context, work application.TransferWorkID, config TransferWorkflowConfig) (domain.TransferJob, error) {
	if err := workflow.SideEffect(ctx, func(workflow.Context) any { return uuid.NewString() }).Get(&work.ExecutionID); err != nil {
		return domain.TransferJob{}, err
	}
	queue := config.ActivityTaskQueue
	if queue == "" {
		queue = "media"
	}
	options := workflow.ActivityOptions{TaskQueue: queue, StartToCloseTimeout: 2 * time.Hour, ScheduleToCloseTimeout: 3 * time.Hour, HeartbeatTimeout: 30 * time.Second, WaitForCancellation: true, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 1}}
	run, cancel := workflow.WithCancel(workflow.WithActivityOptions(ctx, options))
	defer cancel()
	future := workflow.ExecuteActivity(run, TransferActivity, work)
	signals := workflow.GetSignalChannel(ctx, "cancel")
	selector := workflow.NewSelector(ctx)
	selector.AddFuture(future, func(workflow.Future) {})
	selector.AddReceive(signals, func(channel workflow.ReceiveChannel, _ bool) { var value any; channel.Receive(ctx, &value); cancel() })
	for !future.IsReady() {
		selector.Select(ctx)
	}
	var result domain.TransferJob
	err := future.Get(ctx, &result)
	if err != nil {
		closed, _ := workflow.NewDisconnectedContext(ctx)
		closed = workflow.WithActivityOptions(closed, workflow.ActivityOptions{TaskQueue: queue, StartToCloseTimeout: 20 * time.Second, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 3}})
		if markErr := workflow.ExecuteActivity(closed, TransferInterruptActivity, work).Get(closed, nil); markErr != nil {
			return result, errors.Join(err, markErr)
		}
	}
	return result, err
}

// TransferInterruption is the consumer-defined durable orchestration loss port.
type TransferInterruption interface {
	InterruptTransfer(context.Context, application.TransferWorkID) error
}

// TransferActivities owns and joins its Temporal heartbeat around physical work.
type TransferActivities struct {
	worker *application.TransferWorker
	store  TransferInterruption
}

// NewTransferActivities explicitly injects real physical work and exact fencing.
func NewTransferActivities(worker *application.TransferWorker, store TransferInterruption) *TransferActivities {
	return &TransferActivities{worker: worker, store: store}
}

// ProcessTransfer waits for object calls, file cleanup and SQL cessation proof.
func (a *TransferActivities) ProcessTransfer(ctx context.Context, work application.TransferWorkID) (domain.TransferJob, error) {
	if a == nil || a.worker == nil || a.store == nil {
		return domain.TransferJob{}, application.ErrUnavailable
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				activity.RecordHeartbeat(ctx, work)
			}
		}
	}()
	defer func() { close(stop); wg.Wait() }()
	activity.RecordHeartbeat(ctx, work)
	job, err := a.worker.Execute(ctx, work)
	if err != nil {
		return job, temporal.NewNonRetryableApplicationError("media transfer stopped", "MediaTransferFailed", err)
	}
	return job, nil
}

// InterruptTransfer records loss without declaring any physical caller stopped.
func (a *TransferActivities) InterruptTransfer(ctx context.Context, work application.TransferWorkID) error {
	if a == nil || a.store == nil {
		return application.ErrUnavailable
	}
	err := a.store.InterruptTransfer(ctx, work)
	if errors.Is(err, domain.ErrTransferConflict) {
		return nil
	}
	return err
}

// RegisterTransferWorkflow installs orchestration on the existing flow queue.
func RegisterTransferWorkflow(worker worker.Worker) { worker.RegisterWorkflow(TransferWorkflow) }

// RegisterTransferActivities installs actual private object work on the media queue.
func RegisterTransferActivities(worker worker.Worker, activities *TransferActivities) {
	worker.RegisterActivityWithOptions(activities.ProcessTransfer, activity.RegisterOptions{Name: TransferActivity})
	worker.RegisterActivityWithOptions(activities.InterruptTransfer, activity.RegisterOptions{Name: TransferInterruptActivity})
}

// TransferClient is the exact delivery surface used by the existing relay.
type TransferClient interface {
	ExecuteWorkflow(context.Context, client.StartWorkflowOptions, any, ...any) (client.WorkflowRun, error)
	SignalWorkflow(context.Context, string, string, string, any) error
}

// TransferStarter delivers only already proven permanent commands.
type TransferStarter struct{ client TransferClient }

// NewTransferStarter injects the existing relay Temporal client.
func NewTransferStarter(client TransferClient) *TransferStarter {
	return &TransferStarter{client: client}
}

// Deliver signals the original attempt before starting exact cancellation cleanup.
func (s *TransferStarter) Deliver(ctx context.Context, delivery application.TransferDelivery) error {
	if s == nil || s.client == nil {
		return application.ErrUnavailable
	}
	if delivery.Action != "start" && delivery.Action != "cancel" && delivery.Action != "reconcile" {
		return domain.ErrInvalidLibrary
	}
	name := "media-transfer/" + delivery.JobID.String() + "/" + strconv.Itoa(delivery.Attempt)
	if delivery.Action == "cancel" {
		err := s.client.SignalWorkflow(ctx, name, "", "cancel", nil)
		var missing *serviceerror.NotFound
		if err != nil && !errors.As(err, &missing) {
			return fmt.Errorf("signal exact transfer cancellation: %w", err)
		}
	}
	if delivery.Action != "start" {
		name += "/" + delivery.Action + "/" + delivery.RequestID.String()
	}
	_, err := s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: name, TaskQueue: "flow", WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE, WorkflowExecutionErrorWhenAlreadyStarted: true}, TransferWorkflow, delivery.TransferWorkID, TransferWorkflowConfig{ActivityTaskQueue: "media"})
	var started *serviceerror.WorkflowExecutionAlreadyStarted
	if errors.As(err, &started) {
		return nil
	}
	return err
}
