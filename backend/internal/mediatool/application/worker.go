package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// ErrObjectMissing is an exact private-key absence, distinct from storage failures.
var ErrObjectMissing = errors.New("export private object missing")

// WorkerStore fences renderer progress and output publication against cancellation.
type WorkerStore interface {
	Claim(context.Context, WorkID) (Work, error)
	Progress(context.Context, WorkID, int, string) error
	Commit(context.Context, WorkID, mediadomain.MediaAsset, []mediadomain.Rendition) (domain.ExportJob, error)
	Finish(context.Context, WorkID, bool, string) error
	Release(context.Context, WorkID) error
}

// PrivateObjects opens bounded originals and conditionally stores checked artifacts.
type PrivateObjects interface {
	Get(context.Context, string) (io.ReadCloser, error)
	Stat(context.Context, string) (mediaapp.ObjectInfo, error)
	PutIfAbsent(context.Context, string, io.Reader, int64, string, string) error
}

// Worker renders real output and previews, then registers only pending media.
type Worker struct {
	store    WorkerStore
	objects  PrivateObjects
	renderer Renderer
	prober   mediaapp.Prober
	previews mediaapp.Renderer
}

// NewWorker injects actual private storage, local rendering and owning metadata ports.
func NewWorker(store WorkerStore, objects PrivateObjects, renderer Renderer, prober mediaapp.Prober, previews mediaapp.Renderer) *Worker {
	return &Worker{store: store, objects: objects, renderer: renderer, prober: prober, previews: previews}
}

