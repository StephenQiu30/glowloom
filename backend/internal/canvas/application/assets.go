package application

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// AssetCanvas describes an actual authorized document, not a project surrogate.
type AssetCanvas struct {
	Collection       string     `json:"collection"`
	Favorite         bool       `json:"favorite"`
	FavoriteRevision Revision   `json:"favorite_revision" swaggertype:"string"`
	ProjectRevision  int64      `json:"project_revision"`
	CoverAssetID     *uuid.UUID `json:"cover_asset_id" extensions:"x-nullable"`
	CoverUnavailable bool       `json:"cover_unavailable"`
	CanCopyProject   bool       `json:"can_copy_project"`
	ID               uuid.UUID  `json:"id"`
	ProjectID        uuid.UUID  `json:"project_id"`
	Name             string     `json:"name"`
	Kind             string     `json:"kind"`
	Revision         Revision   `json:"revision" swaggertype:"string"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	IsMain           bool       `json:"is_main"`
}

// AssetCursor is the last stable sort tuple within the bound query.
type AssetCursor struct {
	ID   uuid.UUID
	Time time.Time
}

// AssetListInput describes one scope-bound asset query with a half-open date range.
type AssetListInput struct {
	Collection    string
	FavoriteOnly  bool
	Kind          string
	Search        string
	Sort          string
	Order         string
	CreatedFrom   *time.Time
	CreatedBefore *time.Time
	Limit         int
	After         *AssetCursor
}

// AssetPage holds actual documents and the next keyset position.
type AssetPage struct {
	Items []AssetCanvas
	Next  *AssetCursor
}

// AssetStore rechecks the current scope and project authority in its read snapshot.
type AssetStore interface {
	ListAssets(context.Context, identity.Principal, AssetListInput) (AssetPage, error)
}

// AssetAccess is the transaction-bound authority required by an asset inventory.
type AssetAccess interface {
	AuthorizeScope(context.Context, identity.Principal) error
	AuthorizeProject(context.Context, identity.Principal, uuid.UUID) error
	AssetCoverAvailable(context.Context, identity.Principal, uuid.UUID, uuid.UUID) (bool, error)
}

// AssetQuery validates global document browsing independently of graph writes.
type AssetQuery struct{ store AssetStore }

// NewAssetQuery injects the authorized asset read model.
func NewAssetQuery(store AssetStore) *AssetQuery { return &AssetQuery{store: store} }

// NormalizeAssetListInput validates a query before any persistence access.
func NormalizeAssetListInput(input AssetListInput) (AssetListInput, error) {
	kind, err := domain.ParseDocumentKind(input.Kind)
	if err != nil {
		return input, err
	}
	input.Kind = kind
	if input.Collection == "" {
		input.Collection = "current"
	}
	if input.Collection != "current" && input.Collection != "legacy" {
		return input, domain.ErrInvalidCommand
	}
	input.Search = strings.TrimSpace(input.Search)
	if input.Sort == "" {
		input.Sort = "updated"
	}
	if input.Order == "" {
		input.Order = "desc"
	}
	if input.Limit == 0 {
		input.Limit = 30
	}
	if input.Limit < 1 || input.Limit > 200 || !utf8.ValidString(input.Search) || utf8.RuneCountInString(input.Search) > 256 ||
		(input.Sort != "updated" && input.Sort != "created") || (input.Order != "asc" && input.Order != "desc") ||
		(input.CreatedFrom != nil && input.CreatedFrom.IsZero()) || (input.CreatedBefore != nil && input.CreatedBefore.IsZero()) ||
		(input.CreatedFrom != nil && input.CreatedBefore != nil && !input.CreatedFrom.Before(*input.CreatedBefore)) ||
		(input.After != nil && (input.After.ID == uuid.Nil || input.After.Time.IsZero())) {
		return input, domain.ErrInvalidCommand
	}
	return input, nil
}

// List reads one filtered page of ordinary canvases or explicit short dramas.
func (q *AssetQuery) List(ctx context.Context, actor identity.Principal, input AssetListInput) (AssetPage, error) {
	input, err := NormalizeAssetListInput(input)
	if err != nil {
		return AssetPage{}, err
	}
	return q.store.ListAssets(ctx, actor, input)
}
