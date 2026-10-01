package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"time"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// TranscriptionWorker owns speech staging, actual inference and private draft recovery.
type TranscriptionWorker struct {
	store        TranscriptionWorkerStore
	objects      PrivateObjects
	preprocessor AudioPreprocessor
	transcriber  Transcriber
}

// NewTranscriptionWorker injects exactly the existing storage and local speech consumers.
func NewTranscriptionWorker(store TranscriptionWorkerStore, objects PrivateObjects, preprocessor AudioPreprocessor, transcriber Transcriber) *TranscriptionWorker {
	return &TranscriptionWorker{store: store, objects: objects, preprocessor: preprocessor, transcriber: transcriber}
}

type transcriptArtifact struct {
	Version      int               `json:"version"`
	JobID        string            `json:"job_id"`
	Attempt      int               `json:"attempt"`
	SourceSHA256 string            `json:"source_sha256"`
	Draft        domain.Transcript `json:"draft"`
}

// Execute waits for actual native completion after soft cancellation. It cannot
// turn a disconnected or timed-out HTTP call into confirmed remote cessation.
func (w *TranscriptionWorker) Execute(ctx context.Context, id TranscriptionWorkID) (domain.TranscriptionJob, error) {
	if w == nil || w.store == nil || w.objects == nil || w.preprocessor == nil || w.transcriber == nil {
		return domain.TranscriptionJob{}, ErrUnavailable
	}
	work, err := w.store.Claim(ctx, id)
	if err != nil {
		return domain.TranscriptionJob{}, err
	}
	if work.Job.Status == domain.TranscriptionSucceeded {
		return work.Job, nil
	}
	prepareCtx, stopPreparation := context.WithTimeout(ctx, TranscriptionPreparationTimeout)
	defer stopPreparation()
	input := work.Frozen.Input
	if input.DurationMS == nil || work.Frozen.Language != work.Job.Language {
		return domain.TranscriptionJob{}, ErrInvalidTranscription
	}
	key := path.Join("projects", work.Job.ProjectID.String(), "transcriptions", id.JobID.String(), "attempt-"+strconv.Itoa(id.Attempt)+".json")
	draft, err := w.existingTranscript(prepareCtx, key, id, input.SHA256)
	if err == nil {
		if work.InferenceState != "terminal" {
			return domain.TranscriptionJob{}, ErrInferenceUncertain
		}
		return w.store.Complete(ctx, id, draft)
	}
	if !errors.Is(err, ErrObjectMissing) {
		return domain.TranscriptionJob{}, err
	}
	// A terminal response without its private draft is safe to fail for explicit
	// retry, but it is not permission to submit the same attempt a second time.
	if work.InferenceState == "terminal" {
		return domain.TranscriptionJob{}, ErrInvalidTranscription
	}
	source, err := w.stageAudio(prepareCtx, input)
	if err != nil {
		return domain.TranscriptionJob{}, err
	}
	defer func() { _ = source.Close() }()
	if err := w.store.Progress(prepareCtx, id, 20, "preparing_audio"); err != nil {
		return domain.TranscriptionJob{}, err
	}
	pcm, err := w.preprocessor.Prepare(prepareCtx, source, int64(*input.DurationMS))
	if err != nil {
		return domain.TranscriptionJob{}, err
	}
	if pcm == nil || pcm.File == nil {
		return domain.TranscriptionJob{}, ErrInvalidTranscription
	}
	defer func() { _ = pcm.File.Close() }()
	inferenceBudget := TranscriptionInferenceTimeout
	if deadline, ok := ctx.Deadline(); ok {
		inferenceBudget = min(inferenceBudget, time.Until(deadline)-TranscriptionFinalizationTimeout-20*time.Second)
	}
	if inferenceBudget <= 0 {
		return domain.TranscriptionJob{}, ErrUnavailable
	}
	if err := w.store.StartInference(prepareCtx, id); err != nil {
		return domain.TranscriptionJob{}, err
	}
	// HTTP /inference has no request receipt or cancellation-status API. Preserve
	// this owned activity until a terminal native response confirms it stopped;
	// caller cancellation remains durable cancel_requested and discards results.
	inferenceCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), inferenceBudget)
	draft, inferenceErr := w.transcriber.Transcribe(inferenceCtx, pcm.File, work.Frozen.Language, pcm.DurationMS)
	cancel()
	if errors.Is(inferenceErr, ErrInferenceUncertain) {
		return domain.TranscriptionJob{}, inferenceErr
	}
	finalize, stop := context.WithTimeout(context.WithoutCancel(ctx), TranscriptionFinalizationTimeout)
	defer stop()
	if err := w.store.EndInference(finalize, id); err != nil {
		return domain.TranscriptionJob{}, fmt.Errorf("persist native termination: %w", err)
	}
	if inferenceErr != nil {
		return domain.TranscriptionJob{}, inferenceErr
	}
	if err := draft.Validate(); err != nil {
		return domain.TranscriptionJob{}, err
	}
	raw, err := json.Marshal(transcriptArtifact{Version: 1, JobID: id.JobID.String(), Attempt: id.Attempt, SourceSHA256: input.SHA256, Draft: draft})
	if err != nil {
		return domain.TranscriptionJob{}, err
	}
	digest := sha256.Sum256(raw)
	if err := w.objects.PutIfAbsent(finalize, key, bytes.NewReader(raw), int64(len(raw)), "application/json", hex.EncodeToString(digest[:])); err != nil && !errors.Is(err, mediaapp.ErrObjectAlreadyExists) {
		return domain.TranscriptionJob{}, err
	}
	verified, err := w.existingTranscript(finalize, key, id, input.SHA256)
	if err != nil {
		return domain.TranscriptionJob{}, err
	}
	return w.store.Complete(finalize, id, verified)
}
func (w *TranscriptionWorker) existingTranscript(ctx context.Context, key string, id TranscriptionWorkID, sourceSHA string) (domain.Transcript, error) {
	info, err := w.objects.Stat(ctx, key)
	if err != nil {
		return domain.Transcript{}, err
	}
	if info.Size < 1 || info.Size > 8<<20 || info.ContentType != "application/json" || len(info.SHA256) != 64 {
		return domain.Transcript{}, ErrInvalidTranscription
	}
	source, err := w.objects.Get(ctx, key)
	if err != nil {
		return domain.Transcript{}, err
	}
	defer func() { _ = source.Close() }()
	raw, err := io.ReadAll(io.LimitReader(source, info.Size+1))
	if err != nil {
		return domain.Transcript{}, err
	}
	digest := sha256.Sum256(raw)
	if int64(len(raw)) != info.Size || hex.EncodeToString(digest[:]) != info.SHA256 {
		return domain.Transcript{}, ErrInvalidTranscription
	}
	var artifact transcriptArtifact
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&artifact) != nil || decoder.Decode(new(any)) != io.EOF || artifact.Version != 1 || artifact.JobID != id.JobID.String() || artifact.Attempt != id.Attempt || artifact.SourceSHA256 != sourceSHA || artifact.Draft.Validate() != nil {
		return domain.Transcript{}, ErrInvalidTranscription
	}
	return artifact.Draft, nil
}
func (w *TranscriptionWorker) stageAudio(ctx context.Context, input domain.FrozenSource) (*mediaapp.Downloaded, error) {
	if input.ByteSize < 1 || input.ByteSize > 500<<20 || len(input.SHA256) != 64 || input.ObjectKey == "" || (input.Kind != "audio" && input.Kind != "video") {
		return nil, ErrInvalidTranscription
	}
	source, err := w.objects.Get(ctx, input.ObjectKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = source.Close() }()
	file, err := os.CreateTemp("", "lanverse-transcription-source-*")
	if err != nil {
		return nil, err
	}
	result := &mediaapp.Downloaded{File: file, Size: input.ByteSize, MIMEType: input.MIMEType, SHA256: input.SHA256}
	keep := false
	defer func() {
		if !keep {
			_ = result.Close()
		}
	}()
	digest := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, digest), io.LimitReader(source, input.ByteSize+1))
	if err != nil {
		return nil, err
	}
	if n != input.ByteSize || hex.EncodeToString(digest.Sum(nil)) != input.SHA256 {
		return nil, ErrInvalidTranscription
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	keep = true
	return result, nil
}
