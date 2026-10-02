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

// Purge activities separate physical work from orchestration loss fencing.
const (
	PurgeActivity          = "media.ProcessPurge"
	PurgeInterruptActivity = "media.InterruptPurge"
)

// PurgeWorkflowConfig is internal queue configuration, never a public command.
type PurgeWorkflowConfig struct {
	ActivityTaskQueue string
}

// PurgeWorkflow runs one saved attempt without automatic object removal retries.
func PurgeWorkflow(ctx workflow.Context, work application.PurgeWorkID, config PurgeWorkflowConfig) (domain.PurgeJob, error) {
	if err := workflow.SideEffect(ctx, func(workflow.Context) any { return uuid.NewString() }).Get(&work.ExecutionID); err != nil {
		return domain.PurgeJob{}, err
	}
	queue := config.ActivityTaskQueue
	if queue == "" {
		queue = "media"
	}
	options := workflow.ActivityOptions{TaskQueue: queue, StartToCloseTimeout: 2 * time.Hour, ScheduleToCloseTimeout: 3 * time.Hour, HeartbeatTimeout: 30 * time.Second, WaitForCancellation: true, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 1}}
	run, cancel := workflow.WithCancel(workflow.WithActivityOptions(ctx, options))
	defer cancel()
	future := workflow.ExecuteActivity(run, PurgeActivity, work)
	signals := workflow.GetSignalChannel(ctx, "cancel")
	selector := workflow.NewSelector(ctx)
	selector.AddFuture(future, func(workflow.Future) {})
	selector.AddReceive(signals, func(channel workflow.ReceiveChannel, _ bool) { var value any; channel.Receive(ctx, &value); cancel() })
	for !future.IsReady() {
		selector.Select(ctx)
	}
	var result domain.PurgeJob
	err := future.Get(ctx, &result)
	if err != nil {
		closed, _ := workflow.NewDisconnectedContext(ctx)
		closed = workflow.WithActivityOptions(closed, workflow.ActivityOptions{TaskQueue: queue, StartToCloseTimeout: 20 * time.Second, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 3}})
		if markErr := workflow.ExecuteActivity(closed, PurgeInterruptActivity, work).Get(closed, nil); markErr != nil {
			return result, errors.Join(err, markErr)
		}
	}
	return result, err
}

// PurgeInterruption is the consumer-defined durable orchestration loss port.
type PurgeInterruption interface {
	InterruptPurge(context.Context, application.PurgeWorkID) error
}

// PurgeActivities owns and joins its Temporal heartbeat around physical work.
type PurgeActivities struct {
	worker *application.PurgeWorker
	store  PurgeInterruption
}

// NewPurgeActivities explicitly injects real physical work and exact fencing.
func NewPurgeActivities(worker *application.PurgeWorker, store PurgeInterruption) *PurgeActivities {
	return &PurgeActivities{worker: worker, store: store}
}

// ProcessPurge waits for object calls, SQL cessation and exact object absence proof.
func (a *PurgeActivities) ProcessPurge(ctx context.Context, work application.PurgeWorkID) (domain.PurgeJob, error) {
	if a == nil || a.worker == nil || a.store == nil {
		return domain.PurgeJob{}, application.ErrUnavailable
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
		return job, temporal.NewNonRetryableApplicationError("media purge stopped", "MediaPurgeFailed", err)
	}
	return job, nil
}

// InterruptPurge records loss without declaring any physical caller stopped.
func (a *PurgeActivities) InterruptPurge(ctx context.Context, work application.PurgeWorkID) error {
	if a == nil || a.store == nil {
		return application.ErrUnavailable
	}
	err := a.store.InterruptPurge(ctx, work)
	if errors.Is(err, domain.ErrPurgeConflict) {
		return nil
	}
	return err
}

// RegisterPurgeWorkflow installs orchestration on the existing flow queue.
func RegisterPurgeWorkflow(worker worker.Worker) { worker.RegisterWorkflow(PurgeWorkflow) }

// RegisterPurgeActivities installs actual private object work on the media queue.
func RegisterPurgeActivities(worker worker.Worker, activities *PurgeActivities) {
	worker.RegisterActivityWithOptions(activities.ProcessPurge, activity.RegisterOptions{Name: PurgeActivity})
	worker.RegisterActivityWithOptions(activities.InterruptPurge, activity.RegisterOptions{Name: PurgeInterruptActivity})
}

// PurgeClient is the exact delivery surface used by the existing relay.
type PurgeClient interface {
	ExecuteWorkflow(context.Context, client.StartWorkflowOptions, any, ...any) (client.WorkflowRun, error)
	SignalWorkflow(context.Context, string, string, string, any) error
}

// PurgeStarter delivers only already proven permanent commands.
type PurgeStarter struct{ client PurgeClient }

// NewPurgeStarter injects the existing relay Temporal client.
func NewPurgeStarter(client PurgeClient) *PurgeStarter {
	return &PurgeStarter{client: client}
}

// Deliver signals the original attempt before starting exact cancellation cleanup.
func (s *PurgeStarter) Deliver(ctx context.Context, delivery application.PurgeDelivery) error {
	if s == nil || s.client == nil {
		return application.ErrUnavailable
	}
	if delivery.Action != "create" && delivery.Action != "cancel" && delivery.Action != "reconcile" {
		return domain.ErrInvalidLibrary
	}
	name := "media-purge/" + delivery.JobID.String() + "/" + strconv.Itoa(delivery.Attempt)
	if delivery.Action == "cancel" {
		err := s.client.SignalWorkflow(ctx, name, "", "cancel", nil)
		var missing *serviceerror.NotFound
		if err != nil && !errors.As(err, &missing) {
			return fmt.Errorf("signal exact purge cancellation: %w", err)
		}
	}
	if delivery.Action != "create" {
		name += "/" + delivery.Action + "/" + delivery.RequestID.String()
	}
	_, err := s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: name, TaskQueue: "flow", WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE, WorkflowExecutionErrorWhenAlreadyStarted: true}, PurgeWorkflow, delivery.PurgeWorkID, PurgeWorkflowConfig{ActivityTaskQueue: "media"})
	var started *serviceerror.WorkflowExecutionAlreadyStarted
	if errors.As(err, &started) {
		return nil
	}
	return err
}
