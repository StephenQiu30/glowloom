package application

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// ResultReader opens one server-selected original, never a user supplied URL.
type ResultReader interface {
	Get(context.Context, string) (io.ReadCloser, error)
}

// ExportQuery signs pending output for inspection and serves finalized downloads.
type ExportQuery struct {
	store  ExportStore
	signer mediaapp.PreviewSigner
	reader ResultReader
}

// NewExportQuery injects authorization, bounded signed URLs and private result bytes.
func NewExportQuery(store ExportStore, signer mediaapp.PreviewSigner, reader ResultReader) *ExportQuery {
	return &ExportQuery{store: store, signer: signer, reader: reader}
}

// Preview returns the exact output hash and revision required by owner review.
func (q *ExportQuery) Preview(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (ExportPreview, error) {
	if q == nil || q.store == nil || q.signer == nil {
		return ExportPreview{}, ErrUnavailable
	}
	job, asset, err := q.store.PreviewAsset(ctx, actor, project, id)
	if err != nil {
		return ExportPreview{}, err
	}
	const ttl = 10 * time.Minute
	url, err := q.signer.PresignGet(ctx, asset.ObjectKey, ttl)
	if err != nil {
		return ExportPreview{}, err
	}
	return ExportPreview{JobID: job.ID, Revision: job.Revision, SHA256: *job.SHA256, URL: url, ExpiresAt: time.Now().UTC().Add(ttl), Asset: mediaapp.AssetSummary{ID: asset.ID, ProjectID: asset.ProjectID, Kind: string(asset.Kind), FileName: asset.FileName, MIMEType: asset.MimeType, ByteSize: asset.ByteSize, Width: asset.Width, Height: asset.Height, DurationMS: asset.DurationMS, Revision: asset.Revision}}, nil
}

// Download allows an attachment only after the produced output passed manual review.
func (q *ExportQuery) Download(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (io.ReadCloser, int64, error) {
	if q == nil || q.store == nil || q.reader == nil {
		return nil, 0, ErrUnavailable
	}
	job, asset, err := q.store.PreviewAsset(ctx, actor, project, id)
	if err != nil {
		return nil, 0, err
	}
	if job.Status != domain.Succeeded || !asset.CanReference() {
		return nil, 0, ErrConflict
	}
	reader, err := q.reader.Get(ctx, asset.ObjectKey)
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
