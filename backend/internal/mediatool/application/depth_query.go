package application

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

// DepthQuery keeps private object keys behind current project authorization.
type DepthQuery struct {
	store   DepthStore
	signer  mediaapp.PreviewSigner
	reader  ResultReader
	storage *resultAccess
}

// NewRoutedDepthQuery uses immutable physical identity and the actual SDK lease.
func NewRoutedDepthQuery(s DepthStore, signer mediaapp.ObjectSigner, reader mediaapp.ObjectReader, legacySigner mediaapp.PreviewSigner, legacyReader ResultReader) *DepthQuery {
	return &DepthQuery{store: s, storage: &resultAccess{signer: signer, reader: reader, legacySigner: legacySigner, legacyReader: legacyReader}}
}

// NewDepthQuery injects owner queries, existing private signer and result reader.
func NewDepthQuery(s DepthStore, signer mediaapp.PreviewSigner, reader ResultReader) *DepthQuery {
	return &DepthQuery{store: s, signer: signer, reader: reader}
}

// Preview signs only the exact immutable result approved for inspection.
func (q *DepthQuery) Preview(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (DepthPreview, error) {
	if q == nil || q.store == nil || (q.signer == nil && q.storage == nil) {
		return DepthPreview{}, ErrUnavailable
	}
	job, asset, err := q.store.PreviewAsset(ctx, actor, project, id)
	if err != nil {
		return DepthPreview{}, err
	}
	if job.SHA256 == nil || asset.SHA256 == nil || *job.SHA256 != *asset.SHA256 {
		return DepthPreview{}, ErrConflict
	}
	const ttl = 10 * time.Minute
	access := q.storage
	if access == nil {
		access = &resultAccess{legacySigner: q.signer, legacyReader: q.reader}
	}
	grant, err := access.sign(ctx, asset.ObjectKey, asset.ObjectLocation, ttl)
	if err != nil {
		return DepthPreview{}, err
	}
	return DepthPreview{JobID: job.ID, Revision: job.Revision, SHA256: *job.SHA256, URL: grant.URL, ExpiresAt: grant.ExpiresAt, Asset: mediaapp.AssetSummary{ID: asset.ID, ProjectID: asset.ProjectID, Kind: string(asset.Kind), FileName: asset.FileName, MIMEType: asset.MimeType, ByteSize: asset.ByteSize, Width: asset.Width, Height: asset.Height, DurationMS: asset.DurationMS, Revision: asset.Revision}}, nil
}

// Download requires completed review before opening any private bytes.
func (q *DepthQuery) Download(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (io.ReadCloser, int64, error) {
	if q == nil || q.store == nil || (q.reader == nil && q.storage == nil) {
		return nil, 0, ErrUnavailable
	}
	job, asset, err := q.store.PreviewAsset(ctx, actor, project, id)
	if err != nil {
		return nil, 0, err
	}
	if job.Status != domain.DepthSucceeded || !asset.CanReference() {
		return nil, 0, ErrConflict
	}
	access := q.storage
	if access == nil {
		access = &resultAccess{legacySigner: q.signer, legacyReader: q.reader}
	}
	reader, err := access.read(ctx, asset.ObjectKey, asset.ObjectLocation)
	return reader, asset.ByteSize, err
}
