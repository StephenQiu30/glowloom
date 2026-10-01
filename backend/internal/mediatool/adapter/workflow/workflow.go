// Package workflow adapts local export attempts and cancellation to Temporal.
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

// Activity names are explicit local processing contracts on the media queue.
const (
	RenderActivity  = "mediatool.RenderExport"
	FailureActivity = "mediatool.FailExportWorkflow"
)

// ExportWorkflow executes one frozen attempt and waits for actual cancellation acknowledgment.
func ExportWorkflow(ctx workflow.Context, id application.WorkID) (domain.ExportJob, error) {
	mediaCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{TaskQueue: "media", StartToCloseTimeout: 12 * time.Hour, ScheduleToCloseTimeout: 24 * time.Hour, HeartbeatTimeout: 30 * time.Second, WaitForCancellation: true, RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, MaximumInterval: 30 * time.Second, MaximumAttempts: 3}})
	renderCtx, cancel := workflow.WithCancel(mediaCtx)
	defer cancel()
	future := workflow.ExecuteActivity(renderCtx, RenderActivity, id)
	signals := workflow.GetSignalChannel(ctx, "cancel")
	selector := workflow.NewSelector(ctx)
	selector.AddFuture(future, func(workflow.Future) {})
	selector.AddReceive(signals, func(channel workflow.ReceiveChannel, _ bool) {
		var payload any
		channel.Receive(ctx, &payload)
		cancel()
	})
	selector.Select(ctx)
	var job domain.ExportJob
	err := future.Get(ctx, &job)
	if err != nil && !temporal.IsCanceledError(err) {
		cleanup, _ := workflow.NewDisconnectedContext(ctx)
		cleanup = workflow.WithActivityOptions(cleanup, workflow.ActivityOptions{TaskQueue: "media", StartToCloseTimeout: 10 * time.Second, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 3}})
		if finishErr := workflow.ExecuteActivity(cleanup, FailureActivity, id).Get(cleanup, nil); finishErr != nil {
			return job, finishErr
		}
	}
	return job, err
}

// FailureStore distinguishes orchestration loss from actual worker cessation.
type FailureStore interface {
	FailWorkflow(context.Context, application.WorkID) error
}

// Activities owns the worker and cancellation evidence repository.
type Activities struct {
	worker   *application.Worker
	store    application.WorkerStore
	failures FailureStore
}

// NewActivities injects actual rendering and explicitly fenced state ownership.
func NewActivities(worker *application.Worker, store application.WorkerStore, failures FailureStore) *Activities {
	return &Activities{worker: worker, store: store, failures: failures}
}

// RenderExport heartbeats through an owned goroutine, and records cessation only
// after all worker subprocesses have returned and scratch files have closed.
func (a *Activities) RenderExport(ctx context.Context, id application.WorkID) (job domain.ExportJob, err error) {
	if a == nil || a.worker == nil || a.store == nil {
		return domain.ExportJob{}, application.ErrUnavailable
	}
	id.ExecutionID = uuid.New()
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
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if releaseErr := a.store.Release(releaseCtx, id); releaseErr != nil && !errors.Is(releaseErr, application.ErrConflict) {
			err = errors.Join(err, fmt.Errorf("release export execution: %w", releaseErr))
		}
	}()
	job, err = a.worker.Execute(ctx, id)
	if err == nil {
		return job, nil
	}
	cancelled := errors.Is(err, application.ErrCancelled) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil
	// Storage errors can replay and adopt already-persisted private objects.
	nonretry := cancelled || errors.Is(err, application.ErrConflict) || errors.Is(err, application.ErrInvalidExport)
	if nonretry || activity.GetInfo(ctx).Attempt >= 3 {
		finalize, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		code := "render_failed"
		if errors.Is(err, application.ErrUnavailable) {
			code = "dependency_unavailable"
		}
		if finishErr := a.store.Finish(finalize, id, cancelled, code); finishErr != nil && !errors.Is(finishErr, application.ErrConflict) {
			return job, fmt.Errorf("record export worker cessation: %w", finishErr)
		}
	}
	if cancelled {
		return job, temporal.NewCanceledError("local export stopped")
	}
	if nonretry {
		return job, temporal.NewNonRetryableApplicationError("local export rejected", "MediaExportRejected", err)
	}
	return job, err
}

// FailExportWorkflow leaves unconfirmed cancellation unresolved instead of faking it.
func (a *Activities) FailExportWorkflow(ctx context.Context, id application.WorkID) error {
	return a.failures.FailWorkflow(ctx, id)
}

// RegisterWorkflow installs the orchestration on the existing flow queue.
func RegisterWorkflow(w worker.Worker) { w.RegisterWorkflow(ExportWorkflow) }

// RegisterActivities installs actual media processing on the existing media queue.
func RegisterActivities(w worker.Worker, a *Activities) {
	w.RegisterActivityWithOptions(a.RenderExport, activity.RegisterOptions{Name: RenderActivity})
	w.RegisterActivityWithOptions(a.FailExportWorkflow, activity.RegisterOptions{Name: FailureActivity})
}

// Client is the fixed Temporal delivery surface required by the relay.
type Client interface {
	ExecuteWorkflow(context.Context, client.StartWorkflowOptions, any, ...any) (client.WorkflowRun, error)
	SignalWorkflow(context.Context, string, string, string, any) error
}

// Starter delivers exact attempts without duplicating local workflows.
type Starter struct{ client Client }

// NewStarter injects the relay Temporal client.
func NewStarter(c Client) *Starter { return &Starter{client: c} }
func workflowID(id application.WorkID) string {
	return "media-export/" + id.JobID.String() + "/" + strconv.Itoa(id.Attempt)
}

// Deliver starts a stable attempt before a cancellation signal, covering reordered delivery.
func (s *Starter) Deliver(ctx context.Context, d application.Delivery) error {
	if s == nil || s.client == nil {
		return application.ErrUnavailable
	}
	_, err := s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: workflowID(d.WorkID), TaskQueue: "flow", WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE}, ExportWorkflow, d.WorkID)
	if err != nil {
		var started *serviceerror.WorkflowExecutionAlreadyStarted
		if !errors.As(err, &started) {
			return err
		}
	}
	if d.Action == "cancel" {
		err = s.client.SignalWorkflow(ctx, workflowID(d.WorkID), "", "cancel", nil)
		var missing *serviceerror.NotFound
		if errors.As(err, &missing) {
			return nil
		}
		return err
	}
	return nil
}
