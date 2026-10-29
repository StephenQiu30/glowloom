package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

// ArtifactStore exposes authorized output facts without inventing a media asset.
type ArtifactStore interface {
	PreviewArtifact(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.ExportJob, domain.Artifact, error)
}

// ArtifactWorkerStore records an immutable engineering artifact behind the worker fence.
type ArtifactWorkerStore interface {
	CommitArtifact(context.Context, WorkID, domain.Artifact) (domain.ExportJob, error)
}

// ArtifactRenderer independently checks new and adopted ZIP bytes against frozen edits.
type ArtifactRenderer interface {
	Renderer
	VerifyArtifact(context.Context, *os.File, int64, domain.FrozenExport) error
}

func (w *Worker) executeArtifact(ctx context.Context, id WorkID, work Work) (domain.ExportJob, error) {
	store, ok := w.store.(ArtifactWorkerStore)
	if !ok {
		return domain.ExportJob{}, ErrUnavailable
	}
	renderer, ok := w.renderer.(ArtifactRenderer)
	if !ok {
		return domain.ExportJob{}, ErrUnavailable
	}
	if err := ValidateProjectExport(work.Frozen); err != nil {
		return domain.ExportJob{}, err
	}
	if *work.Frozen.Source != work.Job.Source || work.Frozen.OutputKind != work.Job.OutputKind {
		return domain.ExportJob{}, ErrInvalidExport
	}
	artifactID := uuid.NewSHA1(id.JobID, []byte("export-artifact/"+strconv.Itoa(id.Attempt)))
	key := path.Join("projects", work.Job.ProjectID.String(), "exports", work.Job.CreatedAt.UTC().Format("2006/01"), artifactID.String()+".zip")
	files := make([]*mediaapp.Downloaded, 0, len(work.Frozen.Inputs))
	defer func() {
		for _, file := range files {
			_ = file.Close()
		}
	}()
	paths := make(map[uuid.UUID]string)
	for i, input := range work.Frozen.Inputs {
		if err := w.store.Progress(ctx, id, 5+i*14/max(1, len(work.Frozen.Inputs)), "downloading"); err != nil {
			return domain.ExportJob{}, err
		}
		file, e := w.readChecked(ctx, input.ObjectKey, input.ByteSize, input.MIMEType, input.SHA256)
		if e != nil {
			return domain.ExportJob{}, e
		}
		files = append(files, file)
		probe, e := w.prober.Probe(ctx, file)
		if e != nil || string(probe.Kind) != input.Kind || !sameDimension(probe.Width, input.Width) || !sameDimension(probe.Height, input.Height) {
			return domain.ExportJob{}, ErrInvalidExport
		}
		// Older video metadata omitted audio channels. Derive missing stream facts
		// from the same checksum-verified originals on every attempt, including adoption.
		if input.AudioChannels != nil && (probe.AudioChannels == nil || *input.AudioChannels != *probe.AudioChannels) {
			return domain.ExportJob{}, ErrInvalidExport
		}
		work.Frozen.Inputs[i].AudioChannels = probe.AudioChannels
		paths[input.AssetID] = file.File.Name()
	}
	output, err := w.existing(ctx, key, "application/zip")
	if errors.Is(err, ErrObjectMissing) {
		output, err = renderer.Render(ctx, work.Frozen, paths, func(progress int, stage string) error { return w.store.Progress(ctx, id, progress, stage) })
		if err != nil {
			return domain.ExportJob{}, err
		}
		if output == nil || output.File == nil || output.MIMEType != "application/zip" {
			return domain.ExportJob{}, ErrInvalidExport
		}
		if err = renderer.VerifyArtifact(ctx, output.File, output.Size, work.Frozen); err != nil {
			_ = output.Close()
			return domain.ExportJob{}, err
		}
		if err = w.put(ctx, id, key, output); err != nil {
			_ = output.Close()
			if !errors.Is(err, mediaapp.ErrObjectAlreadyExists) {
				return domain.ExportJob{}, err
			}
			output, err = w.existing(ctx, key, "application/zip")
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
	if err = renderer.VerifyArtifact(ctx, output.File, output.Size, work.Frozen); err != nil {
		return domain.ExportJob{}, fmt.Errorf("verify export artifact: %w", err)
	}
	if err = w.store.Progress(ctx, id, 90, "verifying"); err != nil {
		return domain.ExportJob{}, err
	}
	artifact := domain.Artifact{ID: artifactID, JobID: work.Job.ID, ProjectID: work.Job.ProjectID, Attempt: id.Attempt, Format: work.Job.OutputKind, FormatVersion: work.Job.OutputKind.FormatVersion(), FileName: "timeline-" + work.Job.ID.String() + ".zip", MIMEType: "application/zip", ByteSize: output.Size, SHA256: output.SHA256, ObjectKey: key, CreatedAt: time.Now().UTC()}
	return store.CommitArtifact(ctx, id, artifact)
}
func (q *ExportQuery) artifactPreview(ctx context.Context, store ArtifactStore, actor identityapp.Principal, project, id uuid.UUID) (ExportPreview, error) {
	job, artifact, err := store.PreviewArtifact(ctx, actor, project, id)
	if err != nil {
		return ExportPreview{}, err
	}
	access := q.storage
	if access == nil {
		access = &resultAccess{legacySigner: q.signer, legacyReader: q.reader}
	}
	grant, err := access.sign(ctx, artifact.ObjectKey, artifact.ObjectLocation, 10*time.Minute)
	if err != nil {
		return ExportPreview{}, err
	}
	return ExportPreview{JobID: job.ID, Revision: job.Revision, SHA256: artifact.SHA256, URL: grant.URL, ExpiresAt: grant.ExpiresAt, Artifact: &artifact}, nil
}
func (q *ExportQuery) artifactDownload(ctx context.Context, store ArtifactStore, actor identityapp.Principal, project, id uuid.UUID) (io.ReadCloser, int64, error) {
	job, artifact, err := store.PreviewArtifact(ctx, actor, project, id)
	if err != nil {
		return nil, 0, err
	}
	if job.Status != domain.Succeeded {
		return nil, 0, ErrConflict
	}
	access := q.storage
	if access == nil {
		access = &resultAccess{legacySigner: q.signer, legacyReader: q.reader}
	}
	reader, err := access.read(ctx, artifact.ObjectKey, artifact.ObjectLocation)
	return reader, artifact.ByteSize, err
}
