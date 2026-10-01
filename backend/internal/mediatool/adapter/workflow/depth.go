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

	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// Depth activity names identify real execution and timeout fencing separately.
const (
	DepthActivity        = "mediatool.ProcessDepth"
	DepthFailureActivity = "mediatool.InterruptDepth"
)

// DepthWorkflow runs a single command without automatic inference retries.
func DepthWorkflow(ctx workflow.Context, id application.DepthWorkID) (domain.DepthJob, error) {
	if err := workflow.SideEffect(ctx, func(workflow.Context) any { return uuid.NewString() }).Get(&id.ExecutionID); err != nil {
		return domain.DepthJob{}, err
	}
	options := workflow.ActivityOptions{TaskQueue: "media", StartToCloseTimeout: 45 * time.Minute, ScheduleToCloseTimeout: time.Hour, HeartbeatTimeout: 30 * time.Second, WaitForCancellation: true, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 1}}
	run, cancel := workflow.WithCancel(workflow.WithActivityOptions(ctx, options))
	defer cancel()
	future := workflow.ExecuteActivity(run, DepthActivity, id)
	signals := workflow.GetSignalChannel(ctx, "cancel")
	selector := workflow.NewSelector(ctx)
	selector.AddFuture(future, func(workflow.Future) {})
	selector.AddReceive(signals, func(ch workflow.ReceiveChannel, _ bool) { var payload any; ch.Receive(ctx, &payload); cancel() })
	for !future.IsReady() {
		selector.Select(ctx)
	}
	var job domain.DepthJob
	err := future.Get(ctx, &job)
	if err != nil {
		closed, _ := workflow.NewDisconnectedContext(ctx)
		closed = workflow.WithActivityOptions(closed, workflow.ActivityOptions{TaskQueue: "media", StartToCloseTimeout: 20 * time.Second, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 3}})
		if markErr := workflow.ExecuteActivity(closed, DepthFailureActivity, id).Get(closed, nil); markErr != nil {
			return job, markErr
		}
	}
	return job, err
}

// DepthActivities owns physical worker heartbeats and preserves its timeout fence.
type DepthActivities struct {
	worker *application.DepthWorker
	store  application.DepthWorkerStore
}

// NewDepthActivities injects the native consumer and its durable ownership port.
func NewDepthActivities(w *application.DepthWorker, s application.DepthWorkerStore) *DepthActivities {
	return &DepthActivities{worker: w, store: s}
}

// ProcessDepth waits for worker cessation, file cleanup and durable completion.
func (a *DepthActivities) ProcessDepth(ctx context.Context, id application.DepthWorkID) (domain.DepthJob, error) {
	if a == nil || a.worker == nil || a.store == nil {
		return domain.DepthJob{}, application.ErrUnavailable
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
				activity.RecordHeartbeat(ctx, id)
			}
		}
	}()
	defer func() { close(stop); wg.Wait() }()
	activity.RecordHeartbeat(ctx, id)
	job, err := a.worker.Execute(ctx, id)
	if err != nil {
		return job, temporal.NewNonRetryableApplicationError("local depth command failed", "MediaDepthFailed", err)
	}
	return job, nil
}

// InterruptDepth records orchestration loss without declaring a subprocess stopped.
func (a *DepthActivities) InterruptDepth(ctx context.Context, id application.DepthWorkID) error {
	if a == nil || a.store == nil {
		return application.ErrUnavailable
	}
	err := a.store.Interrupt(ctx, id)
	if errors.Is(err, application.ErrWorkerBusy) || errors.Is(err, application.ErrConflict) {
		return nil
	}
	return err
}

// RegisterDepthWorkflow installs native depth orchestration on the flow queue.
func RegisterDepthWorkflow(w worker.Worker) { w.RegisterWorkflow(DepthWorkflow) }

// RegisterDepthActivities installs explicit physical work on the existing media queue.
func RegisterDepthActivities(w worker.Worker, a *DepthActivities) {
	w.RegisterActivityWithOptions(a.ProcessDepth, activity.RegisterOptions{Name: DepthActivity})
	w.RegisterActivityWithOptions(a.InterruptDepth, activity.RegisterOptions{Name: DepthFailureActivity})
}

// DepthStarter delivers exact committed commands to independent fenced consumers.
type DepthStarter struct{ client Client }

// NewDepthStarter injects the existing relay Temporal client.
func NewDepthStarter(c Client) *DepthStarter { return &DepthStarter{client: c} }

func depthWorkflowID(id application.DepthWorkID) string {
	return "media-depth/" + id.JobID.String() + "/" + strconv.Itoa(id.Attempt)
}

// Deliver signals running native work before scheduling cancellation cleanup.
// Explicit AlreadyStarted errors are required: the SDK otherwise returns an old handle.
func (s *DepthStarter) Deliver(ctx context.Context, d application.DepthDelivery) error {
	if s == nil || s.client == nil {
		return application.ErrUnavailable
	}
	if d.Action != "start" && d.Action != "cancel" && d.Action != "reconcile" {
		return application.ErrInvalidDepthInput
	}
	name := depthWorkflowID(d.DepthWorkID)
	if d.Action == "cancel" {
		err := s.client.SignalWorkflow(ctx, name, "", "cancel", nil)
		var missing *serviceerror.NotFound
		if err != nil && !errors.As(err, &missing) {
			return fmt.Errorf("signal exact depth cancellation: %w", err)
		}
	}
	if d.Action != "start" {
		name += "/" + d.Action + "/" + d.RequestID.String()
	}
	_, err := s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: name, TaskQueue: "flow", WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE, WorkflowExecutionErrorWhenAlreadyStarted: true}, DepthWorkflow, d.DepthWorkID)
	var started *serviceerror.WorkflowExecutionAlreadyStarted
	if errors.As(err, &started) {
		return nil
	}
	return err
}
