// Package workflow serializes frozen workspace project copying and recovery.
package workflow

import (
	"context"
	"errors"
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

	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// Activity names identify the copy consumer and unacknowledged execution recovery.
const (
	ExecuteCopyActivity   = "workspace.ExecuteProjectCopy"
	InterruptCopyActivity = "workspace.InterruptProjectCopy"
	CopySignal            = "project-copy-command"
)

// ProjectCopyWorkflow serializes one job's concrete execution and waits for explicit commands.
func ProjectCopyWorkflow(ctx workflow.Context, initial application.ProjectCopyWorkID) (domain.ProjectCopyJob, error) {
	signals := workflow.GetSignalChannel(ctx, CopySignal)
	var job domain.ProjectCopyJob
	for {
		var workerString string
		if err := workflow.SideEffect(ctx, func(workflow.Context) any { return uuid.NewString() }).Get(&workerString); err != nil {
			return job, err
		}
		workerID, err := uuid.Parse(workerString)
		if err != nil {
			return job, err
		}
		id := application.ProjectCopyWorkID{OrgID: initial.OrgID, JobID: initial.JobID, WorkerID: workerID}
		activityCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{TaskQueue: "media", StartToCloseTimeout: 2 * time.Hour, ScheduleToCloseTimeout: 2 * time.Hour, HeartbeatTimeout: 30 * time.Second, WaitForCancellation: true, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 1}})
		future := workflow.ExecuteActivity(activityCtx, ExecuteCopyActivity, id)
		pending := false
		selector := workflow.NewSelector(ctx)
		selector.AddFuture(future, func(workflow.Future) {})
		selector.AddReceive(signals, func(channel workflow.ReceiveChannel, _ bool) {
			var command application.ProjectCopyWorkID
			channel.Receive(ctx, &command)
			if command.OrgID == initial.OrgID && command.JobID == initial.JobID {
				pending = true
			}
		})
		for !future.IsReady() {
			selector.Select(ctx)
		}
		err = future.Get(ctx, &job)
		if err != nil {
			recovery, _ := workflow.NewDisconnectedContext(ctx)
			recovery = workflow.WithActivityOptions(recovery, workflow.ActivityOptions{TaskQueue: "media", StartToCloseTimeout: 15 * time.Second, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 3}})
			if interrupted := workflow.ExecuteActivity(recovery, InterruptCopyActivity, id).Get(recovery, nil); interrupted != nil {
				return job, interrupted
			}
			if ctx.Err() != nil {
				return job, err
			}
		}
		if job.Status == "succeeded" || job.Status == "cancelled" {
			return job, nil
		}
		if pending {
			continue
		}
		var command application.ProjectCopyWorkID
		for {
			signals.Receive(ctx, &command)
			if command.OrgID == initial.OrgID && command.JobID == initial.JobID {
				break
			}
		}
	}
}

// ProjectCopyActivities owns bounded physical execution and joined heartbeat lifetime.
type ProjectCopyActivities struct {
	copyWorker *application.ProjectCopyWorker
	store      application.ProjectCopyExecutionStore
}

// NewProjectCopyActivities injects actual owning modules and persistent worker fences.
func NewProjectCopyActivities(copyWorker *application.ProjectCopyWorker, store application.ProjectCopyExecutionStore) *ProjectCopyActivities {
	return &ProjectCopyActivities{copyWorker: copyWorker, store: store}
}

// ExecuteProjectCopy keeps heartbeats until the synchronous byte consumer actually ends.
func (a *ProjectCopyActivities) ExecuteProjectCopy(ctx context.Context, id application.ProjectCopyWorkID) (domain.ProjectCopyJob, error) {
	if a == nil || a.copyWorker == nil || a.store == nil {
		return domain.ProjectCopyJob{}, application.ErrProjectDependencyUnavailable
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
	return a.copyWorker.Execute(ctx, id)
}

// InterruptProjectCopy preserves timeout uncertainty; it never claims stopped bytes.
func (a *ProjectCopyActivities) InterruptProjectCopy(ctx context.Context, id application.ProjectCopyWorkID) error {
	if a == nil || a.store == nil {
		return application.ErrProjectDependencyUnavailable
	}
	return a.store.Interrupt(ctx, id)
}

// RegisterProjectCopyWorkflow installs the persistent copy workflow on the existing flow worker.
func RegisterProjectCopyWorkflow(w worker.Worker) { w.RegisterWorkflow(ProjectCopyWorkflow) }

// RegisterProjectCopyActivities installs concrete copy execution on the existing media worker.
func RegisterProjectCopyActivities(w worker.Worker, a *ProjectCopyActivities) {
	w.RegisterActivityWithOptions(a.ExecuteProjectCopy, activity.RegisterOptions{Name: ExecuteCopyActivity})
	w.RegisterActivityWithOptions(a.InterruptProjectCopy, activity.RegisterOptions{Name: InterruptCopyActivity})
}

// ProjectCopyClient is the relay's exact start/signal boundary.
type ProjectCopyClient interface {
	ExecuteWorkflow(context.Context, client.StartWorkflowOptions, any, ...any) (client.WorkflowRun, error)
	SignalWorkflow(context.Context, string, string, string, any) error
}

// ProjectCopyStarter delivers verified commands to one stable job workflow.
type ProjectCopyStarter struct{ client ProjectCopyClient }

// NewProjectCopyStarter injects the relay's Temporal client.
func NewProjectCopyStarter(c ProjectCopyClient) *ProjectCopyStarter {
	return &ProjectCopyStarter{client: c}
}

// Deliver starts before signaling so a reordered cancel can start safe cleanup.
func (s *ProjectCopyStarter) Deliver(ctx context.Context, d application.ProjectCopyDelivery) error {
	if s == nil || s.client == nil {
		return application.ErrProjectDependencyUnavailable
	}
	id := "project-copy/" + d.JobID.String()
	_, err := s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: id, TaskQueue: "flow", WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE_FAILED_ONLY, WorkflowExecutionErrorWhenAlreadyStarted: true}, ProjectCopyWorkflow, application.ProjectCopyWorkID{OrgID: d.OrgID, JobID: d.JobID})
	if err != nil {
		var exists *serviceerror.WorkflowExecutionAlreadyStarted
		if !errors.As(err, &exists) {
			return err
		}
	}
	if err == nil {
		return nil
	}
	err = s.client.SignalWorkflow(ctx, id, "", CopySignal, application.ProjectCopyWorkID{OrgID: d.OrgID, JobID: d.JobID})
	return err
}
