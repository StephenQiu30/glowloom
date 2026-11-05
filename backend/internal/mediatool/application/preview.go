package application

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

// ExportQuery signs pending output for inspection and serves finalized downloads.
type ExportQuery struct {
	launchOrigin string
	store        ExportStore
	storage      *resultAccess
}

// NewRoutedExportQuery resolves saved physical identities and preserves SDK expiry.
func NewRoutedExportQuery(store ExportStore, signer mediaapp.ObjectSigner, reader mediaapp.ObjectReader, ttl time.Duration) *ExportQuery {
	return &ExportQuery{store: store, storage: &resultAccess{signer: signer, reader: reader, previewTTL: ttl}}
}

// Preview returns the exact output hash and revision required by owner review.
func (q *ExportQuery) Preview(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (ExportPreview, error) {
	if q == nil || q.store == nil || q.storage == nil || q.storage.signer == nil {
		return ExportPreview{}, ErrUnavailable
	}
	if store, ok := q.store.(ArtifactStore); ok {
		job, err := q.store.Get(ctx, actor, project, id)
		if err != nil {
			return ExportPreview{}, err
		}
		if job.OutputKind.IsArtifact() {
			return q.artifactPreview(ctx, store, actor, project, id)
		}
	}
	job, asset, err := q.store.PreviewAsset(ctx, actor, project, id)
	if err != nil {
		return ExportPreview{}, err
	}
	grant, err := q.storage.sign(ctx, asset.ObjectKey, asset.ObjectLocation)
	url := grant.URL
	if err != nil {
		return ExportPreview{}, err
	}
	preview := ExportPreview{JobID: job.ID, Revision: job.Revision, SHA256: *job.SHA256, URL: url, ExpiresAt: grant.ExpiresAt, Asset: mediaapp.AssetSummary{ID: asset.ID, ProjectID: asset.ProjectID, Kind: string(asset.Kind), FileName: asset.FileName, MIMEType: asset.MimeType, ByteSize: asset.ByteSize, Width: asset.Width, Height: asset.Height, DurationMS: asset.DurationMS, Revision: asset.Revision}}
	if job.OutputKind.Effective() == domain.OutputAudio {
		rendition, err := q.store.PreviewWaveform(ctx, actor, project, id, job.Revision, *job.SHA256)
		if err != nil {
			return ExportPreview{}, err
		}
		waveGrant, err := q.storage.sign(ctx, rendition.ObjectKey, rendition.ObjectLocation)
		waveURL := waveGrant.URL
		if err != nil || waveURL == "" {
			return ExportPreview{}, fmt.Errorf("%w: sign audio waveform", ErrUnavailable)
		}
		preview.Waveform = &ExportWaveformPreview{URL: waveURL, ExpiresAt: waveGrant.ExpiresAt, Width: *rendition.Width, Height: *rendition.Height}
	}
	return preview, nil
}

// Download allows an attachment only after the produced output passed manual review.
func (q *ExportQuery) Download(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (io.ReadCloser, int64, error) {
	if q == nil || q.store == nil || q.storage == nil || q.storage.reader == nil {
		return nil, 0, ErrUnavailable
	}
	if store, ok := q.store.(ArtifactStore); ok {
		job, err := q.store.Get(ctx, actor, project, id)
		if err != nil {
			return nil, 0, err
		}
		if job.OutputKind.IsArtifact() {
			return q.artifactDownload(ctx, store, actor, project, id)
		}
	}
	job, asset, err := q.store.PreviewAsset(ctx, actor, project, id)
	if err != nil {
		return nil, 0, err
	}
	if job.Status != domain.Succeeded || !asset.CanReference() {
		return nil, 0, ErrConflict
	}
	reader, err := q.storage.read(ctx, asset.ObjectKey, asset.ObjectLocation)
	return reader, asset.ByteSize, err
}

// Subtitles downloads deterministic text from frozen, visible subtitle/text clips.
func (q *ExportQuery) Subtitles(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (string, error) {
	if q == nil || q.store == nil {
		return "", ErrUnavailable
	}
	frozen, err := q.store.Frozen(ctx, actor, project, id)
	if err != nil {
		return "", err
	}
	visible := make(map[uuid.UUID]bool)
	for _, track := range frozen.Timeline.Tracks {
		visible[track.ID] = track.Visible
	}
	var clips []canvasdomain.TimelineClip
	for _, clip := range frozen.Timeline.Clips {
		if visible[clip.TrackID] && (clip.Kind == "subtitle" || clip.Kind == "text") && strings.TrimSpace(clip.Text) != "" {
			clips = append(clips, clip)
		}
	}
	slices.SortFunc(clips, func(a, b canvasdomain.TimelineClip) int {
		if a.StartMS < b.StartMS {
			return -1
		}
		if a.StartMS > b.StartMS {
			return 1
		}
		return slices.Compare(a.ID[:], b.ID[:])
	})
	var text strings.Builder
	for i, clip := range clips {
		fmt.Fprintf(&text, "%d\n%s --> %s\n%s\n\n", i+1, srtTime(clip.StartMS), srtTime(clip.StartMS+clip.DurationMS), strings.ReplaceAll(strings.ReplaceAll(clip.Text, "\r\n", "\n"), "\r", "\n"))
	}
	return text.String(), nil
}
func srtTime(ms int64) string {
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, (ms/60000)%60, (ms/1000)%60, ms%1000)
}
