package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"time"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// DepthWorker owns one bounded physical execution and its private file lifetime.
type DepthWorker struct {
	store     DepthWorkerStore
	objects   DepthObjects
	processor DepthProcessor
	prober    mediaapp.Prober
	previews  mediaapp.Renderer
	verifier  DepthVerifier
}

// NewDepthWorker injects real storage, native execution and independent verification.
func NewDepthWorker(s DepthWorkerStore, objects DepthObjects, processor DepthProcessor, prober mediaapp.Prober, previews mediaapp.Renderer, verifier DepthVerifier) *DepthWorker {
	return &DepthWorker{store: s, objects: objects, processor: processor, prober: prober, previews: previews, verifier: verifier}
}

// Execute never replays native inference after dispatch; recovery reads the original manifest.
func (w *DepthWorker) Execute(ctx context.Context, id DepthWorkID) (job domain.DepthJob, err error) {
	if w == nil || w.store == nil || w.objects == nil {
		return job, ErrUnavailable
	}
	work, err := w.store.Claim(ctx, id)
	if err != nil {
		return job, err
	}
	job = work.Job
	if job.Status == domain.DepthReviewRequired || job.Status == domain.DepthSucceeded || job.Status == domain.DepthCancelled {
		return job, nil
	}
	stopped, uncertain, code := true, false, "dependency_unavailable"
	var owned []*mediaapp.Downloaded
	defer func() {
		var closeErr error
		for _, f := range owned {
			if !stopped && f != nil && f.File != nil {
				// An uncertain child may still use the path. Close our descriptor only.
				closeErr = errors.Join(closeErr, f.File.Close())
				f.File = nil
			} else {
				closeErr = errors.Join(closeErr, f.Close())
			}
		}
		err = errors.Join(err, closeErr)
		if err == nil {
			return
		}
		finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		if errors.Is(err, ErrCancelled) && stopped {
			if cleanupErr := w.cleanup(finish, id); cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
				uncertain, code = true, depthObjectErrorCode(cleanupErr, "depth_cleanup_unknown")
			} else {
				uncertain, code = false, ""
			}
		}
		var finishErr error
		job, finishErr = w.store.Finish(finish, id, stopped, uncertain, code)
		err = errors.Join(err, finishErr)
		if finishErr == nil && job.Status == domain.DepthCancelled {
			err = nil
		}
	}()
	if job.CancellationRequested {
		err = ErrCancelled
		return job, err
	}
	if id.Reconcile {
		code = "depth_object_unknown"
		uncertain = true
		if err = w.reconcile(ctx, id); err != nil {
			code = depthObjectErrorCode(err, code)
			return job, err
		}
		job, err = w.store.Commit(ctx, id)
		return job, err
	}
	if w.processor == nil || w.prober == nil || w.previews == nil || w.verifier == nil {
		return job, ErrUnavailable
	}
	if work.ProcessState != domain.DepthProcessNone {
		uncertain = true
		code = "depth_execution_unknown"
		return job, ErrConflict
	}
	if err = w.store.Phase(ctx, id, "downloading"); err != nil {
		return job, err
	}
	input, err := w.read(ctx, DepthObject{ObjectKey: work.Frozen.Input.ObjectKey, ByteSize: work.Frozen.Input.ByteSize, MIMEType: work.Frozen.Input.MIMEType, SHA256: work.Frozen.Input.SHA256})
	if err != nil {
		code = "source_unavailable"
		return job, err
	}
	owned = append(owned, input)
	probe, err := w.prober.Probe(ctx, input)
	if err != nil || probe.Kind != mediadomain.KindVideo || !sameDimension(probe.Width, work.Frozen.Input.Width) || !sameDimension(probe.Height, work.Frozen.Input.Height) || probe.DurationMS == nil || *probe.DurationMS > 15100 {
		code = "depth_input_invalid"
		return job, errors.Join(ErrInvalidDepthInput, err)
	}
	if err = w.store.StartProcess(ctx, id); err != nil {
		return job, err
	}
	stopped = false
	output, processErr := w.processor.Process(ctx, input, work.Frozen.Input.SHA256, func(phase DepthPhase) error { return w.store.Phase(ctx, id, string(phase)) })
	if output != nil && output.File != nil {
		owned = append(owned, output.File)
	}
	if errors.Is(processErr, ErrDepthCessationUncertain) {
		code = "depth_execution_unknown"
		return job, processErr
	}
	stopped = true
	end, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	endErr := w.store.EndProcess(end, id)
	cancel()
	if endErr != nil {
		uncertain = true
		code = "depth_execution_unknown"
		return job, errors.Join(processErr, endErr)
	}
	if processErr != nil {
		code = depthErrorCode(processErr)
		check, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		phaseErr := w.store.Phase(check, id, "verifying")
		cancel()
		if errors.Is(phaseErr, ErrCancelled) {
			return job, ErrCancelled
		}
		return job, processErr
	}
	code = "depth_output_invalid"
	if err = ValidateDepthReceipt(work.Frozen, output); err != nil {
		return job, err
	}
	if err = depthHashFile(output.File); err != nil {
		return job, err
	}
	actual, err := w.verifier.Verify(ctx, output.File)
	if err != nil || !sameDepthFacts(actual, output.Receipt.Output) {
		return job, errors.Join(ErrDepthOutputInvalid, err)
	}
	independent, err := w.prober.Probe(ctx, output.File)
	if err != nil {
		return job, err
	}
	output.Probe = independent
	if err = ValidateDepthReceipt(work.Frozen, output); err != nil {
		return job, err
	}
	if err = w.store.Phase(ctx, id, "previews"); err != nil {
		return job, err
	}
	rendered, err := w.previews.Render(ctx, output.File, output.Probe, "16:9")
	for _, r := range rendered {
		if r.Result != nil {
			owned = append(owned, r.Result)
		}
	}
	if err != nil {
		return job, err
	}
	artifact, files, err := w.artifact(ctx, work, output, rendered)
	if err != nil {
		return job, err
	}
	if err = w.store.FreezeArtifact(ctx, id, artifact); err != nil {
		return job, err
	}
	uncertain = true
	code = "depth_object_unknown"
	if err = w.store.Phase(ctx, id, "storing"); err != nil {
		return job, err
	}
	for i, o := range artifact.Objects {
		if err = w.store.BeginWrite(ctx, id, o.ObjectKey); err != nil {
			return job, err
		}
		if _, err = files[i].File.Seek(0, io.SeekStart); err != nil {
			return job, err
		}
		putErr := w.objects.PutIfAbsent(ctx, o.ObjectKey, files[i].File, o.ByteSize, o.MIMEType, o.SHA256)
		// A transport error is unknown until the same immutable bytes can be read.
		if verifyErr := w.verifyObject(ctx, o); verifyErr != nil {
			code = depthObjectErrorCode(verifyErr, code)
			return job, errors.Join(putErr, verifyErr)
		}
		if err = w.store.ConfirmObject(ctx, id, o.ObjectKey); err != nil {
			return job, err
		}
	}
	// Temporary files have no active children at this point, but their cleanup
	// must succeed before publishing even pending metadata.
	for _, f := range owned {
		if err = f.Close(); err != nil {
			return job, err
		}
	}
	owned = nil
	job, err = w.store.Commit(ctx, id)
	return job, err
}

func depthErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrDepthRuntimeUnavailable):
		return "depth_runtime_unavailable"
	case errors.Is(err, ErrDepthModelMismatch):
		return "depth_model_mismatch"
	case errors.Is(err, ErrDepthBudgetExceeded):
		return "depth_budget_exceeded"
	case errors.Is(err, ErrDepthDeviceUnavailable):
		return "depth_device_unavailable"
	case errors.Is(err, ErrInvalidDepthInput):
		return "depth_input_invalid"
	case errors.Is(err, ErrDepthOutputInvalid):
		return "depth_output_invalid"
	default:
		return "depth_inference_failed"
	}
}

func depthObjectErrorCode(err error, fallback string) string {
	switch {
	case errors.Is(err, ErrDepthOutputInvalid):
		return "depth_object_conflict"
	case errors.Is(err, ErrObjectMissing):
		return "depth_object_absence_unknown"
	default:
		return fallback
	}
}

func sameDepthFacts(a, b DepthVideoFacts) bool {
	return a.Width == b.Width && a.Height == b.Height && a.FrameCount == b.FrameCount && math.Abs(a.FPS-b.FPS) <= 0.00001 && absDuration(a.DurationMS-b.DurationMS) <= 1
}

func depthHashFile(f *mediaapp.Downloaded) error {
	if f == nil || f.File == nil || f.Size < 1 || f.Size > 500<<20 || !domain.ValidDepthSHA(f.SHA256) {
		return ErrDepthOutputInvalid
	}
	if _, err := f.File.Seek(0, io.SeekStart); err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f.File, f.Size+1))
	if err != nil {
		return err
	}
	if n != f.Size || hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
		return ErrDepthOutputInvalid
	}
	_, err = f.File.Seek(0, io.SeekStart)
	return err
}