// Execute adopts an already-created original on activity retry; completed attempts
// return durable facts without rerendering or creating another media asset.
func (w *Worker) Execute(ctx context.Context, id WorkID) (domain.ExportJob, error) {
	if w == nil || w.store == nil || w.objects == nil || w.renderer == nil || w.prober == nil || w.previews == nil {
		return domain.ExportJob{}, ErrUnavailable
	}
	work, err := w.store.Claim(ctx, id)
	if err != nil {
		return domain.ExportJob{}, err
	}
	if work.Job.Status == domain.ReviewRequired || work.Job.Status == domain.Succeeded {
		return work.Job, nil
	}
	assetID := uuid.NewSHA1(id.JobID, []byte("export-output/"+strconv.Itoa(id.Attempt)))
	key := path.Join("projects", work.Job.ProjectID.String(), "video", work.Job.CreatedAt.UTC().Format("2006/01"), assetID.String()+".mp4")
	output, err := w.existing(ctx, key)
	if errors.Is(err, ErrObjectMissing) {
		paths := make(map[uuid.UUID]string, len(work.Frozen.Inputs))
		sources := make([]*mediaapp.Downloaded, 0, len(work.Frozen.Inputs))
		defer func() {
			for _, file := range sources {
				_ = file.Close()
			}
		}()
		for i, input := range work.Frozen.Inputs {
			if err := w.store.Progress(ctx, id, 5+i*14/max(1, len(work.Frozen.Inputs)), "downloading"); err != nil {
				return domain.ExportJob{}, err
			}
			file, err := w.readChecked(ctx, input.ObjectKey, input.ByteSize, input.MIMEType, input.SHA256)
			if err != nil {
				return domain.ExportJob{}, fmt.Errorf("stage authorized export source: %w", err)
			}
			sources = append(sources, file)
			probe, err := w.prober.Probe(ctx, file)
			if err != nil || string(probe.Kind) != input.Kind || !sameDimension(probe.Width, input.Width) || !sameDimension(probe.Height, input.Height) {
				return domain.ExportJob{}, ErrInvalidExport
			}
			paths[input.AssetID] = file.File.Name()
		}
		output, err = w.renderer.Render(ctx, work.Frozen, paths, func(progress int, stage string) error { return w.store.Progress(ctx, id, progress, stage) })
		if err != nil {
			return domain.ExportJob{}, err
		}
		if err = w.put(ctx, key, output); err != nil {
			_ = output.Close()
			if !errors.Is(err, mediaapp.ErrObjectAlreadyExists) {
				return domain.ExportJob{}, err
			}
			output, err = w.existing(ctx, key)
			if err != nil {
				return domain.ExportJob{}, err
			}
		}
	} else if err != nil {
		return domain.ExportJob{}, err
	}
	if output == nil || output.File == nil {
		return domain.ExportJob{}, ErrInvalidExport
	}
	defer func() { _ = output.Close() }()
	probe, err := w.prober.Probe(ctx, output)
	if err != nil || probe.Kind != mediadomain.KindVideo || probe.DurationMS == nil {
		return domain.ExportJob{}, ErrInvalidExport
	}
	width, height := int32(1920), int32(1080)
	switch work.Frozen.Timeline.AspectRatio {
	case "9:16":
		width, height = 1080, 1920
	case "1:1":
		width, height = 1080, 1080
	}
	var duration int64
	visible := make(map[uuid.UUID]bool)
	for _, track := range work.Frozen.Timeline.Tracks {
		visible[track.ID] = track.Visible
	}
	for _, clip := range work.Frozen.Timeline.Clips {
		if visible[clip.TrackID] {
			duration = max(duration, clip.StartMS+clip.DurationMS)
		}
	}
	if probe.Width == nil || probe.Height == nil || *probe.Width != width || *probe.Height != height || absDuration(int64(*probe.DurationMS)-duration) > int64(1000/work.Frozen.Timeline.FPS)+25 {
		return domain.ExportJob{}, ErrInvalidExport
	}
	if err := w.store.Progress(ctx, id, 85, "previews"); err != nil {
		return domain.ExportJob{}, err
	}
	rendered, err := w.previews.Render(ctx, output, probe, work.Frozen.Timeline.AspectRatio)
	if err != nil {
		return domain.ExportJob{}, err
	}
	defer func() {
		for _, file := range rendered {
			if file.Result != nil {
				_ = file.Result.Close()
			}
		}
	}()
	if len(rendered) != 2 || rendered[0].Kind != mediadomain.RenditionPoster || rendered[1].Kind != mediadomain.RenditionProxy720p {
		return domain.ExportJob{}, ErrInvalidExport
	}
	now := time.Now().UTC()
	sha := output.SHA256
	asset := mediadomain.MediaAsset{ID: assetID, ProjectID: work.Job.ProjectID, Kind: mediadomain.KindVideo, Origin: mediadomain.OriginSystem, Status: mediadomain.StatusProcessing, ObjectKey: key, FileName: "timeline-" + id.JobID.String() + ".mp4", MimeType: output.MIMEType, ByteSize: output.Size, SHA256: &sha, Width: probe.Width, Height: probe.Height, DurationMS: probe.DurationMS, FPS: probe.FPS, AudioChannels: probe.AudioChannels, Codec: probe.Codec, ModerationStatus: mediadomain.ModerationPending, Revision: 1, CreateTime: now, UpdateTime: now}
	renditions := make([]mediadomain.Rendition, 0, len(rendered))
	for _, file := range rendered {
		if file.Result == nil || file.Result.File == nil || file.Width < 1 || file.Height < 1 {
			return domain.ExportJob{}, ErrInvalidExport
		}
		objectKey := path.Join(key[:len(key)-len(path.Ext(key))], string(file.Kind)+"."+file.Ext)
		if err := w.put(ctx, objectKey, file.Result); err != nil && !errors.Is(err, mediaapp.ErrObjectAlreadyExists) {
			return domain.ExportJob{}, err
		}
		// Recreated previews may differ across FFmpeg builds. A private existing
		// rendition is accepted only after reading its exact SHA and probing pixels.
		verified, err := w.existingRendition(ctx, objectKey, file.Result.MIMEType, file.Width, file.Height)
		if err != nil {
			return domain.ExportJob{}, err
		}
		rw, rh, size := file.Width, file.Height, verified.Size
		_ = verified.Close()
		renditions = append(renditions, mediadomain.Rendition{ID: uuid.NewSHA1(assetID, []byte(string(file.Kind))), MediaAssetID: assetID, Kind: file.Kind, ObjectKey: objectKey, Width: &rw, Height: &rh, ByteSize: &size, CreateTime: now, UpdateTime: now})
	}
	return w.store.Commit(ctx, id, asset, renditions)
}
func sameDimension(a, b *int32) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func absDuration(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
func (w *Worker) existing(ctx context.Context, key string) (*mediaapp.Downloaded, error) {
	info, err := w.objects.Stat(ctx, key)
	if err != nil {
		return nil, err
	}
	if info.Size < 1 || info.Size > 500<<20 || info.ContentType != "video/mp4" || len(info.SHA256) != 64 {
		return nil, ErrInvalidExport
	}
	return w.readChecked(ctx, key, info.Size, info.ContentType, info.SHA256)
}
func (w *Worker) existingRendition(ctx context.Context, key, mime string, width, height int32) (*mediaapp.Downloaded, error) {
	info, err := w.objects.Stat(ctx, key)
	if err != nil {
		return nil, err
	}
	if info.Size < 1 || info.Size > 500<<20 || info.ContentType != mime || len(info.SHA256) != 64 {
		return nil, ErrInvalidExport
	}
	file, err := w.readChecked(ctx, key, info.Size, mime, info.SHA256)
	if err != nil {
		return nil, err
	}
	probe, err := w.prober.Probe(ctx, file)
	if err != nil || probe.Width == nil || probe.Height == nil || *probe.Width != width || *probe.Height != height {
		_ = file.Close()
		return nil, ErrInvalidExport
	}
	return file, nil
}
func (w *Worker) readChecked(ctx context.Context, key string, size int64, mime, sha string) (*mediaapp.Downloaded, error) {
	if size < 1 || size > 500<<20 || len(sha) != 64 {
		return nil, ErrInvalidExport
	}
	source, err := w.objects.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = source.Close() }()
	file, err := os.CreateTemp("", "lanverse-export-input-*")
	if err != nil {
		return nil, err
	}
	result := &mediaapp.Downloaded{File: file, Size: size, MIMEType: mime, SHA256: sha}
	keep := false
	defer func() {
		if !keep {
			_ = result.Close()
		}
	}()
	hash := sha256.New()
	count, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(source, size+1))
	if err != nil {
		return nil, err
	}
	if count != size || hex.EncodeToString(hash.Sum(nil)) != sha {
		return nil, ErrInvalidExport
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	keep = true
	return result, nil
}
func (w *Worker) put(ctx context.Context, key string, file *mediaapp.Downloaded) error {
	if file == nil || file.File == nil || file.Size < 1 || file.Size > 500<<20 {
		return ErrInvalidExport
	}
	if _, err := file.File.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return w.objects.PutIfAbsent(ctx, key, file.File, file.Size, file.MIMEType, file.SHA256)
}
