package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

var (
	// ErrNotFound hides unavailable and out-of-project media.
	ErrNotFound = errors.New("media record not found")
	// ErrUnavailable reports an unavailable read or signing dependency.
	ErrUnavailable = errors.New("media query unavailable")
	// ErrInvalidQuery rejects unsupported filters or pagination.
	ErrInvalidQuery = errors.New("invalid media query")
)

// AssetReader reads current authorized facts. Implementations retain transaction locks for references.
type AssetReader interface {
	FindAsset(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.MediaAsset, error)
	ListReadyAssets(context.Context, identityapp.Principal, uuid.UUID, string, uuid.UUID, int) ([]domain.MediaAsset, error)
}

// PreviewSigner signs only server-selected private object keys.
type PreviewSigner interface {
	PresignGet(context.Context, string, time.Duration) (string, error)
}

// AssetSummary deliberately excludes object keys, supplier facts and consent details.
type AssetSummary struct {
	ID         uuid.UUID `json:"id"`
	ProjectID  uuid.UUID `json:"project_id"`
	Kind       string    `json:"kind"`
	FileName   string    `json:"file_name"`
	MIMEType   string    `json:"mime_type"`
	ByteSize   int64     `json:"byte_size"`
	Width      *int32    `json:"width,omitempty"`
	Height     *int32    `json:"height,omitempty"`
	DurationMS *int32    `json:"duration_ms,omitempty"`
	Revision   int64     `json:"revision"`
}

// AssetPage uses an opaque-to-UI UUID cursor and never silently truncates a collection.
type AssetPage struct {
	Items      []AssetSummary `json:"items"`
	NextCursor *string        `json:"next_cursor"`
}

// Preview is a short-lived authorized media URL, never persisted as a canvas field.
type Preview struct {
	Asset     AssetSummary `json:"asset"`
	URL       string       `json:"url"`
	ExpiresAt time.Time    `json:"expires_at"`
}

// AssetQuery owns media eligibility and safe projections for preview and canvas references.
type AssetQuery struct {
	store  AssetReader
	signer PreviewSigner
}

// NewAssetQuery injects the authorized facts and private object signer.
func NewAssetQuery(store AssetReader, signer PreviewSigner) *AssetQuery {
	return &AssetQuery{store: store, signer: signer}
}
func summary(a domain.MediaAsset) AssetSummary {
	return AssetSummary{ID: a.ID, ProjectID: a.ProjectID, Kind: string(a.Kind), FileName: a.FileName, MIMEType: a.MimeType, ByteSize: a.ByteSize, Width: a.Width, Height: a.Height, DurationMS: a.DurationMS, Revision: a.Revision}
}
func eligible(a domain.MediaAsset) bool {
	return !a.IsDelete && a.Status == domain.StatusReady && a.ModerationStatus == domain.ModerationPassed && (a.Kind == domain.KindImage || a.Kind == domain.KindVideo || a.Kind == domain.KindAudio || a.Kind == domain.KindModel)
}

// Reference rechecks current project ownership and eligibility before a durable binding.
func (q *AssetQuery) Reference(ctx context.Context, a identityapp.Principal, p, id uuid.UUID) (AssetSummary, error) {
	asset, err := q.reference(ctx, a, p, id)
	if err != nil {
		return AssetSummary{}, err
	}
	return summary(asset), nil
}
func (q *AssetQuery) reference(ctx context.Context, a identityapp.Principal, p, id uuid.UUID) (domain.MediaAsset, error) {
	if q == nil || q.store == nil {
		return domain.MediaAsset{}, ErrUnavailable
	}
	if p == uuid.Nil || id == uuid.Nil {
		return domain.MediaAsset{}, ErrNotFound
	}
	asset, err := q.store.FindAsset(ctx, a, p, id)
	if err != nil {
		return domain.MediaAsset{}, err
	}
	if asset.ProjectID != p || asset.ID != id || !eligible(asset) {
		return domain.MediaAsset{}, ErrNotFound
	}
	return asset, nil
}

// List defaults to canvas media; documents require the explicit document filter.
func (q *AssetQuery) List(ctx context.Context, a identityapp.Principal, p uuid.UUID, kind, cursor string, limit int) (AssetPage, error) {
	if q == nil || q.store == nil {
		return AssetPage{}, ErrUnavailable
	}
	if p == uuid.Nil || limit < 1 || limit > 200 || (kind != "" && kind != "image" && kind != "video" && kind != "audio" && kind != "model" && kind != "document") {
		return AssetPage{}, ErrInvalidQuery
	}
	var after uuid.UUID
	if cursor != "" {
		var err error
		after, err = uuid.Parse(cursor)
		if err != nil || after == uuid.Nil {
			return AssetPage{}, ErrInvalidQuery
		}
	}
	rows, err := q.store.ListReadyAssets(ctx, a, p, kind, after, limit+1)
	if err != nil {
		return AssetPage{}, err
	}
	page := AssetPage{Items: make([]AssetSummary, 0, min(len(rows), limit))}
	if len(rows) > limit {
		next := rows[limit-1].ID.String()
		page.NextCursor = &next
		rows = rows[:limit]
	}
	for _, row := range rows {
		allowed := eligible(row)
		if kind == "document" {
			allowed = row.Kind == domain.KindDocument && row.CanReference() && !row.ContainsRealPerson && row.ConsentRecordID == nil
		}
		if row.ProjectID != p || !allowed || kind != "" && string(row.Kind) != kind {
			return AssetPage{}, ErrUnavailable
		}
		page.Items = append(page.Items, summary(row))
	}
	return page, nil
}

// Preview authorizes before asking the signer for a ten-minute URL.
func (q *AssetQuery) Preview(ctx context.Context, a identityapp.Principal, p, id uuid.UUID) (Preview, error) {
	asset, err := q.reference(ctx, a, p, id)
	if err != nil {
		return Preview{}, err
	}
	if q.signer == nil {
		return Preview{}, ErrUnavailable
	}
	const ttl = 10 * time.Minute
	expires := time.Now().UTC().Add(ttl)
	url, err := q.signer.PresignGet(ctx, asset.ObjectKey, ttl)
	if err != nil || url == "" {
		return Preview{}, fmt.Errorf("%w: sign media preview", ErrUnavailable)
	}
	return Preview{Asset: summary(asset), URL: url, ExpiresAt: expires}, nil
}
