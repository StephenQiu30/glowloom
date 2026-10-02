// Package workflow orchestrates persistent script work on the existing flow/media queues.
package workflow

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

// Stable import activity/signal names identify only committed public command facts.
const (
	ImportExecuteActivity   = "script.ExecuteFileImport"
	ImportInterruptActivity = "script.InterruptFileImport"
	ImportCommandSignal     = "script-import-command"
)

// FileImportWorkflow keeps failed/partial/unknown jobs available for explicit commands.
// Control execution joins the actual owning call; signal receipt is never cessation.
func FileImportWorkflow(ctx workflow.Context, initial application.ImportDelivery) (application.ImportJob, error) {
	options := workflow.ActivityOptions{TaskQueue: "media", StartToCloseTimeout: 30 * time.Minute, ScheduleToCloseTimeout: time.Hour, HeartbeatTimeout: 30 * time.Second, WaitForCancellation: true, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 1}}
	workCtx := workflow.WithActivityOptions(ctx, options)
	signals := workflow.GetSignalChannel(ctx, ImportCommandSignal)
	pending := []application.ImportDelivery{initial}
	var job application.ImportJob
	for {
		if len(pending) == 0 {
			var command application.ImportDelivery
			signals.Receive(ctx, &command)
			if command.JobID == initial.JobID && command.OrgID == initial.OrgID && command.ProjectID == initial.ProjectID {
				pending = append(pending, command)
			}
			continue
		}
		d := pending[0]
		pending = pending[1:]
		future := workflow.ExecuteActivity(workCtx, ImportExecuteActivity, d)
		controlFutures := []workflow.Future{}
		selector := workflow.NewSelector(ctx)
		selector.AddFuture(future, func(workflow.Future) {})
		selector.AddReceive(signals, func(ch workflow.ReceiveChannel, _ bool) {
			var command application.ImportDelivery
			ch.Receive(ctx, &command)
			if command.JobID != initial.JobID || command.OrgID != initial.OrgID || command.ProjectID != initial.ProjectID {
				return
			}
			if command.Action == "cancel" {
				controlFutures = append(controlFutures, workflow.ExecuteActivity(workCtx, ImportExecuteActivity, command))
			} else {
				pending = append(pending, command)
			}
		})
		for !future.IsReady() {
			selector.Select(ctx)
		}
		err := future.Get(ctx, &job)
		for _, control := range controlFutures {
			var controlled application.ImportJob
			if control.Get(ctx, &controlled) == nil && controlled.Revision > job.Revision {
				job = controlled
			}
		}
		if err != nil {
			cleanup, _ := workflow.NewDisconnectedContext(ctx)
			cleanup = workflow.WithActivityOptions(cleanup, workflow.ActivityOptions{TaskQueue: "media", StartToCloseTimeout: 10 * time.Second, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 1}})
			var interrupted application.ImportJob
			if failErr := workflow.ExecuteActivity(cleanup, ImportInterruptActivity, d.ImportWork).Get(cleanup, &interrupted); failErr != nil {
				return job, failErr
			}
			if interrupted.Revision > job.Revision {
				job = interrupted
			}
		}
		if job.Status == "succeeded" || job.Status == "cancelled" {
			return job, nil
		}
		if ctx.Err() != nil {
			return job, ctx.Err()
		}
	}
}

// ImportActivities shares one concrete synchronous worker and its durable store.
type ImportActivities struct {
	worker *application.ImportWorker
	store  application.ImportWorkerStore
}

// NewImportActivities injects the real shared worker and exact state/failure owner.
func NewImportActivities(w *application.ImportWorker, s application.ImportWorkerStore) *ImportActivities {
	return &ImportActivities{worker: w, store: s}
}

// ExecuteFileImport keeps heartbeats while the bounded caller still owns actual I/O.
func (a *ImportActivities) ExecuteFileImport(ctx context.Context, d application.ImportDelivery) (application.ImportJob, error) {
	if a == nil || a.worker == nil || a.store == nil {
		return application.ImportJob{}, application.ErrUnavailable
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
				activity.RecordHeartbeat(ctx, d.ImportWork)
			}
		}
	}()
	defer func() { close(stop); wg.Wait() }()
	activity.RecordHeartbeat(ctx, d.ImportWork)
	job, err := a.worker.Run(ctx, d)
	if err != nil {
		return job, temporal.NewNonRetryableApplicationError("script import requires explicit recovery", "ScriptImportRejected", err)
	}
	return job, nil
}

// InterruptFileImport retains uncertain process/object fences and reads exact state.
func (a *ImportActivities) InterruptFileImport(ctx context.Context, work application.ImportWork) (application.ImportJob, error) {
	if a == nil || a.store == nil {
		return application.ImportJob{}, application.ErrUnavailable
	}
	if err := a.store.FailImportWorkflow(ctx, work, time.Now().UTC()); err != nil {
		return application.ImportJob{}, err
	}
	return a.store.ImportWorkState(ctx, work)
}

// RegisterImportWorkflow installs orchestration on the existing flow worker.
func RegisterImportWorkflow(w worker.Worker) { w.RegisterWorkflow(FileImportWorkflow) }

// RegisterImportActivities installs synchronous extraction/control on the media worker.
func RegisterImportActivities(w worker.Worker, a *ImportActivities) {
	w.RegisterActivityWithOptions(a.ExecuteFileImport, activity.RegisterOptions{Name: ImportExecuteActivity})
	w.RegisterActivityWithOptions(a.InterruptFileImport, activity.RegisterOptions{Name: ImportInterruptActivity})
}

// ImportClient is the exact Temporal start/signal boundary used by relay delivery.
type ImportClient interface {
	ExecuteWorkflow(context.Context, client.StartWorkflowOptions, any, ...any) (client.WorkflowRun, error)
	SignalWorkflow(context.Context, string, string, string, any) error
}

// ImportStarter starts or signals one durable job without treating existing runs as new.
type ImportStarter struct{ client ImportClient }

// NewImportStarter injects the existing relay client.
func NewImportStarter(c ImportClient) *ImportStarter { return &ImportStarter{client: c} }

// Deliver explicitly requests AlreadyStarted errors, then signals the saved command.
func (s *ImportStarter) Deliver(ctx context.Context, d application.ImportDelivery) error {
	if s == nil || s.client == nil {
		return application.ErrUnavailable
	}
	id := "script-import/" + d.JobID.String()
	_, err := s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: id, TaskQueue: "flow", WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE_FAILED_ONLY, WorkflowExecutionErrorWhenAlreadyStarted: true}, FileImportWorkflow, d)
	if err == nil {
		return nil
	}
	var exists *serviceerror.WorkflowExecutionAlreadyStarted
	if !errors.As(err, &exists) {
		return err
	}
	return s.client.SignalWorkflow(ctx, id, "", ImportCommandSignal, d)
}
