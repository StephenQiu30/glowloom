// Package workflow delivers durable voice jobs without repeating paid submissions.
package workflow

import (
	"context"
	"errors"
	"time"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

// RunActivity names the physical execution activity.
const RunActivity = "subject.RunVoiceJob"

// InterruptActivity records orchestration loss without a second paid send.
const InterruptActivity = "subject.InterruptVoiceJob"

// SubjectVoiceWorkflow retries only behind the SQL submission fence.
func SubjectVoiceWorkflow(ctx workflow.Context, id application.VoiceWorkID) (application.VoiceJob, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{TaskQueue: "provider", StartToCloseTimeout: 12 * time.Minute, ScheduleToCloseTimeout: 40 * time.Minute, HeartbeatTimeout: 30 * time.Second, RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, MaximumInterval: time.Minute, MaximumAttempts: 3}})
	var job application.VoiceJob
	err := workflow.ExecuteActivity(ctx, RunActivity, id).Get(ctx, &job)
	if err != nil {
		cleanup, _ := workflow.NewDisconnectedContext(ctx)
		cleanup = workflow.WithActivityOptions(cleanup, workflow.ActivityOptions{TaskQueue: "provider", StartToCloseTimeout: 30 * time.Second, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 3}})
		var closed application.VoiceJob
		if finish := workflow.ExecuteActivity(cleanup, InterruptActivity, id).Get(cleanup, &closed); finish != nil {
			return job, finish
		}
		return closed, err
	}
	return job, nil
}

// Activities contains injected physical execution; secrets never enter workflow history.
type Activities struct{ executor *application.VoiceWorker }

// NewActivities injects actual provider and media execution.
func NewActivities(executor *application.VoiceWorker) *Activities {
	return &Activities{executor: executor}
}

// Run keeps worker liveness while the durable job owner executes.
func (a *Activities) Run(ctx context.Context, id application.VoiceWorkID) (application.VoiceJob, error) {
	if a == nil || a.executor == nil {
		return application.VoiceJob{}, application.ErrVoiceUnavailable
	}
	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				activity.RecordHeartbeat(ctx, id)
			}
		}
	}()
	defer func() { close(done); <-exited }()
	activity.RecordHeartbeat(ctx, id)
	return a.executor.Run(ctx, id)
}

// Interrupt preserves complete receipts or records an uncertain send.
func (a *Activities) Interrupt(ctx context.Context, id application.VoiceWorkID) (application.VoiceJob, error) {
	if a == nil || a.executor == nil {
		return application.VoiceJob{}, application.ErrVoiceUnavailable
	}
	return a.executor.Interrupt(ctx, id)
}

// RegisterWorkflow belongs to flow; RegisterActivities belongs to provider workers.
func RegisterWorkflow(w worker.Worker) { w.RegisterWorkflow(SubjectVoiceWorkflow) }

// RegisterActivities installs physical work on the provider worker.
func (a *Activities) RegisterActivities(w worker.Worker) {
	w.RegisterActivityWithOptions(a.Run, activity.RegisterOptions{Name: RunActivity})
	w.RegisterActivityWithOptions(a.Interrupt, activity.RegisterOptions{Name: InterruptActivity})
}

// Executor is the single Temporal start capability consumed by this adapter.
type Executor interface {
	ExecuteWorkflow(context.Context, client.StartWorkflowOptions, interface{}, ...interface{}) (client.WorkflowRun, error)
}

// Starter uses event identity for repeatable job recovery deliveries.
type Starter struct{ client Executor }

// NewStarter injects the shared Temporal client.
func NewStarter(c Executor) *Starter { return &Starter{client: c} }

// Deliver starts a unique durable delivery; replay of its own event is harmless.
func (s *Starter) Deliver(ctx context.Context, d application.VoiceDelivery) error {
	if s == nil || s.client == nil {
		return application.ErrVoiceUnavailable
	}
	_, err := s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: "subject-voice/" + d.JobID.String() + "/" + d.EventID.String(), TaskQueue: "flow", WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE, WorkflowIDConflictPolicy: enums.WORKFLOW_ID_CONFLICT_POLICY_FAIL, WorkflowExecutionErrorWhenAlreadyStarted: true}, SubjectVoiceWorkflow, application.VoiceWorkID{JobID: d.JobID, RecoveryOnly: d.RecoveryOnly})
	var started *serviceerror.WorkflowExecutionAlreadyStarted
	if errors.As(err, &started) {
		return nil
	}
	return err
}
