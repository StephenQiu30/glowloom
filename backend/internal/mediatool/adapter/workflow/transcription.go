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

// Speech activity names preserve independent native inference payloads.
const (
	TranscribeActivity           = "mediatool.TranscribeSpeech"
	TranscriptionFailureActivity = "mediatool.FailTranscriptionWorkflow"
)

// TranscriptionWorkflow waits for actual HTTP termination after a soft cancel.
// The committed control command is authoritative; its signal must not close the
// submitted native connection and mistake client detachment for cessation.
func TranscriptionWorkflow(ctx workflow.Context, id application.TranscriptionWorkID) (domain.TranscriptionJob, error) {
	mediaCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{TaskQueue: "media", StartToCloseTimeout: application.TranscriptionActivityTimeout, ScheduleToCloseTimeout: time.Hour, HeartbeatTimeout: 30 * time.Second, WaitForCancellation: true, RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, MaximumInterval: 30 * time.Second, MaximumAttempts: 3}})
	future := workflow.ExecuteActivity(mediaCtx, TranscribeActivity, id)
	signals := workflow.GetSignalChannel(ctx, "cancel")
	selector := workflow.NewSelector(ctx)
	selector.AddFuture(future, func(workflow.Future) {})
	selector.AddReceive(signals, func(channel workflow.ReceiveChannel, _ bool) { var payload any; channel.Receive(ctx, &payload) })
	for !future.IsReady() {
		selector.Select(ctx)
	}
	var job domain.TranscriptionJob
	err := future.Get(ctx, &job)
	if err != nil {
		cleanup, _ := workflow.NewDisconnectedContext(ctx)
		cleanup = workflow.WithActivityOptions(cleanup, workflow.ActivityOptions{TaskQueue: "media", StartToCloseTimeout: 10 * time.Second, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 3}})
		if finishErr := workflow.ExecuteActivity(cleanup, TranscriptionFailureActivity, id).Get(cleanup, nil); finishErr != nil {
			return job, finishErr
		}
	}
	return job, err
}

// TranscriptionActivities owns physical work and explicit completion evidence.
type TranscriptionActivities struct {
	worker *application.TranscriptionWorker
	store  application.TranscriptionWorkerStore
}

// NewTranscriptionActivities injects actual speech execution and durable fences.
func NewTranscriptionActivities(worker *application.TranscriptionWorker, store application.TranscriptionWorkerStore) *TranscriptionActivities {
	return &TranscriptionActivities{worker: worker, store: store}
}

// TranscribeSpeech keeps heartbeats while soft cancellation waits for native completion.
func (a *TranscriptionActivities) TranscribeSpeech(ctx context.Context, id application.TranscriptionWorkID) (job domain.TranscriptionJob, err error) {
	if a == nil || a.worker == nil || a.store == nil {
		return job, application.ErrUnavailable
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
		release, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if releaseErr := a.store.Release(release, id); releaseErr != nil && !errors.Is(releaseErr, application.ErrConflict) {
			err = errors.Join(err, fmt.Errorf("release transcription owner: %w", releaseErr))
		}
	}()
	job, err = a.worker.Execute(ctx, id)
	if err == nil {
		return job, nil
	}
	uncertain := errors.Is(err, application.ErrInferenceUncertain)
	cancelled := errors.Is(err, application.ErrCancelled) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil
	rejected := uncertain || cancelled || errors.Is(err, application.ErrInvalidTranscription) || errors.Is(err, domain.ErrInvalidTranscript) || errors.Is(err, application.ErrNoAudio) || errors.Is(err, application.ErrNoSpeech) || errors.Is(err, application.ErrConflict)
	if rejected || activity.GetInfo(ctx).Attempt >= 3 {
		finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		code := "transcription_failed"
		switch {
		case uncertain:
			code = "inference_unknown"
		case errors.Is(err, application.ErrNoAudio):
			code = "no_audio_stream"
		case errors.Is(err, application.ErrNoSpeech):
			code = "no_speech"
		case errors.Is(err, domain.ErrInvalidTranscript):
			code = "invalid_result"
		case errors.Is(err, application.ErrUnavailable):
			code = "dependency_unavailable"
		}
		if finishErr := a.store.Finish(finish, id, !uncertain, code); finishErr != nil && !errors.Is(finishErr, application.ErrConflict) {
			return job, finishErr
		}
	}
	if cancelled {
		return job, temporal.NewCanceledError("local speech preparation stopped")
	}
	if rejected {
		return job, temporal.NewNonRetryableApplicationError("local speech inference rejected", "MediaTranscriptionRejected", err)
	}
	return job, err
}

// FailTranscriptionWorkflow preserves active or uncertain remote work for reconciliation.
func (a *TranscriptionActivities) FailTranscriptionWorkflow(ctx context.Context, id application.TranscriptionWorkID) error {
	return a.store.FailWorkflow(ctx, id)
}

// RegisterTranscriptionWorkflow installs speech orchestration on the existing flow queue.
func RegisterTranscriptionWorkflow(w worker.Worker) { w.RegisterWorkflow(TranscriptionWorkflow) }

// RegisterTranscriptionActivities installs actual speech consumers on the existing media queue.
func RegisterTranscriptionActivities(w worker.Worker, a *TranscriptionActivities) {
	w.RegisterActivityWithOptions(a.TranscribeSpeech, activity.RegisterOptions{Name: TranscribeActivity})
	w.RegisterActivityWithOptions(a.FailTranscriptionWorkflow, activity.RegisterOptions{Name: TranscriptionFailureActivity})
}

// TranscriptionStarter delivers only exact committed attempts, preserving duplicate safety.
type TranscriptionStarter struct{ client Client }

// NewTranscriptionStarter injects the existing relay Temporal client.
func NewTranscriptionStarter(c Client) *TranscriptionStarter { return &TranscriptionStarter{client: c} }
func transcriptionWorkflowID(id application.TranscriptionWorkID) string {
	return "media-transcription/" + id.JobID.String() + "/" + strconv.Itoa(id.Attempt)
}

// Deliver starts the exact attempt before cancellation, covering reordered outbox delivery.
func (s *TranscriptionStarter) Deliver(ctx context.Context, d application.TranscriptionDelivery) error {
	if s == nil || s.client == nil {
		return application.ErrUnavailable
	}
	_, err := s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: transcriptionWorkflowID(d.TranscriptionWorkID), TaskQueue: "flow", WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE}, TranscriptionWorkflow, d.TranscriptionWorkID)
	if err != nil {
		var started *serviceerror.WorkflowExecutionAlreadyStarted
		if !errors.As(err, &started) {
			return err
		}
	}
	if d.Action == "cancel" {
		err = s.client.SignalWorkflow(ctx, transcriptionWorkflowID(d.TranscriptionWorkID), "", "cancel", nil)
		var missing *serviceerror.NotFound
		if errors.As(err, &missing) {
			return nil
		}
		return err
	}
	return nil
}