func (w *DepthWorker) read(ctx context.Context, o DepthObject) (result *mediaapp.Downloaded, err error) {
	if o.ByteSize < 1 || o.ByteSize > 500<<20 || !domain.ValidDepthSHA(o.SHA256) {
		return nil, ErrDepthOutputInvalid
	}
	source, err := w.objects.Get(ctx, o.ObjectKey)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, source.Close())
		if err != nil && result != nil {
			err = errors.Join(err, result.Close())
			result = nil
		}
	}()
	file, err := os.CreateTemp("", "lanverse-depth-input-*")
	if err != nil {
		return nil, err
	}
	result = &mediaapp.Downloaded{File: file, Size: o.ByteSize, MIMEType: o.MIMEType, SHA256: o.SHA256}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, h), io.LimitReader(source, o.ByteSize+1))
	if err != nil {
		return result, err
	}
	if n != o.ByteSize || hex.EncodeToString(h.Sum(nil)) != o.SHA256 {
		return result, ErrDepthOutputInvalid
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	return result, nil
}

func (w *DepthWorker) verifyObject(ctx context.Context, o DepthObject) error {
	info, err := w.objects.Stat(ctx, o.ObjectKey)
	if err != nil {
		return err
	}
	if info.Size != o.ByteSize || info.ContentType != o.MIMEType || info.SHA256 != o.SHA256 {
		return ErrDepthOutputInvalid
	}
	file, err := w.read(ctx, o)
	if err != nil {
		return err
	}
	return file.Close()
}

func (w *DepthWorker) reconcile(ctx context.Context, id DepthWorkID) error {
	objects, err := w.store.Objects(ctx, id)
	if err != nil {
		return err
	}
	if len(objects) != 3 {
		return ErrDepthOutputInvalid
	}
	for _, o := range objects {
		if o.DeleteStarted || o.Status == "removed" {
			return ErrConflict
		}
		if !o.WriteStarted {
			return ErrConflict
		}
		if err = w.verifyObject(ctx, o); err != nil {
			return err
		}
		if err = w.store.ConfirmObject(ctx, id, o.ObjectKey); err != nil {
			return err
		}
	}
	return nil
}

func (w *DepthWorker) cleanup(ctx context.Context, id DepthWorkID) error {
	objects, err := w.store.Objects(ctx, id)
	if err != nil {
		return err
	}
	for _, o := range objects {
		if o.Status == "removed" {
			continue
		}
		if !o.WriteStarted {
			if _, err = w.objects.Stat(ctx, o.ObjectKey); !errors.Is(err, ErrObjectMissing) {
				return errors.Join(ErrConflict, err)
			}
			if err = w.store.ConfirmRemoved(ctx, id, o.ObjectKey); err != nil {
				return err
			}
			continue
		}
		verifyErr := w.verifyObject(ctx, o)
		if errors.Is(verifyErr, ErrObjectMissing) && o.DeleteStarted {
			if err = w.store.ConfirmRemoved(ctx, id, o.ObjectKey); err != nil {
				return err
			}
			continue
		}
		if verifyErr != nil {
			return verifyErr
		}
		if o.Status == "pending" {
			if err = w.store.ConfirmObject(ctx, id, o.ObjectKey); err != nil {
				return err
			}
		}
		if err = w.store.BeginRemove(ctx, id, o.ObjectKey); err != nil {
			return err
		}
		removeErr := w.objects.Remove(ctx, o.ObjectKey)
		if _, err = w.objects.Stat(ctx, o.ObjectKey); !errors.Is(err, ErrObjectMissing) {
			return errors.Join(removeErr, err, ErrConflict)
		}
		if err = w.store.ConfirmRemoved(ctx, id, o.ObjectKey); err != nil {
			return err
		}
	}
	return nil
}

