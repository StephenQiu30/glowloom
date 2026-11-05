// Package task owns bounded execution of frozen media processing attempts.
package task

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// Media tool task kinds identify export, transcription and depth execution.
const (
	Export        = "mediatool.export"
	Transcription = "mediatool.transcription"
	Depth         = "mediatool.depth"
)

func execID(j taskruntime.Job, epoch int64) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(j.Kind+"/"+j.BusinessID+"/"+strconv.FormatInt(epoch, 10)))
}

// ExportStore owns durable export delivery and launch facts.
type ExportStore interface {
	application.WorkerStore
	FailWorkflow(context.Context, application.WorkID) error
}

// ExportHandler executes admitted export work using its original task reference.
func ExportHandler(worker *application.Worker, store ExportStore) taskruntime.Handler {
	return taskruntime.WithReference(12*time.Hour, func(ctx context.Context, j taskruntime.Job, id application.WorkID) (err error) {
		// The current SQL epoch fences all domain writes from the abandoned
		// process; reclaim only the exact previous execution of this task.
		if j.Epoch > 1 {
			id.ExecutionID = execID(j, j.Epoch-1)
			if err := store.Release(ctx, id); err != nil && !errors.Is(err, application.ErrConflict) {
				return err
			}
		}
		id.ExecutionID = execID(j, j.Epoch)
		defer func() {
			stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			if release := store.Release(stop, id); release != nil && !errors.Is(release, application.ErrConflict) {
				err = errors.Join(err, fmt.Errorf("release export: %w", release))
			}
		}()
		_, err = worker.Execute(ctx, id)
		if err == nil || ctx.Err() != nil && !errors.Is(context.Cause(ctx), taskruntime.ErrStopRequested) {
			return err
		}
		cancelled := errors.Is(err, application.ErrCancelled) || errors.Is(context.Cause(ctx), taskruntime.ErrStopRequested)
		if !cancelled && !errors.Is(err, application.ErrConflict) && !errors.Is(err, application.ErrInvalidExport) && j.Failures < 2 {
			return err
		}
		code := "render_failed"
		if errors.Is(err, application.ErrUnavailable) {
			code = "dependency_unavailable"
		}
		if errors.Is(err, application.ErrNoAudio) {
			code = "no_audio_stream"
		}
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		finish := store.Finish(finishCtx, id, cancelled, code)
		if errors.Is(finish, application.ErrConflict) {
			return store.FailWorkflow(finishCtx, id)
		}
		return finish
	})
}

// TranscriptionHandler executes admitted transcription work using its original task reference.
func TranscriptionHandler(worker *application.TranscriptionWorker, store application.TranscriptionWorkerStore) taskruntime.Handler {
	return taskruntime.WithReference(30*time.Minute, func(ctx context.Context, j taskruntime.Job, id application.TranscriptionWorkID) (err error) {
		// The current SQL epoch fences all domain writes from the abandoned
		// process; reclaim only the exact previous execution of this task.
		if j.Epoch > 1 {
			id.ExecutionID = execID(j, j.Epoch-1)
			if err := store.Release(ctx, id); err != nil && !errors.Is(err, application.ErrConflict) {
				return err
			}
		}
		id.ExecutionID = execID(j, j.Epoch)
		defer func() {
			stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			if release := store.Release(stop, id); release != nil && !errors.Is(release, application.ErrConflict) {
				err = errors.Join(err, fmt.Errorf("release transcription: %w", release))
			}
		}()
		_, err = worker.Execute(ctx, id)
		if err == nil || ctx.Err() != nil && !errors.Is(context.Cause(ctx), taskruntime.ErrStopRequested) {
			return err
		}
		uncertain := errors.Is(err, application.ErrInferenceUncertain)
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
			if j.Failures < 2 {
				return err
			}
			code = "dependency_unavailable"
		case errors.Is(context.Cause(ctx), taskruntime.ErrStopRequested), errors.Is(err, application.ErrInvalidTranscription), errors.Is(err, application.ErrCancelled), errors.Is(err, application.ErrConflict):
		default:
			if j.Failures < 2 {
				return err
			}
		}
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		finish := store.Finish(finishCtx, id, !uncertain, code)
		if errors.Is(finish, application.ErrConflict) {
			return store.FailWorkflow(finishCtx, id)
		}
		return finish
	})
}

// DepthHandler executes admitted depth work using its original task reference.
func DepthHandler(worker *application.DepthWorker, store application.DepthWorkerStore) taskruntime.Handler {
	return taskruntime.WithReference(45*time.Minute, func(ctx context.Context, j taskruntime.Job, id application.DepthWorkID) error {
		if j.Epoch > 1 {
			id.ExecutionID = execID(j, j.Epoch-1)
			if err := store.Interrupt(ctx, id); err != nil && !errors.Is(err, application.ErrWorkerBusy) && !errors.Is(err, application.ErrConflict) {
				return err
			}
		}
		id.ExecutionID = execID(j, j.Epoch)
		_, err := worker.Execute(ctx, id)
		if err != nil && ctx.Err() == nil {
			if mark := store.Interrupt(ctx, id); mark != nil && !errors.Is(mark, application.ErrWorkerBusy) && !errors.Is(mark, application.ErrConflict) {
				return mark
			}
		}
		return err
	})
}