func (w *DepthWorker) artifact(ctx context.Context, work DepthWork, o *DepthOutput, rendered []mediaapp.RenditionFile) (DepthArtifact, []*mediaapp.Downloaded, error) {
	var a DepthArtifact
	if len(rendered) != 2 {
		return a, nil, ErrDepthOutputInvalid
	}
	id, key := DepthOutputIdentity(work.Job)
	now := time.Now().UTC().Truncate(time.Microsecond)
	sha := o.File.SHA256
	p := o.Probe
	a = DepthArtifact{JobID: work.Job.ID, Attempt: work.Job.Attempt, InputSHA256: work.Frozen.Input.SHA256, Receipt: o.Receipt, Asset: mediadomain.MediaAsset{ID: id, ProjectID: work.Job.ProjectID, Kind: mediadomain.KindVideo, Origin: mediadomain.OriginSystem, Status: mediadomain.StatusProcessing, ModerationStatus: mediadomain.ModerationPending, ObjectKey: key, FileName: "depth-" + work.Job.ID.String() + ".mp4", MimeType: "video/mp4", ByteSize: o.File.Size, SHA256: &sha, Width: p.Width, Height: p.Height, DurationMS: p.DurationMS, FPS: p.FPS, Codec: p.Codec, Revision: 1, CreateTime: now, UpdateTime: now}, Objects: []DepthObject{{Kind: "original", ObjectKey: key, SHA256: sha, ByteSize: o.File.Size, MIMEType: "video/mp4"}}}
	files := []*mediaapp.Downloaded{o.File}
	for i, r := range rendered {
		kind, mime, ext, width, height := "poster", "image/png", "png", int32(640), int32(360)
		if i == 1 {
			kind, mime, ext, width, height = "proxy_720p", "video/mp4", "mp4", 1280, 720
		}
		if string(r.Kind) != kind || r.Ext != ext || r.Width != width || r.Height != height || r.Result == nil || r.Result.MIMEType != mime {
			return a, nil, ErrDepthOutputInvalid
		}
		if err := depthHashFile(r.Result); err != nil {
			return a, nil, err
		}
		p, err := w.prober.Probe(ctx, r.Result)
		if err != nil || p.Width == nil || p.Height == nil || *p.Width != width || *p.Height != height || i == 0 && p.Kind != mediadomain.KindImage || i == 1 && (p.Kind != mediadomain.KindVideo || p.Codec == nil || *p.Codec != "h264" || p.AudioChannels != nil) {
			return a, nil, errors.Join(ErrDepthOutputInvalid, err)
		}
		rkey := path.Join(key[:len(key)-len(path.Ext(key))], kind+"."+ext)
		size := r.Result.Size
		a.Renditions = append(a.Renditions, mediadomain.Rendition{ID: uuid.NewSHA1(id, []byte(kind)), MediaAssetID: id, Kind: r.Kind, ObjectKey: rkey, Width: &width, Height: &height, ByteSize: &size, CreateTime: now, UpdateTime: now})
		a.Objects = append(a.Objects, DepthObject{Kind: kind, ObjectKey: rkey, SHA256: r.Result.SHA256, ByteSize: size, MIMEType: mime})
		files = append(files, r.Result)
	}
	if _, _, err := DepthArtifactDigest(work.Job, work.Frozen, a); err != nil {
		return a, nil, fmt.Errorf("bind native depth result: %w", err)
	}
	return a, files, nil
}
